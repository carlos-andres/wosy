#!/usr/bin/env python3
"""qa-record.py: the PostToolUse and PostToolUseFailure hook that records a test run of a task-bound session (harness v2 R26; A4).

For a Bash call of a session bound to a task, when the command is a test run, it calls `protocolo --qa <task> tests
<pass|fail> "<text>"`, which appends one ledger event and sets gate.json qa.tests; this hook writes neither file. A pass is
a PostToolUse with no sign of failure. A fail is a PostToolUseFailure, whose `Exit code N` is kept, or a PostToolUse whose
response is interrupted or starts with a non-zero `Exit code N`. A run whose exit a later `|`, `;`, `||`, `&` or newline
can hide (a `|` is trusted when the command names pipefail) is recorded as a fail marked hidden, never as a pass. One event
per tool_use_id: an id already in the ledger tail is not recorded again. It never disturbs the session: every path exits 0
with no stdout. The binding lookup is anchor.py's. $QA_PROTOCOLO names the protocolo it calls.

Test runs, by the first word after any VAR=value, `rtk` and `env` prefix: pytest, py.test, python* [-flags] -m pytest or
-m unittest, phpunit at any path, php artisan test, php <path>/phpunit, go test, cargo test, npm|pnpm|yarn test or run test,
vitest, jest, bats, and a proof script of the bound task, <task>/proofs/<name>.sh run directly or through bash or sh.

Before the command reaches the ledger, *** replaces the value of a VAR=value, of a credential flag (--password, --token,
--api-key and the like, after = or a space), the userinfo of a URL, an Authorization header value, the user:password of
curl -u or --user, and the password attached to -p of a mysql client. A command with none of these is recorded unchanged.
"""
import json, os, re, sys

PROTOCOLO = os.environ.get("QA_PROTOCOLO") or "/Users/you/.devwork/bin/protocolo"
HINT = re.compile(r"test|jest|phpunit|bats|proofs/")
ASSIGN = re.compile(r"[A-Za-z_][A-Za-z0-9_]*=")
SEPS = {"&&", "||", ";", "|", "&", "|&", ";;"}
CMD_MAX, TIMEOUT, TAIL = 300, 20, 65536
CRED = r"--(?:[A-Za-z0-9]+[-_])*(?:password|passwd|pass|token|secret|api[-_]?key|access[-_]key|auth|key)"
VALUE = r"""("[^"]*"|'[^']*'|[^\s"']\S*)"""
MASKS = (
    (re.compile(r"(?<!\S)([A-Za-z_][A-Za-z0-9_]*)=\S+"), r"\1=***"),
    (re.compile(rf"(?<!\S)({CRED})(=|\s+(?!-))" + VALUE, re.I), r"\1\2***"),
    (re.compile(r"\b([A-Za-z][A-Za-z0-9+.-]*://)[^\s/?#@:'\"]*:[^\s/?#'\"]*@"), r"\1***:***@"),
    (re.compile(r"\b(authorization:\s*)(?:([A-Za-z][\w-]*\s+))?(\"[^\"]*\"|'[^']*'|[^\s\"']+)", re.I),
     lambda m: m[1] + (m[2] or "") + "***"),
    (re.compile(r"(\bcurl\b[^;&|\n]*?\s(?:-u|--user(?![\w-]))(?:\s+|=)?)([\"']?)[^\s:\"']+:[^\s\"']+"), r"\1\2***:***"),
    (re.compile(r"(?:^|(?<=[\s;|&(/]))((?:mysql\w*|mariadb[\w-]*)(?=\s)[^\n;|&]*?\s-p)\S+"), r"\1***"),
)


def parse(cmd):
    """[(words, separator after them)] of a command line; parentheses are dropped."""
    import shlex
    lex = shlex.shlex(cmd.replace("\n", " ; "), posix=True, punctuation_chars=True)
    lex.whitespace_split = True
    segs, cur = [], []
    for tok in lex:
        if tok in ("(", ")"):
            continue
        if tok in SEPS:
            segs.append((cur, tok))
            cur = []
        else:
            cur.append(tok)
    return segs + [(cur, "")]


def kind(w, cwd):
    """"test", the absolute path of a candidate proof script, or None."""
    while w and (ASSIGN.match(w[0]) or w[0] in ("rtk", "env")):
        w = w[1:]
    if not w:
        return None
    b, a = os.path.basename(w[0]), w[1:]
    if b in ("pytest", "py.test", "phpunit", "vitest", "jest", "bats"):
        return "test"
    if re.fullmatch(r"python[0-9.]*", b):
        j = 0
        while j < len(a) and a[j].startswith("-") and a[j] not in ("-m", "-c"):
            j += 1
        return "test" if a[j:j + 1] == ["-m"] and a[j + 1:j + 2] in (["pytest"], ["unittest"]) else None
    if b == "php":
        return "test" if a[:2] == ["artisan", "test"] or (a and os.path.basename(a[0]) == "phpunit") else None
    if b in ("go", "cargo"):
        return "test" if a[:1] == ["test"] else None
    if b in ("npm", "pnpm", "yarn"):
        return "test" if a[:1] == ["test"] or a[:2] == ["run", "test"] else None
    script = w[0]
    if b in ("bash", "sh"):
        while a and a[0].startswith("-"):
            if a[0] == "-c":
                return None
            a = a[1:]
        script = a[0] if a else ""
    if not (script.endswith(".sh") and os.path.basename(os.path.dirname(script)) == "proofs"):
        return None
    if os.path.isabs(script):
        return os.path.normpath(script)
    return os.path.normpath(os.path.join(cwd, script)) if cwd else None


