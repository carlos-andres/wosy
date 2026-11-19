#!/usr/bin/env python3
"""orchestrator.py: the orchestrator protocol of a task-bound session (harness v2 R30; research/close/30-protocol-steps.tsv).

PreToolUse on Bash, Agent|Task, Write|Edit|MultiEdit and Read, and Stop. One function per approved row, named by its id.
Deny rows (P13, P30, P38, P55, P79, P80, P81, S43) exit 2 with the reason on stderr. Warn rows (P12, P18, P28, P84) never
change the decision: PreToolUse gets hookSpecificOutput.additionalContext, Stop gets a top-level systemMessage. P12, P18 and
P28 judge the main session only; a subagent payload carries agent_id. An unbound session is never judged; the binding lookup
is anchor.py's (gate.py's). It fails open: bad input exits 0 with one line on stderr, and a rule that raises is skipped with
one line on stderr while every other rule still judges the call.
The home is $CLAUDE_CONFIG_DIR, else the home holding this file; classifier.tsv, secret_guard.py and anchor.py sit in its
hooks/.
"""
import ast, bisect, calendar, csv, functools, glob, json, os, re, shlex, sys, time

sys.dont_write_bytecode = True
HOME = os.environ.get("CLAUDE_CONFIG_DIR") or os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
HOOKS = os.path.join(HOME, "hooks")
CONTEXT_CUT = 450000  # about 45 % of a 1M window, the cut point of protocol section 8 step 7
SEPS = set("&|;()")
ASSIGN = re.compile(r"[A-Za-z_][A-Za-z0-9_]*=")
# Words that run the command after them, with the options of each that take the next word as their value.
WRAP = {"env": {"-u", "--unset", "-C", "--chdir", "-S", "--split-string"}, "sudo": {"-u", "-g", "-h", "-p", "-C", "-D", "-r",
        "-t", "-U"}, "xargs": {"-I", "-J", "-L", "-n", "-P", "-s", "-E", "-d", "-a", "-R", "-S"}, "nice": {"-n"},
        "timeout": {"-s", "-k", "--signal", "--kill-after"}, "time": {"-o", "-f"}, "exec": {"-a"},
        **dict.fromkeys(("rtk", "proxy", "command", "nohup", "builtin", "!", "{", "if", "then", "do", "else", "elif", "while",
                         "until"), set())}
DELETERS = {"rm", "rmdir", "unlink"}
SHELLS = {"bash", "sh", "zsh", "dash"}
CLOSE = re.compile(r"(S\d+[a-z]?) closed?\b")
SUBST = re.compile(r"\$\(([^()]*)\)|`([^`]*)`")
HEREDOC = re.compile(r"<<-?\s*['\"]?([A-Za-z_][A-Za-z0-9_]*)['\"]?")
QUOTES = {"'", '"', "$'"}


def heredoc_words(line, ctx):
    """(offset, delimiter, offset of the command it feeds) of each heredoc the line opens: a << or <<- before a word,
    outside quotes or in a $( ) inside double quotes. A here-string <<<, and a << in quotes or in a comment, open none.
    ctx is the stack of open quotes and parentheses; it carries from one line to the next."""
    out, start, i = [], 0, 0
    while i < len(line):
        ch, top = line[i], ctx[-1] if ctx else ""
        if top in ("'", "$'"):
            if ch == "\\" and top == "$'":
                i += 1
            elif ch == "'":
                ctx.pop()
        elif top == '"':
            if ch == '"':
                ctx.pop()
            elif ch == "\\":
                i += 1
            elif line.startswith("$(", i):
                ctx.append("(")
                i, start = i + 1, i + 2
        elif ch == "\\":
            i += 1
        elif line.startswith("$'", i):
            ctx.append("$'")
            i += 1
        elif ch in "'\"":
            ctx.append(ch)
        elif ch == "(":
            ctx.append("(")
            start = i + 1
        elif ch == ")" and ctx:
            ctx.pop()
        elif ch in ";&|":
            start = i + 1
        elif ch == "#" and (i == 0 or line[i - 1].isspace() or line[i - 1] in ";&|()<>"):
            break
        elif line.startswith("<<<", i):
            i += 2
        elif line.startswith("<<", i):
            m = HEREDOC.match(line, i)
            if m:
                out.append((i, m[1], start))
                i = m.end() - 1
        i += 1
    return out


