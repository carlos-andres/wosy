#!/usr/bin/env python3
"""gate.py: the phase gate of a task-bound session (harness v2 R02, R03, R12, R13, R22, R24; A4, A10).

PreToolUse on Write, Edit, MultiEdit, NotebookEdit and AskUserQuestion, and a stage of dispatch-bash.sh for Bash.
Exit 0 allows, exit 2 denies with the reason on stderr. A session is bound when <bind dir>/<session_id> names its task
folder; the bind dir is $GATE_BIND_DIR, else $CLAUDE_CONFIG_DIR/state/bind, else state/bind of the home holding this file.
An unbound session is never judged. A broken binding blocks every write and lets reads and AskUserQuestion through.
"""
import json, os, re, sys

PROTOCOLO = "/Users/you/.devwork/bin/protocolo"
ORDER = ("discovery", "planning", "execution", "qa", "closed", "continuity")
HANDOFF = ("backup/", "next-session-prompt.md")
WRITE = {"discovery": ("research/",) + HANDOFF, "planning": ("research/", "prd.md", "spec.md", "plan.md", "proofs/") + HANDOFF,
         "execution": ("research/", "prd.md", "spec.md", "plan.md", "proofs/", "sql/", "bin/") + HANDOFF,
         "qa": ("research/qa/",) + HANDOFF, "closed": (), "continuity": HANDOFF}
PROTECTED = ("gate.json", "gate.json.tmp", "logbook.jsonl", "logbook.seal")
ROUND_MARK = "gate.json.round-failed"
# The options of each deleting or creating command whose value is the next word, so that word is no operand.
VALUE_OPTS = {"rm": (), "truncate": ("-s", "-r"), "touch": ("-r", "-t", "-d"), "mv": ("-t",)}
TAR_VAL = set("fCbTXgKNV")
TAR_LONG = {"--create": "c", "--append": "r", "--update": "u", "--catenate": "A", "--concatenate": "A", "--extract": "x",
            "--get": "x", "--to-stdout": "O", "--file": "f", "--directory": "C"}
LAYOUT = (("gate.json", "logbook.jsonl", "context.md", "spec.md", "prd.md", "plan.md", "next-session-prompt.md"),
          ("research/", "sql/", "proofs/", "bin/", "backup/", "archive/"))
SQL = {"mysql", "mysqlp", "psql", "sqlite3"}
INTERP = re.compile(r"^(python[0-9.]*|perl|node|ruby|php)$")
SHELLS = {"bash", "sh", "zsh"}
SKIP = {"rtk", "proxy", "builtin", "export", "declare", "typeset", "local", "readonly",
        "then", "do", "else", "elif", "if", "while", "until", "!", "{", "}"}
# Wrappers that run the command after their options, with the options that take the next word as their value.
WRAPPERS = {"env": {"-u", "--unset", "-C", "--chdir", "-P", "-S", "--split-string"}, "nice": {"-n", "--adjustment"},
            "timeout": {"-s", "--signal", "-k", "--kill-after"}, "nohup": set(), "time": {"-o", "-f", "--output", "--format"},
            "command": set(), "exec": {"-a"}, "stdbuf": {"-i", "-o", "-e", "--input", "--output", "--error"},
            "sudo": {"-u", "-g", "-h", "-p", "-C", "-D", "-r", "-t", "-U", "--user", "--group", "--host", "--prompt"},
            "xargs": {"-I", "-J", "-L", "-n", "-P", "-s", "-E", "-d", "-a", "-R", "-S", "--max-args", "--max-procs",
                      "--max-lines", "--max-chars", "--delimiter", "--arg-file"},
            "uv": {"-w", "--with", "--with-editable", "--with-requirements", "-p", "--python", "--project", "--directory",
                   "--env-file", "--extra", "--group", "--only-group", "--no-group", "--package", "-i", "--index",
                   "--default-index", "--index-url", "--extra-index-url", "-f", "--find-links", "--cache-dir",
                   "--config-file", "--color", "--index-strategy", "--keyring-provider", "--resolution", "--prerelease",
                   "--exclude-newer", "--link-mode", "-P", "--upgrade-package", "--reinstall-package", "-C",
                   "--config-setting", "--python-platform", "--python-preference"}}
FIND_EXEC = {"-exec", "-execdir", "-ok", "-okdir"}
ARG = "$(xargs or find input)"
# A patch with no file named writes the files its diff names.
UNREAD = "the files its diff names, which the gate does not read"
WRITE_OPS = {">", ">>", ">|", "&>", "&>>", ">&"}
OPS = (">>", ">|", ">&", "&>>", "&>", "&&", "||", "|&", "<<<", "<<-", "<<", "<&", ";;", ">", "<", "|", ";", "&", "(", ")", "\n")
SEPS = {";", "&&", "||", "|", "&", "|&", "(", "\n", ";;"}
KEYWORDS = {"if", "then", "else", "elif", "while", "until", "do", "!", "{", "time"}
UNKNOWN = None


class Deny(Exception):
    pass


def deny(rule, msg):
    raise Deny(f"BLOCKED by gate ({rule}): {msg}")


def parens(s):
    out, stack = {}, []
    for j, ch in enumerate(s):
        if ch == "(":
            stack.append(j)
        elif ch == ")" and stack:
            out[stack.pop()] = j
    return out


def arith_end(s, i, match):
    """End of a `(( ... ))` arithmetic command at i, or -1 when the parens close apart, as in the subshell `((cmd) )`."""
    j = match.get(i + 1)
    if j is None or match.get(i) != j + 1 or "$(" in s[i + 2:j] or "`" in s[i + 2:j]:
        return -1
    return j + 2


def sub_end(s, i):
    """Index of the ) or ` that closes the command substitution opening at i, or len(s) when it never closes."""
    n = len(s)
    if s[i] == "`":
        j = i + 1
        while j < n and s[j] != "`":
            j += 2 if s[j] == "\\" else 1
        return min(j, n)
    j, depth = i + 2, 1
    while j < n:
        c = s[j]
        if c == "\\":
            j += 1
        elif c == "'":
            j = s.find("'", j + 1)
            if j < 0:
                return n
        elif c == '"':
            j += 1
            while j < n and s[j] != '"':
                if s[j] == "\\":
                    j += 1
                elif s.startswith("$(", j) or s[j] == "`":
                    j = sub_end(s, j)
                j += 1
        elif s.startswith("$(", j) or c == "`":
            j = sub_end(s, j)
        elif c == "(":
            depth += 1
        elif c == ")":
            depth -= 1
            if depth == 0:
                return j
        j += 1
    return n


def subst(s, i, j):
    """The command bodies of the substitution s[i:j + 1]; `$(( ))` is arithmetic and yields only what it nests."""
    if s[i] == "`":
        return [re.sub(r"\\([\\`$])", r"\1", s[i + 1:j])]
    return subs_in(s[i + 3:j - 1]) if s.startswith("$((", i) and s[j - 1:j] == ")" else [s[i + 2:j]]


