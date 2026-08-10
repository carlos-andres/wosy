#!/usr/bin/env bash
# write-location-warn.sh — PreToolUse hook for Write|Edit. ADVISORY (does NOT block).
#
# Per CSI verdict 2026-05-20 action #11 + wosy audit 2026-05-22 P1 #6:
#   [HOOK] Steer .md artifact writes into .devwork/ instead of project root /
#   Desktop / ~/. Originally logged SILENTLY (Claude never saw it at decision
#   time = shelfware); now ALSO injects hookSpecificOutput.additionalContext so
#   the warning is visible to Claude the moment the write is proposed.
#
# Promotion path to a hard block: after the warn log shows ~a week of zero
# false-positives, replace the advisory emit with a real block via
#   {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",
#    "permissionDecisionReason":"..."}}
# NOT `exit 1` — exit 1 does NOT block (tool proceeds); only exit 2 (stderr→Claude)
# or permissionDecision:"deny" actually block.
#
# Reads JSON event from stdin. Always exits 0 (fail-open): if jq is missing or
# the payload is unparseable, no context is emitted and the tool proceeds as before.

set -uo pipefail

# detect-platform lib (XDG vs Library log paths). Fail-open stub keeps prior macOS path.
_dp_lib="${HOME}/.claude/hooks/lib/detect-platform.sh"
if [ -r "$_dp_lib" ]; then . "$_dp_lib"; else log_dir_for() { echo "$HOME/Library/Logs/claude-$1"; }; fi
LOG_DIR="$(log_dir_for write-location)"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/$(date +%Y%m%d).log"

input="$(cat 2>/dev/null || true)"
tool="$(printf '%s' "$input" | jq -r '.tool_name // empty' 2>/dev/null)"
path="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"

[ -z "$path" ] && exit 0
[ "$tool" != "Write" ] && [ "$tool" != "Edit" ] && exit 0

# Only flag markdown writes.
case "$path" in
  *.md|*.MD) ;;
  *) exit 0 ;;
esac

# Whitelist: anything inside a .devwork/ tree is fine.
case "$path" in
  */.devwork/*|*/.archive_devwork/*) exit 0 ;;
esac

# Whitelist: Claude config tree itself.
case "$path" in
  "$HOME"/.claude/*) exit 0 ;;
esac

# Whitelist: standard project root files (legitimate, never generated artifacts).
case "$(basename "$path")" in
  CLAUDE.md|README.md|RTK.md|CHANGELOG.md|LICENSE|LICENSE.md|CONTRIBUTING.md|CODE_OF_CONDUCT.md|SECURITY.md|AUTHORS|AUTHORS.md|NOTICE|NOTICE.md) exit 0 ;;
esac

# Whitelist: wosy-style repo mirrors of ~/.claude/ — same trust as ~/.claude/* itself.
# Trigger: somewhere in the ancestor chain there is a directory whose `claude/`
# subdir contains the canonical agents/ + hooks/ + skills/ deploy structure.
# Catches both the direct case (path is inside claude/{agents,hooks,skills}/)
# AND siblings of that claude/ in the same repo (docs/, audits/, reference-flags/,
# brain/, etc.) so a repo like ~/Documents/GitHub/wosy/ is whitelisted whole.
_dir="$(dirname "$path")"
_depth=0
while [ "$_dir" != "/" ] && [ -n "$_dir" ] && [ "$_depth" -lt 8 ]; do
  if [ -d "$_dir/claude/agents" ] && [ -d "$_dir/claude/hooks" ] && [ -d "$_dir/claude/skills" ]; then
    exit 0
  fi
  _dir="$(dirname "$_dir")"
  _depth=$((_depth + 1))
done

# Whitelist: GitHub repo metadata (issue / PR templates, workflows).
case "$path" in
  */.github/*) exit 0 ;;
esac

# Per-project kill switch: wosy_enforce=0 in the nearest .devwork/wosy.flags
# (walking up from the FILE's dir, not the session cwd) silences this advisory for
# that project. Fail-open: lib missing or no flags => advisory stays on (prior).
_wosy_lib="${HOME}/.claude/hooks/lib/wosy-flags.sh"
if [ -r "$_wosy_lib" ]; then
  # shellcheck source=/dev/null
  . "$_wosy_lib"
  [ "$(wosy_flag wosy_enforce "$(dirname "$path")")" = "0" ] && exit 0
fi

# Anything else: a markdown artifact heading outside .devwork/.
# 1) keep the telemetry log, 2) surface it to Claude via additionalContext.
printf '[%s] tool=%s path=%s\n' "$(date +%Y-%m-%dT%H:%M:%S%z)" "$tool" "$path" >> "$LOG"

ctx="Markdown artifact path is outside .devwork/: ${path}

Per the artifact-location rule, generated markdown (plans, specs, recaps, drafts,
notes) belongs under the nearest .devwork/: real artifacts in
.devwork/{plans,specs,schema,decisions,research,tasks/<id>}/, throwaway/scratch in
.devwork/_scratch/. If no .devwork/ exists up-tree, ask before creating one.
Reconsider this path, or confirm it is intentional. (Whitelisted, never warns:
standard repo-root files like CLAUDE.md/README.md/CHANGELOG.md/LICENSE/
CONTRIBUTING.md/CODE_OF_CONDUCT.md/SECURITY.md, plus ~/.claude/*,
any */.devwork/* path, .github/*, and wosy-style claude/{agents,hooks,skills}/
deploy mirrors.)"

# Advisory only: emit additionalContext, do NOT set permissionDecision (the
# normal permission flow stays intact). Fail-open if jq is unavailable: the
# `|| true` keeps exit status 0 and the tool proceeds with no injected context.
jq -nc --arg ctx "$ctx" \
  '{hookSpecificOutput:{hookEventName:"PreToolUse", additionalContext:$ctx}}' \
  2>/dev/null || true

exit 0
