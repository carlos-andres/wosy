#!/opt/homebrew/bin/python3 -I
"""telemetry.py [--session <id-prefix> ...] [--home-all <home> ...] [--out <tsv>]: one row per session and one per dispatch of this task, from the transcripts.

A session of the task: its cwd is the task folder, or a tool call names the task folder, or it shares a bridge session with one that does.
--session selects by id prefix instead, --home-all every session of a home; both add the sessions that share a bridge session.
Beside <out> it writes <stem>-denies.tsv (gate denials per session and rule, main transcript and its subagents), <stem>-nodes.tsv
(dispatches summed per plan node) and <stem>-context.tsv (context tokens per assistant turn of each main session).
Prints ids, counts and names only. Exit 0 ok, 1 when a dispatch row is MISSING its model, effort or usage, 2 when no session is found.
"""
import collections, datetime, glob, json, os, re, sys

T = "/Users/you/.devwork/tasks/<your-task>"
HOMES = (("personal", "/Users/you/.claude"), ("trial", "/Users/you/.claude-trial"))
SINCE = datetime.datetime(2026, 11, 12).timestamp()
PROJ = re.sub(r"[^A-Za-z0-9]", "-", T)
FILE = re.compile(r"[\w.-]+\.(?:md|py|tsv|csv|json|jsonl|sh|txt|ya?ml|sql|php|js|ts|toml|log)\b")
AGENTMSG = re.compile(r"Subagent hand-back|agent-message from|<task-notification|Another Claude session sent a message")
NODE = re.compile(r"\b[Nn]ode\b:?\s+([A-Z]{1,2}\d{1,3}[a-z]?)\b")
ROWS = (("locate", "locate"), ("bulk extract", "bulk extract"), ("history and why", "history and why"),
        ("external docs", "external docs and versions"), ("consolidate, judge", "consolidate, judge, decide over evidence"),
        ("build code", "build code"), ("gate a finished change", "gate a finished change"), ("load-probe", "load-probe a handoff"))
COLS = ["kind", "session", "chain", "home", "start", "node", "row", "agent", "agent_id", "model", "effort", "status", "user_turns", "peak_ctx",
        "tokens", "input", "output", "cache_creation", "cache_read", "tool_calls", "seconds", "denials", "reverified", "redone", "gate_denies"]
NODE_COLS = ["node", "dispatches", "tokens", "tool_calls", "seconds", "redone", "agents"]
CTX_COLS = ["session", "turn", "timestamp", "context", "after_handback"]
GATE = re.compile(r"(?:PreToolUse:\S+ hook error: \[[^\]\n]*\]: )?BLOCKED by gate \(([^)\n]*)\)")
U =("input_tokens", "output_tokens", "cache_creation_input_tokens", "cache_read_input_tokens")


def load(p, needle=""):
    out = []
    for l in open(p, errors="replace"):
        if needle not in l:
            continue
        try:
            out.append(json.loads(l))
        except Exception:
            pass
    return out


def secs(a, b):
    return round((datetime.datetime.fromisoformat(b) - datetime.datetime.fromisoformat(a)).total_seconds()) if a and b else 0


def text(c):
    return c if isinstance(c, str) else " ".join(x.get("text", "") for x in c or [] if isinstance(x, dict) and x.get("type") == "text")


def usage(rows):
    msgs, models, tools = {}, collections.Counter(), set()
    for d in rows:
        m = d.get("message") or {}
        if d.get("type") != "assistant":
            continue
        if m.get("model") and m["model"] != "<synthetic>":
            models[m["model"]] += 1
        if m.get("usage"):
            msgs[m.get("id") or id(m)] = m["usage"]
        tools |= {x["id"] for x in m.get("content") or [] if isinstance(x, dict) and x.get("type") == "tool_use"}
    sums = [sum(u.get(k) or 0 for u in msgs.values()) for k in U]
    peak = max((sum(u.get(k) or 0 for k in U if k != "output_tokens") for u in msgs.values()), default=0)
    last = sum(list(msgs.values())[-1].get(k) or 0 for k in U) if msgs else None
    return msgs, "+".join(m for m, _ in models.most_common()), sums, peak, last, len(tools)


