#!/usr/bin/env bash
# output-telemetry.sh — Stop hook. Telemetry-only (does NOT block).
#
# Logs two output-discipline signals per assistant turn:
#   - oversized DUMP turns: long AND structurally sparse — char_count > 12000
#     with fewer than 15% of lines being markdown structure (headings, list
#     items, blockquotes, table rows, ordered items, code fences). This is the
#     Headlines-Not-Dumps signal. The prior char-only rule (>8000) measured
#     length, not dumping, and fired on ~94% of turns — table-rich forensic
#     answers are long but structured; the density factor separates a genuine
#     wall-of-dump from those.
#   - prose SELECT/INSERT/UPDATE/DELETE mentions in assistant text.
# `table_emitted` is still recorded as neutral metadata but is NO LONGER a
# trigger — markdown tables are frequently the correct format, so a table alone
# is not a violation.
#
# Stop hooks fire AFTER a response is generated. They can't unsend; they log.
# Each row carries struct_lines + density so the 12000/0.15 thresholds can be
# re-tuned against real flagged turns.
#
# Reads JSON event from stdin per Claude Code hook contract.
# Always exits 0 — never blocks.
#
# Promotion path: when genuine-dump rows fall <10/week, this can be hard-
# converted to `exit 2` on the oversized signal (Stop hook blocks only on
# exit 2 per Anthropic hooks contract; exit 1 is non-blocking).

set -uo pipefail

# detect-platform lib (XDG vs Library log paths). Fail-open stub keeps prior macOS path.
_dp_lib="${HOME}/.claude/hooks/lib/detect-platform.sh"
if [ -r "$_dp_lib" ]; then . "$_dp_lib"; else log_dir_for() { echo "$HOME/Library/Logs/claude-$1"; }; fi
LOG_DIR="$(log_dir_for output-telemetry)"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/$(date +%Y%m%d).jsonl"

# The Stop hook event payload contains the session transcript path (sessionId-based).
# Resolve the most-recent assistant turn and check it.
input="$(cat 2>/dev/null || true)"
transcript_path="$(printf '%s' "$input" | jq -r '.transcript_path // empty' 2>/dev/null)"

# No transcript means we can't measure; bail quietly.
[ -z "$transcript_path" ] || [ ! -f "$transcript_path" ] && exit 0

# Last assistant message text. Pull the last "assistant" turn's content text.
last_assistant_text="$(tac "$transcript_path" 2>/dev/null \
  | jq -r 'select(.type=="assistant") | .message.content[]? | select(.type=="text") | .text' 2>/dev/null \
  | head -200)"

# Empty means nothing to measure.
[ -z "$last_assistant_text" ] && exit 0

# Measurements
char_count=${#last_assistant_text}
line_count=$(printf '%s\n' "$last_assistant_text" | wc -l | tr -d ' ')
[ "$line_count" -gt 0 ] || line_count=1
has_md_table=$(printf '%s' "$last_assistant_text" | grep -qE '^\s*\|.*\|.*\|\s*$|^\s*\|---+\|' && echo 1 || echo 0)

# Structural-line count: lines that are markdown structure (headings, list
# items, blockquotes, table rows, ordered items, code fences). A well-organized
# answer is mostly structure; a raw dump is a wall of prose.
struct_lines=$(printf '%s\n' "$last_assistant_text" \
  | grep -cE '^[[:space:]]*(#{1,6}[[:space:]]|[-*+][[:space:]]|>|\||[0-9]+[.)][[:space:]]|```)' 2>/dev/null || true)
struct_lines=${struct_lines:-0}

# "Oversized" = a genuine dump: long AND structurally sparse. Integer form of
# (struct_lines / line_count < 0.15) is (struct_lines*100 < line_count*15).
if [ "$char_count" -gt 12000 ] && [ $((struct_lines * 100)) -lt $((line_count * 15)) ]; then
  oversized=1
else
  oversized=0
fi

# Action #15 Stop-hook side: count prose SQL SELECT/INSERT/UPDATE/DELETE mentions in
# assistant text. Distinguishes from legitimate code blocks by counting bare prose
# occurrences (rough proxy — false-positive tolerable since this is telemetry only).
prose_sql_count=$(printf '%s' "$last_assistant_text" \
  | grep -cE '\b(SELECT|INSERT INTO|UPDATE|DELETE FROM)[[:space:]]+(\*|DISTINCT|FROM|INTO|SET|[a-zA-Z_][a-zA-Z0-9_]*[[:space:]]+(FROM|SET|WHERE|VALUES))' \
  2>/dev/null || true)
prose_sql_count=${prose_sql_count:-0}

# Log only a genuine-dump turn or a prose-SQL turn. A markdown table alone is
# no longer a trigger (it is recorded as metadata when the row logs for another
# reason).
if [ "$oversized" = "1" ] || [ "$prose_sql_count" -gt 0 ]; then
  jq -nc \
    --arg ts "$(date +%Y-%m-%dT%H:%M:%S%z)" \
    --arg transcript "$transcript_path" \
    --argjson chars "$char_count" \
    --argjson lines "$line_count" \
    --argjson struct "$struct_lines" \
    --argjson oversized "$oversized" \
    --argjson table "$has_md_table" \
    --argjson prose_sql "$prose_sql_count" \
    '{ts:$ts, transcript:$transcript, chars:$chars, lines:$lines, struct_lines:$struct, density:(if $lines>0 then (($struct*1000/$lines|floor)/1000) else 0 end), oversized:($oversized==1), table_emitted:($table==1), prose_sql_count:$prose_sql}' \
    >> "$LOG" 2>/dev/null || true
fi

# Always allow.
exit 0
