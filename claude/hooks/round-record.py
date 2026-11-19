#!/usr/bin/env python3
"""round-record.py: the PostToolUse hook that records an AskUserQuestion round of a task-bound session (harness v2 R03; A4).

For an AskUserQuestion of a session bound to a task, it calls `protocolo --round <task> --asked '<json>'` once, which adds
one to rounds in gate.json and records the round: a question whose text equals a registered one takes its answer, any
other is added as the owner's. The questions come from tool_input.questions. The answers come from tool_response when it is a
dict whose answers is a non-empty dict keyed by question text, else from the transcript through gate.py's last_answer,
else they are null and the round still counts. A list answer (multiSelect) is joined with ", ". An approval round (one
question, header Publish, Phase or QA, offering its pair's label; gate.py's approval_round) is neither counted nor recorded.
When the round cannot be written (protocolo exits non-zero or times out, tool_input.questions is no list of questions, or
anything else raises) it writes gate.json.round-failed beside gate.json and prints one warning line on stderr; gate.py
denies AskUserQuestion while the mark exists, and the next good `protocolo --round` removes it. It never blocks: every
path exits 0. An unbound session, or one whose binding is broken, is left alone. The binding lookup is gate.py's,
imported from this folder. $ROUND_PROTOCOLO names the protocolo it calls.
"""
import json, os, subprocess, sys, time

TIMEOUT = 20


def answer(v):
    if isinstance(v, list):
        v = ", ".join(str(x) for x in v)
    return v if isinstance(v, str) and v.strip() else None


def fail(gate, task, why):
    mark = os.path.join(task, gate.ROUND_MARK)
    try:
        open(mark, "w", encoding="utf-8").write(f"{time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())} {why}\n")
        done = f"wrote {mark}"
    except OSError as e:
        done = f"could not write {mark} ({type(e).__name__})"
    why = " ".join(why.split())[-300:]
    print(f"round-record: this AskUserQuestion round was not recorded ({why}); {done}; "
          f"The owner runs `! {gate.PROTOCOLO} --round {task}`", file=sys.stderr)


def record(gate, ev, task):
    ti = ev.get("tool_input")
    qs = ti.get("questions") if isinstance(ti, dict) else None
    if not isinstance(qs, list) or not qs or not all(isinstance(x, dict) and isinstance(x.get("question"), str)
                                                     and x["question"].strip() for x in qs):
        return fail(gate, task, "tool_input.questions is not a list of questions")
    if gate.approval_round(qs):
        return
    resp = ev.get("tool_response")
    got = resp["answers"] if isinstance(resp, dict) and isinstance(resp.get("answers"), dict) and resp["answers"] else None
    if got is None and isinstance(ev.get("transcript_path"), str):
        last = gate.last_answer(ev["transcript_path"])
        got = last[1] if last else None
    asked = [{"q": x["question"], "a": answer((got or {}).get(x["question"]))} for x in qs]
    proto = os.environ.get("ROUND_PROTOCOLO") or gate.PROTOCOLO
    try:
        r = subprocess.run([sys.executable, "-B", proto, "--round", task, "--asked", json.dumps(asked, ensure_ascii=False)],
                           capture_output=True, text=True, timeout=TIMEOUT)
    except (OSError, subprocess.TimeoutExpired) as e:
        return fail(gate, task, f"protocolo --round: {type(e).__name__}")
    if r.returncode:
        fail(gate, task, f"protocolo --round exit {r.returncode}: {(r.stdout + r.stderr).strip()}")


def main():
    ev = json.loads(sys.stdin.read() or "null")
    if not isinstance(ev, dict) or ev.get("tool_name") != "AskUserQuestion" or ev.get("hook_event_name") != "PostToolUse":
        return
    sys.dont_write_bytecode = True
    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    import gate
    st = gate.state(ev.get("session_id"))
    if not st:
        return
    if st["error"]:
        print(f"round-record: round not recorded, the binding is broken: {st['error']}", file=sys.stderr)
        return
    try:
        record(gate, ev, st["task"])
    except Exception as e:
        fail(gate, st["task"], f"{type(e).__name__}: {e}")


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        print(f"round-record: {type(e).__name__}: {' '.join(str(e).split())[:300]}", file=sys.stderr)
    sys.exit(0)