def denied(x):
    k = text(x.get("content")).lstrip()
    return x.get("is_error") and (k.startswith(("Permission", "PreToolUse:", "The user doesn't want to proceed")) or "denied" in k[:160].lower())


def gate_denies(rows):
    tags = {}
    for d in rows:
        c = (d.get("message") or {}).get("content")
        if d.get("type") == "user" and isinstance(c, list):
            for x in c:
                m = isinstance(x, dict) and x.get("type") == "tool_result" and x.get("is_error") and GATE.match(text(x.get("content")).lstrip())
                if m:
                    tags[x.get("tool_use_id") or id(x)] = m.group(1)
    return collections.Counter(tags.values())


def context(sid, rows):
    first, ctx, back, flag = {}, {}, set(), {}
    handback = False
    for d in rows:
        m = d.get("message") or {}
        c = m.get("content")
        if d.get("type") == "assistant":
            if isinstance(c, list):
                back |= {x["id"] for x in c if isinstance(x, dict) and x.get("type") == "tool_use" and x.get("name") in ("Agent", "Task")}
            if m.get("usage"):
                key = m.get("id") or id(m)
                if key not in first:
                    first[key], flag[key], handback = d.get("timestamp", ""), int(handback), False
                ctx[key] = sum(m["usage"].get(k) or 0 for k in U if k != "output_tokens")
        elif d.get("type") in ("user", "attachment", "queue-operation"):
            tur = d.get("toolUseResult") if isinstance(d.get("toolUseResult"), dict) else {}
            ids = {x.get("tool_use_id") for x in c if isinstance(x, dict) and x.get("type") == "tool_result"} if isinstance(c, list) else set()
            if ids:
                handback = handback or bool(ids & back) and tur.get("status") != "async_launched"
            else:
                handback = handback or bool(AGENTMSG.search(json.dumps(d)))
    return [dict(session=sid, turn=i, timestamp=first[k][:19], context=ctx[k], after_handback=flag[k]) for i, k in enumerate(first, 1)]


def brief_tags(brief):
    node = NODE.search(brief)
    at = re.search(r"\brow\b", brief, re.I)
    tail = brief[at.start():at.start() + 200].lower() if at else ""
    return node.group(1) if node else "", min(((tail.find(k), label) for k, label in ROWS if k in tail), default=(0, ""))[1]


def agent_stats(sub, mj):
    msgs, model, sums, _, last, ntools = usage(sub)
    effort = mj.get("effort") or next((d["effort"] for d in sub if d.get("effort")), "")
    lack = [k for k, v in (("transcript", sub), ("model", model), ("effort", effort), ("usage", msgs)) if not v]
    ts_sub = [d["timestamp"] for d in sub if d.get("timestamp")]
    return lack, dict(model=model or "-", effort=effort or "-", status="MISSING:" + ",".join(lack) if lack else "ok",
                      tokens=last if last is not None else "-", input=sums[0], output=sums[1], cache_creation=sums[2], cache_read=sums[3],
                      tool_calls=ntools, seconds=secs(ts_sub[0], ts_sub[-1]) if ts_sub else 0)


def find_sessions(prefixes, homes_all=()):
    roots = list(HOMES) + [(os.path.basename(r.rstrip("/")).lstrip(".") or r, r) for r in homes_all if r not in dict((b, a) for a, b in HOMES)]
    files = [(h, p) for h, root in roots for p in glob.glob(f"{root}/projects/*/*.jsonl")]
    recent = [(h, p) for h, p in files if os.path.getmtime(p) >= SINCE]
    if prefixes or homes_all:
        alls = tuple(r.rstrip("/") + "/projects/" for r in homes_all)
        picked = {p for h, p in files if any(os.path.basename(p).startswith(x) for x in prefixes) or (alls and p.startswith(alls))}
    else:
        picked = {p for h, p in files if os.path.basename(os.path.dirname(p)) == PROJ}
        for h, p in recent:
            if p not in picked and any(T in l and '"tool_use"' in l for l in open(p, errors="replace")):
                picked.add(p)
    bridges = {p: {d.get("bridgeSessionId") for d in load(p, '"bridgeSessionId"') if d.get("type") == "bridge-session"} - {None} for h, p in recent}
    grow = True
    while grow:
        b = set().union(*(bridges.get(p, set()) for p in picked))
        more = {p for p, s in bridges.items() if s & b} - picked
        picked |= more
        grow = bool(more)
    home = dict((p, h) for h, p in files)
    return [(home[p], p, bridges.get(p, set())) for p in sorted(picked)]


