#!/usr/bin/env python3
"""session-audit.py - SessionEnd hook. Side-effect only, by design.

WHY THIS IS NOT THE OLD SHELL VERSION
-------------------------------------
The predecessor printed a scorecard to stderr. Per the hooks reference, SessionEnd discards
stderr AND any JSON output: it cannot reach the user or Claude, because the session is already
terminating. Verified against 880 transcripts spanning 2026-07-15 to 2026-09-17: the separator
string it prints appears nowhere except inside sessions that were reading the hook's source.
It had been writing to a black hole since 2026-06-11.

So this version does the only thing a SessionEnd hook can usefully do: it WRITES A FILE.

  <logdir>/<YYYYMMDD>-<session>.json   one record, machine-readable, queryable with DuckDB
  <logdir>/<YYYYMMDD>-<session>.md     the same thing as a one-screen scorecard for a human

logdir defaults to ~/Library/Logs/claude-session-audit on macOS, matching what the other hooks
already do via log_dir_for, and it lives OUTSIDE ~/.claude so the history survives a reinstall.

WHAT IT REPORTS, and why each one earns its line
------------------------------------------------
  tools        call count and error count per tool
  hotspots     files edited more than 4 times, the predecessor's one real signal, kept
  burn         result bytes per tool, p50 / p90 / max. This is context spend, which is the
               thing that actually runs out. A single 566 KB Read is worth knowing about.
  retries      Bash commands issued more than once, and whether the outcome ever flipped.
               A flip means the second attempt worked, which marks where the session fought.
  recovery     seconds between a failed result and the next tool call.

Percentiles are NEAREST-RANK on the sorted sample, not interpolated. DuckDB's quantile_cont
interpolates, so the numbers in queries/*.sql differ slightly from these by design. Whichever
you read, the method is stated beside it.

FAILS OPEN, ALWAYS. Any error exits 0 having written nothing. A session must never fail to
close because its audit could not be computed.

Python 3.8+, standard library only. No pandas: the work here is counting and sorting, and a
dependency on a non-system interpreter is not worth 227 ms of import for that.
"""
from __future__ import annotations

import json
import os
import sys
from collections import Counter, defaultdict
from datetime import datetime

VERSION = "1.0.0"
HOTSPOT_MIN = 5          # the predecessor used "> 4"
MAX_RETRY_SHOWN = 5
EDIT_TOOLS = ("Edit", "Write", "MultiEdit", "NotebookEdit")


def log_dir() -> str:
    override = os.environ.get("SESSION_AUDIT_LOG_DIR")
    if override:
        return override
    home = os.path.expanduser("~")
    mac = os.path.join(home, "Library", "Logs")
    if os.path.isdir(mac):
        return os.path.join(mac, "claude-session-audit")
    xdg = os.environ.get("XDG_STATE_HOME") or os.path.join(home, ".local", "state")
    return os.path.join(xdg, "claude-session-audit")


def blocks(ev, kind):
    msg = ev.get("message") or {}
    content = msg.get("content")
    if not isinstance(content, list):
        return []
    return [b for b in content if isinstance(b, dict) and b.get("type") == kind]


def parse_ts(s):
    if not isinstance(s, str):
        return None
    try:
        return datetime.fromisoformat(s.replace("Z", "+00:00"))
    except ValueError:
        return None


def nearest_rank(xs, q):
    """Nearest-rank percentile. Stated explicitly because DuckDB interpolates instead."""
    if not xs:
        return 0
    s = sorted(xs)
    idx = min(len(s) - 1, max(0, int(round(q * len(s))) - 1))
    return s[idx]


def build(transcript_path, session_id, reason, cwd):
    events = []
    with open(transcript_path, encoding="utf-8", errors="replace") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                events.append(json.loads(line))
            except ValueError:
                continue

    uses = {}                      # id -> (tool, input, ts)
    for ev in events:
        ts = ev.get("timestamp")
        for b in blocks(ev, "tool_use"):
            uses[b.get("id")] = (b.get("name") or "?", b.get("input") or {}, ts)

    results = {}                   # id -> (size, is_error, ts)
    for ev in events:
        ts = ev.get("timestamp")
        for b in blocks(ev, "tool_result"):
            c = b.get("content")
            text = c if isinstance(c, str) else json.dumps(c, ensure_ascii=False, default=str)
            results[b.get("tool_use_id")] = (len(text), bool(b.get("is_error")), ts)

    calls = Counter(t for t, _, _ in uses.values())
    errors = Counter()
    sizes = defaultdict(list)
    for tid, (tool, _, _) in uses.items():
        if tid in results:
            size, is_err, _ = results[tid]
            sizes[tool].append(size)
            if is_err:
                errors[tool] += 1

    hotspots = Counter()
    for tool, inp, _ in uses.values():
        if tool in EDIT_TOOLS and isinstance(inp, dict):
            p = inp.get("file_path")
            if isinstance(p, str) and p:
                hotspots[p] += 1

    runs = defaultdict(list)
    for tid, (tool, inp, _) in uses.items():
        if tool == "Bash" and isinstance(inp, dict):
            cmd = (inp.get("command") or "").strip()
            if cmd:
                runs[cmd].append(results.get(tid, (0, False, None))[1])
    retries = {c: outs for c, outs in runs.items() if len(outs) > 1}
    flipped = {c: outs for c, outs in retries.items() if len(set(outs)) > 1}

    seq = []
    for ev in events:
        ts = parse_ts(ev.get("timestamp"))
        if ts is None:
            continue
        for b in blocks(ev, "tool_result"):
            seq.append((ts, "res", bool(b.get("is_error"))))
        for b in blocks(ev, "tool_use"):
            seq.append((ts, "use", False))
    seq.sort(key=lambda x: x[0])
    gaps = []
    for i, (ts, kind, err) in enumerate(seq):
        if kind != "res" or not err:
            continue
        for nts, nkind, _ in seq[i + 1:]:
            if nkind == "use":
                gaps.append(round((nts - ts).total_seconds(), 1))
                break

    total_burn = sum(sum(v) for v in sizes.values())
    return {
        "schema": 1,
        "version": VERSION,
        "session_id": session_id,
        "reason": reason,
        "cwd": cwd,
        "ended_at": datetime.now().astimezone().isoformat(timespec="seconds"),
        "transcript": transcript_path,
        "events": len(events),
        "percentile_method": "nearest-rank",
        "tools": [
            {"tool": t, "calls": n, "errors": errors.get(t, 0),
             "p50": nearest_rank(sizes.get(t, []), 0.5),
             "p90": nearest_rank(sizes.get(t, []), 0.9),
             "max": max(sizes[t]) if sizes.get(t) else 0,
             "bytes": sum(sizes.get(t, []))}
            for t, n in calls.most_common()
        ],
        "total_result_bytes": total_burn,
        "hotspots": [{"file": f, "edits": n}
                     for f, n in hotspots.most_common() if n >= HOTSPOT_MIN],
        "retries": {
            "repeated": len(retries),
            "flipped": len(flipped),
            "examples": [{"command": c[:200], "runs": len(runs[c])}
                         for c in list(flipped)[:MAX_RETRY_SHOWN]],
        },
        "recovery_seconds": {
            "samples": len(gaps),
            "p50": nearest_rank(gaps, 0.5),
            "p90": nearest_rank(gaps, 0.9),
            "max": max(gaps) if gaps else 0,
        },
    }


