#!/usr/bin/env bash
#
# dispatch-bash.sh - run the Bash PreToolUse stages IN ORDER, inside one hook entry.
#
# WHY THIS EXISTS. Measured on 2026-09-17 against Claude Code 2.1.267, twice and by two
# independent harnesses: every hook in a matcher group runs in PARALLEL. Three hooks declared
# A, B, C started within 21 microseconds of each other, and the one declared THIRD started
# first. A sibling hook therefore never sees another hook's `updatedInput`: every test hook
# received the original command while the shell executed the rewritten one.
#
# The consequence for this install is a defect, not a style point. rtk rewrites a Bash command
# through `updatedInput`, and the credential guard must judge WHAT WILL RUN. Wired as sibling
# entries the guard judges `git status` while the shell runs `rtk git status`.
#
# Array position is not an ordering guarantee, so the only way to order stages is to put them
# in one process. That is this file.
#
# THE STAGE LIST IS DATA, NOT CODE. A first version hardcoded two stages, and a review found
# that its installer silently dropped the three other hooks the live chain carried. This file
# now reads its stages from a list, so it can only run what the installer enumerated, and the
# installer can only enumerate what it found.
#
#   stages file:  one stage per line, in execution order
#                 a line beginning with `?` is OPTIONAL: a failure there does not block
#                 a line beginning with `!` or with nothing is a GATE: any failure blocks
#                 a blank line or a `#` comment is ignored
#
# FAIL BEHAVIOUR, and it deliberately differs from the harness. Claude Code documents PreToolUse
# as fail-open: a timed-out hook does not block. A gate that cannot answer must not pass an
# unjudged command, so for a GATE stage this file blocks on:
#     any non-zero exit, a timeout, a missing executable, and an unreadable stages file.
# A first version blocked only on a timeout, which a review showed was the least likely
# failure. A crashing guard exited 1 and its command passed with the traceback discarded.
#
set -uo pipefail

STAGES="${STAGES:-/Users/you/.claude/hooks/dispatch-bash.stages}"
STAGE_TIMEOUT="${STAGE_TIMEOUT:-5}"

EVENT="$(cat)"
TMP="$(mktemp -d)"
trap 'rm -rf -- "${TMP}"' EXIT

block() { printf 'BLOCKED by bash-chain: %s\n' "$1" >&2; exit 2; }

# An empty event is not something to guess at. A gate cannot judge nothing.
case "${EVENT}" in
  "") block "the hook received empty input, so no stage could judge the command." ;;
esac
printf '%s' "${EVENT}" | jq -e . >/dev/null 2>&1 \
  || block "the hook received input that is not valid JSON, so no stage could judge the command."

[ -r "${STAGES}" ] \
  || block "the stage list at ${STAGES} is missing or unreadable, so no gate ran. Re-run the bash-chain installer."

CUR="${EVENT}"          # the event as the next stage will see it
LAST_JSON=""            # the most recent stage envelope, for the final decision
LAST_CMD=""             # the most recent rewrite, so it survives a later silent stage
RAN=0
GATES=0

while IFS= read -r line || [ -n "${line}" ]; do
  case "${line}" in ""|\#*) continue ;; esac

  optional=0
  case "${line}" in
    \?*) optional=1; line="${line#\?}" ;;
    \!*) line="${line#\!}" ;;
  esac
  line="${line# }"
  [ -n "${line}" ] || continue
  [ "${optional}" -eq 1 ] || GATES=$((GATES + 1))

  # A gate whose executable is absent is a gate that did not run. That is the failure the
  # review found fails open, so it blocks here.
  bin="${line%% *}"
  if ! command -v "${bin}" >/dev/null 2>&1 && [ ! -x "${bin}" ]; then
    if [ "${optional}" -eq 1 ]; then
      continue
    fi
    block "gate '${bin}' is not executable, so the command was never judged."
  fi

  printf '%s' "${CUR}" > "${TMP}/in.json"
  # shellcheck disable=SC2086
  timeout "${STAGE_TIMEOUT}" ${line} < "${TMP}/in.json" > "${TMP}/out" 2> "${TMP}/err"
  rc=$?
  out="$(cat "${TMP}/out")"
  err="$(cat "${TMP}/err")"
  RAN=$((RAN + 1))

  if [ "${rc}" -ne 0 ]; then
    if [ "${rc}" -eq 2 ]; then
      # A deliberate block. The stage's own message is what Claude is shown.
      printf '%s\n' "${err:-BLOCKED by ${bin}}" >&2
      exit 2
    fi
    if [ "${optional}" -eq 1 ]; then
      # An optional stage that failed contributes nothing and costs nothing.
      [ -n "${err}" ] && printf '%s: %s\n' "${bin}" "${err}" >&2
      continue
    fi
    if [ "${rc}" -eq 124 ]; then
      block "gate '${bin}' did not answer within ${STAGE_TIMEOUT}s. A gate that cannot judge a command does not pass it."
    fi
    block "gate '${bin}' exited ${rc}. A gate that fails does not pass the command. Its output was: ${err:-<empty>}"
  fi

  # Success. Carry a rewrite forward so the NEXT stage judges what will actually run.
  case "${out}" in
    \{*\})
      LAST_JSON="${out}"
      cmd="$(printf '%s' "${out}" | jq -r '.hookSpecificOutput.updatedInput.command // empty' 2>/dev/null || true)"
      if [ -n "${cmd}" ]; then
        LAST_CMD="${cmd}"
        CUR="$(printf '%s' "${CUR}" | jq -c --arg c "${cmd}" '.tool_input.command = $c' 2>/dev/null || printf '%s' "${CUR}")"
      fi
      ;;
    "") : ;;
    *)  # Plain text on a zero exit is advisory. PreToolUse stdout is never shown, so it goes
        # to stderr where the debug log at least keeps it.
        printf '%s: %s\n' "${bin}" "${out}" >&2 ;;
  esac
  [ -n "${err}" ] && printf '%s: %s\n' "${bin}" "${err}" >&2
done < "${STAGES}"

[ "${GATES}" -gt 0 ] \
  || block "the stage list declares no gate, so nothing judged the command. Re-run the bash-chain installer."

# One envelope out. The last stage's decision stands, and a rewrite from any stage survives.
if [ -n "${LAST_JSON}" ]; then
  printf '%s' "${LAST_JSON}" | jq -c --arg c "${LAST_CMD}" '
    .hookSpecificOutput.hookEventName = "PreToolUse"
    | if ($c | length) > 0 and ((.hookSpecificOutput.updatedInput // null) == null)
      then .hookSpecificOutput.updatedInput = {command: $c}
      else . end'
elif [ -n "${LAST_CMD}" ]; then
  jq -nc --arg c "${LAST_CMD}" \
    '{hookSpecificOutput:{hookEventName:"PreToolUse",updatedInput:{command:$c}}}'
fi
exit 0
