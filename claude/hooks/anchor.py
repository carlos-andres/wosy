#!/usr/bin/env python3
"""anchor.py: the UserPromptSubmit hook of a task-bound session (harness v2 R04, R08; A4).

A #<token> in the prompt runs `protocolo --load <token>` and injects it; when the token is a task, the session is first
bound through `protocolo --bind`, so gate.json is written only by protocolo. Every prompt of a bound session gets one
anchor line under 600 characters. An unbound session with no token gets nothing. It never blocks: every error exits 0
with one line that names it. The binding lookup is gate.py's: <bind dir>/<session_id> names the task folder; the bind dir
is $GATE_BIND_DIR, else $CLAUDE_CONFIG_DIR/state/bind, else state/bind of the home holding this file.

Token: `#` at the start of the prompt or after whitespace or `(`, then a letter, then letters, digits, `-` or `_`, ending
in a letter or digit, followed by whitespace, the end, or one of . , ; : ! ? ). Fenced and inline code is removed first.
Only the first token counts. $ANCHOR_PROTOCOLO names the protocolo it calls.
"""
import json, os, re, subprocess, sys, time

PROTOCOLO = os.environ.get("ANCHOR_PROTOCOLO") or "/Users/you/.devwork/bin/protocolo"
LINE_MAX, LOAD_BUDGET = 599, 45
TOKEN = re.compile(r"(?<![^\s(])#([A-Za-z][A-Za-z0-9_-]*[A-Za-z0-9])(?=[\s.,;:!?)]|$)")
CODE = re.compile(r"(```|~~~).*?(\1|\Z)|`[^`\n]*`", re.S)


def bind_dir():
    return os.environ.get("GATE_BIND_DIR") or os.path.join(
        os.environ.get("CLAUDE_CONFIG_DIR") or os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "state", "bind")


def state(sid):
    """None when unbound, else {"task", "gate", "error"}: gate.py's lookup, read-only."""
    if not sid or not re.fullmatch(r"[A-Za-z0-9_-]+", sid):
        return None
    idx = os.path.join(bind_dir(), sid)
    try:
        raw = open(idx, encoding="utf-8").read().strip()
    except FileNotFoundError:
        return None
    st = {"task": None, "gate": None, "error": None}
    if not raw or not os.path.isdir(raw):
        st["error"] = f"the bind index {idx} names no task folder ({raw or 'empty'})"
        return st
    st["task"] = os.path.realpath(raw)
    gp = os.path.join(st["task"], "gate.json")
    try:
        g = json.load(open(gp, encoding="utf-8"))
        if not isinstance(g, dict) or not g.get("phase"):
            raise ValueError("no phase")
    except (OSError, ValueError) as e:
        st["error"] = f"{gp} is unreadable ({type(e).__name__}: {e})"
        return st
    if sid not in (g.get("sessions") or []):
        st["error"] = f"{gp} does not list session {sid}, so the bind index is stale"
        return st
    st["gate"] = g
    return st


def fm_value(path, key):
    """One scalar of a context.md frontmatter, enough for goal and done_when; no YAML parser."""
    try:
        m = re.match(r"---\n(.*?)\n---[ \t]*\n", open(path, encoding="utf-8").read(), re.S)
    except OSError:
        return None
    v = m and re.search(rf"^{key}:[ \t]*(.*)$((?:\n[ \t]+.*)*)", m[1], re.M)
    if not v:
        return None
    s = v[1].strip()
    if s in ("", ">", "|", ">-", "|-"):
        return " ".join(v[2].split()) or None
    if s.startswith('"'):
        try:
            return json.loads(s)
        except ValueError:
            return s.strip('"')
    if s.startswith("'"):
        return s[1:-1].replace("''", "'")
    return None if s in ("null", "~") else s


def field(st, key):
    """gate.json first (the frozen copy), context.md before --validate-context."""
    return st["gate"].get(key) or fm_value(os.path.join(st["task"], "context.md"), key)


def anchor_line(st):
    if st["error"]:
        return f"anchor: the binding is broken: {st['error']}"[:LINE_MAX]
    g = st["gate"]
    qs = g.get("questions") or []
    opened = [q.get("id", "?") for q in qs if q.get("status") == "open"]
    ids = ",".join(opened[:12]) + (f",+{len(opened) - 12}" if len(opened) > 12 else "")
    answered = sum(q.get("status") == "answered" for q in qs)
    head = f"anchor: task {g.get('task_id')} | phase {g.get('phase')} | goal: "
    tail = (f" | open questions {len(opened)}" + (f" ({ids})" if opened else "")
            + f" | answered {answered} | rounds {g.get('rounds') or 0}")
    goal = " ".join(str(field(st, "goal") or "none yet").split())
    room = LINE_MAX - len(head) - len(tail)
    if len(goal) > room:
        goal = goal[:max(0, room - 3)] + "..."
    return head + goal + tail


def protocolo(args, deadline):
    return subprocess.run([sys.executable, "-B", PROTOCOLO, *args], capture_output=True, text=True,
                          timeout=max(1, deadline - time.monotonic()), env=os.environ | {"GATE_BIND_DIR": bind_dir()})


def load(tok, sid):
    """The load output for the token; binds the session first when the token is a task that loads cleanly."""
    deadline = time.monotonic() + LOAD_BUDGET
    r = protocolo(["--load", tok], deadline)
    if r.returncode == 3:
        return f"anchor: #{tok} resolved to no task and no world; nothing loaded"
    m = re.match(r"== load: task \S+ at (.+) ==$", (r.stdout.splitlines() or [""])[0])
    if r.returncode == 0 and m and sid and re.fullmatch(r"[A-Za-z0-9_-]+", sid):
        b = protocolo(["--bind", m[1], "--session", sid], deadline)
        if b.returncode != 0:
            return f"{r.stdout.rstrip()}\nanchor: the bind failed: {' '.join((b.stdout + b.stderr).split())[-300:]}"
        r = protocolo(["--load", m[1]], deadline)
    out = (r.stdout + r.stderr).rstrip()
    if r.returncode == 0:
        return out
    return (f"{out}\nanchor: the load of #{tok} stopped at a broken input (exit {r.returncode}); the session was not bound. "
            f"In this turn tell the owner which input is broken and offer the owner the port of the task: add to its context.md only "
            f"what the load says is missing, with no diff step and no backup copy, and ask him for any value no file gives. "
            f"The owner then freezes it with `! {PROTOCOLO} --validate-context {m[1] if m else '<task folder>'}` and sends #{tok} again")


def main():
    ev = json.loads(sys.stdin.read() or "null")
    if not isinstance(ev, dict):
        raise ValueError("the hook received no JSON object")
    sid, prompt = ev.get("session_id"), ev.get("prompt") or ""
    tok = TOKEN.search(CODE.sub(" ", prompt)) if "#" in prompt else None
    st = state(sid)
    if tok is None and st is None:
        return None
    parts = [load(tok[1], sid)] if tok else []
    st = state(sid) if tok else st
    if st is not None:
        parts.insert(0, anchor_line(st))
    return "\n".join(parts)


def emit(text):
    print(json.dumps({"hookSpecificOutput": {"hookEventName": "UserPromptSubmit", "additionalContext": text}}))


if __name__ == "__main__":
    try:
        out = main()
        if out:
            emit(out)
    except Exception as e:
        emit(f"anchor: error, nothing injected: {type(e).__name__}: {' '.join(str(e).split())[:300]}")
    sys.exit(0)