def heredocs(cmd):
    """(the logical lines of the command with heredoc bodies removed, [(words of the command that reads it, body)]): a body
    is data of its reader, not commands. It runs to the next line that holds only its delimiter; with no such line nothing
    is dropped. A newline inside quotes continues the logical line."""
    rows, kept, docs, ctx, cur, at, i = cmd.split("\n"), [], [], [], [], {}, 0
    for j, r in enumerate(rows):
        at.setdefault(r.strip(), []).append(j)
    while i < len(rows):
        line, readers = rows[i], {}
        cur.append(line)
        i += 1
        for pos, word, start in heredoc_words(line, ctx):
            js = at.get(word, [])
            k = bisect.bisect_left(js, i)
            if k < len(js):
                if start not in readers:
                    w = line_segments(line[start:pos])
                    readers[start] = head(w[-1]) if w else []
                docs.append((readers[start], "\n".join(rows[i:js[k]])))
                i = js[k] + 1
        if not QUOTES & set(ctx):
            kept.append("\n".join(cur))
            cur = []
    return kept + ["\n".join(cur)] * bool(cur), docs


def segments_v2(cmd, kept=None):
    """The word lists of each simple command, logical line by logical line; heredoc bodies, comments and redirection
    targets are dropped, and the body of a backtick pair is judged as a command line of its own. kept is heredocs(cmd)[0]
    when the caller holds it already. P13 only; every other rule reads segments_v1."""
    segs = []
    for line in heredocs(cmd)[0] if kept is None else kept:
        for part in [line] + re.findall(r"`([^`]*)`", line):
            segs += line_segments(part)
    return segs


# The v1 parser is pre2's, kept verbatim but for the _v1 names: every rule segments with it, and P13 denies what it denies.
def unheredoc_v1(cmd):
    """The command with heredoc bodies removed: their lines are data, not commands."""
    out, lines, i = [], cmd.split("\n"), 0
    while i < len(lines):
        out.append(lines[i])
        ends = re.findall(r"<<-?\s*['\"]?([A-Za-z_][A-Za-z0-9_]*)['\"]?", lines[i])
        i += 1
        for e in ends:
            while i < len(lines) and lines[i].strip() != e:
                i += 1
            i += 1
    return "\n".join(out)


def segments_v1(cmd):
    """The word lists of each simple command, line by line; heredoc bodies, comments and redirection targets are dropped,
    and the body of a backtick pair is judged as a command line of its own."""
    segs = []
    for line in unheredoc_v1(cmd).split("\n"):
        for part in [line] + re.findall(r"`([^`]*)`", line):
            segs += line_segments(part)
    return segs


# Several rules segment the same command; parse it once. No caller changes the lists it returns.
segments_v1 = functools.lru_cache(maxsize=None)(segments_v1)


def uncomment(line):
    """The line up to its comment: an unquoted # at the start of a word. A # in quotes or inside a word is text."""
    quote, prev, i = None, " ", 0
    while i < len(line):
        ch = line[i]
        if quote:
            if ch == quote:
                quote = None
            elif ch == "\\" and quote == '"':
                i += 1
        elif ch == "\\":
            i += 1
        elif ch in "'\"":
            quote = ch
        elif ch == "#" and (prev.isspace() or prev in ";&|()<>"):
            return line[:i]
        prev = line[i] if i < len(line) else prev
        i += 1
    return line


