#!/usr/bin/env bash
# Protocol validator. Implements P1 to P7 of protocol-spec.md and NOTHING else.
#
# This file and protocol-spec.md are one artifact. Round 16 measured what happens when a checker's
# grammar and the instructions given to the party it checks live in two documents: they drift in
# silence and a correct answer is rejected on a constraint nobody stated. Change a rule here and
# change it there in the same edit.
#
# A STRUCTURAL check tests the property. A TEXTUAL check tests only that the document says the right
# words, and an author who writes the phrase without doing the thing defeats it. The ratio between
# them is computed below and compared against the one protocol-spec.md declares, so the number is NOT
# written here: two copies drift and three copies drift faster. P4 and P3 both moved from textual to
# structural on 2026-09-14. P4 moved after rounds 20 to 26 all passed it while no pre-registration
# file was written for any of them. P3 moved after the kickstart of round 20 failed it on phrasing
# while the fixture it describes was correctly declared and correctly left broken.
#
# M8 to M12 of the spec are rules for conducting a round, not properties of a document. This command
# cannot see them and does not pretend to. Its silence is not their satisfaction.
#
# Usage: check-protocol.sh <kickstart.md> [protocol-spec.md]
#   The spec path is an argument so the ratio assertion can run against a doctored copy. A checker
#   with no input that makes it fail is a baseline, whatever it prints.
# Exit 0 when every check passes. Exit 1 on any failure.
set -uo pipefail

DOC="${1:?usage: check-protocol.sh <kickstart.md>}"
[ -f "$DOC" ] || { echo "BREACH: $DOC does not exist"; exit 1; }
BASE="$(cd "$(dirname "$DOC")" && pwd)"

# --- portable resolution: instruments come from THIS script's folder --------------------------
# The document's own folder still wins when it carries a bin/, so a task folder that ships its
# own laboratory keeps it. Otherwise the tools beside this script answer. Before this block the
# checker could only validate a document that lived next to bin/, which made every folder but
# one unvalidatable. BASE stays the DOCUMENT's folder: P4 and P7 ask whether a citation resolves
# for the receiver, and the receiver reads from the document's folder, not from here.
TOOLS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
toolpath() { if [ -e "$BASE/bin/$1" ]; then printf '%s' "$BASE/bin/$1"; else printf '%s' "$TOOLS/$1"; fi; }

if [ -n "${2:-}" ]; then SPEC="$2"
elif [ -f "$BASE/protocol-spec.md" ]; then SPEC="$BASE/protocol-spec.md"
else SPEC="$TOOLS/protocol.md"; fi
fail=0
struct_pass=0; struct_total=0; text_pass=0; text_total=0

say() { printf '%-4s %-10s %-9s %s\n' "$1" "$2" "$3" "$4"; }

ok()   { say "$1" "PASS" "$2" "$3"; [ "$2" = "structural" ] && struct_pass=$((struct_pass+1)) || text_pass=$((text_pass+1)); }
bad()  { say "$1" "FAIL" "$2" "$3"; fail=1; }
tally(){ [ "$1" = "structural" ] && struct_total=$((struct_total+1)) || text_total=$((text_total+1)); }

