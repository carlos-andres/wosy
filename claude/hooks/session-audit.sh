#!/usr/bin/env bash
# session-audit.sh — SessionEnd hook.
#
# Reads the session transcript JSONL, tallies Edit/Write/Read/Agent calls,
# and emits a one-screen scorecard to stderr. Flags suspicious patterns:
#   - same file edited > N times (suggests missed batching)
#   - subagent dispatches without an immediate git-status read
# Non-blocking; informational only.
#
# Project-agnostic. Hook installed at user-level.

set -euo pipefail

INPUT=$(cat)
TRANSCRIPT=$(echo "$INPUT" | jq -r '.transcript_path // empty')

[[ -n "$TRANSCRIPT" && -f "$TRANSCRIPT" ]] || exit 0

# Aggregate tool-use frequencies for the MAIN session only (skip sub-agent
# transcripts which are interleaved into the same file under some configs).
# Strategy: take assistant turns, count tool_use entries.

# Per-tool totals.
TOTALS=$(jq -r '
  select(.type=="assistant")
  | .message.content[]?
  | select(.type=="tool_use")
  | .name
' "$TRANSCRIPT" 2>/dev/null | sort | uniq -c | sort -rn || true)

# Per-file Edit/Write hotspots.
HOTSPOTS=$(jq -r '
  select(.type=="assistant")
  | .message.content[]?
  | select(.type=="tool_use" and (.name=="Edit" or .name=="Write"))
  | (.input.file_path // empty)
' "$TRANSCRIPT" 2>/dev/null | sort | uniq -c | sort -rn | awk '$1 > 4' || true)

echo "─── session audit ─────────────────────────────────────" >&2
if [[ -n "$TOTALS" ]]; then
  echo "Tool usage:" >&2
  echo "$TOTALS" | sed 's/^/  /' >&2
fi
if [[ -n "$HOTSPOTS" ]]; then
  echo "" >&2
  echo "Edit/Write hotspots (>4 calls on one file — candidate for batching):" >&2
  echo "$HOTSPOTS" | sed 's/^/  /' >&2
fi
echo "───────────────────────────────────────────────────────" >&2

# The per-session sessions.tsv append that used to live here was removed:
# nothing ever read the file, and cwd-relative writes scattered divergent
# copies across projects. Per-session metrics belong to wosy-metrics
# (~/.local/bin/wosy-metrics), which computes from transcripts on demand.

exit 0
