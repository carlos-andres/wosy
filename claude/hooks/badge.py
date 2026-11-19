#!/usr/bin/env python3
"""badge.py: the SubagentStart hook of a task-bound session (harness v2 R23; A4).

For a session bound to a task it injects the task badge into the subagent: task id, phase, goal, done_when and the path of
context.md. Goal and done_when come from gate.json, else from context.md before --validate-context. Unbound: nothing.
It never blocks a dispatch: every error exits 0 with one line. The binding lookup is anchor.py's, which is gate.py's.
"""
import json, os, sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
FIELD_MAX = 400


def emit(text):
    print(json.dumps({"hookSpecificOutput": {"hookEventName": "SubagentStart", "additionalContext": text}}))


def badge(st):
    from anchor import field
    if st["error"]:
        return f"Task badge: the binding of this session is broken: {st['error']}"
    one = lambda v: (lambda s: s if len(s) <= FIELD_MAX else s[:FIELD_MAX - 3] + "...")(" ".join(str(v or "none yet").split()))
    g = st["gate"]
    return (f"Task badge: this subagent works for task {g.get('task_id')}, phase {g.get('phase')}.\n"
            f"goal: {one(field(st, 'goal'))}\n"
            f"done_when: {one(field(st, 'done_when'))}\n"
            f"context: {os.path.join(st['task'], 'context.md')}")


if __name__ == "__main__":
    try:
        from anchor import state
        ev = json.loads(sys.stdin.read() or "null")
        st = state(ev.get("session_id")) if isinstance(ev, dict) else None
        if st is not None:
            emit(badge(st))
    except Exception as e:
        emit(f"Task badge: error, nothing injected: {type(e).__name__}: {' '.join(str(e).split())[:300]}")
    sys.exit(0)