def subs_in(s):
    """The command bodies of the substitutions in text where quotes are literal: a heredoc body, an arithmetic."""
    out, i = [], 0
    while i < len(s):
        if s[i] == "\\":
            i += 1
        elif s.startswith("$(", i) or s[i] == "`":
            j = sub_end(s, i)
            out += subst(s, i, j)
            i = j
        i += 1
    return out


def lex(s):
    """Shell words and operators; quotes are removed from words, which come back as ('word', text, substitution
    bodies), and heredoc bodies as ('body', text, n, substitution bodies), none when the delimiter is quoted.
    Inside `[[ ]]` and `(( ))` at command position, < and > compare and are kept as words."""
    toks, word, inword, i, n = [], [], False, 0, len(s)
    want, pending, hd, subs, quoted = False, [], 0, [], False
    cmd, test, match = True, False, parens(s) if "((" in s else {}

    def flush():
        nonlocal word, inword, want, cmd, test, subs, quoted
        if inword:
            w = "".join(word)
            toks.append(("word", w, tuple(subs)))
            if want:
                pending.append((w, toks[-2][1] == "<<-", len(pending) + hd, quoted))
                want = False
            test = cmd and w == "[[" or test and w != "]]"
            cmd = cmd and w in KEYWORDS
        word, inword, subs, quoted = [], False, [], False

    while i < n:
        c = s[i]
        if c == "\\" and i + 1 < n:
            if s[i + 1] != "\n":
                word.append(s[i + 1])
                inword = quoted = True
            i += 2
        elif c == "'":
            j = s.find("'", i + 1)
            j = n if j < 0 else j
            word.append(s[i + 1:j])
            inword, quoted, i = True, True, j + 1
        elif c == '"':
            j, buf = i + 1, []
            while j < n and s[j] != '"':
                if s[j] == "\\" and j + 1 < n and s[j + 1] in '"\\$`\n':
                    buf.append(s[j + 1])
                    j += 2
                elif s.startswith("$(", j) or s[j] == "`":
                    e = sub_end(s, j)
                    subs += subst(s, j, e)
                    buf.append(s[j:e + 1])
                    j = e + 1
                else:
                    buf.append(s[j])
                    j += 1
            word.append("".join(buf))
            inword, quoted, i = True, True, j + 1
        elif c == "$" and s.startswith("$(", i) or c == "`":
            j = sub_end(s, i)
            subs += subst(s, i, j)
            word.append(s[i:j + 1])
            inword, i = True, j + 1
        elif c in " \t":
            flush()
            i += 1
        elif c == "#" and not inword:
            while i < n and s[i] != "\n":
                i += 1
        elif c == "(" and cmd and not inword and s.startswith("((", i) and (e := arith_end(s, i, match)) > 0:
            toks.append(("word", s[i:e], ()))
            cmd, i = False, e
        else:
            op = next((o for o in OPS if s.startswith(o, i)), None)
            if op is None or test and op in ("<", ">"):
                word.append(c)
                inword, i = True, i + 1
                continue
            if op[0] in "<>" and inword and "".join(word).isdigit():
                word, inword = [], False
            flush()
            i += len(op)
            if op == "\n" and pending:
                for delim, dash, k, q in pending:
                    body = []
                    while i < n:
                        j = s.find("\n", i)
                        j = n if j < 0 else j
                        line = s[i:j]
                        i = j + 1
                        if (line.lstrip("\t") if dash else line) == delim:
                            break
                        body.append(line)
                    text = "\n".join(body)
                    toks.append(("body", text, k, () if q else tuple(subs_in(text))))
                hd += len(pending)
                pending = []
            toks.append(("op", op))
            want = op in ("<<", "<<-")
            cmd, test = op in SEPS, test and op in ("&&", "||", "(", ")")
    flush()
    return toks


def segments(toks):
    """Simple commands as words w, redirect targets r, heredoc bodies b, substitution bodies s and the operator op that
    ends them; each ( and ) of a subshell comes back as the marker {"(": True} or {")": True}."""
    new = lambda: {"w": [], "r": [], "b": [], "s": [], "op": None}
    segs, cur, owner, i = [], new(), {}, 0
    hd = 0
    while i < len(toks):
        k, t = toks[i][0], toks[i][1]
        nxt = toks[i + 1] if i + 1 < len(toks) and toks[i + 1][0] == "word" else None
        if k == "body":
            o = owner.get(toks[i][2], cur)
            o["b"].append(t)
            o["s"] += toks[i][3]
        elif k == "word":
            cur["w"].append(t)
            cur["s"] += toks[i][2]
        elif t in WRITE_OPS or t in ("<<", "<<-", "<", "<&", "<<<"):
            if t in ("<<", "<<-"):
                owner[hd], hd = cur, hd + 1
            elif nxt is not None:
                cur["s"] += nxt[2]
                if t in WRITE_OPS and not (t == ">&" and (nxt[1].isdigit() or nxt[1] == "-")):
                    cur["r"].append(nxt[1])
            i += nxt is not None
        else:
            cur["op"] = t
            segs += [cur, {t: True}] if t in ("(", ")") else [cur]
            cur = new()
        i += 1
    segs.append(cur)
    return [s for s in segs if s.get("w") or s.get("r") or s.get("s") or "(" in s or ")" in s]


ASSIGN = re.compile(r"^([A-Za-z_][A-Za-z0-9_]*)=(.*)$", re.S)
VAR = re.compile(r"\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?")


def expand(tok, vars_):
    if tok.startswith("~") and (len(tok) == 1 or tok[1] == "/"):
        tok = os.path.expanduser("~") + tok[1:]
    if "$(" in tok or "`" in tok or any("," in b or ".." in b for b in re.findall(r"\{([^{}]*)\}", tok)):
        return UNKNOWN
    miss = []

    def sub(m):
        v = vars_[m[1]] if m[1] in vars_ else os.environ.get("HOME") if m[1] == "HOME" else None
        if v is None or "$" in v:
            miss.append(m[1])
            return ""
        return v
    out = VAR.sub(sub, tok)
    return UNKNOWN if miss or "$" in out else out


def uv_run(argv):
    """Index of `run` in `uv [global options] run ...`, else None."""
    i = 1
    while i < len(argv) and argv[i].startswith("-"):
        i += 2 if argv[i] in WRAPPERS["uv"] else 1
    return i if argv[i:i + 1] == ["run"] else None


def wrapper(argv):
    w = os.path.basename(argv[0])
    return argv[0] in SKIP or w in WRAPPERS and (w != "uv" or uv_run(argv) is not None)


