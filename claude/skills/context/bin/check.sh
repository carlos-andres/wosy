#!/bin/bash
# Read the current Claude Code session transcript and print context usage.
# Mirrors the math in ~/.claude/statusline/statusline.sh but pulls from the
# session JSONL on disk instead of stdin (which only the statusline receives).

set -u

# Parse --silent: emit a single `pct=N` line and exit, suitable for hook
# consumers (e.g. context-checkpoint.sh) that branch on threshold without
# parsing the human-facing block. Errors still go to stderr unchanged.
silent=0
if [ "${1:-}" = "--silent" ]; then
    silent=1
fi

# 1) Resolve project transcript directory: cwd path with '/' → '-' prefix-encoded.
proj_dir="$HOME/.claude/projects/$(pwd | tr '/' '-')"
if [ ! -d "$proj_dir" ]; then
    echo "no session dir at $proj_dir" >&2
    exit 1
fi

# 2) Most recent TOP-LEVEL .jsonl is the current main session
#    (subagent jsonls live under <session-id>/subagents/ and are excluded).
session=$(find "$proj_dir" -maxdepth 1 -type f -name '*.jsonl' -print0 2>/dev/null \
    | xargs -0 ls -t 2>/dev/null \
    | head -1)
if [ -z "${session:-}" ]; then
    echo "no .jsonl transcript in $proj_dir" >&2
    exit 1
fi

# 3) Pull the last assistant-message usage record.
#    Each line is a JSON message; assistant messages carry .message.usage.
last_usage_line=$(awk '/"usage"/ {l=$0} END {print l}' "$session")
if [ -z "$last_usage_line" ]; then
    echo "no usage record in $session" >&2
    exit 1
fi

usage=$(printf '%s' "$last_usage_line" | jq -c '.message.usage // empty' 2>/dev/null)
if [ -z "$usage" ]; then
    echo "could not parse .message.usage from last line" >&2
    exit 1
fi

input=$(printf '%s' "$usage" | jq -r '.input_tokens // 0')
cc=$(printf '%s' "$usage" | jq -r '.cache_creation_input_tokens // 0')
cr=$(printf '%s' "$usage" | jq -r '.cache_read_input_tokens // 0')
output=$(printf '%s' "$usage" | jq -r '.output_tokens // 0')
total=$((input + cc + cr))

# Priority 1: state file relayed by statusline — transcript loses the [1m] suffix so
#   1M sessions read ~5x inflated without it; values outside [100k,2M] treated as corrupt.
model=$(awk '/"model":"/ {
    match($0, /"model":"[^"]*"/);
    if (RSTART > 0) {
        s = substr($0, RSTART+9, RLENGTH-10);
        if (s != "") last = s;
    }
} END { print last }' "$session")

size=""
session_id=$(basename "$session" .jsonl)
state_file="$HOME/.claude/state/context-window-${session_id}"
if [ -f "$state_file" ]; then
    sf_val=$(head -1 "$state_file" 2>/dev/null | tr -d '[:space:]')
    case "$sf_val" in
        ''|*[!0-9]*) ;;
        *) if [ "$sf_val" -ge 100000 ] && [ "$sf_val" -le 2000000 ]; then
               size=$sf_val
           fi ;;
    esac
fi

#    Priority 2 (fallback): static model → window table.
if [ -z "$size" ]; then
    case "$model" in
        # 1M-window variants carry a [1m] suffix on any family (e.g. opus-4-8[1m]),
        # so this must match before the per-family branches. Quoted to defuse [..] globbing.
        *"[1m]"*)                          size=1000000 ;;
        *opus-4-7*|*opus-4-8*|*opus-4-9*) size=400000 ;;
        *opus*)                            size=200000 ;;
        *sonnet*)                          size=200000 ;;
        *haiku*)                           size=200000 ;;
        *)                                 size=200000 ;;
    esac
fi

if [ "$size" -gt 0 ]; then
    pct=$((total * 100 / size))
else
    pct=0
fi

# Silent mode short-circuit: hook-friendly single key=value, no fmt overhead.
if [ "$silent" = "1" ]; then
    printf 'pct=%d\n' "$pct"
    exit 0
fi

# Format with commas + k-shortened pair.
fmt_k() {
    local n=$1
    if [ "$n" -ge 1000 ]; then
        awk "BEGIN {printf \"%.0fk\", $n / 1000}"
    else
        printf "%d" "$n"
    fi
}

used_k=$(fmt_k "$total")
size_k=$(fmt_k "$size")

# Output: same shape as statusline (used/size + %).
printf "%s/%s (%d%%) — input=%s cache_create=%s cache_read=%s output=%s — model=%s\n" \
    "$used_k" "$size_k" "$pct" \
    "$(fmt_k "$input")" "$(fmt_k "$cc")" "$(fmt_k "$cr")" "$(fmt_k "$output")" \
    "$model"