def dispatches(sid, chain, rows, subdir):
    calls, results, notes, meta = [], {}, {}, {}
    for f in glob.glob(f"{subdir}/agent-*.meta.json") + glob.glob(f"{subdir}/agent-*.json"):
        try:
            j = json.load(open(f))
        except Exception:
            continue
        meta[j.get("toolUseId")] = (os.path.basename(f).split(".")[0][6:], j)
    for i, d in enumerate(rows):
        c = (d.get("message") or {}).get("content")
        if d.get("type") == "assistant" and isinstance(c, list):
            calls += [(i, d.get("timestamp", ""), x) for x in c if isinstance(x, dict) and x.get("type") == "tool_use" and x.get("name") in ("Agent", "Task")]
        elif d.get("type") == "user" and isinstance(c, list):
            for x in c:
                if isinstance(x, dict) and x.get("type") == "tool_result":
                    results[x.get("tool_use_id")] = (i, x, d.get("toolUseResult") if isinstance(d.get("toolUseResult"), dict) else {})
        if d.get("type") in ("user", "attachment", "queue-operation"):
            s = json.dumps(d)
            if AGENTMSG.search(s):
                for a in set(re.findall(r"\ba[0-9a-f]{16}\b", s)):
                    notes.setdefault(a, []).append(i)
    out = []
    for i, ts, x in calls:
        inp = x.get("input") or {}
        node, row = brief_tags(inp.get("prompt") or "")
        ri, res, tur = results.get(x["id"], (None, {}, {}))
        aid = tur.get("agentId") or meta.get(x["id"], ("", {}))[0]
        mj = next((j for a, j in meta.values() if a == aid), {}) if aid else {}
        r = dict(kind="dispatch", session=sid, chain=chain, start=ts[:19], node=node, row=row,
                 agent=mj.get("agentType") or inp.get("subagent_type") or "general-purpose", agent_id=aid or "-")
        path = f"{subdir}/agent-{aid}.jsonl"
        if not aid and res and denied(res):
            out.append(r | dict(model="-", effort=inp.get("effort") or "-", status="denied", tokens=0, input=0, output=0, cache_creation=0,
                                cache_read=0, tool_calls=0, seconds=0, reverified="na"))
            continue
        sub = load(path) if aid and os.path.exists(path) else []
        back = [n for n in notes.get(aid, []) if n != ri] + ([] if tur.get("status") == "async_launched" or ri is None else [ri])
        report = json.dumps([rows[n] if n != ri else res.get("content") for n in back])
        out.append(r | agent_stats(sub, mj)[1] | dict(reverified=check(rows, min(back, default=None), {n for n in FILE.findall(report) if len(n) > 6})))
    return out


# reverified: after the parent sees the report, a non-Agent tool call names a file the report names before the parent's next text of 200+ characters
def check(rows, seen, names):
    if seen is None or not names:
        return "na"
    for d in rows[seen + 1:]:
        if d.get("type") != "assistant":
            continue
        for x in (d.get("message") or {}).get("content") or []:
            if not isinstance(x, dict):
                continue
            if x.get("type") == "text" and len(x.get("text", "").strip()) >= 200:
                return 0
            if x.get("type") == "tool_use" and x.get("name") not in ("Agent", "Task") and any(n in json.dumps(x.get("input")) for n in names):
                return 1
    return 0