def strip(words, vars_, outs=None):
    """argv after assignments and wrappers (rtk, env, timeout, xargs, uv run...); records VAR=value of a bare assignment
    and of an export or env, and appends the file of `time -o` to outs. xargs appends its input, or puts it at the -I
    string, as an unresolvable ARG."""
    def keep(m):
        v = expand(m[2], vars_)
        vars_[m[1]] = v if v is not None else "$" + m[1]
    i = 0
    while i < len(words) and (m := ASSIGN.match(words[i])):
        keep(m)
        i += 1
    argv = words[i:]
    while argv and wrapper(argv):
        w, argv, repl = os.path.basename(argv[0]), argv[1:], None
        if w == "command" and argv[:1] and argv[0] in ("-v", "-V"):
            return []
        if w == "uv":
            argv = argv[uv_run(["uv"] + argv):]
        takes = WRAPPERS.get(w, set())
        while argv and (argv[0].startswith("-") or ASSIGN.match(argv[0])):
            a = argv[0]
            if m := ASSIGN.match(a):
                keep(m)
                argv = argv[1:]
                continue
            if a == "--":
                argv = argv[1:]
                break
            if w == "time" and (m := re.fullmatch(r"-[ahlpqvV]+o(.*)", a)):
                a = "-o" + m[1]
            name, val, used = (a, argv[1] if len(argv) > 1 else "", 2) if a in takes else \
                (*a.split("=", 1), 1) if a.startswith("--") and "=" in a else \
                (a[:2], a[2:], 1) if a[:2] in takes else (a, None, 1)
            argv = argv[used:]
            if w == "time" and name in ("-o", "--output") and outs is not None:
                outs.append(val)
            elif w == "xargs" and (name in ("-I", "-J", "--replace") or name.startswith("-i")):
                repl = val or (name[2:] if name.startswith("-i") else "") or "{}"
            elif w == "env" and name in ("-S", "--split-string"):
                argv = (val or "").split() + argv
        if w == "timeout":
            argv = argv[1:]
        if w == "xargs" and argv:
            argv = [x.replace(repl, ARG) for x in argv] if repl else argv + [ARG]
    return argv


def commands(argv, vars_):
    """argv, then each command its find -exec, -execdir, -ok or -okdir runs, with {} as the unresolvable ARG."""
    out, i = [argv], 0
    while argv and os.path.basename(argv[0]) == "find" and i < len(argv):
        if argv[i] in FIND_EXEC:
            j = next((k for k in range(i + 1, len(argv)) if argv[k] in (";", "+")), len(argv))
            out.append(strip([x.replace("{}", ARG) for x in argv[i + 1:j]], vars_))
            i = j
        i += 1
    return out


# Each path group is greedy over a class that excludes its own terminator, and code_targets strips it, so no
# two quantifiers compete for the same spaces: lazy groups between \s* went super-linear on `open(` + spaces.
CODE_WRITES = [
    (re.compile(r"\bopen\(([^,()]+),\s*(?:mode\s*=\s*)?['\"][rbt]*[wax+]"), 1),
    (re.compile(r"Path\(([^()]+)\)\s*\.\s*(?:write_text|write_bytes|unlink|touch|rename|replace|open\(\s*['\"][rbt]*[wax+])"), 1),
    (re.compile(r"\b(?:shutil\.(?:copy\w*|move)|os\.(?:rename|replace)|copyFile(?:Sync)?|rename(?:Sync)?)\([^,()]+,([^,()]+)[,)]"), 1),
    (re.compile(r"\b(?:os\.(?:remove|unlink)|shutil\.rmtree|(?:writeFile|appendFile|createWriteStream|unlink|rm)(?:Sync)?"
                r"|file_put_contents|File\.write)\(([^,()]+)[,)]"), 1),
    (re.compile(r"\b(?:fopen|File\.open)\(([^,()]+),\s*['\"][rbt]*[wax+]"), 1),
    (re.compile(r"\bos\.open\(([^,()]+),(?!\s*os\.O_RDONLY\s*[,)])"), 1),
    (re.compile(r"\bopen\s*(?:\(\s*)?(?:my\s+)?[$\w]+\s*,\s*['\"]\s*(?:\+?>{1,2}|\+<)([^'\"]*)['\"](?:\s*,([^,()]+)[,)])?"), 2),
]
LITERAL = re.compile(r"""^(['"`])([^'"`$]+)\1$""")


def code_targets(code):
    out = []
    for rx, groups in CODE_WRITES:
        for m in rx.finditer(code):
            first = (m[1] or "").strip()
            raw = (first or (m[2] if groups == 2 else None) or "").strip()
            lit = LITERAL.match(raw)
            out.append(raw if groups == 2 and first and not lit else lit[2] if lit else UNKNOWN)
    return out


def positionals(args, with_arg=()):
    out, i = [], 0
    while i < len(args):
        a = args[i]
        if a in with_arg:
            i += 2
            continue
        if not a.startswith("-") or a == "-":
            out.append(a)
        i += 1
    return out


def opt_files(args, rx):
    """The files of the options rx matches: the value attached in any group, else the next word; `-` is stdout."""
    out = []
    for k, a in enumerate(args):
        if m := re.fullmatch(rx, a):
            v = next((g for g in m.groups() if g), None) or (args[k + 1] if k + 1 < len(args) else "-")
            out += [v] if v != "-" else []
    return out


def awk_targets(code):
    """The files a print or printf of an awk program redirects to with > or >>: a lone string literal, else UNKNOWN."""
    out = []
    for m in re.finditer(r"\bprintf?\b", code):
        j, depth, n = m.end(), 0, len(code)
        while j < n and not (depth == 0 and code[j] in ";}\n|"):
            c = code[j]
            if c == '"':
                j += 1
                while j < n and code[j] != '"':
                    j += 2 if code[j] == "\\" else 1
            elif c in "([":
                depth += 1
            elif c in ")]":
                depth -= 1
            elif c == ">" and depth == 0:
                lit = re.match(r'>?\s*"((?:[^"\\]|\\.)*)"\s*(?:$|[;}\n])', code[j + 1:])
                out.append(lit[1] if lit else UNKNOWN)
                break
            j += 1
    return out


def tar_targets(args):
    """The archive tar creates, appends or updates (-f), and the folder it extracts into (-C, else the cwd)."""
    mode, val, i = set(), {}, 0
    old = bool(args) and re.fullmatch(r"[A-Za-z]+", args[0]) is not None
    while i < len(args):
        a, i = args[i], i + 1
        if a.startswith("--"):
            k, eq, v = a.partition("=")
            ch = TAR_LONG.get(k)
            if ch in ("f", "C"):
                if not eq:
                    v, i = (args[i] if i < len(args) else ""), i + 1
                val[ch] = v
            elif ch:
                mode.add(ch)
            continue
        dash = a.startswith("-")
        letters = a[1:] if dash else a if old and i == 1 else ""
        for p, ch in enumerate(letters):
            if ch not in TAR_VAL:
                mode.add(ch)
                continue
            if dash and letters[p + 1:]:
                val[ch] = letters[p + 1:]
            else:
                val[ch], i = (args[i] if i < len(args) else ""), i + 1
            if dash:
                break
    out = [val["f"]] if mode & set("cruA") and val.get("f", "-") != "-" else []
    return out + ([val.get("C") or "."] if "x" in mode and "O" not in mode else [])


