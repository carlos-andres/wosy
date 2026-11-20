#!/usr/bin/env python3
"""budget-record.py: the PostToolUse hook that keeps the inline budget of a main session for orchestrator.py rule P91.

On Read, Bash, Agent and Task. One counter per session id in <home>/state/budget/<session_id>, a plain integer: a Read or
Bash call adds the chars of its result as the model receives it, read from tool_response; an Agent or Task call sets it to
0. A Bash result is stdout plus stderr, and a persisted one counts its preview only; a Read text result is its content plus
the "<number><tab>" prefix of each line; any other Read result (an image, a PDF) counts 0. A subagent call (agent_id in
the payload) is never counted. A missing or unreadable counter counts as 0. Bound and unbound sessions alike. It never
blocks and prints nothing on success; every path exits 0. The home is $CLAUDE_CONFIG_DIR, else the home holding this file.
"""
import fcntl, json, os, re, sys

HOME = os.environ.get("CLAUDE_CONFIG_DIR") or os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PREVIEW = 2200  # the persisted-output preview the model receives in place of a long output, about 2,200 chars


def size(tool, r):
    if isinstance(r, str):
        return len(r)
    if not isinstance(r, dict):
        return 0
    if tool == "Bash":
        n = len(str(r.get("stdout") or "")) + len(str(r.get("stderr") or ""))
        return min(n, PREVIEW) if r.get("persistedOutputPath") else n
    f = r.get("file")
    if not isinstance(f, dict) or not isinstance(f.get("content"), str):
        return 0
    start = f.get("startLine") if isinstance(f.get("startLine"), int) else 1
    lines = f.get("numLines") if isinstance(f.get("numLines"), int) else 0
    return len(f["content"]) + sum(len(str(i)) + 1 for i in range(start, start + lines))


def main():
    ev = json.loads(sys.stdin.read() or "null")
    if not isinstance(ev, dict) or ev.get("hook_event_name") != "PostToolUse" or "agent_id" in ev:
        return
    tool, sid = ev.get("tool_name"), re.sub(r"[^A-Za-z0-9_-]", "", str(ev.get("session_id") or ""))
    if not sid or tool not in ("Read", "Bash", "Agent", "Task"):
        return
    os.makedirs(os.path.join(HOME, "state", "budget"), exist_ok=True)
    fd = os.open(os.path.join(HOME, "state", "budget", sid), os.O_RDWR | os.O_CREAT, 0o644)
    try:
        # Parallel tool calls finish together: the lock keeps one read-add-write from losing another's chars.
        fcntl.flock(fd, fcntl.LOCK_EX)
        try:
            n = int(os.read(fd, 64) or 0)
        except ValueError:
            n = 0
        n = 0 if tool in ("Agent", "Task") else n + size(tool, ev.get("tool_response"))
        os.lseek(fd, 0, os.SEEK_SET)
        os.ftruncate(fd, 0)
        os.write(fd, f"{n}\n".encode())
    finally:
        os.close(fd)


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        print(f"budget-record: not counted, {type(e).__name__}: {' '.join(str(e).split())[:300]}", file=sys.stderr)
    sys.exit(0)