def session_row(home, p, chain, rows):
    sid = os.path.basename(p)[:8]
    msgs, model, sums, peak, _, ntools = usage(rows)
    turns = den = 0
    for d in rows:
        c = (d.get("message") or {}).get("content")
        if d.get("type") != "user":
            continue
        if isinstance(c, list):
            den += sum(1 for x in c if isinstance(x, dict) and x.get("type") == "tool_result" and denied(x))
        elif isinstance(c, str) and not d.get("isMeta") and c.strip() and not c.startswith(("<", "[Request", "Caveat")) and not AGENTMSG.search(c[:400]):
            turns += 1
    ts = [d["timestamp"] for d in rows if d.get("timestamp")]
    return dict(kind="session", session=sid, chain=chain, home=home, start=ts[0][:19] if ts else "", model=model or "-", user_turns=turns,
                peak_ctx=peak, input=sums[0], output=sums[1], cache_creation=sums[2], cache_read=sums[3], tool_calls=ntools,
                seconds=secs(ts[0], ts[-1]) if ts else 0, denials=den)


def main(argv):
    prefixes = [argv[i + 1] for i, a in enumerate(argv) if a == "--session" and i + 1 < len(argv)]
    homes_all = [os.path.abspath(argv[i + 1]) for i, a in enumerate(argv) if a == "--home-all" and i + 1 < len(argv)]
    out = argv[argv.index("--out") + 1] if "--out" in argv else f"{T}/research/telemetry.tsv"
    found = find_sessions(prefixes, homes_all)
    if not found:
        print("sessions=0 dispatches=0 missing=0 out=not-written", file=sys.stderr)
        return 2
    loaded = [(h, p, b, load(p)) for h, p, b in found]
    start = {p: next((d["timestamp"] for d in rows if d.get("timestamp")), "~") for h, p, b, rows in loaded}
    bridge, chain = {p: b for h, p, b, _ in loaded}, {}
    for h, p, b, rows in sorted(loaded, key=lambda t: start[t[1]]):
        first = next((q for q in chain if b & bridge[q]), None)
        chain[p] = chain[first] if first else os.path.basename(p)[:8]
    sess, disp, denies, ctx = [], [], [], []
    for h, p, b, rows in loaded:
        sid, sd = os.path.basename(p)[:8], p[:-6] + "/subagents"
        srow, gate = session_row(h, p, chain[p], rows), collections.Counter()
        for sp in [p] + sorted(glob.glob(f"{sd}/agent-*.jsonl")):
            sr = rows if sp == p else load(sp)
            gate += gate_denies(sr)
            disp += [r | {"home": h} for r in dispatches(sid, chain[p], sr, sd)]
        sess.append(srow | {"gate_denies": sum(gate.values())})
        denies += [dict(session=sid, home=h, rule=k, count=v) for k, v in sorted(gate.items())]
        ctx += context(sid, rows)
    for r in disp:
        r["redone"] = "na" if not r["node"] else int(any(o is not r and o["agent"] == r["agent"] and o["node"] == r["node"] and o["start"] > r["start"] for o in disp))
    nodes = {}
    for r in disp:
        nodes.setdefault(r["node"] or "-", []).append(r)
    nrows = [dict(node=n, dispatches=len(rs), tokens=sum(r["tokens"] for r in rs if isinstance(r["tokens"], int)), tool_calls=sum(r["tool_calls"] for r in rs),
                  seconds=sum(r["seconds"] for r in rs), redone=sum(r["redone"] == 1 for r in rs), agents=len({r["agent"] for r in rs}))
             for n, rs in sorted(nodes.items(), key=lambda t: (t[0] == "-", t[0]))]
    missing = [r for r in disp if r["status"].startswith("MISSING")]
    stem = out[:-4] if out.endswith(".tsv") else out
    for path, cols, rs in ((out, COLS, sorted(sess, key=lambda r: r["start"]) + sorted(disp, key=lambda r: r["start"])),
                           (f"{stem}-denies.tsv", ["session", "home", "rule", "count"], denies), (f"{stem}-nodes.tsv", NODE_COLS, nrows),
                           (f"{stem}-context.tsv", CTX_COLS, ctx)):
        with open(path, "w") as f:
            f.write("\t".join(cols) + "\n")
            for r in rs:
                f.write("\t".join(str(r.get(c, "-")) if r.get(c, "-") != "" else "-" for c in cols) + "\n")
    for r in missing:
        print(f"MISSING\t{r['session']}\t{r['agent']}\t{r['agent_id']}\t{r['status'][8:]}", file=sys.stderr)
    print(f"sessions={len(sess)} dispatches={len(disp)} missing={len(missing)} out={out}")
    return 1 if missing else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