def absolute(t, cwd, vars_):
    p = UNKNOWN if t is UNKNOWN else expand(t, vars_)
    if p is UNKNOWN or cwd is UNKNOWN and not os.path.isabs(p):
        return UNKNOWN
    return os.path.normpath(p if os.path.isabs(p) else os.path.join(cwd, p))


def analyse(cmd, cwd, vars_, depth=0):
    """(write targets as (absolute path or UNKNOWN, raw, hard-link source), argvs) of a shell command. A target at or
    under a link that an earlier `ln` of the same command made is UNKNOWN: the link does not exist yet, so realpath
    cannot follow it. A cwd the gate cannot follow (`cd -`, an unresolved cd, popd) is UNKNOWN, so a later relative
    target is UNKNOWN; a subshell's cd and assignments end at its `)`, and a cd in a pipeline or in the background
    changes nothing. Every $( ) and backtick body is analysed as a command of its own."""
    targets, argvs, links = [], [], []

    def add(fs, c, src=False):
        for f in fs:
            p = UNKNOWN if f is UNKNOWN or f == UNREAD else expand(f, vars_)
            p = p if p is UNKNOWN or os.path.isabs(p) else UNKNOWN if c is UNKNOWN else os.path.join(c, p)
            u = p is not UNKNOWN and any((a := os.path.normpath(p)) == ln or a.startswith(ln + os.sep) for ln in links)
            targets.append((UNKNOWN if u else p, f, src))

    # Lazy, so a segment's assignments and cd reach only the segments after it.
    def runs():
        nonlocal cwd, targets, argvs
        saved, prev = [], None
        for seg in segments(lex(cmd)):
            if "(" in seg:
                saved.append((cwd, dict(vars_)))
                prev = None
                continue
            if ")" in seg:
                if saved:
                    cwd, keep = saved.pop()
                    vars_.clear()
                    vars_.update(keep)
                continue
            piped, prev = seg["op"] in ("|", "|&", "&") or prev in ("|", "|&"), seg["op"]
            for body in seg["s"]:
                t2, a2 = analyse(body, cwd, dict(vars_), depth)
                targets += t2
                argvs += a2
            outs = []
            cmds = commands(strip(seg["w"], vars_, outs), vars_)
            add(seg["r"] + outs, cwd)
            for words in cmds:
                yield seg, words, piped
    for seg, words, piped in runs():
        argv = [w if (x := expand(w, vars_)) is UNKNOWN else x for w in words]
        if not argv:
            continue
        argvs.append(argv)
        name, args, found = os.path.basename(argv[0]), argv[1:], []
        shell_c = next((k for k, a in enumerate(args) if re.fullmatch(r"-[A-Za-z]*c[A-Za-z]*", a)), None) if name in SHELLS else None
        if name in ("cd", "pushd", "popd"):
            pos = positionals(args)
            follow = name != "popd" and len(pos) < 2 and not (pos and re.match(r"-$|~-(/|$)", pos[0])) and \
                (name == "cd" or pos and pos[0][:1] not in "+-")
            # pushd -n only adds to the stack; the cwd stays.
            if not piped and not (name == "pushd" and "-n" in args):
                cwd = UNKNOWN if not follow else absolute(pos[0], cwd, vars_) if pos else os.path.expanduser("~")
        elif name == "tee":
            found += positionals(args)
        elif name == "sed" and any(re.match(r"^-[A-Za-z]*i|^--in-place", a) for a in args):
            script, rest, i = False, [], 0
            while i < len(args):
                a = args[i]
                if a in ("-e", "-f", "--expression", "--file"):
                    script, i = True, i + 2
                    continue
                if a == "-i" and i + 1 < len(args) and (args[i + 1] == "" or re.match(r"^\.[^/]*$", args[i + 1])):
                    i += 2
                    continue
                if not a.startswith("-"):
                    rest.append(a)
                i += 1
            found += rest if script else rest[1:]
        elif name in ("cp", "install", "ln"):
            if "-t" in args and args.index("-t") + 1 < len(args):
                found += [args[args.index("-t") + 1]]
            else:
                pos = positionals(args)
                found += pos[-1:] if len(pos) > 1 else [os.path.basename(p) for p in pos[:1] if name == "ln"]
            # A hard link is a second name of its source, so a write through it writes the source.
            if name == "ln" and not any(a == "--symbolic" or re.fullmatch(r"-[A-Za-z]*s[A-Za-z]*", a) for a in args) or \
                    name == "cp" and any(a == "--link" or re.fullmatch(r"-[A-Za-z]*l[A-Za-z]*", a) for a in args):
                pos = positionals(args, ("-t", "-S"))
                add(pos if "-t" in args else pos[:-1] if len(pos) > 1 else pos, cwd, True)
        elif name == "link":
            found += args[1:2]
            add(args[:1], cwd, True)
        elif name in VALUE_OPTS:
            found += positionals(args, VALUE_OPTS[name])
        elif name == "dd":
            found += [a[3:] for a in args if a.startswith("of=")]
        elif name == "wget":
            for k, a in enumerate(args):
                m = re.fullmatch(r"-[qvdNcSrkKmpxHLFE]*([Ooa])(.*)|--(?:output-document|output-file|append-output)(=?)(.*)", a)
                if m:
                    v = m[2] if m[1] else m[4] if m[3] else ""
                    found += [f for f in [v or (args[k + 1] if k + 1 < len(args) else "-")] if f != "-"]
        elif name == "curl":
            found += opt_files(args, r"-[sSLfkviIlnNqjgG0-46#ZaBpRMVOJ]*o(.*)|--output(?:=(.*))?")
        elif name == "sort":
            found += opt_files(args, r"-[bdfgiMhnRrsuVcCmz]*o(.*)|--output(?:=(.*))?")
        elif name == "rsync":
            pos = positionals(args, ("-e", "-f", "-B", "-M", "-T", "--rsh", "--filter", "--exclude", "--include",
                                     "--exclude-from", "--include-from", "--files-from", "--log-file", "--temp-dir",
                                     "--backup-dir", "--partial-dir", "--compare-dest", "--copy-dest", "--link-dest",
                                     "--chmod", "--chown", "--suffix", "--rsync-path", "--password-file", "--out-format"))
            # A colon before the first slash names a remote host, as rsync itself reads it.
            found += [p for p in pos[-1:] if len(pos) > 1 and not re.match(r"[^/]*:", p)]
        elif re.fullmatch(r"[gmn]?awk", name):
            code = [args[k + 1] for k, a in enumerate(args[:-1]) if a in ("-e", "--source")]
            if not code and "-f" not in args:
                code = positionals(args, ("-F", "-v"))[:1]
            found += [t for c in code for t in awk_targets(c)]
        elif name == "tar":
            found += tar_targets(args)
        elif name == "patch" and not {"--dry-run", "--check", "-C"} & set(args):
            d = opt_files(args, r"-d(.+)?|--directory(?:=(.*))?")[-1:]
            pos = positionals(args, ("-p", "-d", "-i", "-o", "-r", "-B", "-D", "-F", "-V", "-Y", "-z", "-g", "--directory",
                                     "--input", "--output", "--reject-file", "--prefix", "--suffix"))
            found += [os.path.join(*d, f) for f in opt_files(args, r"-o(.+)?|--output(?:=(.*))?") or pos[:1]] or [UNREAD]
        elif name == "git":
            sub, rest = git_sub(args)
            if sub == "restore" and not ({"-S", "--staged"} & set(rest) and not {"-W", "--worktree"} & set(rest)):
                pre = args[:len(args) - len(rest) - 1]
                base = [pre[k + 1] for k in range(len(pre) - 1) if pre[k] == "-C"]
                paths = positionals(rest, ("-s", "--source")) + \
                    [UNKNOWN for a in rest if a.startswith("--pathspec-from-file")]
                found += [p if p is UNKNOWN else os.path.join(*base, p) for p in paths]
        elif shell_c is not None or name == "eval" and args:
            inner = " ".join(args) if name == "eval" else (positionals(args[shell_c + 1:], ("-o", "-O")) or [""])[0]
            if depth >= 3:
                deny("R24 nesting limit", f"`{name}` nests a fourth shell -c or eval, past the gate's nesting limit of 3, so the "
                     f"gate cannot judge `{inner[:80]}`. Run it with fewer levels of nesting.")
            t2, a2 = analyse(inner, cwd, dict(vars_), depth + 1)
            targets += t2
            argvs += a2
        elif INTERP.match(name):
            code = [args[i + 1] for i, a in enumerate(args[:-1]) if a in ("-c", "-e", "-E", "-r")] + seg["b"]
            found += code_targets("\n".join(code))
            if name == "perl" and any(re.match(r"^-[A-Za-z]*i", a) for a in args):
                found += positionals(args, ("-e", "-E"))
        add(found, cwd)
        if name == "ln":
            links += [a for f in found if (a := absolute(f, cwd, vars_)) is not UNKNOWN]
    return targets, argvs


