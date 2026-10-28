#!/usr/bin/env bash
#
# audit-wiring.sh - audit a Claude Code home for wiring defects.
#
# The point of a clean reinstall is that everything installed is wired correctly. This finds
# the four ways that goes wrong:
#
#   LEAK      a wired command points at a file that does not exist, or a hook file on disk
#             is wired nowhere. Either way the install and the config disagree.
#   DUPLICATE the same command is wired twice inside the SAME matcher group, so it runs twice
#             per call. The same command under DIFFERENT matchers is normal and not reported:
#             secret_guard.py is deliberately wired under three.
#   OVERLAP   two matcher groups in one event both match a tool, so every hook in both fires
#             on one call. This is how a chain grows past what anyone reads.
#   ORDER     rtk must be first in the effective Bash chain and secret_guard.py must be last.
#             The effective chain is the dispatcher stage list once bash-chain owns the group,
#             because array position in settings.json is NOT an ordering guarantee. Measured
#             2026-09-17: three hooks declared A, B, C started within 21 microseconds and the
#             one declared third started first.
#             rtk rewrites the command, and the guard must judge what will actually run.
#
# What it CANNOT see, and the reason this is printed rather than only documented:
#
#   A plugin wires hooks outside settings.json. Nothing here reads plugin configuration, so a
#   plugin-provided hook is invisible to every check below. A corporate home on 2026-09-18 fired
#   two SessionStart hooks and one UserPromptSubmit hook that appear in no settings.json while
#   this script printed CLEAN at the same moment. That verdict was true about its sources and
#   silent about everything else. The SOURCES block exists so a reader cannot take CLEAN to mean
#   "nothing else runs". The README is not in front of the person reading the verdict.
#
# Read-only. It never edits settings.json.
#
# Usage:
#   ./audit-wiring.sh                    audit $HOME/.claude
#   ./audit-wiring.sh /path/to/.claude   audit another home
#
set -uo pipefail

