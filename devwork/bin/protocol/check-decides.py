#!/usr/bin/env python3
"""Class-3 check of a half-B state ledger: does the command decide the claim it is attached to?

A state claim fails in three independent ways. Classes 1 and 2 were separated on 2026-09-13.

  Class 1 - internally inconsistent. claim and expected disagree. bin/check-ledger.py, deterministic.
  Class 2 - internally consistent, externally false. Both agree and both disagree with what the
            command returns now. Only a receiver that runs the command finds it.
  Class 3 - the command under-determines the claim. The output agrees with expected and proves
            something WEAKER than the claim asserts. Class 1 passes it, class 2 passes it, and the
            claim is false anyway. That is this file.

The live instance: C7 asserted "12 judge rounds have run" behind a command that greps one heading in
one file and returns 1. The output matched expected. Fourteen rounds had run. The command established
the presence of one string; it was written as though it established a total.

No function of (claim, command, expected) alone separates that row from the two healthy rows beside
it, because C5, C6 and C7 are syntactically isomorphic: all three are presence rows whose asserted
value sits inside the pattern being matched. The separation needs one more input, so the ledger row
gains a seventh cell that DECLARES what the command's output means, drawn from a closed vocabulary:

  count(<population>)   the output is the size of that population
  value(<key>)          the output is 1 iff <key> holds the asserted value, and some token of
                        <key> must appear in the command
  presence(<statement>) the output is 1 iff that statement's literal is present, and NOTHING stronger

Four checks run per row. Only the first three can report class 3.

  1 VOCABULARY   the decides cell parses as one of the three forms, or the row is malformed and
                 blocks the round, per the schema rule that a missing column is a malformed claim.
  2 ARITY        a presence declaration may not carry a claim whose asserted value is a bare cardinal.
                 A presence command establishes presence and never a total. This is what fires on C7.
  3 ENUMERATOR   a count declaration requires the command to COMPUTE the number rather than contain
                 it. Bracket expressions are stripped first, because [0-9] is a character class and
                 not the digit nine. This exists so that relabelling a presence row as count(...) --
                 the only relabel that would legitimately license a total -- does not escape check 2.
  4 KEY          a value declaration requires some token of its ARG to appear in the command, so
                 that the command locates an assignment of the thing being declared. This is a
                 COHERENCE check and not a teeth check: it catches a declaration aimed at the wrong
                 command, not a dishonest one. Round 16 relaxed it from a bare identifier to any
                 token, because a blind declaring party wrote `num_ctx key in bin/blind-judge.py`
                 -- correct, and rejected on a grammar it had never been told about.

What this file does NOT decide, stated here because it is the design's load-bearing weakness: it
decides CONSISTENCY BETWEEN THE DECLARATION AND THE CLAIM. It does not decide whether the declaration
is honest. Check 3 puts a mechanical obligation on count(...) because count is the only term that
licenses a cardinal total, but value(...) and presence(...) remain honour-bound, and an author who
mislabels a presence command as value(...) defeats this file with nothing in the row to detect it.
That is the sender-grades-itself configuration Shattuck measured at a 19-point over-credit
[V] Military Review Mar-Apr 2000 p. 69. The mitigation is not mechanical: the declaration has to be
written by a party that is not the claimant, which is the move already made for grading.

Usage: check-decides.py <handoff.md>
Exit 0 when every row's command decides its claim. Exit 1 on any class-3 row or malformed row.
"""
import os, re, sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from ledger_row import parse as parse_row

BOLD = re.compile(r"\*\*(.+?)\*\*")
DECL = re.compile(r"^(count|value|presence)\((.+)\)$")
BRACKET = re.compile(r"\[[^\]]*\]")
TOKEN = re.compile(r"[A-Za-z_][A-Za-z0-9_./-]{2,}")


def contains_standalone(number, text):
    """True when `number` appears in `text` as a whole number rather than inside a longer one."""
    return re.search(r"(?<!\d)%s(?!\d)" % re.escape(number), text) is not None


def check(path):
    rows = bad = 0
    for line in open(path, encoding="utf-8"):
        m = parse_row(line, min_cells=6)
        if not m:
            continue
        cid, cells = m
        claim, command, expected = cells[0], cells[1], cells[2]
        decides = cells[5].strip("`")
        rows += 1

        d = DECL.match(decides)
        if not d:
            print("%-4s MALFORMED          decides cell is %r, which is not one of "
                  "count(...) value(...) presence(...) -> BLOCKS THE ROUND" % (cid, decides))
            bad += 1
            continue
        form, arg = d.group(1), d.group(2).strip()

        bold = BOLD.search(claim)
        if not bold:
            print("%-4s NO-ASSERTION       the claim cell marks no asserted value in bold" % cid)
            continue
        asserted = bold.group(1).strip()
        cardinal = asserted.replace(",", "").isdigit()

        if form == "presence" and cardinal:
            print("%-4s UNDER-DETERMINED   claim asserts the cardinal %s, but the command is "
                  "declared presence(%s). A presence command establishes presence and never a "
                  "total -> CLASS 3" % (cid, asserted, arg))
            bad += 1
            continue

        if form == "count":
            if cardinal and asserted != expected:
                print("%-4s COUNT-INCOHERENT   count(%s) must return the asserted value, but the "
                      "claim asserts %s and expected says %s" % (cid, arg, asserted, expected))
                bad += 1
                continue
            if cardinal and contains_standalone(asserted, BRACKET.sub("", command)):
                print("%-4s NOT-AN-ENUMERATION count(%s) is claimed, but %s already appears inside "
                      "the command's own pattern. An enumeration computes the number, it does not "
                      "carry it -> CLASS 3" % (cid, arg, asserted))
                bad += 1
                continue
            print("%-4s DECIDES            count(%s) returns the asserted total %s"
                  % (cid, arg, asserted))
            continue

        if form == "value":
            hits = [t for t in TOKEN.findall(arg) if t in command]
            if not hits:
                print("%-4s KEY-ABSENT         value(%s) is claimed, but no token of it appears in "
                      "the command, so the command locates no assignment of it" % (cid, arg))
                bad += 1
                continue
            print("%-4s DECIDES            value(%s) is located by the command via %s, which "
                  "carries %s" % (cid, arg, hits[0], asserted))
            continue

        print("%-4s DECIDES            presence(%s) supports the claim, which asserts nothing "
              "stronger" % (cid, arg))

    if rows == 0:
        print("BREACH: no ledger rows found in %s" % path)
        return 1
    print()
    print("%d rows checked, %d under-determined or malformed" % (rows, bad))
    print("DECIDES OK" if bad == 0 else "DECIDES BROKEN")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(check(sys.argv[1]))