# --- one citation grammar and one resolution grammar, read by P3, P4 and P7 -----------------------
# A citation may be folder-local or an absolute path into another tree. It is resolved AS WRITTEN
# first and only then as a basename: stripping the directory unconditionally reported two absolute
# paths as missing on 2026-09-14. These checks all ask whether a cited name resolves, and two readers
# of one question with two grammars is the defect that bin/ledger_row.py was written to end.
EXT='^/?[A-Za-z0-9_~][A-Za-z0-9_/.~-]*\.(md|sh|py|txt)$'
cited()    { rg -o '`[^`]+`' "$1" | tr -d '`' | tr ' ' '\n' | rg "$EXT" | sort -u; }
# A citation is tried AS WRITTEN only when it is ABSOLUTE. A relative form is resolved against
# BASE, never against the caller's working directory. Before 2026-09-15 `[ -e "$1" ]` ran first
# for every form, so `bin/run-chain.sh` resolved whenever the READER happened to stand in a folder
# holding one. The same document was then conformant or not depending on where it was checked,
# and the hub census returned 0 from one task folder and 1 from /private/tmp the same minute.
resolves() { local b="${1##*/}"; case "$1" in /*) [ -e "$1" ] && return 0 ;; esac; [ -e "$BASE/$1" ] || [ -e "$BASE/$b" ] || [ -e "$BASE/bin/$b" ] || [ -e "$TOOLS/$b" ]; }

# --- the section-0 slice, found by role rather than by a hardcoded number -------------------------
S0_START=$(rg -n '^## §0' "$DOC" | head -1 | cut -d: -f1)
if [ -z "$S0_START" ]; then
  tally structural; bad P1 structural "no §0 startup-chain section found"
else
  S0_END=$(awk -v s="$S0_START" 'NR>s && /^## /{print NR; exit}' "$DOC")
  [ -z "$S0_END" ] && S0_END=$(wc -l < "$DOC")
  tally structural
  if sed -n "${S0_START},${S0_END}p" "$DOC" | rg -q 'exit 1|BROKEN|FAIL|BREACH'; then
    ok P1 structural "§0 carries a check whose declared outcome is a failure"
  else
    bad P1 structural "§0 is all green - a chain with no control that must fail is a baseline"
  fi
fi

# --- P2 textual, and it is the floor ---------------------------------------------------------------
# P2 asks the receiver to report one thing it had to resolve. That is a property of what the receiver
# does after reading, not of this file, so no command reading the document can test it. It stays
# textual on purpose and the spec says so rather than leaving the ratio looking improvable.
tally textual
rg -qi 'name one thing' "$DOC" \
  && ok P2 textual "the receiver is asked to name one thing it had to resolve" \
  || bad P2 textual "nothing demands the receiver name one thing it resolved"

# --- P3 structural: a registered fixture is marked where it is named, and is still broken ---------
# Until 2026-09-14 this asked whether the phrase appeared anywhere in the file, so a document
# satisfied it with a sentence three sections away from the name it protects. The kickstart of round
# 20 failed it on phrasing while the fixture it describes was correctly declared and correctly left
# broken. It now reads bin/fixtures.txt. For every registered fixture the document CITES, a line that
# NAMES the fixture must carry the marker, and the registered checker must still exit non-zero over
# it. A document citing no registered fixture passes and the line says so, because the old rule
# demanded the phrase from documents that plant nothing and its own message called that case noise.
# The judging checker is read from the registry into a variable and is never written literally here.
tally structural
FIXREG="$(toolpath fixtures.txt)"
p3_bad=""; p3_seen=0
if [ -f "$FIXREG" ]; then
  while read -r fx ck _; do
    case "$fx" in ''|\#*) continue;; esac
    cited "$DOC" | sed 's#.*/##' | rg -q -F -x "$fx" || continue
    p3_seen=$((p3_seen+1))
    if ! rg -F "$fx" "$DOC" | rg -qi 'do not repair|live fixture'; then
      p3_bad="$p3_bad $fx:unmarked"
    elif python3 "$(toolpath "$ck")" "$BASE/$fx" >/dev/null 2>&1; then
      p3_bad="$p3_bad $fx:no-longer-broken"
    fi
  done < "$FIXREG"
fi
if [ -n "$p3_bad" ]; then
  bad P3 structural "registered fixtures not protected here:$p3_bad"
elif [ "$p3_seen" -gt 0 ]; then
  ok P3 structural "$p3_seen registered fixture(s) cited, each marked on a naming line and still failing"
else
  ok P3 structural "no registered fixture is cited, so there is nothing here to protect"
fi

# --- P4 structural: the named pre-registration resolves on disk -----------------------------------
# Until 2026-09-14 this matched the word `prereg` anywhere in the document, so an author satisfied it
# by writing the word and registering nothing. Rounds 20 to 26 each passed it and none of them left a
# seal file on disk. It now demands a cited file OF THE PRE-REGISTRATION CLASS, recognised by the
# basename prefix, which resolves under the grammar above. Resolution alone is already P7's job; what
# P4 adds is that a seal is named at all. Whether it was written BEFORE the round stays unmechanized,
# like M8 to M12.
tally structural
seal=""
while read -r f; do
  [ -z "$f" ] && continue
  case "${f##*/}" in prereg*) if resolves "$f"; then seal="$f"; break; fi ;; esac
done < <(cited "$DOC")
[ -n "$seal" ] \
  && ok P4 structural "a pre-registration is named and resolves: $seal" \
  || bad P4 structural "no pre-registration file is named - the word alone leaves a failed expectation nowhere to be recorded"

# --- P5 textual, and the structural form was measured and rejected --------------------------------
# The structural form would require the document to name a control that resolves on disk. It fails
# control-malformed-20260914.md, which cites no control at all and whose declared failing set is P1,
# P6 and P7. Converting P5 would break the secondary criterion every rule change here has carried.
tally textual
rg -qi 'control that must fail, run in the same block' "$DOC" \
  && ok P5 textual "the same-block control rule is stated" \
  || bad P5 textual "the same-block control rule is absent"