def line_segments(line):
    line = uncomment(line)
    try:
        lex = shlex.shlex(line, posix=True, punctuation_chars=True)
        lex.whitespace_split, lex.commenters = True, ""
        toks = list(lex)
    except ValueError:
        # Unbalanced quotes: a plain split on whitespace and separators still shows the command words.
        toks = re.findall(r"[;&|()<>]+|[^\s;&|()<>]+", line)
    segs, cur, skip = [], [], False
    for tok in toks:
        if tok and set(tok) <= SEPS | {"<", ">"}:
            if "<" in tok or ">" in tok:
                skip = True
            else:
                segs.append(cur)
                cur = []
            continue
        if skip:
            skip = False
        else:
            cur.append(tok)
    return [s for s in segs + [cur] if s]


def head(w):
    """The words from the command itself on, past VAR=value, wrappers, their options and a timeout duration; a lookup such
    as `command -v rm` runs nothing and yields no words."""
    i, opts, duration = 0, None, False
    while i < len(w):
        x = w[i]
        if x == "command" and w[i + 1:i + 2] and w[i + 1][:2] in ("-v", "-V"):
            return []
        if x in WRAP:
            opts, duration = WRAP[x], x == "timeout"
        elif opts is not None and x in opts:
            i += 1
        elif not ASSIGN.match(x) and not (opts is not None and x.startswith("-")):
            if not (duration and re.fullmatch(r"\d+(\.\d+)?[smhd]?", x)):
                break
            duration = False
        i += 1
    return w[i:]


def interp(w):
    """A python call collapsed to its script: python3 -B x/protocolo ... -> x/protocolo ..."""
    if w and re.fullmatch(r"python[0-9.]*", os.path.basename(w[0])):
        w = w[1:]
        while w and w[0].startswith("-"):
            w = w[1:]
    return w


def expand(s, env):
    """s with each $NAME and ${NAME} that env holds replaced by its value; any other $ stays as written."""
    return re.sub(r"\$(?:\{([A-Za-z_][A-Za-z0-9_]*)\}|([A-Za-z_][A-Za-z0-9_]*))", lambda m: env.get(m[1] or m[2], m[0]), s)


def ts_of(s):
    return calendar.timegm(time.strptime(s[:19], "%Y-%m-%dT%H:%M:%S"))


def lines(path):
    try:
        with open(path, encoding="utf-8") as f:
            for line in f:
                try:
                    d = json.loads(line)
                except ValueError:
                    continue
                if isinstance(d, dict):
                    yield d
    except OSError:
        return


def plan_ids(task):
    """Node ids of the tasks: block of plan.md and research/close/plan.md of the task."""
    ids = set()
    for p in ("plan.md", "research/close/plan.md"):
        try:
            text = open(os.path.join(task, p), encoding="utf-8").read()
        except OSError:
            continue
        m = re.search(r"(?m)^tasks:\n((?:[ \t].*\n|\n)*)", text)
        ids |= set(re.findall(r"(?m)^\s*-\s*id:\s*([A-Za-z0-9_.-]+)", m[1])) if m else set()
    return ids


def names(text, word, flags=0):
    return re.search(rf"(?<![A-Za-z0-9_-]){re.escape(word)}(?![A-Za-z0-9_-])", text, flags) is not None


def same_bytes(path, folder, pattern="**/*"):
    data = open(path, "rb").read()
    for f in glob.glob(os.path.join(folder, pattern), recursive=True):
        if os.path.isfile(f) and os.path.getsize(f) == len(data) and open(f, "rb").read() == data:
            return True
    return False


