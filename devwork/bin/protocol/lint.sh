#!/usr/bin/env bash
# lint.sh — every mechanical defect of a transfer document, in ONE pass, before the file is final.
#
# WHY. The rendered document is half bookkeeping: 81 of 159 lines on 2026-09-15, §0 and §4. All the
# other instruments in this folder are checkers, none generates anything, so a model writes that half
# by hand and pays again for each error. The backtick-in-prose defect alone fired six times across
# five documents on 2026-09-15, and each firing cost a render, a failed check, a repair, a re-check.
# This command turns those passes into one.
#
# WHAT IT IS NOT. It does not reimplement a single rule. It RUNS the existing checkers and refuses to
# stop at the first failure, then adds the one comparison none of them makes on its own: the §0 tally
# token against bin/chain.txt. A rule lives in exactly one place, and that place is the checker.
# A lint that disagreed with a checker would be a second opinion, which is the defect this avoids.
#
# POSITION INDEPENDENT. BASE is the DOCUMENT's folder and TOOLS is this script's folder. The verdict
# is a property of the document, never of the caller's working directory. That dependence was
# measured and closed in check-protocol.sh on 2026-09-15 and this command is built without it.
#
#   lint.sh <doc> [chain.txt]
#
# Exit 0 when nothing is reported. Exit 1 on any defect.
set -uo pipefail

DOC="${1:?usage: lint.sh <doc> [chain.txt]}"
[ -f "$DOC" ] || { echo "BREACH: no document at $DOC"; exit 1; }
BASE="$(cd "$(dirname "$DOC")" && pwd)"; DOC="$BASE/$(basename "$DOC")"
TOOLS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAIN="${2:-$BASE/bin/chain.txt}"

tool() { if [ -e "$BASE/bin/$1" ]; then printf '%s' "$BASE/bin/$1"; else printf '%s' "$TOOLS/$1"; fi; }

defects=0
row() { printf '%-16s %-8s %s\n' "$1" "$2" "$3"; [ "$2" = "FAIL" ] && defects=$((defects+1)); return 0; }

# --- the four checkers, every one of them run, none of them allowed to end the pass ---------------
run_checker() {                       # name, command...
  local label="$1"; shift
  local out rc
  out="$("$@" "$DOC" 2>&1)"; rc=$?
  if [ "$rc" -eq 0 ]; then row "$label" "ok" "clean"; else
    row "$label" "FAIL" "exit $rc"
    printf '%s\n' "$out" | rg -N 'FAIL|BREACH|INCONSISTENT|CLASS2-FAIL' | sed 's/^/                          /'
  fi
}

run_checker frontmatter "$(tool check-frontmatter.sh)"
run_checker protocol    "$(tool check-protocol.sh)"
run_checker closure     "$(tool check-closure.sh)"
if rg -q '^\| *[A-Z][0-9]+ *\|' "$DOC"; then
  run_checker ledger  python3 "$(tool check-ledger.py)" --run
  run_checker decides python3 "$(tool check-decides.py)"
else
  row ledger "ok" "no ledger table in this document"
fi

# --- the one comparison no single checker makes: §0's declared tally against the chain table -------
# run-chain.sh makes it, but only by running all eleven rows. Before the document is final the rows
# may not be repointed yet, and the author needs the disagreement without paying for the chain.
if [ -f "$CHAIN" ]; then
  declared="$(rg -o -N -r '$1 $2 $3' 'chain-declared ([0-9]+) checks ([0-9]+) green ([0-9]+) red' "$DOC" | head -1)"
  total=$(rg -c -N '^[A-Z][0-9]+\|' "$CHAIN" || echo 0)
  green=$(rg -c -N '^[A-Z][0-9]+\|0\|' "$CHAIN" || echo 0)
  red=$((total-green))
  computed="$total $green $red"
  if [ -z "$declared" ]; then
    row tally "ok" "this document declares no chain-declared token"
  elif [ "$declared" = "$computed" ]; then
    row tally "ok" "declares $declared and $(basename "$CHAIN") computes $computed"
  else
    row tally "FAIL" "declares $declared and $(basename "$CHAIN") computes $computed"
  fi
else
  row tally "ok" "no chain table beside this document"
fi

echo
if [ "$defects" -eq 0 ]; then
  echo "LINT CLEAN — $(basename "$DOC")"
else
  # The two defects that fired most on 2026-09-15, named with their repair so the next pass is the last.
  echo "LINT $defects defective check(s) — $(basename "$DOC")"
  echo "  a cited name that resolves to nothing is usually a file name written in backticks inside prose."
  echo "  a BREACH names a file that resolves but is absent from the load order. Add it to §4, or unquote the mention."
fi
exit $([ "$defects" -eq 0 ] && echo 0 || echo 1)