# --- P6 structural -------------------------------------------------------------------------------
# The old rule asked for a "Not your turn" section. D3 and the hub's schema README both measure that
# form as an anti-pattern, because naming excluded work supplies the vocabulary to hallucinate with.
# The rule now asks for the replacing act beside each exclusion. It is existential on purpose: it
# demands the positive form and does not police a negative list sitting beside it, which would fail
# every document written before round 18.
# rg -c prints nothing and exits 1 on no match, so the value is captured and defaulted, never piped.
tally structural
POS=$(rg -c '^- (Instead of|En lugar de) .+, \*\*.+\*\*' "$DOC" || true)
POS=${POS:-0}
[ "$POS" -gt 0 ] \
  && ok P6 structural "scope control is positive: $POS excluded acts carry their replacement" \
  || bad P6 structural "no positively-framed scope control - state the act that replaces each exclusion"

# --- P7 structural: every name the document cites resolves on disk -------------------------------
# Until round 18 this scanned the load-order section alone, and its pattern required a backtick
# touching the filename. Every citation sitting inside a command span was therefore invisible: §0
# named eight files and contributed zero of them. The span is now split on whitespace and each token
# that ENDS in a known extension is tested, which is why `run-blind-judge.sh.bak` is not mistaken for
# `run-blind-judge.sh`. P7 tests EXISTENCE only. Reachability from the load order is a different
# property and it belongs to bin/check-closure.sh, which is still blind to a name inside a command.
tally structural
missing=""
while read -r f; do
  [ -z "$f" ] && continue
  resolves "$f" || missing="$missing $f"
done < <(cited "$DOC")
if [ -z "$missing" ]; then
  ok P7 structural "every cited name resolves on disk"
else
  bad P7 structural "cited names that resolve to nothing:$missing"
fi

# --- the three ledger checks that already exist, wired in ----------------------------------------
echo
for pair in "check-closure.sh:authority closure" "check-ledger.py:class 1" "check-decides.py:class 3" "check-controls.sh:control exclusion"; do
  tool="${pair%%:*}"; what="${pair#*:}"
  if [ -e "$(toolpath "$tool")" ]; then
    printf '%-4s %-10s %-9s %s\n' "--" "WIRED" "existing" "$what -> bin/$tool"
  else
    printf '%-4s %-10s %-9s %s\n' "--" "ABSENT" "existing" "$what -> bin/$tool NOT FOUND"
    fail=1
  fi
done

echo
echo "structural $struct_pass/$struct_total   textual $text_pass/$text_total"

# --- the declared ratio and the computed one are one artifact, asserted by command ----------------
# Round 27 left the ratio in three places: this file's header, the prose of protocol-spec.md, and the
# tally above. Nothing compared them, and a rule left in prose lasts one round in this folder. The
# token is anchored on a key that appears nowhere else, because the spec quotes several
# `structural N/N` strings of validator output and a naive pattern would match one of those first.
# This is NOT a P-rule: seven checkable rules is claim K3, and this asserts a property of the
# validator and its spec rather than of the document under test, so it fails every run until repaired.
COMPUTED="$struct_total/$((struct_total + text_total))"
if [ ! -f "$SPEC" ]; then
  printf '%-4s %-10s %-9s %s\n' "--" "RATIO" "no spec" "$SPEC does not exist, so the declared ratio cannot be read"
  fail=1
else
  DECLARED=$(rg -o -N -r '$1' 'ratio-declared structural ([0-9]+/[0-9]+)' "$SPEC" | head -1)
  if [ -z "$DECLARED" ]; then
    printf '%-4s %-10s %-9s %s\n' "--" "RATIO" "ABSENT" "$(basename "$SPEC") declares no ratio-declared token"
    fail=1
  elif [ "$DECLARED" = "$COMPUTED" ]; then
    printf '%-4s %-10s %-9s %s\n' "--" "RATIO" "agrees" "$(basename "$SPEC") declares $DECLARED and this command computes $COMPUTED"
  else
    printf '%-4s %-10s %-9s %s\n' "--" "RATIO" "MISMATCH" "$(basename "$SPEC") declares $DECLARED and this command computes $COMPUTED - the spec and the checker are one artifact"
    fail=1
  fi
fi
echo "M8 to M12 of protocol-spec.md are method rules. This command cannot see them."
[ $fail -eq 0 ] && echo "PROTOCOL OK" || echo "PROTOCOL BROKEN"
exit $fail