class Ctx:
    def __init__(self, ev, task):
        self.ev, self.task, self.main = ev, task, "agent_id" not in ev
        self.inp = ev.get("tool_input") if isinstance(ev.get("tool_input"), dict) else {}
        self.cwd = ev.get("cwd") if isinstance(ev.get("cwd"), str) else "/"
        self.tp = ev.get("transcript_path") if isinstance(ev.get("transcript_path"), str) else ""
        self.ledger = os.path.join(task, "logbook.jsonl")
        self.segs = [head(s) for s in segments_v1(self.inp.get("command") or "")] if ev.get("tool_name") == "Bash" else []
        self.file = self.path(self.inp["file_path"]) if isinstance(self.inp.get("file_path"), str) else ""

    def path(self, p):
        return os.path.realpath(os.path.join(self.cwd, os.path.expanduser(p)))

    def events(self):
        """(type, text) of each protocolo --event of the command aimed at the bound task's ledger. The ledger argument is
        read with the VAR=value assignments made earlier in the command and $HOME; a $ left in it before logbook.jsonl
        counts as the bound ledger, so an unresolved variable is judged rather than passed. A $( ) or backtick substitution
        reads as such a $, and its body is read as a command of its own."""
        cmd, bodies, n = self.inp.get("command") or "", [], 1
        while n:
            cmd, n = SUBST.subn(lambda m: bodies.append(m[1] if m[2] is None else m[2]) or "$__SUB", cmd)
        for c in [cmd] + bodies:
            # In a subshell, a pipeline or after && or || an assignment may not reach the event, so a changed value is unknown.
            env, unsure = {"HOME": os.path.expanduser("~")}, re.search(r"[()|&]", c)
            for s in segments_v1(c):
                a = s[1:] if s[:1] == ["export"] else s
                if a and all(ASSIGN.match(x) for x in a):
                    for k, v in (x.split("=", 1) for x in a):
                        v = expand(v, env)
                        env[k] = "$" + k if unsure and env.get(k, v) != v else v
                    continue
                w = interp(head(s))
                if w and os.path.basename(w[0]) == "protocolo" and "--event" in w:
                    i = w.index("--event")
                    led = expand(w[i + 1], env) if len(w) > i + 3 else ""
                    if led and (self.path(led) == self.ledger or ("$" in led and os.path.basename(led) == "logbook.jsonl")):
                        yield w[i + 2], w[i + 3]

    def closes(self):
        for typ, text in self.events():
            m = CLOSE.match(text) if typ == "verification" else None
            if m:
                yield m[1]

    def bash_history(self):
        """Commands of the main session's Bash calls in the transcript, in order."""
        for d in lines(self.tp):
            if d.get("type") == "assistant" and not d.get("isSidechain"):
                for c in (d.get("message") or {}).get("content") or []:
                    if isinstance(c, dict) and c.get("type") == "tool_use" and c.get("name") == "Bash":
                        yield str((c.get("input") or {}).get("command") or "")


def rule_P12(c):
    if not c.main:
        return None
    p = c.file
    under = lambda d: p.startswith(os.path.join(c.task, d) + os.sep)
    if p and not under("proofs") and (p.endswith((".py", ".sh")) or under("bin") or under("hooks")):
        return ("warn", f"the main session writes {p}. The main session decides, briefs and verifies; code and scripts belong "
                        "to a builder subagent. A diagnosed fix under 10 lines is the exception: record it in "
                        "research/dispatch-log.tsv as a main line with result redone.")
    return None


def rule_P13(c):
    # Command words only: interpreter code that deletes (os.remove, shutil.rmtree) and a script run are out of scope.
    # The union of both parsers: v1 keeps every deny of pre2, v2 adds the forms v1 misses.
    hit = next(filter(None, map(deletion_v1, c.segs)), None) or deletes_v2(c.inp.get("command") or "")
    if hit:
        scripts = sorted(glob.glob(os.path.join(c.task, "bin", "cleanup-*.sh")), key=os.path.getmtime)
        script = scripts[-1] if scripts else os.path.join(c.task, "bin", "cleanup-<phase>.sh")
        return ("deny", f"`{hit}` deletes, and every deleting command is the owner's. Add the path to {script}, show its dry "
                        f"run, and give the owner the line `bash {script} --go`.")
    return None


def deletes_v2(cmd, depth=0):
    """The first deleting command of a command line, the heredoc bodies a shell reads judged as command lines too."""
    kept, docs = heredocs(cmd)
    for s in segments_v2(cmd, kept):
        hit = deletion_v2(s, depth)
        if hit:
            return hit
    for reader, body in docs if depth < 2 else []:
        hit = reader and os.path.basename(reader[0]) in SHELLS and deletes_v2(body, depth + 1)
        if hit:
            return hit
    return None


