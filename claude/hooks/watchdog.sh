#!/usr/bin/env bash
# watchdog.sh — PreToolUse shim: route inline DB/ssh + KB-file writes to the brain.
#
# DISABLED BY DEFAULT. Provision-then-enable: install only after `brain` is on PATH
# and the project opts in via .devwork/wosy.flags. Fail-open everywhere.
#
# Contract (Claude Code PreToolUse): reads the event JSON on stdin
# (.tool_name/.tool_input.command/.tool_input.file_path/.cwd); exit 0 = allow,
# exit 2 = BLOCK with the stderr reason fed back to Claude. (Exit 1 is NOT a
# block — Claude Code treats it as advisory and the tool proceeds; see
# write-location-warn.sh:14 for the same convention.) Classification + the flag
# gating + per-class FIX hint all live in the brain binary (single source of
# truth) — this shim is a thin JSON relay.
#
# 2026-05-29 (1.3.2 D-1 closed): the FIX hint moved INTO brain guard's JSON
# response as a `fix_hint` field. The shim no longer has a per-tool case
# statement — it just relays whatever brain guard emitted. If you want to
# change a hint, edit guard.go's block() call sites, not this file.
set -uo pipefail

input="$(cat)"
tool="$(printf '%s' "$input"    | jq -r '.tool_name // empty'            2>/dev/null)"
command="$(printf '%s' "$input" | jq -r '.tool_input.command // empty'   2>/dev/null)"
path="$(printf '%s' "$input"    | jq -r '.tool_input.file_path // empty' 2>/dev/null)"
cwd="$(printf '%s' "$input"     | jq -r '.cwd // empty'                  2>/dev/null)"

BRAIN="${BRAIN_BIN:-$HOME/.local/bin/brain}"
[ -x "$BRAIN" ] || exit 0   # brain missing -> fail-open (prior behavior)

# brain guard exits 2 on BLOCK (a signal, not a failure) — capture stdout regardless.
# 5s timeout: if brain hangs (cold start, blocked I/O), don't stall Claude Code for the
# full 600s PreToolUse budget. timeout exit -> empty stdout -> action defaults to "allow"
# (fail-open) — same shape as the missing-binary case above.
out="$(timeout 5 "$BRAIN" guard --tool="$tool" --command="$command" --path="$path" --cwd="$cwd" 2>/dev/null)"
action="$(printf '%s' "$out" | jq -r '.action // "allow"' 2>/dev/null)"

if [ "$action" = "block" ]; then
  reason="$(printf '%s' "$out"   | jq -r '.reason // "routed"' 2>/dev/null)"
  route="$(printf '%s'  "$out"   | jq -r '.route // empty'     2>/dev/null)"
  fix_hint="$(printf '%s' "$out" | jq -r '.fix_hint // empty'  2>/dev/null)"
  printf 'WATCHDOG: %s%s\n' "$reason" "${route:+ -> $route}" >&2
  [ -n "$fix_hint" ] && printf 'FIX: %s\n' "$fix_hint" >&2
  exit 2
fi
exit 0
