#!/usr/bin/env bash
# nonconformant.sh — counts task folders whose transfer document does not satisfy the protocol.
#
# WHY. Adoption is the property the protocol cannot assert about itself. On 2026-09-15 the schema
# lived in one hand-written folder, no skill emitted it, and three other folders carried transfer
# documents that no command had ever read. This counts those folders, so a ledger row can carry
# the number and an expiry rather than a memory.
#
# A folder counts as CONFORMANT when at least one of its transfer documents passes
# check-protocol.sh. A folder with no transfer document at all is not counted either way: it has
# nothing to transfer and is not a defect.
#
# Prints the count on the last line, so a ledger row can assert it.
# Exit 0 always — this is a census, not a gate. A census that fails the run it reports on cannot
# be read from inside a chain row that also wants the number.
set -uo pipefail

ROOT="${1:-/Users/you/.devwork/tasks}"
CHECK="${CHECK:-/Users/you/.devwork/bin/protocol/check-protocol.sh}"
SPEC="${SPEC:-/Users/you/.devwork/dist/protocol.md}"

[ -d "$ROOT" ] || { echo "BREACH: $ROOT is not a directory"; echo "nonconformant: unknown"; exit 0; }
[ -x "$CHECK" ] || { echo "BREACH: $CHECK is not executable"; echo "nonconformant: unknown"; exit 0; }

bad=0; good=0; empty=0
for d in "$ROOT"/*/; do
  [ -d "$d" ] || continue
  name="$(basename "$d")"
  mapfile -t docs < <(find "$d" -maxdepth 1 -type f \( -name 'kickstart*.md' -o -name 'handoff*.md' \) 2>/dev/null | sort)
  if [ "${#docs[@]}" -eq 0 ]; then
    printf '%-38s %s\n' "$name" "no transfer document"
    empty=$((empty+1))
    continue
  fi
  pass=""
  for doc in "${docs[@]}"; do
    if "$CHECK" "$doc" "$SPEC" >/dev/null 2>&1; then pass="$(basename "$doc")"; break; fi
  done
  if [ -n "$pass" ]; then
    printf '%-38s %s\n' "$name" "CONFORMANT via $pass"
    good=$((good+1))
  else
    printf '%-38s %s\n' "$name" "nonconformant, ${#docs[@]} document(s), none passes"
    bad=$((bad+1))
  fi
done

echo
echo "conformant: $good   without a transfer document: $empty"
echo "nonconformant: $bad"