def deletion_v1(w, depth=0):
    w = head(w)
    if not w:
        return None
    b, a = os.path.basename(w[0]), w[1:]
    if b in DELETERS:
        return b
    if b == "find":
        if "-delete" in a:
            return "find -delete"
        for i, x in enumerate(a[:-1]):
            if x in ("-exec", "-execdir", "-ok", "-okdir") and os.path.basename(a[i + 1]) in DELETERS:
                return f"find {x} {a[i + 1]}"
    if b == "git":
        j = 0
        while j < len(a) and a[j].startswith("-"):
            j += 2 if a[j] in ("-C", "-c") else 1
        if a[j:j + 1] in (["clean"], ["rm"]):
            return f"git {a[j]}"
    if b in ("bash", "sh", "zsh") and "-c" in a and depth < 2 and a.index("-c") + 1 < len(a):
        for s in segments_v1(a[a.index("-c") + 1]):
            hit = deletion_v1(s, depth + 1)
            if hit:
                return hit
    return None


def deletion_v2(w, depth=0):
    w = head(w)
    if not w:
        return None
    b, a = os.path.basename(w[0]), w[1:]
    if b in DELETERS:
        return b
    if b == "find":
        if "-delete" in a:
            return "find -delete"
        for i, x in enumerate(a[:-1]):
            if x in ("-exec", "-execdir", "-ok", "-okdir") and os.path.basename(a[i + 1]) in DELETERS:
                return f"find {x} {a[i + 1]}"
    if b == "git":
        j = 0
        while j < len(a) and a[j].startswith("-"):
            j += 2 if a[j] in ("-C", "-c") else 1
        if a[j:j + 1] in (["clean"], ["rm"]):
            return f"git {a[j]}"
    # A c flag counts among the shell's leading options only: a word after the script operand belongs to the script.
    k, j = None, 0
    while b in SHELLS and k is None and j < len(a) and a[j][:1] in ("-", "+") and a[j] != "--":
        k = j if re.fullmatch(r"-[A-Za-z]*c[A-Za-z]*", a[j]) else None
        j += 2 if a[j] in ("-o", "+o", "-O", "+O", "--rcfile", "--init-file") else 1
    if depth < 2 and k is not None and k + 1 < len(a):
        return deletes_v2(a[k + 1], depth + 1)
    if depth < 2 and b == "eval":
        return deletes_v2(" ".join(a), depth + 1)
    return None


def watched(p, tp):
    parts = p.split(os.sep)
    return "hooks" in parts[:-1] or parts[-1] == "protocolo" or (p.endswith(".jsonl") and ("projects" in parts or p == tp))


def span(b, a):
    """(lines shown, file arguments) of a cat, head, tail or sed call; inf when the whole file is shown."""
    inf = float("inf")
    if b == "cat":
        return inf, [x for x in a if not x.startswith("-")]
    valued = ("-n", "-c") if b in ("head", "tail") else ("-e",)
    plain = [x for i, x in enumerate(a) if not x.startswith("-") and not (i and a[i - 1] in valued)]
    if b in ("head", "tail"):
        m = re.search(r"(?:^| )(?:-n ?|--lines=|-)(\+?)(\d+)(?= |$)", " ".join(a))
        return (inf if m and m[1] else int(m[2]) if m else 10), plain
    script = next((a[i + 1] for i in range(len(a) - 1) if a[i] == "-e"), None) or (plain.pop(0) if plain else None)
    files = plain
    m = re.fullmatch(r"(\d+)(?:,(\d+))?p", script or "")
    return (int(m[2] or m[1]) - int(m[1]) + 1 if m and "-n" in a else inf), files