# --- --selftest -------------------------------------------------------------
# The SOURCES block is worthless if it prints the same text whatever the home holds. These controls
# drive THIS script against two homes that differ in one property, and each assertion names the
# input that flips it. A block of constants fails every row.
if [ "${1:-}" = "--selftest" ]; then
  SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
  ST_FAIL=0
  # The main audit has cleaned up after itself since before this selftest existed. Follow it.
  # Declared empty first, because `set -u` makes the trap fail on an unset name.
  TA=""; TB=""; TC=""; TD=""; TE=""
  trap 'rm -rf -- ${TA:+"${TA}"} ${TB:+"${TB}"} ${TC:+"${TC}"} ${TD:+"${TD}"} ${TE:+"${TE}"}' EXIT
  st() {  # st <label> <got> <want>
    if [ "$2" = "$3" ]; then printf '  [OK]   control: %s -> %s, expected %s\n' "$1" "$2" "$3"
    else printf '  [FAIL] control: %s -> %s, expected %s\n' "$1" "$2" "$3"; ST_FAIL=$((ST_FAIL + 1)); fi
  }

  # Home A: the dispatcher owns the Bash group and the stage list is on disk.
  TA="$(mktemp -d)"; A="${TA}/.claude"; mkdir -p "${A}/hooks"
  printf '#!/bin/sh\nexit 0\n' > "${A}/hooks/dispatch-bash.sh"; chmod +x "${A}/hooks/dispatch-bash.sh"
  printf '# stages\n' > "${A}/hooks/dispatch-bash.stages"
  printf '{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"bash %s/hooks/dispatch-bash.sh"}]}]}}\n' "${A}" > "${A}/settings.json"

  # Home B: no dispatcher and no stage list, so the chain is read from settings.json.
  TB="$(mktemp -d)"; B="${TB}/.claude"; mkdir -p "${B}"
  printf '{}\n' > "${B}/settings.json"

  # Home C: the dispatcher DOES own the group and the stage list is unreadable. This is the input
  # that separates the two IGNORED messages. A single message for both states reads as a wrong
  # diagnosis on a home that is wired correctly and merely has a permissions defect.
  TC="$(mktemp -d)"; C="${TC}/.claude"; mkdir -p "${C}/hooks"
  printf '#!/bin/sh\nexit 0\n' > "${C}/hooks/dispatch-bash.sh"; chmod +x "${C}/hooks/dispatch-bash.sh"
  printf '# stages\n' > "${C}/hooks/dispatch-bash.stages"; chmod 000 "${C}/hooks/dispatch-bash.stages"
  printf '{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"bash %s/hooks/dispatch-bash.sh"}]}]}}\n' "${C}" > "${C}/settings.json"

  # Home E: its wired command names a MISSING file in shell quotes. A parser that cannot see
  # past a quote skips the entry and prints "none", which is indistinguishable from a pass.
  TE="$(mktemp -d)"; E="${TE}/.claude"; mkdir -p "${E}/hooks"
  printf '{"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"[ ! -f %s ] || %s hook"}]}]}}\n' \
    "'${E}/hooks/gone.sh'" "'${E}/hooks/gone.sh'" > "${E}/settings.json"

  # Home D: carries a settings.local.json whose hook must be counted and named.
  TD="$(mktemp -d)"; D="${TD}/.claude"; mkdir -p "${D}/hooks"
  printf '#!/bin/sh\nexit 0\n' > "${D}/hooks/local-probe.sh"; chmod +x "${D}/hooks/local-probe.sh"
  printf '{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"%s/hooks/local-probe.sh"}]}]}}\n' "${D}" > "${D}/settings.local.json"
  printf '{}\n' > "${D}/settings.json"

  OA="$(bash "${SELF}" "${A}" 2>&1)"
  OB="$(bash "${SELF}" "${B}" 2>&1)"
  OC="$(bash "${SELF}" "${C}" 2>&1)"
  OD="$(bash "${SELF}" "${D}" 2>&1)"
  OE="$(bash "${SELF}" "${E}" 2>&1)"

  has() { printf '%s\n' "$1" | grep -qF -- "$2" && echo yes || echo no; }

  st "a home with a stage list reports it READ" "$(has "${OA}" "read: ${A}/hooks/dispatch-bash.stages")" yes
  st "a home with no stage list does NOT report it read" "$(has "${OB}" "hooks/dispatch-bash.stages")" no
  st "a home with no stage list names settings.json as the chain source" "$(has "${OB}" "the Bash group inside settings.json")" yes
  st "a home WITH a stage list does not also claim settings.json is the chain source" "$(has "${OA}" "the Bash group inside settings.json")" no
  st "each home names its OWN settings.json" "$(has "${OA}" "read: ${A}/settings.json")" yes
  st "one home never names the other home" "$(has "${OA}" "${B}")" no
  st "an unreadable stage list is reported as unreadable" "$(has "${OC}" "is NOT READABLE")" yes
  st "an unreadable stage list is not blamed on a missing dispatcher" "$(has "${OC}" "no dispatcher owns the Bash group")" no
  # These two assert fixed text rather than a property of the home, so they hold whatever the home
  # holds. They catch the deletion of the boundary, not a block that stopped reading the home.
  st "the plugin boundary is stated" "$(has "${OA}" "cannot see a hook that a plugin wires")" yes
  st "the verdict is scoped to the listed sources" "$(has "${OA}" "not a claim that nothing else runs")" yes
  # The pair that proves the overlay is read rather than announced. A home that has one must
  # report it READ and count its hook; a home that has none must say so and count nothing.
  st "a home with settings.local.json reports it READ" "$(has "${OD}" "read: ${D}/settings.local.json")" yes
  st "its hook is counted, so the overlay is parsed and not just named" "$(has "${OD}" "Wired hook entries: 1")" yes
  st "a home without one says so instead of staying silent" "$(has "${OA}" "so this home has no local hook overlay")" yes
  # Asserts the FINDING text, never the section header. The header prints on every home, so a
  # control anchored to it passes whatever the parser does, which is the defect it must catch.
  st "a quoted path to a missing file is reported, not skipped" "$(has "${OE}" "points at a missing file")" yes

  printf '\n'
  if [ "${ST_FAIL}" -eq 0 ]; then printf 'SELFTEST OK. 14 controls, 0 failed.\n'; exit 0
  else printf 'SELFTEST FAILED. %s of 14 controls failed.\n' "${ST_FAIL}"; exit 1; fi
fi

CLAUDE_HOME="${1:-${CLAUDE_HOME:-${HOME}/.claude}}"
SETTINGS="${CLAUDE_HOME}/settings.json"
STAGES="${CLAUDE_HOME}/hooks/dispatch-bash.stages"
LOCAL="${CLAUDE_HOME}/settings.local.json"

