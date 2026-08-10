#!/usr/bin/env bash
# handoff-check.sh — PostToolUse hook for Edit/Write. Non-blocking.
#
# Handoff files are resume-canonical: the next session's first action is to
# read the paths they cite, so a typo'd or stale path silently poisons that
# session's opening reads. This hook verifies, at the moment a handoff is
# written, that every locally-verifiable absolute path in the new content
# exists, and emits an advisory listing the missing ones.
#
# Scope guards (false-positive containment):
#   - Self-gates on the written file's basename matching *handoff*.md
#     (case-insensitive) — handoffs live wherever the user writes them, so
#     the gate is by name, not by directory.
#   - Only paths under locally-verifiable roots ($HOME, /Users, /Volumes,
#     /tmp) are checked; server/remote paths cited in a handoff cannot be
#     test'ed from this machine and are skipped.
#   - Tokens carrying glob or placeholder characters (* ? < >) describe
#     patterns, not concrete files — skipped as unverifiable.
#
# Advisory only — NEVER blocks: a hard block on the wrap-up artifact would
# strand the session with no way to write its own handoff. No once-per-session
# marker: content changes between writes and the check is cheap, so every
# handoff write is re-checked.
#
# Reads JSON event from stdin (.tool_input.file_path, .tool_input.content for
# Write / .tool_input.new_string for Edit). Always exits 0. Emits stdout per
# the PostToolUse contract:
#   {hookSpecificOutput:{hookEventName:"PostToolUse", additionalContext:<text>}}

set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0

input="$(cat 2>/dev/null || true)"
file_path="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"
[ -n "$file_path" ] || exit 0

# Self-gate: basename matches *handoff*.md, case-insensitive (bash 3.2 has no
# ${var,,} — fold via tr).
base_lc="$(basename "$file_path" | tr '[:upper:]' '[:lower:]')"
case "$base_lc" in
  *handoff*.md) ;;
  *) exit 0 ;;
esac

# Write carries full content; Edit carries the replacement chunk. Either way
# only the NEW text is checked — pre-existing paths were checked when written.
content="$(printf '%s' "$input" | jq -r '.tool_input.content // .tool_input.new_string // empty' 2>/dev/null)"
[ -n "$content" ] || exit 0

# Tokenize on whitespace + markdown/punctuation delimiters so a path embedded
# in `backticks`, (parens), [brackets], "quotes" or a file:line reference comes
# out as a bare token. < > * ? stay INSIDE tokens on purpose: a pattern like
# prefix-<id>.md must survive as one token so the glob/placeholder skip below
# can drop it whole, instead of its truncated prefix surfacing as missing.
candidates="$(printf '%s' "$content" \
  | tr -s '[:space:]`"'"'"'()[],;:' '\n' \
  | grep -E '^~?/[^/]' 2>/dev/null \
  | sed -E 's/\.+$//' \
  | sort -u | head -200)"
[ -n "$candidates" ] || exit 0

missing=""
miss_count=0
# Tokens are whitespace-free by construction, so word-splitting is safe.
# Glob expansion is NOT: a token with [brackets] would silently vanish under
# an inherited nullglob — disable it for the unquoted expansion.
set -f
for p in $candidates; do
  case "$p" in
    *[\*\?\<\>]*) continue ;;
  esac
  case "$p" in
    "~"/*) p="$HOME${p#"~"}" ;;
  esac
  case "$p" in
    "$HOME"/*|/Users/*|/Volumes/*|/tmp/*) ;;
    *) continue ;;
  esac
  if [ ! -e "$p" ]; then
    missing="${missing}  - ${p}
"
    miss_count=$((miss_count + 1))
  fi
done
set +f

[ "$miss_count" -gt 0 ] || exit 0

msg="[handoff-check] ${file_path} references ${miss_count} local path(s) that do not exist:
${missing}A future session will try to read these first. Fix each path in the handoff, or annotate it as unverified if it is intentional (planned output, removed file)."

jq -nc --arg ctx "$msg" \
  '{hookSpecificOutput:{hookEventName:"PostToolUse", additionalContext:$ctx}}' 2>/dev/null

exit 0