def rule_P18(c):
    if not c.main:
        return None
    tool, hits = c.ev.get("tool_name"), []
    if tool == "Read":
        p, limit = c.file, c.inp.get("limit")
        if p and watched(p, c.tp) and not (isinstance(limit, int) and limit <= 10):
            hits.append(p)
    elif tool == "Bash":
        for w in c.segs:
            if w and os.path.basename(w[0]) in ("cat", "sed", "head", "tail"):
                n, files = span(os.path.basename(w[0]), w[1:])
                hits += [c.path(f) for f in files if n > 10 and watched(c.path(f), c.tp)]
    if hits:
        return ("warn", f"the main session reads {hits[0]} beyond 10 lines. Hook, protocolo and guard source and transcripts "
                        "stay out of the main context: ask a subagent for counts and the verdict line, or Read with a limit "
                        "of 10 lines or fewer.")
    return None


def subagent_stops(c, node):
    """Stop times of the subagents whose brief names node on its first line; None in the list for one still running."""
    uses, results, notes = {}, {}, {}
    for d in lines(c.tp):
        t, msg = d.get("type"), d.get("message") if isinstance(d.get("message"), dict) else {}
        content = msg.get("content")
        if t == "assistant" and not d.get("isSidechain"):
            for x in content or []:
                if isinstance(x, dict) and x.get("type") == "tool_use" and x.get("name") in ("Agent", "Task"):
                    first = str((x.get("input") or {}).get("prompt") or "").lstrip().split("\n")[0]
                    if names(first, node):
                        uses[x.get("id")] = True
        elif t in ("user", "queue-operation") and not d.get("isSidechain"):
            text = d.get("content") if t == "queue-operation" else content
            for x in content if isinstance(content, list) and t == "user" else []:
                if isinstance(x, dict) and x.get("type") == "tool_result":
                    results[x.get("tool_use_id")] = (d.get("timestamp"), json.dumps(x.get("content")))
            if isinstance(text, str) and "<task-notification>" in text:
                for tid in re.findall(r"<tool-use-id>([^<]+)</tool-use-id>", text):
                    notes.setdefault(tid, d.get("timestamp"))
    stops = []
    for tid in uses:
        if tid in notes:
            stops.append(ts_of(notes[tid]))
        elif tid in results:
            stops.append(None if "Async agent launched" in results[tid][1] else ts_of(results[tid][0]))
    return stops


def rule_P28(c):
    if not c.main:
        return None
    for typ, text in c.events():
        if typ != "verification":
            continue
        for node in sorted(n for n in plan_ids(c.task) if names(text, n)):
            stops = subagent_stops(c, node)
            if not stops:
                continue
            proof = re.compile(rf"proofs/{re.escape(node)}\.sh\b")
            if None in stops:
                why = f"a subagent of {node} is still running"
            else:
                last = max(stops)
                if any(e.get("type") == "verification" and str(e.get("text", "")).startswith("QA tests ")
                       and proof.search(e["text"]) and ts_of(e.get("ts", "")) >= last for e in lines(c.ledger)):
                    continue
                why = f"the ledger holds no QA run of proofs/{node}.sh after its last subagent stopped"
            return ("warn", f"this verification names {node} and {why}. Re-run `bash {c.task}/proofs/{node}.sh` and its "
                            "control yourself first; mark your own runs [V] and the agent's claims [I].")
    return None


def rule_P30(c):
    p = c.file
    if (os.path.basename(p) not in ("settings.json", "settings.local.json")
            or not os.path.basename(os.path.dirname(p)).startswith(".claude")):
        return None
    if os.path.isfile(p) and not same_bytes(p, os.path.join(c.task, "backup")):
        return ("deny", f"no file under {c.task}/backup/ holds the current bytes of {p}. Wire last: "
                        f"`mkdir -p {c.task}/backup/<node> && cp {p} {c.task}/backup/<node>/`, then edit.")
    return None