TMP="$(mktemp -d)"
trap 'rm -rf -- "${TMP}"' EXIT

FINDINGS=0
finding() { FINDINGS=$((FINDINGS + 1)); printf '  [%s] %s\n' "$1" "$2"; }
section() { printf '\n== %s ==\n' "$*"; SECTION_START="${FINDINGS}"; }
closing() { [[ "${FINDINGS}" -eq "${SECTION_START}" ]] && printf '  none\n'; return 0; }

# --- the effective PreToolUse Bash chain ------------------------------------
# bash-chain replaces the sibling Bash group with ONE dispatcher and moves the real execution
# order into hooks/dispatch-bash.stages. An instrument that reads settings.json alone then sees a
# single entry and reports a correct home as broken. So when the dispatcher owns the group, the
# stage list IS the chain. A leading ? marks an optional stage and is not part of the command.
# Duplicated on purpose rather than shared: bash-chain installs LAST, so a shared library would
# stop every earlier component from verifying before it exists.
bash_group() {
  jq -r '[ (.hooks.PreToolUse // [])[] | select(.matcher == "Bash") | .hooks[]? | .command // empty ] | .[]' "${MERGED}" 2>/dev/null
}

# Which document the effective chain actually comes from. SOURCES reports what this returns, so the
# two can never disagree: a block that named the stage list while effective_chain read the array
# would be a lie printed under a heading claiming honesty.
chain_source() {
  if bash_group | grep -q 'dispatch-bash\.sh' && [ -r "${STAGES}" ]; then
    printf 'stages\n'
  else
    printf 'settings\n'
  fi
}

effective_chain() {
  if [ "$(chain_source)" = "stages" ]; then
    sed -e 's/^[?!]//' -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "${STAGES}"
  else
    bash_group | grep -v '^[[:space:]]*$' || true
  fi
}

# How many wired points name a command, counting every event. The PreToolUse Bash group is read
# through effective_chain instead, so it is subtracted from the whole-document count and nothing
# is counted twice in either state: before bash-chain the chain IS that group, and after it the
# chain is the stage list.
wired_count() {  # wired_count <literal substring>
  local lit="$1" all inbash chain
  all="$(jq --arg s "${lit}" '[ (.hooks // {}) | .[]? | .[]? | .hooks[]? | select((.command // "") | contains($s)) ] | length' "${MERGED}")"
  inbash="$(jq --arg s "${lit}" '[ (.hooks.PreToolUse // [])[] | select(.matcher == "Bash") | .hooks[]? | select((.command // "") | contains($s)) ] | length' "${MERGED}")"
  chain="$(effective_chain | grep -Fc -- "${lit}" || true)"
  printf '%s\n' "$(( all - inbash + chain ))"
}

[[ -f "${SETTINGS}" ]] || { printf '[FAIL] no settings.json at %s\n' "${SETTINGS}" >&2; exit 1; }
jq -e . "${SETTINGS}" >/dev/null 2>&1 || { printf '[FAIL] %s is not valid JSON\n' "${SETTINGS}" >&2; exit 1; }

