#!/usr/bin/env bash
# equivalence.sh — counts divergences between the two copies of the protocol checker.
#
# WHY. On 2026-09-15 the checkers were copied out of the measured laboratory at
# /Users/you/.devwork/tasks/factory-redesign-20260911/bin/ into a portable bundle, and the copy
# was patched to resolve its tools from its own location. Copying a measured instrument creates a
# second source of truth. This command is the assertion that the two have not drifted.
#
# The subject is the five rows of the laboratory chain that invoke check-protocol.sh, green and
# red alike. A comparison over green inputs only would pass on a checker that had stopped
# discriminating, so the red inputs are the control inside the test.
#
# Prints the divergence count on the last line, so a ledger row can assert it.
# Exit 0 when the two agree on every input. Exit 1 on any divergence.
set -uo pipefail

LAB="${LAB:-/Users/you/.devwork/tasks/factory-redesign-20260911}"
OLD="$LAB/bin/check-protocol.sh"
NEW="${NEW:-/Users/you/.devwork/bin/protocol/check-protocol.sh}"

for f in "$OLD" "$NEW"; do
  [ -x "$f" ] || { echo "BREACH: $f is not executable"; echo "divergences: unknown"; exit 1; }
done

# The five declared inputs, read from the laboratory chain rather than retyped here. A list typed
# into this file would go stale the moment a row moved, and would then measure a subject that is
# no longer the chain's.
mapfile -t ROWS < <(rg -N '^[A-Z][0-9]+\|' "$LAB/bin/chain.txt" | rg -F 'check-protocol.sh')
[ "${#ROWS[@]}" -gt 0 ] || { echo "BREACH: no check-protocol.sh rows found in $LAB/bin/chain.txt"; echo "divergences: unknown"; exit 1; }

div=0
for row in "${ROWS[@]}"; do
  cmd="${row##*|}"
  # Strip the leading "bin/check-protocol.sh " and keep the arguments as the row wrote them.
  args="${cmd#*check-protocol.sh }"
  # shellcheck disable=SC2086
  set -- $args
  argv=()
  for a in "$@"; do case "$a" in /*) argv+=("$a") ;; *) argv+=("$LAB/$a") ;; esac; done

  a_out="$("$OLD" "${argv[@]}" 2>&1)"; a_rc=$?
  b_out="$("$NEW" "${argv[@]}" 2>&1)"; b_rc=$?

  if [ "$a_out" = "$b_out" ] && [ "$a_rc" = "$b_rc" ]; then
    printf '%-9s SAME   exit=%s  %s\n' "${row%%|*}" "$a_rc" "$args"
  else
    printf '%-9s DIFF   old exit=%s new exit=%s  %s\n' "${row%%|*}" "$a_rc" "$b_rc" "$args"
    diff <(printf '%s' "$a_out") <(printf '%s' "$b_out") | head -6
    div=$((div+1))
  fi
done

echo
echo "inputs compared: ${#ROWS[@]}"
echo "divergences: $div"
[ "$div" -eq 0 ]