def rule_P38(c):
    first = str(c.inp.get("prompt") or "").lstrip().split("\n")[0]
    clf = os.path.join(HOOKS, "classifier.tsv")
    with open(clf, encoding="utf-8") as f:
        rows = [r[0] for r in csv.reader(f, delimiter="\t") if r and r[0] != "work"]
    forms = {x.strip() for r in rows for x in (r, r.split(":")[0]) if x.strip()}  # the full cell or its part before ":"
    ids = plan_ids(c.task)
    missing = [what for what, good in (
        (f"a classifier row (a work value of {clf})", any(names(first, x, re.I) for x in forms)),
        ("a node id of the task's plan", not ids or any(names(first, n, re.I) for n in ids))) if not good]
    if missing:
        return ("deny", f"the first line of the brief lacks {' and '.join(missing)}. Put both on line 1, as in: "
                        'Classifier row: "build code" (builder, opus, high). Plan node: <id>.')
    return None


def guard_names():
    """The guard's own protected-name pattern, read from its source without running it."""
    for node in ast.parse(open(os.path.join(HOOKS, "secret_guard.py"), encoding="utf-8").read()).body:
        if isinstance(node, ast.Assign) and getattr(node.targets[0], "id", "") == "KEY_MATERIAL_RE":
            return re.compile(ast.literal_eval(node.value.args[0]))
    raise LookupError("KEY_MATERIAL_RE is not in secret_guard.py")


def rule_P55(c):
    if not any(w and (os.path.basename(w[0]) == "ensayo" or (os.path.basename(w[0]) == "claude" and {"-p", "--print"} & set(w)))
               for w in c.segs):
        return None
    pat = guard_names()
    cmd = expand(c.inp.get("command") or "", {"HOME": os.path.expanduser("~")})
    for tok in re.findall(r"[A-Za-z0-9_.~+\-/]+", cmd):
        if pat.search(tok) and os.path.exists(c.path(tok)):
            return ("deny", f"the prompt of this live call names {c.path(tok)}, an existing file with a guard-protected name. "
                            "A live deny test aims at a path with the protected name that does not exist.")
    return None


def isolation_found(c, phase):
    """A find newer than backup/<phase>.stamp after a touch, in the main session's Bash history."""
    touched = False
    for cmd in c.bash_history():
        for w in [head(s) for s in segments_v1(cmd)]:
            touched = touched or (bool(w) and os.path.basename(w[0]) == "touch")
            a = w[1:]
            if (touched and w and os.path.basename(w[0]) == "find" and "-newer" in a[:-1]
                    and a[a.index("-newer") + 1].endswith(f"backup/{phase}.stamp")):
                return True
    return False


def rule_P79(c):
    for phase in c.closes():
        if not isolation_found(c, phase):
            return ("deny", f"the {phase} close has no isolation find in this session. Run `touch <scratch file> && find "
                            f"<roots> -newer {c.task}/backup/{phase}.stamp -type f` and see the scratch file reported first.")
    return None


def rule_P80(c):
    for phase in c.closes():
        log = os.path.join(c.task, "research", "dispatch-log.tsv")
        try:
            with open(log, encoding="utf-8") as f:
                found = any(r.get("agent") == "main" and r.get("node") == phase for r in csv.DictReader(f, delimiter="\t"))
        except OSError:
            found = False
        if not found:
            return ("deny", f"{log} has no line with agent main and node {phase}. Add it first: model, peak context from "
                            "research/telemetry.tsv, dispatch count and subagent tokens.")
    return None


def rule_P81(c):
    p, h = c.file, os.path.join(c.task, "next-session-prompt.md")
    if p == h and os.path.isfile(h) and not same_bytes(h, os.path.join(c.task, "backup"), "next-session-prompt.md.*"):
        return ("deny", f"no backup/next-session-prompt.md.* holds the current bytes of {h}. Copy it first: "
                        f"`cp {h} {c.task}/backup/next-session-prompt.md.<date>-<phase>`.")
    return None