# --- the merged document: settings.json plus the local overlay --------------
# Claude Code reads settings.local.json too, and its hooks fire alongside the ones in
# settings.json. An audit that reads only settings.json therefore prints CLEAN while hooks it
# never saw are running. Measured 2026-09-19 on this home: `impeccable` wrote two hooks into
# settings.local.json, settings.json stayed byte-identical, and this script reported
# "CLEAN. 6 wired entries" while 8 were live. Both statements were true, which is the defect.
# The overlay's groups are CONCATENATED per event, because that is what the harness does: a
# local group does not replace the group of the same event, it adds to it.
MERGED="${TMP}/merged.json"
LOCAL_STATE="absent"
if [ -f "${LOCAL}" ]; then
  if jq -e . "${LOCAL}" >/dev/null 2>&1; then
    LOCAL_STATE="read"
    jq -s '
      .[0] as $a | .[1] as $b
      | ($a.hooks // {}) as $ah | ($b.hooks // {}) as $bh
      | $a * { hooks: (
          reduce ((($ah | keys_unsorted) + ($bh | keys_unsorted)) | unique)[] as $ev
            ({}; .[$ev] = (($ah[$ev] // []) + ($bh[$ev] // []))) ) }
    ' "${SETTINGS}" "${LOCAL}" > "${MERGED}"
  else
    LOCAL_STATE="unparseable"
    cp "${SETTINGS}" "${MERGED}"
  fi
else
  cp "${SETTINGS}" "${MERGED}"
fi

printf 'Auditing %s\n' "${CLAUDE_HOME}"

# Flatten to: <event> <TAB> <matcher> <TAB> <command>
jq -r '
  (.hooks // {}) as $h
  | $h | keys_unsorted[] as $ev
  | $h[$ev][]
  | (.matcher // "<none>") as $m
  | .hooks[]?
  | [$ev, $m, (.command // "")] | @tsv
' "${MERGED}" > "${TMP}/flat.tsv"

cut -f3 "${TMP}/flat.tsv" > "${TMP}/cmds.txt"
effective_chain >> "${TMP}/cmds.txt"   # stage-list entries are wired too
TOTAL="$(wc -l < "${TMP}/flat.tsv" | tr -d ' ')"
printf 'Wired hook entries: %s\n' "${TOTAL}"

# --- SOURCES ----------------------------------------------------------------
# Printed before the findings, never after, so it cannot be cut off with the verdict.
printf '\n== SOURCES - what this audit read, and what it cannot see ==\n'
printf '  read: %s\n' "${SETTINGS}"
if [ "$(chain_source)" = "stages" ]; then
  printf '  read: %s\n' "${STAGES}"
  printf '        the dispatcher owns the Bash group, so the stage list IS the effective chain\n'
else
  printf '  read: the Bash group inside settings.json, which is the effective chain here\n'
  # Two different states reach this branch and they have different fixes, so they get different
  # messages. An unreadable stage list is a permissions defect on a home that IS wired for the
  # dispatcher. A stage list nobody owns is a leftover file.
  if [ -e "${STAGES}" ]; then
    if bash_group | grep -q 'dispatch-bash\.sh'; then
      printf '  IGNORED: %s exists but is NOT READABLE, so the chain falls back to settings.json\n' "${STAGES}"
    else
      printf '  IGNORED: %s exists but no dispatcher owns the Bash group, so it is not the chain\n' "${STAGES}"
    fi
  fi
fi
case "${LOCAL_STATE}" in
  read)        printf '  read: %s\n' "${LOCAL}"
               printf '        its hooks fire alongside settings.json, so they are counted here\n' ;;
  unparseable) printf '  IGNORED: %s exists but is NOT VALID JSON, so its hooks are NOT counted\n' "${LOCAL}" ;;
  absent)      printf '  absent: %s, so this home has no local hook overlay\n' "${LOCAL}" ;;
esac
printf '  NOT read: plugin configuration. This audit cannot see a hook that a plugin wires,\n'
printf '            because a plugin hook never appears in settings.json.\n'
printf '  NOT read: skills, agents and output styles. None of them wires a hook.\n'
printf '  The verdict below covers the sources listed as read. It is not a claim that nothing else runs.\n'

# --- LEAK: wired command points at a missing file --------------------------
section "LEAK - wired but missing on disk"
while IFS=$'\t' read -r ev m cmd; do
  # Quotes are stripped FIRST. A shell-quoted path is one token that starts with a quote, so
  # `grep '^/'` never matched it and the entry fell out of the loop. The section then printed
  # "none" for an entry it had skipped, which reads exactly like an entry it had cleared.
  # Measured 2026-09-19 against the hook impeccable writes into settings.local.json.
  path="$(printf '%s' "${cmd}" | tr -d '"'"'"'"' | tr ' ' '\n' | grep -E '\.sh$|\.py$|^/' | tail -1)"
  [[ -z "${path}" ]] && continue
  path="${path/#\$\{HOME\}/${HOME}}"
  path="${path/#\~/${HOME}}"
  [[ "${path}" == /* ]] || continue
  [[ -e "${path}" ]] || finding LEAK "${ev}/${m}: points at a missing file -> ${path}"
done < "${TMP}/flat.tsv"
closing

# --- LEAK: hook file on disk wired nowhere ---------------------------------
section "LEAK - present on disk, wired nowhere"
if [[ -d "${CLAUDE_HOME}/hooks" ]]; then
  for f in "${CLAUDE_HOME}"/hooks/*.sh "${CLAUDE_HOME}"/hooks/*.py; do
    [[ -e "${f}" ]] || continue
    base="$(basename "${f}")"
    grep -qF -- "${base}" "${TMP}/cmds.txt" || finding LEAK "hooks/${base} exists but nothing wires it"
  done
fi
closing

# --- DUPLICATE: same command twice inside the SAME matcher group ------------
section "DUPLICATE - same command twice in one matcher group"
sort "${TMP}/flat.tsv" | uniq -c | awk '$1 > 1 { n=$1; $1=""; sub(/^ +/,""); print n "\t" $0 }' > "${TMP}/dupes.tsv"
while IFS=$'\t' read -r n rest; do
  [[ -z "${n:-}" ]] && continue
  finding DUPLICATE "${rest} -> wired ${n} times in the same group, so it runs ${n}x per call"
done < "${TMP}/dupes.tsv"
closing

# --- OVERLAP: two matcher groups in one event match the same tool -----------
section "OVERLAP - one tool matched by two groups in the same event"
# Emit "<event> <TAB> <tool>" once per (group, tool), then count repeats per event+tool.
awk -F'\t' '
  { key = $1 SUBSEP $2 }
  seen[key]++ { next }
  {
    n = split($2, parts, "|")
    for (i = 1; i <= n; i++) if (parts[i] != "" && parts[i] != "<none>") print $1 "\t" parts[i]
  }
' "${TMP}/flat.tsv" | sort | uniq -c | awk '$1 > 1 { print $1 "\t" $2 "\t" $3 }' > "${TMP}/overlap.tsv"
while IFS=$'\t' read -r n ev tool; do
  [[ -z "${n:-}" ]] && continue
  finding OVERLAP "${ev}: tool '${tool}' is matched by ${n} groups - every hook in all ${n} fires on one call"
done < "${TMP}/overlap.tsv"
closing

# --- ORDER: rtk first, secret_guard last in the Bash chain ------------------
section "ORDER - PreToolUse Bash chain"
FIRST="$(effective_chain | head -1)"
LAST="$(effective_chain | tail -1)"
CHAIN_N="$(effective_chain | grep -c . || true)"
printf '  chain length: %s\n' "${CHAIN_N}"
if grep -qF 'rtk hook claude' "${TMP}/cmds.txt"; then
  if [[ "${FIRST}" == "rtk hook claude" ]]; then
    printf '  rtk is first\n'
  else
    finding ORDER "rtk is wired but not first (first is: ${FIRST:-<none>}) - later guards judge a command that never runs"
  fi
fi
if grep -qF 'secret_guard.py' "${TMP}/cmds.txt"; then
  if [[ "${LAST}" == *secret_guard.py* ]]; then
    printf '  secret_guard.py is last\n'
  else
    finding ORDER "secret_guard.py is wired but not last (last is: ${LAST:-<none>})"
  fi
fi
[[ "${CHAIN_N}" -gt 8 ]] && finding OVERLAP "Bash chain is ${CHAIN_N} hooks deep - every Bash call pays all of them"
closing

# --- LEAK: hook sources a lib that is not installed -------------------------
section "LEAK - hook sources a missing library"
if [[ -d "${CLAUDE_HOME}/hooks" ]]; then
  for f in "${CLAUDE_HOME}"/hooks/*.sh; do
    [[ -e "${f}" ]] || continue
    grep -oE '\$\{HOME\}/\.claude/hooks/lib/[A-Za-z0-9_.-]+\.sh' "${f}" 2>/dev/null | sort -u > "${TMP}/libs.txt" || true
    while read -r lib; do
      [[ -z "${lib:-}" ]] && continue
      resolved="${lib/#\$\{HOME\}/${HOME}}"
      [[ -e "${resolved}" ]] || finding LEAK "hooks/$(basename "${f}") sources a missing lib -> ${resolved}"
    done < "${TMP}/libs.txt"
  done
fi
closing

printf '\n----------------------------------------\n'
if [[ "${FINDINGS}" -eq 0 ]]; then
  printf 'CLEAN. %s wired entries, 0 findings.\n' "${TOTAL}"
else
  printf '%s finding(s) across %s wired entries.\n' "${FINDINGS}" "${TOTAL}"
fi
[[ "${FINDINGS}" -eq 0 ]]
