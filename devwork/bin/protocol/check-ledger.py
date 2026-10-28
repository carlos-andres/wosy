#!/usr/bin/env python3
"""Structural check of a half-B state ledger. Class-1 failures only.

A state claim can fail in two independent ways and the schema conflated them until 2026-09-13:

  Class 1 - internally inconsistent. The claim cell and the expected cell disagree, or the claim
            disagrees with its own command's pattern. Decidable from the text alone, so it needs
            no folder, no model and no judgement. That is this file.
  Class 2 - internally consistent and externally false. Claim and expected agree with each other
            and both disagree with what the command returns now. No amount of reading finds it.
            Only running the command does. That is the receiver's job, not this file's.

Round 13's plant was class 1, which is why a blind judge with no folder could catch it. Nothing in
the schema required the comparison that caught it, so it was luck dressed as a result. This file
removes the luck: class 1 is now a command, and the judge is left to do what only a judge can.

Class 2 was named here on 2026-09-13 and implemented on 2026-09-15, behind --run. Until that day
nothing on disk ran a ledger command, so `LEDGER OK` meant only that a row agreed with itself. It
reported OK over a row claiming 5 whose command returned 6.

Usage: check-ledger.py [--run] <handoff.md>
  default   class 1 only. Byte-identical to the behaviour every existing caller depends on.
  --run     class 1, then class 2: execute each row's command and compare its value to `expected`.

Class-2 contract, declared so it is a grammar and not an accident of the rows that exist today:
  value    the last non-empty line of combined stdout+stderr, with a leading "<label>: " stripped
  band     `exact` compares for equality. `at-least` parses both as numbers and needs value >= expected
  expires  a date earlier than today UTC makes the row EXPIRED: excluded, not trusted, not a failure
  exit     a non-zero exit from the command is a class-2 failure, reported with its code
  timeout  10 seconds per row, so one hanging command cannot wedge the Stop hook that runs the chain

QUARANTINE was struck from the failure-action vocabulary on 2026-09-15. An expired claim is
EXCLUDED, the same action every other failure carries. A vocabulary entry that never fires reads
as live and is not.

Exit 0 when every row is consistent or has no internal check available. Exit 1 on any inconsistency.
"""
import datetime, os, re, subprocess, sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from ledger_row import parse as parse_row

BOLD = re.compile(r"\*\*(.+?)\*\*")


def longest_common_run(a, b, floor):
    """True when a and b share a run of at least `floor` characters."""
    for size in range(len(a), floor - 1, -1):
        for start in range(0, len(a) - size + 1):
            if a[start:start + size] in b:
                return True
    return False


LABEL = re.compile(r"^[A-Za-z][A-Za-z ._-]*:\s*")


def command_value(command, cwd):
    """Run a ledger row's command and return (value, exit_code, error_or_None)."""
    cmd = command.strip().strip("`")
    try:
        r = subprocess.run(cmd, shell=True, cwd=cwd, capture_output=True,
                           text=True, timeout=10)
    except subprocess.TimeoutExpired:
        return None, None, "timed out after 10s"
    except Exception as exc:                       # noqa: BLE001 - reported, never raised
        return None, None, "could not run: %s" % exc
    out = (r.stdout or "") + (r.stderr or "")
    lines = [ln.strip() for ln in out.splitlines() if ln.strip()]
    if not lines:
        return "", r.returncode, None
    return LABEL.sub("", lines[-1]), r.returncode, None


def class2(cid, cells, cwd, today):
    """Return 'pass', 'fail' or 'expired'. Prints one row either way."""
    command, expected = cells[1], cells[2]
    band = cells[3] if len(cells) > 3 else "exact"
    expires = cells[4] if len(cells) > 4 else ""

    if expires and re.match(r"^\d{4}-\d{2}-\d{2}$", expires) and expires < today:
        print("%-4s EXPIRED            expired %s, today is %s -> EXCLUDED, not trusted"
              % (cid, expires, today))
        return "expired"

    value, code, err = command_value(command, cwd)
    if err:
        print("%-4s CLASS2-FAIL        %s" % (cid, err))
        return "fail"
    if code != 0:
        print("%-4s CLASS2-FAIL        command exited %s, value %r" % (cid, code, value))
        return "fail"

    if band == "at-least":
        try:
            ok = float(value) >= float(expected)
        except ValueError:
            print("%-4s CLASS2-FAIL        band at-least needs numbers, got value %r expected %r"
                  % (cid, value, expected))
            return "fail"
    else:
        ok = value == expected

    if ok:
        print("%-4s CLASS2-OK          command returned %r, band %s, expected %r"
              % (cid, value, band, expected))
        return "pass"
    print("%-4s CLASS2-FAIL        command returned %r, band %s, expected %r -> EXCLUSION"
          % (cid, value, band, expected))
    return "fail"


def check(path, run=False):
    rows = bad = 0
    for line in open(path, encoding="utf-8"):
        m = parse_row(line, min_cells=3)
        if not m:
            continue
        cid, cells = m
        claim, command, expected = cells[0], cells[1], cells[2]
        rows += 1

        bold = BOLD.search(claim)
        if not bold:
            print("%-4s NO-INTERNAL-CHECK  the claim cell marks no asserted value in bold" % cid)
            continue
        asserted = bold.group(1).strip()

        # A count row: the command returns the asserted quantity itself.
        if asserted == expected:
            print("%-4s CONSISTENT         count row, claim==expected==%s" % (cid, expected))
            continue

        # A presence row: the command returns a match count, and the asserted value lives inside
        # the pattern being matched. Falsify the claim and the pattern stops carrying it.
        hay = command.lower()
        needle = asserted.lower()
        if needle.isdigit():
            if re.search(r"(?<!\d)%s(?!\d)" % re.escape(needle), hay):
                print("%-4s CONSISTENT         presence row, %s appears in its own command" % (cid, asserted))
                continue
        elif longest_common_run(needle, hay, 8):
            print("%-4s CONSISTENT         presence row, claim text appears in its own command" % cid)
            continue

        print("%-4s INCONSISTENT       claim asserts %r, expected cell says %r, and the command "
              "does not carry the assertion -> EXCLUSION" % (cid, asserted, expected))
        bad += 1

    if rows == 0:
        print("BREACH: no ledger rows found in %s" % path)
        return 1
    print()
    print("%d rows checked, %d inconsistent" % (rows, bad))

    c2_bad = c2_exp = 0
    if run:
        print()
        cwd = os.path.dirname(os.path.abspath(path)) or "."
        today = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d")
        for line in open(path, encoding="utf-8"):
            m = parse_row(line, min_cells=3)
            if not m:
                continue
            verdict = class2(m[0], m[1], cwd, today)
            if verdict == "fail":
                c2_bad += 1
            elif verdict == "expired":
                c2_exp += 1
        print()
        print("class 2: %d failing, %d expired and excluded" % (c2_bad, c2_exp))

    total_bad = bad + c2_bad
    print("LEDGER OK" if total_bad == 0 else "LEDGER BROKEN")
    return 1 if total_bad else 0


if __name__ == "__main__":
    args = [a for a in sys.argv[1:] if a != "--run"]
    sys.exit(check(args[0], run="--run" in sys.argv[1:]))
