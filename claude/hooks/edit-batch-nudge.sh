#!/usr/bin/env bash
# edit-batch-nudge.sh — PostToolUse hook for Edit/Write.
#
# Tracks how many times each file has been edited in the current session
# (keyed by session_id). When the count crosses a threshold for a single file,
# emit additionalContext suggesting batched edits (replace_all / multi-line
# old_string). A full-content Write resets the file's counter (it IS the
# batched remedy). Task-folder status.md checkpoint files are exempt.
#
# State lives at /tmp/claude-edit-log-<session-id>.tsv (rotated by session).
# Cheap, ephemeral, no cross-session bleed.
#
# Project-agnostic. Hook installed at user-level.

set -euo pipefail

THRESHOLD=2   # nudge starting at the 3rd edit of a single file
# (session data: 26 Edit→Edit pairs/session; threshold 5 caught only 2 files)

INPUT=$(cat)
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty')
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // empty')
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Defensive: missing fields → exit clean (don't break the harness)
[[ -n "$SESSION_ID" && -n "$FILE_PATH" ]] || exit 0

# Task-folder status.md files are checkpoint logs: appending a progress entry
# per work step is their designed usage, not a batching failure. Exempt them.
case "$FILE_PATH" in
  */.devwork/tasks/*/status.md) exit 0 ;;
esac

LOG="/tmp/claude-edit-log-${SESSION_ID}.tsv"

# A Write replaces the file's full content — exactly the batched remedy the
# nudge asks for — so it clears the file's edit history instead of counting
# against it. Without this reset the escalation is unsatisfiable: complying
# with "design ONE batched Write" would itself trigger the next ABORT.
if [[ "$TOOL_NAME" == "Write" && -f "$LOG" ]]; then
  grep -vFx "$FILE_PATH" "$LOG" > "${LOG}.tmp" 2>/dev/null || true
  [[ -f "${LOG}.tmp" ]] && mv "${LOG}.tmp" "$LOG"
fi

printf '%s\n' "$FILE_PATH" >> "$LOG"
# grep -c prints "0" AND exits 1 on zero matches; `|| echo 0` would yield "0\n0".
COUNT=$(grep -cFx "$FILE_PATH" "$LOG" 2>/dev/null || true)
COUNT=${COUNT:-0}

if (( COUNT > THRESHOLD )); then
  REL="${FILE_PATH/#$HOME/~}"
  # Tier escalation: 3× heads-up / 4× stop / 5+× abort, with diffs-queued count.
  if (( COUNT >= 5 )); then
    MSG="ABORT BATCHING FAILURE: ${REL} edited ${COUNT}× sequentially this session. Read the file NOW, design ONE batched Write of the full content (a full Write resets this counter). diffs queued: ${COUNT}."
  elif (( COUNT == 4 )); then
    MSG="STOP: ${REL} edited ${COUNT}× sequentially this session. Read the whole file, design a single batched edit, Write the new full content (a full Write resets this counter). diffs queued: ${COUNT}."
  else
    MSG="Heads-up: ${REL} has been edited ${COUNT}× this session. If these are sequential single-line edits, consider batching with replace_all=true or a multi-line old_string. diffs queued: ${COUNT}."
  fi
  jq -n --arg ctx "$MSG" '{
    hookSpecificOutput: {
      hookEventName: "PostToolUse",
      additionalContext: $ctx
    }
  }'
fi

exit 0