def git_sub(args):
    i = 0
    while i < len(args) and args[i].startswith("-"):
        i += 2 if args[i] in ("-C", "-c", "--git-dir", "--work-tree", "--namespace") else 1
    return (args[i], args[i + 1:]) if i < len(args) else (None, [])


def branch_write(sub, rest):
    if sub == "commit":
        return "commit"
    if sub == "checkout" and any(a in ("-b", "-B", "--orphan") for a in rest):
        return "checkout -b"
    if sub == "switch" and any(a in ("-c", "-C", "--create", "--force-create", "--orphan") for a in rest):
        return "switch -c"
    if sub == "branch":
        if any(a in ("--list", "-l") for a in rest):
            return None
        pos = positionals(rest, ("--contains", "--no-contains", "--merged", "--no-merged", "--points-at", "--sort", "--format"))
        flags = {"-d", "-D", "-m", "-M", "-c", "-C", "-f", "-u", "--delete", "--move", "--copy", "--force",
                 "--set-upstream-to", "--unset-upstream", "--edit-description"}
        if pos or flags & set(rest) or any(a.startswith("--set-upstream-to=") for a in rest):
            return "branch"
    if sub in ("merge", "cherry-pick", "revert", "am", "rebase"):
        return sub
    if sub == "tag":
        if any(a in ("-l", "--list", "-v", "--verify") for a in rest):
            return None
        pos = positionals(rest, ("--contains", "--no-contains", "--merged", "--no-merged", "--points-at", "--sort",
                                 "--format", "-m", "-F", "-u", "--message", "--file", "--local-user", "--cleanup"))
        flags = {"-a", "-s", "-f", "-d", "-m", "-F", "-u", "-e", "--annotate", "--sign", "--force", "--delete", "--message",
                 "--file", "--local-user", "--edit"}
        if pos or flags & set(rest) or any(a.startswith(("--message=", "--file=")) for a in rest):
            return "tag"
    return None


GH_VAL = {"-b", "--body", "-F", "--body-file", "-t", "--title", "-B", "--base", "-H", "--head", "-R", "--repo", "-T",
          "--template", "-A", "--author-email", "--subject", "--match-head-commit", "--add-label", "--remove-label",
          "--add-reviewer", "--remove-reviewer", "--add-assignee", "--remove-assignee", "--add-project", "--remove-project",
          "--milestone"}
API_VAL = ("-X", "--method", "-f", "--raw-field", "-F", "--field", "-H", "--header", "--input", "-q", "--jq", "-t",
           "--template", "--cache", "-p", "--preview", "--hostname")


def publish(argv):
    """(the command family as the publish gate names it, the PR or issue number or branch positionals it carries)."""
    name, args = os.path.basename(argv[0]), argv[1:]
    if name == "git":
        sub, rest = git_sub(args)
        return ("git push", positionals(rest, ("-o", "--push-option", "--repo", "--receive-pack", "--exec"))[1:]) \
            if sub == "push" else None
    if name != "gh" or len(args) < 2:
        return None
    a, b = args[0], args[1]
    if a == "pr" and b in ("create", "comment", "review", "merge", "edit") or a == "issue" and b == "comment":
        # gh pr merge reads -m, -r, -s and -d as switches; the other subcommands take a value after them.
        val = GH_VAL | ({"-r", "--reviewer", "-a", "--assignee", "-l", "--label", "-m", "-p", "--project"} if b != "merge" else set())
        return f"gh {a} {b}", positionals(args[2:], val)[:1]
    if a == "repo" and b == "create" and "--public" in args:
        return "gh repo create", []
    if a == "repo" and b == "edit" and any(x in ("public", "--visibility=public") for x in args):
        return "gh repo edit", []
    end = positionals(args[1:], API_VAL)[:1] if a == "api" else []
    if not end or not re.search(r"/(?:pulls|issues)/\d+/(?:comments|reviews)|/pulls/?$", end[0]):
        return None
    method = [m.upper() for m in opt_files(args[1:], r"-X(.+)?|--method(?:=(.*))?")]
    field = any(x in ("-f", "-F", "--field", "--raw-field", "--input") or
                x.startswith(("-f", "-F", "--field=", "--raw-field=", "--input=")) for x in args[1:])
    if field or {"POST", "PATCH", "PUT"} & set(method):
        return "gh api", re.findall(r"/(?:pulls|issues)/(\d+)", end[0])[:1]
    return None


