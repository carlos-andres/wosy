#!/usr/bin/env python3
"""Checks that a handover carries the seven load questions, and that L1 is runnable.

The load block and the judge battery are the same object: the receiver answers these to start and
the judge answers them to grade, so the document is its own answer key. Written 2026-09-16 against
`/Users/you/.devwork/_scratch/pasos-handover-20260916.md`.

L1 and L4 have no recovery route by construction. Every other question can be answered by running a
command, so only these two make the document load-bearing, and only these two are hard failures.

Usage: check-load-block.py <handover.md>
Exit 0 when all seven are present and L1 carries a runnable absolute command.
"""
import re, sys, subprocess, json

REQUIRED = {
    "L1": "the act and the command that runs it",
    "L2": "the file that owns current state",
    "L3": "what is blocked, and on whom",
    "L4": "what can proceed now",
    "L5": "what is ruled out",
    "L6": "what only the owner knows",
    "L7": "what is not your turn",
}
HARD = {"L1", "L4"}          # no recovery route exists for these
PLACEHOLDER = re.compile(r"<[^>]+>")


def code_spans(path):
    """Code spans via pandoc's AST. A regex over backticks cannot tell a span from a fence."""
    try:
        out = subprocess.run(["pandoc", "-f", "markdown", "-t", "json", path],
                             capture_output=True, text=True, check=True).stdout
    except Exception as e:
        print(f"FATAL: pandoc failed: {e}", file=sys.stderr)
        sys.exit(2)
    spans = []
    def walk(n):
        if isinstance(n, dict):
            if n.get("t") == "Code":
                spans.append(n["c"][1])
            for v in n.values():
                walk(v)
        elif isinstance(n, list):
            for v in n:
                walk(v)
    walk(json.loads(out))
    return spans


def main():
    if len(sys.argv) < 2:
        sys.exit("usage: check-load-block.py <handover.md>")
    doc = sys.argv[1]
    text = open(doc, encoding="utf-8").read()
    lines = text.splitlines()

    present, answers = {}, {}
    for key in REQUIRED:
        for i, line in enumerate(lines):
            if re.match(rf"^\s*{key}[.:]\s+\S", line):
                present[key] = i + 1
                answers[key] = line
                break

    fail = 0
    for key, why in REQUIRED.items():
        if key in present:
            print(f"{key}   PRESENT  line {present[key]:3d}  {why}")
        else:
            tier = "HARD" if key in HARD else "soft"
            print(f"{key}   MISSING  {tier:8s}  {why}")
            fail = 1

    # L1 must name a command a receiver can paste. A placeholder is not an answer.
    if "L1" in present:
        runnable = [s for s in code_spans(doc)
                    if s.startswith("/Users/") or " /Users/" in s]
        usable = [s for s in runnable if not PLACEHOLDER.search(s)]
        if not runnable:
            print("L1   FAIL     carries no absolute path in a code span")
            fail = 1
        elif not usable:
            print("L1   FAIL     every candidate command carries a <placeholder>")
            fail = 1
        else:
            print(f"L1   RUNNABLE {len(usable)} absolute command(s), no placeholder")

    print()
    print("LOAD BLOCK OK" if fail == 0 else "LOAD BLOCK BROKEN")
    return fail


if __name__ == "__main__":
    sys.exit(main())
