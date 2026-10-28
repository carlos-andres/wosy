#!/usr/bin/env bash
# check-frontmatter.sh — the eighth instrument. Enforces the 8-key envelope of dist/schema.md.
#
# WHY THIS EXISTS. Until this file, all eight frontmatter keys were unenforced by any command
# (defect 3 of /Users/you/.devwork/_scratch/plan-20260915.md). That includes `para:`, which the
# corpus study measured as the strongest discriminator available: 52 of 52 in the healthy donor
# series against 3 of 136 in the failing one. The protocol checked seven properties of the body
# and zero properties of the envelope that carries the strongest signal.
#
# WHAT IT DOES NOT DO. It reads presence, order, shape and resolvability. It cannot read whether
# `para:` names the heading the next session actually opens with, or whether `modo:` bounds the
# role honestly. Those are judgements. A green run here means the envelope is well formed, never
# that it is true. Same honest boundary as P1 to P7: the command sees the document, not the round.
#
# Usage: check-frontmatter.sh <document.md>
# Exit 0 when every check passes. Exit 1 on any failure. Every failure NAMES the key at fault, so
# a control that deletes one key can assert on the name and not merely on the exit code.
set -uo pipefail

DOC="${1:?usage: check-frontmatter.sh <document.md>}"
[ -f "$DOC" ] || { echo "BREACH: $DOC does not exist"; exit 1; }
BASE="$(cd "$(dirname "$DOC")" && pwd)"
fail=0

say() { printf '%-4s %-10s %-9s %s\n' "$1" "$2" "$3" "$4"; }
ok()  { say "$1" "PASS" "$2" "$3"; }
bad() { say "$1" "FAIL" "$2" "$3"; fail=1; }

# --- the envelope slice ---------------------------------------------------------------------
# The block runs from a line 1 that is exactly `---` to the next `---`. A document whose first
# line is anything else has no envelope at all, which is a different failure from a missing key
# and is reported as such.
if [ "$(head -1 "$DOC")" != "---" ]; then
  say "F0" "FAIL" "structural" "no frontmatter block - line 1 of $(basename "$DOC") is not ---"
  echo
  echo "FRONTMATTER BROKEN"
  exit 1
fi
END=$(awk 'NR>1 && /^---[[:space:]]*$/ {print NR; exit}' "$DOC")
if [ -z "$END" ]; then
  say "F0" "FAIL" "structural" "frontmatter block is never closed - no second --- in $(basename "$DOC")"
  echo
  echo "FRONTMATTER BROKEN"
  exit 1
fi
BLOCK="$(sed -n "2,$((END-1))p" "$DOC")"
ok "F0" "structural" "frontmatter block found, lines 2 to $((END-1))"

# --- F1 the eight keys, present and in order --------------------------------------------------
# Order is part of the schema: the donor series held this envelope identical across 52 consecutive
# files. An out-of-order envelope still loads, so this is the weakest of the checks here and it is
# reported separately from presence, never folded into it.
EXPECTED=(tipo proyecto part_of fecha supersede sesion_previa para modo)
FOUND=()
while IFS= read -r line; do
  case "$line" in
    [a-z_]*:*) FOUND+=("${line%%:*}") ;;
  esac
done <<< "$BLOCK"

missing=""
for k in "${EXPECTED[@]}"; do
  hit=0
  for f in "${FOUND[@]:-}"; do [ "$f" = "$k" ] && hit=1 && break; done
  [ "$hit" -eq 1 ] || missing="$missing $k"
done
if [ -n "$missing" ]; then
  bad "F1" "structural" "missing key:$missing"
else
  ok "F1" "structural" "all 8 keys present: ${EXPECTED[*]}"
fi

actual_order="$(printf '%s ' "${FOUND[@]:-}" | tr -s ' ')"
expected_order="$(printf '%s ' "${EXPECTED[@]}" | tr -s ' ')"
if [ -z "$missing" ] && [ "$actual_order" != "$expected_order" ]; then
  bad "F2" "structural" "keys out of schema order - found: ${actual_order% }"
elif [ -z "$missing" ]; then
  ok "F2" "structural" "keys in schema order"
else
  say "F2" "SKIP" "structural" "order not judged while a key is missing"
fi

val() { printf '%s\n' "$BLOCK" | sed -n "s/^$1:[[:space:]]*//p" | head -1; }

# --- F3 no key carries an empty value ---------------------------------------------------------
# A present-but-empty key is the failure mode a presence test invites. `para:` with nothing after
# it passes any grep for the key name and carries none of the signal the key was measured for.
empty=""
for k in "${EXPECTED[@]}"; do
  case " $missing " in *" $k "*) continue;; esac
  v="$(val "$k")"
  [ -n "$v" ] || empty="$empty $k"
done
if [ -n "$empty" ]; then
  bad "F3" "structural" "key present with empty value:$empty"
else
  ok "F3" "structural" "every present key carries a value"
fi

# --- F4 tipo is kickstart ----------------------------------------------------------------------
# D13, decision-artifact-20260915.md: one artifact, both roles inside it. The handoff and the
# kickstart are the same document. A document declaring `tipo: handoff` is the retired two-file
# shape and is refused here rather than silently validated as if it were the current one.
t="$(val tipo)"
case "$t" in
  kickstart) ok "F4" "structural" "tipo is kickstart, the one artifact of D13" ;;
  "")        bad "F4" "structural" "tipo carries no value" ;;
  handoff)   bad "F4" "structural" "tipo is handoff - the two-document shape was retired by D13, use kickstart" ;;
  *)         bad "F4" "structural" "tipo is $t - the protocol validates tipo: kickstart" ;;
esac

# --- F5 fecha is a UTC calendar date -----------------------------------------------------------
# Shape only. Whether it matches the UTC clock is the stamping rule's job, and that rule compares
# against modification time, which goes stale the moment the file is edited. Asserting it here
# would make every later edit of a correct document fail.
f="$(val fecha)"
if printf '%s' "$f" | rg -q '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'; then
  ok "F5" "structural" "fecha is a YYYY-MM-DD date: $f"
else
  bad "F5" "structural" "fecha is not YYYY-MM-DD: ${f:-<empty>}"
fi

# --- F6 supersede resolves, or declares itself the first of its chain ---------------------------
# The single-successor chain is what makes "read only the latest" mechanically safe. A supersede
# pointing at nothing breaks the chain silently, which is worse than having no chain.
s="$(val supersede)"
case "$s" in
  ""|none|NONE|first)
    ok "F6" "structural" "supersede declares no predecessor - first document of its chain" ;;
  *)
    if [ -e "$s" ] || [ -e "$BASE/$s" ] || [ -e "$BASE/${s##*/}" ]; then
      ok "F6" "structural" "supersede resolves: $s"
    else
      bad "F6" "structural" "supersede names a file that does not resolve: $s"
    fi ;;
esac

echo
if [ "$fail" -eq 0 ]; then
  echo "FRONTMATTER OK"
else
  echo "FRONTMATTER BROKEN"
fi
exit "$fail"
