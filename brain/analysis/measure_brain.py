#!/usr/bin/env python3
"""P1.5 — reproduce the bench ratios from the REAL brain binary (not transcribed).

Drives ./brain against a fresh store loaded with the pilot, counts actual bytes of
each command line (input) and stdout (output), tokens_est = chars/4 (the bench's
primary metric), and compares to research/platform-evidence/bench/results.csv.

  consolidation = fragments full-load  vs  one brain record (you stop loading 59 fragments)
  router        = load-whole-doc       vs  one brain CLI line returning one section
  queryable     = read-whole-to-filter vs  brain list --where (indexed query)
"""
import csv, json, os, subprocess, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
BRAIN = ROOT / "brain"
STORE = "/tmp/measure_brain.db"
PILOT = ROOT / "schema/examples/task.acme-subscription-hotfix.json"
TID = "acme-subscription-hotfix"
BENCH = ROOT / "../research/platform-evidence/bench"
FRAGMENTS = BENCH / "fragments_bundle.md"
RESULTS = BENCH / "results.csv"


def est(s) -> int:
    if isinstance(s, bytes):
        s = s.decode("utf-8", "replace")
    return round(len(s) / 4)


def run(*args):
    """Return (input_chars, output_str). input = the command line the agent types."""
    cmd = [str(BRAIN), *map(str, args)]
    line = " ".join(["brain", *map(str, args)])  # what the agent actually fires
    out = subprocess.run(cmd, capture_output=True, text=True)
    if out.returncode != 0 and "REJECTED" not in out.stderr:
        sys.stderr.write(out.stderr)
    return line, out.stdout


def baselines():
    b = {}
    with open(RESULTS) as f:
        for r in csv.DictReader(f):
            b[(r["variant"], r["operation"], r["mode"])] = int(r["total_tokens"])
    return b


def main():
    for ext in ("", "-wal", "-shm"):
        try:
            os.remove(STORE + ext)
        except FileNotFoundError:
            pass
    subprocess.run([str(BRAIN), "import", "--store", STORE, str(PILOT)],
                   check=True, capture_output=True)

    # whole-record full load via the brain (what "direct" pays to do any op)
    _, whole = run("get", "--store", STORE, "--task", TID)
    W = est(whole)
    F = est(FRAGMENTS.read_text())
    b = baselines()

    # --- router: per-op direct(load whole doc) vs router(one CLI line + section) ---
    ops = []
    # read_field
    li, lo = run("get", "--store", STORE, "--task", TID, "--section", "status")
    ops.append(("read_field", W, est(li) + est(lo)))
    # update_field
    li, lo = run("set", "--store", STORE, "--task", TID, "--section", "status", "--input", "active")
    ops.append(("update_field", W, est(li) + est(lo)))
    # append
    li, lo = run("append", "--store", STORE, "--task", TID, "--section", "todo",
                 "--input", '{"id":"m1","desc":"measure-probe","status":"open"}')
    ops.append(("append", W, est(li) + est(lo)))
    # query (list open todos)
    li, lo = run("list", "--store", STORE, "todos", "--where", "status=open")
    query_router = est(li) + est(lo)
    ops.append(("query", W, query_router))

    direct_avg = sum(d for _, d, _ in ops) / len(ops)
    router_avg = sum(r for _, _, r in ops) / len(ops)

    consolidation = F / W
    router = direct_avg / router_avg
    queryable = W / query_router

    print("== P1.5 measured from the real brain binary (tokens_est = chars/4) ==\n")
    print(f"fragments full-load (baseline today) : {F:>7} tok   [{FRAGMENTS.name}]")
    print(f"one brain record  (brain get full)   : {W:>7} tok\n")
    print(f"{'operation':<14}{'direct(load doc)':>18}{'router(brain)':>16}{'ratio':>9}")
    for name, d, r in ops:
        print(f"{name:<14}{d:>18}{r:>16}{d/r:>8.1f}x")
    print()

    rows = [
        ("consolidation", consolidation, 35.9, "fragments -> 1 record"),
        ("router (avg)",  router,        34.7, "load-doc -> one CLI line"),
        ("queryable",     queryable,     19.2, "read-to-filter -> indexed list"),
    ]
    print(f"{'metric':<16}{'measured':>10}{'bench':>9}{'verdict':>10}   note")
    ok = True
    for name, meas, base, note in rows:
        # tolerance: same order of magnitude (>= base/3) AND clearly a big win (>= 10x)
        passed = meas >= base / 3 and meas >= 10
        ok = ok and passed
        print(f"{name:<16}{meas:>9.1f}x{base:>8.1f}x{('PASS' if passed else 'FAIL'):>10}   {note}")

    # cross-check that our fragments baseline matches the bench's recorded one
    print(f"\nbaseline cross-check: bench fragments full_load (direct) = "
          f"{b[('fragments','full_load','direct')]} tok ; measured {F} tok")
    print("\n" + ("ALL RATIOS REPRODUCED" if ok else "RATIO(S) OUT OF TOLERANCE"))
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
