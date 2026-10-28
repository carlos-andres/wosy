#!/usr/bin/env bash
# Every control document is excluded from authority bundles.
#
# bin/superseded.txt is not a history list. bin/run-blind-judge.sh and bin/run-halfb-gate.sh read it
# by name to keep files out of what a judge is shown, because those files are evidence and not
# authority. A control is evidence by definition, so every control belongs on that list.
#
# Round 19 built control-r18-negonly-20260914.md and did not add it. It would have entered a judge
# bundle as authority, and no check in the chain reported it. This command is that report.
#
# This is NOT a P-rule. protocol-spec.md defines seven checkable rules and that seven is claim K3.
# Adding an eighth would mutate a measured subject to gain a check, so this wires in beside
# check-closure.sh, check-ledger.py and check-decides.py instead.
#
# Usage: check-controls.sh [exclusion-list]
#   The list path is an argument so the control can run against a doctored copy. A checker with no
#   input that makes it fail is a baseline, whatever it prints.
# Exit 0 when every control is excluded. Exit 1 when one is not.
set -uo pipefail
T="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SUP="${1:-$T/bin/superseded.txt}"
[ -f "$SUP" ] || { echo "BREACH: exclusion list not found at $SUP"; exit 1; }

missing=0; total=0
while IFS= read -r p; do
  c="$(basename "$p")"
  total=$((total+1))
  # -F -x compares literally. A filename is full of dots and a regex dot matches any character.
  if printf '%s\n' "$(cat "$SUP")" | rg -q -F -x "$c"; then
    printf '%-4s %-38s %s\n' "OK" "$c" "excluded"
  else
    printf '%-4s %-38s %s\n' "BAD" "$c" "NOT on the exclusion list - it would enter a judge bundle as authority"
    missing=$((missing+1))
  fi
done < <(fd -t f '^control-.*\.md$' "$T" --max-depth 1 | sort)

echo
echo "$total control documents checked, $missing not excluded"
echo "list: $SUP"
if [ "$missing" -eq 0 ]; then echo "CONTROLS OK"; exit 0; else echo "CONTROLS BROKEN"; exit 1; fi