def rule_S43(c):
    h = os.path.join(c.task, "next-session-prompt.md")
    for phase in c.closes():
        if not os.path.isfile(h):
            return None
        mt = int(os.path.getmtime(h))
        for e in lines(c.ledger):
            # protocolo --probe writes "LOAD-PROBE <GO|NO-GO> blocking=<n>: <prompt>"; only a GO with no blocking gap counts.
            m = re.fullmatch(r"LOAD-PROBE GO blocking=0: (.+)", str(e.get("text", "")))
            if e.get("type") == "verification" and m and os.path.realpath(m[1]) == h and ts_of(e.get("ts", "")) >= mt:
                return None
        return ("deny", f"no LOAD-PROBE GO with blocking 0 follows the last write of {h}, so the {phase} close is early. "
                        f"Run `protocolo --probe {h}`, close the blocking gaps it names, and close after a GO with none.")
    return None


def rule_P84(c):
    """Warn once per session, on Stop, when the last main turn passes CONTEXT_CUT tokens of context."""
    tp, sid = c.tp, re.sub(r"[^A-Za-z0-9_-]", "", str(c.ev.get("session_id")))
    marker = os.path.join(HOME, "state", f"orchestrator-P84-{sid}")
    if os.path.exists(marker) or not os.path.isfile(tp):
        return None
    with open(tp, "rb") as f:
        f.seek(max(0, os.path.getsize(tp) - 4000000))
        tail = f.read().decode("utf-8", "replace").splitlines()
    for line in reversed(tail):
        try:
            d = json.loads(line)
        except ValueError:
            continue
        u = (d.get("message") or {}).get("usage") if d.get("type") == "assistant" and not d.get("isSidechain") else None
        if isinstance(u, dict):
            used = sum(int(u.get(k) or 0) for k in ("input_tokens", "cache_read_input_tokens", "cache_creation_input_tokens"))
            if used <= CONTEXT_CUT:
                return None
            os.makedirs(os.path.dirname(marker), exist_ok=True)
            open(marker, "w").write(f"{used}\n")
            return ("warn", f"the last turn used {used:,} context tokens, past {CONTEXT_CUT:,}. A clean cut is due: close "
                            "the phase, write the handoff and continue in a fresh session.")
    return None


WRITES = ("Write", "Edit", "MultiEdit")
RULES = {  # rule id: the tools it judges, one line per rule, run in this order
    "P12": WRITES,
    "P13": ("Bash",),
    "P18": ("Read", "Bash"),
    "P28": ("Bash",),
    "P30": WRITES,
    "P38": ("Agent", "Task"),
    "P55": ("Bash",),
    "P79": ("Bash",),
    "P80": ("Bash",),
    "P81": WRITES,
    "S43": ("Bash",),
    "P84": ("Stop",),
}


def main():
    ev = json.loads(sys.stdin.read())
    if not isinstance(ev, dict):
        raise ValueError("stdin holds no JSON object")
    sys.path.insert(0, HOOKS)
    from anchor import state
    st = state(ev.get("session_id"))
    hook = ev.get("hook_event_name")
    if not st or st["error"] or hook not in ("PreToolUse", "Stop"):
        return
    tool = "Stop" if hook == "Stop" else ev.get("tool_name")
    c, deny, warn = Ctx(ev, st["task"]), [], []
    for rid, tools in RULES.items():
        if tool not in tools:
            continue
        try:
            got = globals()[f"rule_{rid}"](c)
        except Exception as e:
            print(f"orchestrator: rule {rid} not judged, {type(e).__name__}: {' '.join(str(e).split())[:200]}", file=sys.stderr)
            continue
        if got:
            (deny if got[0] == "deny" else warn).append(f"[{rid}] {got[1]}")
    if deny:
        print("BLOCKED by orchestrator " + "\nBLOCKED by orchestrator ".join(deny), file=sys.stderr)
        sys.exit(2)
    text = "\n".join("orchestrator warning " + x for x in warn)
    if warn and hook == "Stop":
        print(json.dumps({"systemMessage": text}))
    elif warn:
        print(json.dumps({"hookSpecificOutput": {"hookEventName": "PreToolUse", "additionalContext": text}}))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as e:
        print(f"orchestrator: not judged, {type(e).__name__}: {' '.join(str(e).split())[:300]}", file=sys.stderr)
    sys.exit(0)
