#!/opt/homebrew/bin/bash
# Two copies of check-closure.sh must return the same verdict on the same document.
#
# equivalence.sh guards check-protocol.sh and cannot be repointed: it hardcodes OLD at line 18 and
# filters chain rows by that name. This is the missing twin, written 2026-09-16.
#
# The defect under test, recorded 2026-09-15 and unrepaired: the identical file reports CLOSED
# through a task folder's bin and BREACH through its portable path, because it resolves its tools
# folder from BASH_SOURCE. A control that comes back green here would mean the defect is gone.
set -uo pipefail
T="${T:-/Users/you/.devwork/tasks/handoff-protocol-impl-20260915}"
DOC="${1:-kickstart-adoption-20260915.md}"
A="$T/bin/check-closure.sh"
B="/Users/you/.devwork/bin/protocol/check-closure.sh"
for f in "$A" "$B"; do [ -f "$f" ] || { echo "FATAL: missing $f"; exit 2; }; done
cmp -s "$A" "$B" && ident="byte-identical" || ident="DIFFERENT BYTES"
( cd "$T" && bash "$A" "$DOC" >/dev/null 2>&1 ); ea=$?
( cd "$T" && bash "$B" "$DOC" >/dev/null 2>&1 ); eb=$?
printf 'document      %s\n' "$DOC"
printf 'copies        %s\n' "$ident"
printf 'folder bin    exit=%s  %s\n' "$ea" "$A"
printf 'portable      exit=%s  %s\n' "$eb" "$B"
if [ "$ea" -eq "$eb" ]; then
  echo "EQUIVALENT: both copies agree"
  exit 0
fi
echo "DIVERGENT: the same bytes return different verdicts depending on invocation path"
exit 1