def find(cmd, cwd):
    """([(segment index, kind)], the separator after each segment), following `cd` for relative proof paths."""
    segs, hits = parse(cmd), []
    for n, (words, _) in enumerate(segs):
        if words[:1] == ["cd"]:
            to = words[1] if len(words) > 1 else "~"
            if to[:1] in ("~", "-", "$"):
                cwd = None
            elif os.path.isabs(to) or cwd:
                cwd = os.path.normpath(os.path.join(cwd or "/", to))
            continue
        k = kind(words, cwd)
        if k:
            hits.append((n, k))
    return hits, [s for _, s in segs]


def outcome(ev):
    """(passed, note) from the event; a PostToolUse that shows a failure is a fail."""
    if ev.get("hook_event_name") == "PostToolUseFailure":
        m = re.match(r"\s*Exit code (\d+)", str(ev.get("error") or ""))
        return False, f"Exit code {m[1]}" if m else "interrupted" if ev.get("is_interrupt") else "failed, no exit code in the error"
    resp = ev.get("tool_response")
    if isinstance(resp, dict) and resp.get("interrupted"):
        return False, "interrupted"
    for s in [resp] if isinstance(resp, str) else [resp.get("stdout"), resp.get("stderr")] if isinstance(resp, dict) else []:
        m = re.match(r"\s*Exit code ([1-9][0-9]*)", str(s or ""))
        if m:
            return False, f"Exit code {m[1]}"
    return True, "exit 0"


def seen(led, tid):
    try:
        with open(led, "rb") as f:
            f.seek(0, 2)
            f.seek(max(0, f.tell() - TAIL))
            return f"(tool_use_id {tid})".encode() in f.read()
    except OSError:
        return False


def main():
    ev = json.loads(sys.stdin.read() or "null")
    if not isinstance(ev, dict) or ev.get("tool_name") != "Bash" or ev.get("hook_event_name") not in ("PostToolUse", "PostToolUseFailure"):
        return
    ti, cwd = ev.get("tool_input"), ev.get("cwd")
    cmd = ti.get("command") if isinstance(ti, dict) else None
    if not isinstance(cmd, str) or not HINT.search(cmd):
        return
    hits, seps = find(cmd, cwd if isinstance(cwd, str) and os.path.isabs(cwd) else None)
    if not hits:
        return
    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    from anchor import state
    st = state(ev.get("session_id"))
    if not st or st["error"]:
        return
    task = st["task"]
    hits = [n for n, k in hits if k == "test" or os.path.dirname(os.path.realpath(k)) == os.path.join(task, "proofs")]
    tid = str(ev.get("tool_use_id") or "")
    tid = tid if re.fullmatch(r"[A-Za-z0-9_-]+", tid) else ""
    led = os.path.join(task, "logbook.jsonl")
    if not hits or (tid and seen(led, tid)):
        return
    passed, note = outcome(ev)
    hide = next((s for s in seps[hits[0]:] if s and s != "&&" and not (s == "|" and "pipefail" in cmd)), None)
    if passed and hide:
        passed, note = False, f"exit hidden by `{hide}`, not counted as a pass"
    shown = cmd
    for rx, to in MASKS:
        shown = rx.sub(to, shown)
    shown = " ".join(shown.split())
    shown = shown if len(shown) <= CMD_MAX else shown[:CMD_MAX - 3] + "..."
    text = f"{note}: {shown}" + (f" (tool_use_id {tid})" if tid else "")
    tp = ev.get("transcript_path")
    import subprocess
    r = subprocess.run([sys.executable, "-B", PROTOCOLO, "--qa", task, "tests", "pass" if passed else "fail", text]
                       + (["--pointer", tp] if isinstance(tp, str) and os.path.isabs(tp) else []),
                       capture_output=True, text=True, timeout=TIMEOUT)
    if r.returncode:
        print(f"qa-record: protocolo --qa exit {r.returncode}: {' '.join((r.stdout + r.stderr).split())[-300:]}", file=sys.stderr)


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        print(f"qa-record: {type(e).__name__}: {' '.join(str(e).split())[:300]}", file=sys.stderr)
    sys.exit(0)