def last_answer(path):
    """(questions, answers, tool_use id) of the last AskUserQuestion answered after the owner's last prompt in the transcript
    tail, or None. A prompt queued during a turn counts as the owner's prompt; a sidechain entry never counts. Answers count
    only for an AskUserQuestion call seen in the tail, and any others fail closed. The questions come from the tool
    result, else from the call. protocolo carries a copy of this reader; proofs/C6-caller.sh holds the two to one decision."""
    try:
        with open(path, "rb") as f:
            f.seek(0, 2)
            f.seek(max(0, f.tell() - 2_000_000))
            lines = f.read().decode("utf-8", "replace").splitlines()
    except OSError:
        return None
    last, asked = None, {}
    for line in lines:
        if '"user"' not in line and '"AskUserQuestion"' not in line and '"queued_command"' not in line:
            continue
        try:
            e = json.loads(line)
        except ValueError:
            continue
        if not isinstance(e, dict) or e.get("isSidechain"):
            continue
        c = (e.get("message") or {}).get("content")
        if e.get("type") == "assistant":
            for b in c if isinstance(c, list) else []:
                if isinstance(b, dict) and b.get("type") == "tool_use" and b.get("name") == "AskUserQuestion":
                    asked[b.get("id")] = (b.get("input") or {}).get("questions") or []
            continue
        if e.get("type") == "attachment":
            last = None if (e.get("attachment") or {}).get("type") == "queued_command" else last
            continue
        if e.get("type") != "user" or e.get("isMeta"):
            continue
        if isinstance(c, list) and any(isinstance(b, dict) and b.get("type") == "tool_result" for b in c):
            r = e.get("toolUseResult")
            if isinstance(r, dict) and isinstance(r.get("answers"), dict) and r["answers"]:
                ids = [b.get("tool_use_id") for b in c if isinstance(b, dict) and b.get("type") == "tool_result"]
                last = (r.get("questions") or asked[ids[0]], r["answers"], ids[0]) if ids and all(i in asked for i in ids) else None
        elif c:
            last = None
    return last


def named(text, word):
    # A period right after the word counts as a boundary only when it ends a sentence: "planning." yes, "planning.md" no.
    return re.search(r"(?<![\w./-])" + re.escape(word) + r"(?![\w/-]|\.\S)", text) is not None


def approved(path, act, tokens):
    """True when the last answered AskUserQuestion of the turn has a question with header Publish whose text names act
    and every token on word boundaries, and the owner chose the label Publish for it."""
    got = last_answer(path)
    return bool(got) and any(isinstance(q, dict) and q.get("header") == "Publish" and
                             got[1].get(q.get("question")) in ("Publish", ["Publish"]) and
                             all(named(str(q.get("question")), x) for x in [act] + tokens) for q in got[0])


# The header of an approval question and the option label that approves.
PAIRS = {"Publish": "Publish", "Phase": "Mover", "QA": "Registrar"}
REVIEW = ("code-review", "simplify", "ponytail-review", "security-review")


def approval_round(qs):
    """True for a round of exactly one question whose header is Publish, Phase or QA and that offers its pair's label."""
    return (isinstance(qs, list) and len(qs) == 1 and isinstance(qs[0], dict) and qs[0].get("header") in PAIRS
            and any(isinstance(o, dict) and o.get("label") == PAIRS[qs[0]["header"]] for o in qs[0].get("options") or []))


def move_key(mode, arg):
    """(header, the word the text names, the alternatives of that word) of a gated protocolo call."""
    if mode == "--qa":
        return "QA", arg, REVIEW
    return "Phase", "validate-context" if mode == "--validate-context" else arg, ORDER + ("validate-context",)


def move_approval(path, task_id, mode, arg, spent, result=None):
    """The tool_use id that approves `protocolo <mode> <task> [arg] [result]`, or None: the last AskUserQuestion answered
    after the owner's last prompt has one question with the mode's header, the owner chose its pair's label, the text names the
    task id and the word on word boundaries and no other alternative of the word, for --qa it names exactly one of pass
    and fail and that is the call's result, and the id is not in spent. protocolo carries a copy of this function."""
    got = last_answer(path)
    if not got or len(got[0]) != 1 or not isinstance(got[0][0], dict) or got[2] in spent:
        return None
    header, word, alts = move_key(mode, arg)
    q, label = got[0][0], PAIRS[header]
    text = str(q.get("question"))
    ok = (word and q.get("header") == header and got[1].get(q.get("question")) in (label, [label]) and named(text, task_id)
          and named(text, word) and not any(named(text, x) for x in alts if x != word)
          and (mode != "--qa" or [x for x in ("pass", "fail") if named(text, x)] == [result]))
    return got[2] if ok else None


def allowed(rel, ws):
    return any(rel == w.rstrip("/") or w.endswith("/") and rel.startswith(w) for w in ws)


def one_writer(p, st, via):
    if os.path.basename(p).lower() in PROTECTED or p.startswith(st["bind"] + os.sep) or p == st["bind"]:
        deny("R22 one writer", f"{via} writes {p}. gate.json, the ledger and the bind index are written only by protocolo, "
             f"in every phase. Record an event with `{PROTOCOLO} --event <logbook.jsonl> <type> \"<text>\"`; a phase move is "
             f"The owner's `! {PROTOCOLO} --phase {st['task'] or '<task>'} <phase>`.")


def claude_home(transcript):
    """CLAUDE_CONFIG_DIR, else the home whose projects/<folder>/ holds the transcript; never an assumed ~/.claude."""
    if os.environ.get("CLAUDE_CONFIG_DIR"):
        return os.path.realpath(os.environ["CLAUDE_CONFIG_DIR"])
    d = os.path.dirname(transcript or "")
    while d and d != os.path.dirname(d):
        if os.path.basename(os.path.dirname(d)) == "projects":
            return os.path.realpath(os.path.dirname(os.path.dirname(d)))
        d = os.path.dirname(d)
    return None


def outside_ok(p, st):
    """True for the two places outside its task a bound session writes: <home>/projects/<one folder>/memory/ and below,
    and the _scratch/ of the umbrella whose .devwork/tasks/ holds the task."""
    t, home = st["task"], st.get("home")
    root = os.path.dirname(os.path.dirname(t))
    if os.path.basename(os.path.dirname(t)) == "tasks" and os.path.basename(root) == ".devwork" and \
            p.startswith(os.path.join(root, "_scratch") + os.sep):
        return True
    parts = os.path.relpath(p, os.path.join(home, "projects")).split(os.sep) if home else []
    return len(parts) > 2 and parts[0] != os.pardir and parts[1] == "memory"


