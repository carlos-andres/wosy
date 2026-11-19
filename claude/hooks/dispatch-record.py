#!/usr/bin/env python3
"""dispatch-record.py: the SubagentStop hook that records one live telemetry row per finished subagent of a task-bound session.

It appends one row to <task>/research/telemetry-live.tsv (header written when absent): the dispatch columns of telemetry.tsv with
kind dispatch-live, plus recorded_at (UTC). Model, effort, tokens, usage sums, tool calls and seconds come from the subagent
transcript and its meta file through telemetry.py's own functions ($TELEMETRY_LIB names it); node and row from the brief, the
first user message of that transcript. chain, reverified and redone are not known at stop time: `-`. A value it cannot read is
MISSING. A resumed agent that stops again gets another row, so a reader takes the last row per agent_id. An empty agent_type
(an internal agent, never an Agent call) and an unbound session write nothing. Only ids, counts and names are written.
Exit 2 or a block decision would keep the subagent running, so every path exits 0 with no stdout. The binding lookup is anchor.py's.
"""
import datetime, importlib.util, json, os, signal, sys, time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
TELEMETRY = os.environ.get("TELEMETRY_LIB") or "/Users/you/.devwork/bin/telemetry.py"
LIMIT = 10


def lib():
    spec = importlib.util.spec_from_file_location("telemetry", TELEMETRY)
    tel = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(tel)
    return tel


def row(ev, tel):
    path = ev.get("agent_transcript_path") or ""
    sub = tel.load(path) if path.endswith(".jsonl") and os.path.isfile(path) else []
    mj = {}
    for m in (path[:-6] + ".meta.json", path[:-6] + ".json"):
        try:
            mj = json.load(open(m))
            break
        except Exception:
            pass
    mj = mj if isinstance(mj, dict) else {}
    lack, stats = tel.agent_stats(sub, mj)
    if "usage" in lack:
        stats |= dict.fromkeys(("tokens", "input", "output", "cache_creation", "cache_read"), "MISSING")
    if "transcript" in lack:
        stats |= dict(tool_calls="MISSING", seconds="MISSING")
    stats |= {k: "MISSING" for k in ("model", "effort") if stats[k] == "-"}
    brief = next((tel.text((d.get("message") or {}).get("content")) for d in sub if d.get("type") == "user"), None)
    node, rowlabel = tel.brief_tags(brief) if brief is not None else ("MISSING", "MISSING")
    tp = ev.get("transcript_path") or path
    home = next((h for h, root in tel.HOMES if tp.startswith(root + "/")), None)
    home = home or (os.path.basename(tp.split("/projects/")[0]).lstrip(".") if "/projects/" in tp else "MISSING")
    ts = [d["timestamp"] for d in sub if d.get("timestamp")]
    return stats | dict(kind="dispatch-live", session=str(ev.get("session_id"))[:8], chain="-", home=home, start=ts[0][:19] if ts else "MISSING",
                        node=node or "-", row=rowlabel or "-", agent=mj.get("agentType") or ev.get("agent_type"),
                        agent_id=ev.get("agent_id") or "MISSING", user_turns="-", peak_ctx="-", denials="-", reverified="-", redone="-",
                        recorded_at=datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"))


def write(task, cols, r):
    d = os.path.join(task, "research")
    os.makedirs(d, exist_ok=True)
    out = os.path.join(d, "telemetry-live.tsv")
    if not os.path.exists(out):
        tmp = f"{out}.{os.getpid()}.tmp"
        with open(tmp, "w") as f:
            f.write("\t".join(cols) + "\n")
        try:
            os.link(tmp, out)
        except FileExistsError:
            pass
        finally:
            os.unlink(tmp)
    line = "\t".join(" ".join(str(r.get(c, "-")).split()) or "-" for c in cols) + "\n"
    fd = os.open(out, os.O_WRONLY | os.O_APPEND)
    try:
        os.write(fd, line.encode())
    finally:
        os.close(fd)


def main():
    ev = json.loads(sys.stdin.read() or "null")
    if not isinstance(ev, dict) or not ev.get("agent_type"):
        return
    from anchor import state
    st = state(ev.get("session_id"))
    if st is None or st["error"] or not st["task"]:
        return
    tel = lib()
    r = row(ev, tel)
    # At SubagentStop the subagent transcript is not flushed yet (seen 2026-11-14: usage readable about 50 ms later).
    for _ in range(int(float(os.environ.get("DISPATCH_RECORD_WAIT", "6")) * 2)):
        if "MISSING" not in str(r.get("status")):
            break
        time.sleep(0.5)
        r = row(ev, tel)
    write(st["task"], tel.COLS[:tel.COLS.index("redone") + 1] + ["recorded_at"], r)


def stop(*_):
    os._exit(0)


if __name__ == "__main__":
    signal.signal(signal.SIGALRM, stop)
    signal.alarm(LIMIT)
    try:
        main()
    except BaseException:
        pass
    os._exit(0)