def render(rec):
    def kb(n):
        return f"{n/1024:.0f}KB" if n >= 1024 else f"{n}B"

    out = [f"# session audit — {rec['session_id'][:8]}", ""]
    out.append(f"Ended {rec['ended_at']} ({rec['reason']}). {rec['events']} events, "
               f"{kb(rec['total_result_bytes'])} of tool output.")
    out.append("")
    out.append("## Tools")
    out.append("")
    out.append("| tool | calls | errors | p50 | p90 | max | total |")
    out.append("|---|--:|--:|--:|--:|--:|--:|")
    for t in rec["tools"][:10]:
        out.append(f"| {t['tool']} | {t['calls']} | {t['errors']} | {kb(t['p50'])} | "
                   f"{kb(t['p90'])} | {kb(t['max'])} | {kb(t['bytes'])} |")
    out.append("")
    out.append("Percentiles are nearest-rank.")

    if rec["hotspots"]:
        out += ["", f"## Edit hotspots (>= {HOTSPOT_MIN} edits on one file)", ""]
        for h in rec["hotspots"]:
            out.append(f"- **{h['edits']}x** {h['file']}")
        out.append("")
        out.append("Each one is a candidate for batching into a single write.")

    r = rec["retries"]
    if r["repeated"]:
        out += ["", "## Repeated Bash commands", "",
                f"{r['repeated']} command(s) ran more than once, "
                f"and the outcome flipped on {r['flipped']}."]
        for e in r["examples"]:
            out.append(f"- {e['runs']}x  `{e['command'][:90]}`")
        if r["flipped"]:
            out.append("")
            out.append("A flip means a later attempt succeeded where an earlier one failed. "
                       "That is where the session fought something.")

    rs = rec["recovery_seconds"]
    if rs["samples"]:
        out += ["", "## Recovery after a failure", "",
                f"{rs['samples']} failed result(s). Seconds to the next tool call: "
                f"p50 {rs['p50']}, p90 {rs['p90']}, max {rs['max']}."]
    return "\n".join(out) + "\n"


def main():
    try:
        raw = sys.stdin.read()
    except Exception:
        return 0
    try:
        ev = json.loads(raw) if raw.strip() else {}
    except ValueError:
        return 0

    tp = ev.get("transcript_path") or ""
    if not tp or not os.path.isfile(tp):
        return 0

    session_id = ev.get("session_id") or "unknown"
    try:
        rec = build(tp, session_id, ev.get("reason") or "other", ev.get("cwd") or "")
    except Exception:
        return 0

    d = log_dir()
    stem = f"{datetime.now().strftime('%Y%m%d')}-{session_id[:8]}"
    try:
        os.makedirs(d, exist_ok=True)
        tmp = os.path.join(d, f".{stem}.tmp")
        with open(tmp, "w", encoding="utf-8") as fh:
            json.dump(rec, fh, ensure_ascii=False)
        os.replace(tmp, os.path.join(d, f"{stem}.json"))
        tmp = os.path.join(d, f".{stem}.md.tmp")
        with open(tmp, "w", encoding="utf-8") as fh:
            fh.write(render(rec))
        os.replace(tmp, os.path.join(d, f"{stem}.md"))
    except OSError:
        return 0

    # SessionEnd discards stdout and stderr. Printing is for `--report`, below, not for the hook.
    return 0


if __name__ == "__main__":
    if "--report" in sys.argv:
        # Manual use: render a transcript to stdout without writing anything.
        args = [a for a in sys.argv[1:] if a != "--report"]
        if not args:
            print("usage: session-audit.py --report <transcript.jsonl>", file=sys.stderr)
            sys.exit(2)
        print(render(build(args[0], os.path.basename(args[0])[:8], "manual", os.getcwd())))
        sys.exit(0)
    sys.exit(main())