def check_path(path, raw, st, via):
    if path is UNKNOWN:
        deny("R24 phase write list", f"{via} writes to `{raw}` and the gate cannot resolve it to a file. Write the literal path, "
             "or use the Write tool, so the gate can judge it.")
    if path in ("/dev/null", "/dev/stdout", "/dev/stderr", "/dev/tty") or path.startswith("/dev/fd/"):
        return
    p = os.path.realpath(path)
    one_writer(p, st, via)
    if st["error"]:
        deny("gate state", f"{st['error']} The write to {p} was not judged, so it is blocked.")
    t, phase, frozen = st["task"], st["gate"]["phase"], st["gate"].get("frozen")
    if p == t or p.startswith(t + os.sep):
        rel = p[len(t) + 1:]
        if rel == "context.md" and frozen:
            deny("R22 frozen context.md", f"{via} changes {p}, which the owner validated at {frozen.get('at')}. A change is his: he edits "
                 f"it and runs `! {PROTOCOLO} --validate-context {t}`.")
        if allowed(rel, WRITE[phase]) or rel == "context.md" and phase == "discovery":
            return
        top = rel.split(os.sep)[0]
        if top and top not in {n.rstrip("/") for n in LAYOUT[0] + LAYOUT[1]} and not os.path.lexists(os.path.join(t, top)):
            ext = os.path.splitext(rel)[1]
            kind = "sql/" if ext == ".sql" else "bin/" if ext in (".sh", ".py") else ""
            put = {"discovery": "research/" + kind, "planning": "research/" + kind, "execution": kind or "research/",
                   "qa": "research/qa/", "continuity": "backup/"}.get(phase)
            deny("R20 task layout", f"{via} creates the top-level {'folder' if os.sep in rel else 'file'} {top} in {t}, outside "
                 f"the fixed task layout: files {', '.join(LAYOUT[0])}; folders {', '.join(LAYOUT[1])}. "
                 + (f"Put it under {put} in phase {phase}." if put else f"Phase {phase} writes only what protocolo generates; "
                    f"The owner reopens the task with `! {PROTOCOLO} --phase {t} <phase>`."))
        later = next((q for q in ORDER if allowed(rel, WRITE[q])), None)
        way = (f"{rel} opens in phase {later}: the owner moves the phase with `! {PROTOCOLO} --phase {t} {later}`."
               if later and ORDER.index(later) > ORDER.index(phase) else "The owner moves the phase with "
               f"`! {PROTOCOLO} --phase {t} <phase>` when the work needs it.")
        deny("R02 phase write list", f"{via} writes {rel} in phase {phase}, which writes only: "
             f"{', '.join(WRITE[phase]) or 'nothing'}. {way}")
    if phase not in ("closed", "continuity") and outside_ok(p, st):
        return
    repos = [os.path.realpath(r) for r in st["gate"].get("repos") or []]
    if phase == "execution" and any(p == r or p.startswith(r + os.sep) for r in repos):
        return
    deny("R02 phase write list", f"{via} writes {p}, outside the task folder {t}. Phase {phase} writes only inside it"
         + (f" and in the task's repos ({', '.join(repos) or 'none listed in gate.json'})" if phase == "execution" else "")
         + ". Write under research/ instead, or ask the owner.")


def judge_bash(cmd, cwd, st, transcript):
    targets, argvs = analyse(cmd, cwd or st.get("task") or "/", {})
    for argv in argvs:
        name = os.path.basename(argv[0])
        if name in SQL:
            deny("R12 SQL by the owner only", f"`{name}` is a SQL client, and the owner runs every query. Write the query to a .sql file under "
                 "research/sql/ (or sql/ in execution) and hand the owner the one `! ` line that runs it.")
        pr = positionals(argv[1:], ("-W", "-X", "-c", "-m"))[:1] if INTERP.match(name) else argv[:1]
        if not (pr and os.path.basename(pr[0]).lower() == "protocolo"):
            continue
        if {"--bind", "--round", "--answer"} & set(argv):
            deny("R02 phase moves", "protocolo --bind, --round and --answer change gate.json, and only the owner runs them, with "
                 "a `! ` line. Hand him the line.")
        rest = argv[argv.index(pr[0]) + 1:]
        flags = sorted(a for a in rest if a.startswith("-"))
        task = st["task"] or "<task>"
        # Any redirect, pipe, list or substitution is refused on the raw text, so 2>&1 is caught with no file target.
        alone = len(argvs) == 1 and not re.search(r"[$`<>|&;]", cmd)
        own = lambda mode: bool(st["task"] and (w := rest[rest.index(mode) + 1:][:1]) and os.path.isabs(w[0])
                                and os.path.realpath(w[0]) == st["task"])
        how = (f"the whole command, on its own task folder written as an absolute path: no second command, no other option, "
               "no redirect, and no $, backtick, <, >, |, & or ; in the text")
        modes = [m for m in ("--question", "--measured") if m in argv]
        if modes and (len(modes) > 1 or not alone or not own(modes[0]) or flags not in (
                [modes[0]], sorted([modes[0], {"--question": "--owner", "--measured": "--pointer"}[modes[0]]]))):
            deny("R03 question register", f"a bound session runs, as {how}, either `{PROTOCOLO} --question {task} \"<text>\" "
                 f"--owner claude|user` or `{PROTOCOLO} --measured {task} <Qn> \"<result>\" --pointer <file in the task>`.")
        for mode in ("--phase", "--validate-context", "--qa"):
            if mode not in argv:
                continue
            arg = "" if mode == "--validate-context" else (rest[rest.index(mode) + 2:][:1] or [""])[0]
            result = (rest[rest.index(mode) + 3:][:1] or [""])[0] if mode == "--qa" else None
            header, word, _ = move_key(mode, arg)
            tid = st["gate"]["task_id"] if not st["error"] else "<task id>"
            form = [mode] if mode != "--qa" else [mode, "--pointer"]
            if alone and own(mode) and flags in ([mode], sorted(form)) and (mode != "--qa" or arg in REVIEW) and \
                    not st["error"] and move_approval(transcript, tid, mode, arg, st["gate"].get("approvals") or [], result):
                continue
            qa = mode == "--qa"
            ask = (f"Ask him with AskUserQuestion: one question with header `{header}`, whose text names `{tid}` and "
                   f"`{word or ('<item>' if qa else '<target>')}`"
                   + (f" and the result, `{result if result in ('pass', 'fail') else 'pass` or `fail'}`, the one the "
                      "command carries" if qa else "")
                   + f", with an option labelled `{PAIRS[header]}`. The text names the task id and one "
                   + ("item and one result (never both pass and fail)" if qa else "target") + " as separate words, with "
                   f"no other {'review item' if qa else 'phase'} named. Once he chooses {PAIRS[header]}, run "
                   f"`{PROTOCOLO} {mode} {task} "
                   + (f"{word or '<item>'} {result if result in ('pass', 'fail') else 'pass|fail'} \"<text>\"" if qa else arg)
                   + f"` as {how}.")
            if mode != "--qa":
                deny("R02 phase moves", f"protocolo {mode} changes gate.json, and only the owner approves it. {ask}")
            # The qa-record hook's own protocolo --qa tests call runs inside the hook, so this deny never reaches it.
            if arg not in REVIEW:
                deny("R26 QA checklist", "protocolo --qa records an item of the close checklist, and a bound session does "
                     "not record its own result. The tests item is the qa-record hook's, from the test run itself; report "
                     "the result with its pointer, and the close runs the frozen done_when as the test.")
            deny("R26 QA checklist", "protocolo --qa records a review item of the close checklist, and only the owner approves "
                 f"its result. {ask}")
    phase = None if st["error"] else st["gate"]["phase"]
    for argv in argvs:
        if os.path.basename(argv[0]) == "git":
            sub, rest = git_sub(argv[1:])
            what = branch_write(sub, rest) or ("push" if sub == "push" else None)
            if what and phase != "execution":
                deny("R02 phase gate", f"git {what}: a branch, a commit or a push runs only in phase execution; the task is in "
                     f"{phase or 'an unreadable state'}. The owner moves it with `! {PROTOCOLO} --phase {st['task']} execution`.")
        pub = publish(argv)
        if pub and not approved(transcript, *pub):
            act, tokens = pub
            deny("R13 publish gate", f"`{act}` publishes, and the last AskUserQuestion answered since the owner's last prompt does "
                 "not approve it. Run the pre-publish check (branch target, commit convention, review done, destination), then "
                 f"ask the owner with AskUserQuestion: one question with header `Publish`, whose text names `{act}`"
                 + "".join(f" and `{x}`" for x in tokens) + ", with an option labelled `Publish`; run it again once he "
                 "chooses Publish.")
    for path, raw, src in targets:
        if not src:
            check_path(path, raw, st, "This Bash command")
        elif path is UNKNOWN:
            check_path(path, raw, st, "This Bash command's hard link (ln without -s)")
        else:
            one_writer(os.path.realpath(path), st, "This Bash command's hard link (ln without -s)")


