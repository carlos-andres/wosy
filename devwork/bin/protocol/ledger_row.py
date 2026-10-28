"""The one grammar for a half-B ledger row, shared by every checker that reads one.

Written 2026-09-14, after measuring that two checkers of the same ledger disagreed about what a
row is. bin/check-ledger.py anchored on `C\\d+` with six columns. bin/check-decides.py anchored on
`[A-Z]\\d+` with seven. Over kickstart-20260914-r18.md, whose rows are K1 to K4, the first reported
`no ledger rows found` and the second reported `4 rows checked`. One ledger, two readers, two
incompatible answers, and neither could report the disagreement.

A row id is one uppercase letter followed by digits. The letter carries genre and never grammar:
C for a carried claim, K for a kickstart claim. A checker that hard-codes one letter is blind to
every other genre, which is the defect this file exists to remove.

Cells are returned as a list so a checker takes the columns it needs and ignores the rest. The
schema's first four columns are fixed as id, claim, command, expected. Anything after them is
optional and checker-specific, which is how a six-column ledger and a seven-column ledger read
through the same grammar.
"""
import re

ROW = re.compile(r"^\|\s*([A-Z]\d+)\s*\|(.*)\|\s*$")


def parse(line, min_cells=3):
    """Return (row_id, cells) for a ledger row, or None when the line is not one.

    `cells` holds every column after the id, stripped. `min_cells` is the number of columns the
    calling checker needs, so a row too short for that checker is not a row for that checker.
    """
    m = ROW.match(line.rstrip("\n"))
    if not m:
        return None
    cells = [c.strip() for c in m.group(2).split("|")]
    if len(cells) < min_cells:
        return None
    return m.group(1), cells
