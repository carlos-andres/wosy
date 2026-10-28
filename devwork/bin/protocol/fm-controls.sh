#!/usr/bin/env bash
# fm-controls.sh — counts the frontmatter controls in a task folder.
#
# WHY A SCRIPT AND NOT A ONE-LINER. The ledger row grammar at ledger_row.py:20 splits a row on
# '|', so any pipe inside a cell shifts every column after it — including a pipe escaped as \| for
# markdown. On 2026-09-15 that shift moved the command text into the `expected` cell and
# check-ledger.py reported the row INCONSISTENT for the wrong reason. A counting command that
# needs a pipe therefore has to live in a file, not in the cell.
#
# A frontmatter control is a control-*.md that exercises check-frontmatter.sh. The protocol
# controls, which exercise check-protocol.sh, are counted separately and are not this number.
set -uo pipefail
DIR="${1:-.}"
/bin/ls -1 "$DIR"/control-*.md 2>/dev/null | rg -cv 'noprereg|ratio-stale|malformed|fixture-unmarked|ledgerfalse' || echo 0