def judge_ask(inp, st):
    qs = inp.get("questions") or []
    if len(qs) > 4:
        deny("R03 question rounds", f"{len(qs)} questions in one round; the cap is 4. Keep the 4 that only the owner can answer.")
    if st["error"]:
        return
    g = st["gate"]
    # The round rules are discovery's; an approval round asks the owner to decide an action, never to answer research.
    if g["phase"] != "discovery" or approval_round(qs):
        return
    mark = os.path.join(st["task"], ROUND_MARK)
    if os.path.lexists(mark):
        deny("R03 question rounds", f"{mark} says the last AskUserQuestion round was not recorded, so the round count is "
             f"unknown. The owner records it with `! {PROTOCOLO} --round {st['task']}`, which also removes the mark.")
    rounds = g.get("rounds") or 0
    if rounds >= 2:
        deny("R03 question rounds", f"gate.json records rounds={rounds}; the cap is 2, so no third AskUserQuestion round. "
             "Decide from the evidence and state the assumption; the owner opens another round himself if he wants one.")
    for q in g.get("questions") or []:
        if q.get("owner") == "claude" and q.get("status") == "open":
            deny("R03 research before questions", f"{q.get('id')} is measurable and still open: {q.get('q')} Measure it first, write the "
                 f"measurement to a file in the task, and close it with `{PROTOCOLO} --measured {st['task']} {q.get('id')} "
                 f"\"<result>\" --pointer <that file>`; the owner can also record it with "
                 f"`! {PROTOCOLO} --answer {st['task']} {q.get('id')} \"<answer>\"`.")


def state(sid):
    bind = os.environ.get("GATE_BIND_DIR") or os.path.join(
        os.environ.get("CLAUDE_CONFIG_DIR") or os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "state", "bind")
    if not sid or not re.fullmatch(r"[A-Za-z0-9_-]+", sid):
        return None
    idx = os.path.join(bind, sid)
    try:
        raw = open(idx, encoding="utf-8").read().strip()
    except FileNotFoundError:
        return None
    st = {"bind": os.path.realpath(bind), "task": None, "gate": None, "error": None}
    fix = f"The owner re-binds with `! {PROTOCOLO} --bind <task> --session {sid}`, or removes {idx} to unbind."
    if not raw or not os.path.isdir(raw):
        st["error"] = f"The bind index {idx} names no task folder ({raw or 'empty'}). {fix}"
        return st
    st["task"] = os.path.realpath(raw)
    gp = os.path.join(st["task"], "gate.json")
    try:
        g = json.load(open(gp, encoding="utf-8"))
        if not isinstance(g, dict) or g.get("phase") not in WRITE:
            raise ValueError(f"phase {g.get('phase') if isinstance(g, dict) else '?'!r} is not one of {', '.join(ORDER)}")
    except (OSError, ValueError) as e:
        st["error"] = f"{gp} is unreadable ({type(e).__name__}: {e}). {fix}"
        return st
    if sid not in (g.get("sessions") or []):
        st["error"] = f"{gp} does not list session {sid} in sessions, so the bind index is stale. {fix}"
        return st
    st["gate"] = g
    return st


def main():
    ev = json.loads(sys.stdin.read() or "null")
    if not isinstance(ev, dict):
        deny("gate input", "the hook received no JSON object, so the call was not judged.")
    st = state(ev.get("session_id"))
    if st is None:
        return
    st["home"] = claude_home(ev.get("transcript_path"))
    tool, inp = ev.get("tool_name"), ev.get("tool_input") or {}
    if tool == "Bash":
        judge_bash(inp.get("command") or "", ev.get("cwd"), st, ev.get("transcript_path") or "")
    elif tool in ("Write", "Edit", "MultiEdit", "NotebookEdit"):
        path = inp.get("file_path") or inp.get("notebook_path") or ""
        check_path(path if os.path.isabs(path) else os.path.join(ev.get("cwd") or "/", path), path, st, tool)
    elif tool == "AskUserQuestion":
        judge_ask(inp, st)


if __name__ == "__main__":
    try:
        main()
    except Deny as d:
        print(d, file=sys.stderr)
        sys.exit(2)
    except Exception as e:
        print(f"BLOCKED by gate (internal error): {type(e).__name__}: {e}. The call was not judged, so it is blocked; "
              f"report this line. The owner can unbind the session by removing its file under the bind dir.", file=sys.stderr)
        sys.exit(2)
    sys.exit(0)
