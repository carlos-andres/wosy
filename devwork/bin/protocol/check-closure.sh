#!/usr/bin/env bash
# Transitive closure of AUTHORITY.
# Rule: authority does not propagate through non-authoritative evidence.
# Only the mandatory tier of the load order is expanded. Every other tier is
# reachable but terminal — you may open it to verify a citation, you do not
# follow its citations onward. Exits 1 on any breach.
#
# Round 19 widened the extractor to the one P7 gained in round 18. The old pattern required a
# backtick touching the filename, so every name inside a command span was invisible to this checker.
# kickstart-20260914.md was cited six times by two mandatory-tier documents, none of them solo
# backticked, and this command returned CLOSED for its whole life. A span is now split on whitespace
# and only a token that ENDS in a known extension is taken, so run-blind-judge.sh.bak is never read
# as run-blind-judge.sh.
set -uo pipefail
T="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOC="${1:-$T/kickstart-20260913.md}"
PAT='^/?[A-Za-z0-9_~][A-Za-z0-9_/.~-]*\.(md|sh|py|txt)$'
names() { rg -o '`[^`]+`' "$1" | tr -d '`' | tr ' ' '\n' | rg "$PAT" | sed 's#.*/##' | sort -u; }
# Takes the file as its first argument. Delegation has to read a SECOND document's load order, and
# a slice function pinned to $DOC cannot do that.
sect() { sed -n "$2" "$1" | rg -o '`[^`]+`' | tr -d '`' | tr ' ' '\n' | rg "$PAT" | sed 's#.*/##' | sort -u; }

# The load-order section is found by role, not by a hardcoded number, and the tier break matches
# either language. Pinning this to one document's section numbering is how the checker went stale
# the moment a new kickstart superseded the old one.
load_start() { rg -n '^## §[0-9]+ · (Load order|Orden de carga)' "$1" | head -1 | cut -d: -f1; }
load_end()   { awk -v s="$2" 'NR>s && /^## /{print NR; exit}' "$1"; }
tier_break() { awk -v s="$2" 'NR>s && /^(Conditional|Condicionales)/{print NR; exit}' "$1"; }

LOAD_START=$(load_start "$DOC")
[ -z "$LOAD_START" ] && { echo "BREACH: no load-order section found in $(basename "$DOC")"; exit 1; }
LOAD_END=$(load_end "$DOC" "$LOAD_START"); [ -z "$LOAD_END" ] && LOAD_END=$(wc -l < "$DOC")
# Expand only the mandatory tier: load-order heading to the first tier break, either language.
TIER=$(tier_break "$DOC" "$LOAD_START"); [ -z "$TIER" ] && TIER="$LOAD_END"
LOAD=$(sect "$DOC" "${LOAD_START},${LOAD_END}p")
LOAD=$(printf '%s\n%s\n' "$LOAD" "$(/bin/ls -1 "$T/bin")" | sort -u)
MANDATORY=$(sect "$DOC" "${LOAD_START},${TIER}p")

# --- delegation -----------------------------------------------------------------------------------
# A kickstart may name another kickstart in its mandatory tier instead of copying that document's
# load order. Before 2026-09-14 that was unrepresentable here: the checker demanded every load order
# be closed under its own mandatory tier, so a delegating kickstart reported 59 breaches of which 58
# were the delegate's own tiers, sitting on disk and perfectly reachable. Copying them across is what
# drifts, and the copy is what a later session believes, so the checker follows the delegation.
#
# The delegate's whole load order becomes reachable and its mandatory tier stays expandable, because
# authority declared mandatory by an authoritative document is still authority. Iterated to a fixed
# point rather than one level deep, since a prediction made over the first level has already missed a
# second-level breach in this folder once.
delegated=""
while : ; do
  added=0
  for d in $MANDATORY; do
    case " $delegated " in *" $d "*) continue;; esac
    dp=$(fd -H -t f "^$d$" "$T" 2>/dev/null | head -1)
    [ -n "$dp" ] || continue
    ds=$(load_start "$dp"); [ -n "$ds" ] || continue
    de=$(load_end "$dp" "$ds"); [ -z "$de" ] && de=$(wc -l < "$dp")
    dt=$(tier_break "$dp" "$ds"); [ -z "$dt" ] && dt="$de"
    delegated="$delegated $d"
    LOAD=$(printf '%s\n%s\n' "$LOAD" "$(sect "$dp" "${ds},${de}p")" | sort -u)
    MANDATORY=$(printf '%s\n%s\n' "$MANDATORY" "$(sect "$dp" "${ds},${dt}p")" | sort -u)
    added=1
  done
  [ "$added" = 0 ] && break
done
[ -n "$delegated" ] && echo "DELEGATED load order from:$delegated"
# scratchpad and hook artifacts named as history, never as authority
IGNORE='^(session-ALL2\.txt|mine\.py|mine3\.py|write-location-warn\.sh)$'
# A document does not cite itself. The load order says what to LOAD, and the reader already holds
# this one. Without this exemption every kickstart breaches on the commands in its own §0.
SELF="$(basename "$DOC")"

frontier=$(names "$DOC"); seen=""; breach=0
while [ -n "$frontier" ]; do
  next=""
  for f in $frontier; do
    case " $seen " in *" $f "*) continue;; esac
    seen="$seen $f"
    printf '%s' "$f" | rg -q "$IGNORE" && continue
    [ "$f" = "$SELF" ] && continue
    # -F -x compares literally. A filename is full of dots, and under a regex match a dot is any
    # character, so a name could be satisfied by a different file that merely looks like it.
    if ! printf '%s\n' "$LOAD" | rg -q -F -x "$f"; then echo "BREACH: $f"; breach=1; continue; fi
    printf '%s\n' "$MANDATORY" | rg -q -F -x "$f" || continue   # terminal tier
    p=$(fd -H -t f "^$f$" "$T" 2>/dev/null | head -1)
    [ -n "$p" ] && next="$next $(names "$p")"
  done
  frontier=$(printf '%s\n' $next | sort -u)
done
[ $breach -eq 0 ] && echo "CLOSED: authority closure holds" || echo "closure BROKEN"
exit $breach
