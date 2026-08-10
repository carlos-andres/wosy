#!/usr/bin/env bash
# md-hardwrap-warn.sh — PreToolUse hook for Write|Edit. ADVISORY (does NOT block).
#
# Enforces the single-line-prose convention the user has corrected 3+ times:
#   "Never write markdown prose with hard wraps at 75-80 chars — paragraphs and
#    bullets are single continuous lines; only tables and fenced code may wrap."
# Detects hard-wrapped prose/bullets in the .md content being written and warns
# via hookSpecificOutput.additionalContext so Claude sees it at write time and
# can re-emit single-line. Fenced code blocks and table rows are exempt (they
# may wrap legitimately).
#
# Promotion path to a hard block: after the warn shows ~a week of zero
# false-positives, replace the advisory emit with a real block via
#   {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",
#    "permissionDecisionReason":"..."}}
# NOT `exit 1` — exit 1 does NOT block; only exit 2 (stderr->Claude) or
# permissionDecision:"deny" actually block.
#
# Reads JSON event from stdin. Always exits 0 (fail-open): jq missing or an
# unparseable payload => no context emitted, tool proceeds as before.

set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0

input="$(cat 2>/dev/null || true)"
tool="$(printf '%s' "$input" | jq -r '.tool_name // empty' 2>/dev/null)"
path="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"

[ -z "$path" ] && exit 0
[ "$tool" != "Write" ] && [ "$tool" != "Edit" ] && exit 0

# Only markdown writes.
case "$path" in
  *.md|*.MD|*.markdown) ;;
  *) exit 0 ;;
esac

# Whitelist files that conventionally hard-wrap (not the user's own prose docs),
# and vendored trees. Keeps the advisory low-noise.
case "$(basename "$path")" in
  CHANGELOG.md|CHANGELOG.MD|LICENSE.md|LICENSE.MD) exit 0 ;;
esac
case "$path" in
  */node_modules/*|*/vendor/*|*/.git/*) exit 0 ;;
esac

# Content being written: Write carries .content, Edit carries .new_string.
content="$(printf '%s' "$input" | jq -r '.tool_input.content // .tool_input.new_string // empty' 2>/dev/null)"
[ -z "$content" ] && exit 0

# Per-project kill switch: wosy_enforce=0 in the nearest .devwork/wosy.flags
# (walking up from the FILE's dir) silences this advisory. Fail-open.
_wosy_lib="${HOME}/.claude/hooks/lib/wosy-flags.sh"
if [ -r "$_wosy_lib" ]; then
  # shellcheck source=/dev/null
  . "$_wosy_lib"
  [ "$(wosy_flag wosy_enforce "$(dirname "$path")")" = "0" ] && exit 0
fi

# Detect hard-wrapped prose. A "wrap event" = a previous eligible prose/bullet
# line that is long (>= THRESH chars) and does not end in an intentional hard
# break, immediately followed by a plain continuation line (not a new block:
# not blank, heading, list item, blockquote, hr, table row, or code fence).
# Fenced code, table rows, and a leading YAML frontmatter block (between the
# first-line `---` and its closing `---`) are exempt as wrap origins. THRESH=64
# reliably catches 75-80-col wraps while sparing deliberately-short separate lines.
result="$(printf '%s\n' "$content" | awk -v THRESH=64 '
  BEGIN { infront=0; infence=0; prev_elig=0; prevlen=0; prevtrail=0; flag=0; firstline=0; ln=0 }
  {
    ln++; line=$0; t=line; sub(/^[ \t]+/,"",t)
    if (ln==1 && line=="---") { infront=1; prev_elig=0; next }
    if (infront) { if (line=="---") infront=0; prev_elig=0; next }
    if (t ~ /^(```|~~~)/) { infence = !infence; prev_elig=0; next }
    if (infence)          { prev_elig=0; next }
    if (line ~ /^[ \t]*$/) { prev_elig=0; next }

    istable = (line ~ /\|/)
    ishead  = (t ~ /^#/)
    isul    = (t ~ /^[-*+][ \t]/)
    isol    = (t ~ /^[0-9]+[.)][ \t]/)
    isquote = (t ~ /^>/)
    ishr    = (t ~ /^(-{3,}|={3,}|\*{3,}|_{3,})[ \t]*$/)
    isblock = (ishead || isul || isol || isquote || ishr || istable)

    if (prev_elig && prevlen >= THRESH && prevtrail == 0 && isblock == 0) {
      flag++; if (firstline == 0) firstline = ln
    }

    if (ishead || ishr || isquote || istable) { prev_elig = 0 }
    else {
      prev_elig = 1; prevlen = length(line)
      prevtrail = (line ~ /(  +$|\\$)/) ? 1 : 0
    }
  }
  END { print flag "|" firstline }
')"

count="${result%%|*}"
firstline="${result##*|}"
[ "${count:-0}" -gt 0 ] 2>/dev/null || exit 0

ctx="Markdown hard-wrap detected in ${path} (${count} wrapped block(s), first near line ${firstline}).

Convention (corrected 3+ times): paragraphs and bullets are SINGLE continuous
lines — do not hard-wrap prose at 75-80 chars. Only tables and fenced code may
span multiple lines. Re-emit this content with each paragraph and each bullet on
one line (soft-wrap in the editor, not hard newlines). If the flagged lines are
intentional (e.g. deliberate line breaks with two trailing spaces), ignore this."

jq -nc --arg ctx "$ctx" \
  '{hookSpecificOutput:{hookEventName:"PreToolUse", additionalContext:$ctx}}' \
  2>/dev/null || true

exit 0
