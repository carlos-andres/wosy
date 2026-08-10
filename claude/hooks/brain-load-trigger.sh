#!/usr/bin/env bash
# brain-load-trigger.sh — UserPromptSubmit hook. Non-blocking.
#
# Phase III Wave B-1 (Fork 4 / A-03 §4.2 / C-03 §5.2 trigger #1).
#
# Inject `brain load <slug>` bundle as additionalContext when ANY of:
#   (a) `#contexto <slug>` hashtag in the user prompt.
#   (b) `/brain-load <slug>` slash command at prompt start (defensive belt-and-
#       suspenders for the canonical /brain-load skill path).
#   (c) cwd path contains `/projects/<slug>/` AND no explicit slug from (a)/(b).
#       Outermost match wins on nested project trees (head -1 on the grep) —
#       conscious choice, matches the workspace-rooted resolution model.
#
# Slug character class is `[a-z0-9][a-z0-9-]*` to match the `project.id` JSON-
# Schema pattern `^[a-z0-9][a-z0-9-]{1,63}$` defined in plan.yml.schema.json.
# A slug that doesn't conform won't resolve to a `project` row in brain.db
# anyway, so tighter matching just fails fast.
#
# Idempotency: a session-scoped marker file per (session_id, slug) prevents
# re-injecting the same bundle on every UserPromptSubmit in the same session.
# State dir: ~/.claude/state/ (skill-nudge convention; see skill-invocation-
# nudge.sh). Marker is TOUCHED ONLY ON SUCCESSFUL EMIT — if jq fails the
# next prompt retries (no permanent suppression).
#
# Reads JSON event from stdin (.prompt, .session_id, .cwd). Always exits 0
# (no blocking). Emits stdout JSON per Claude Code UserPromptSubmit contract:
#   {hookSpecificOutput:{hookEventName:"UserPromptSubmit", additionalContext:...}}
#
# Silent exits (no injection, no marker write):
#   - $BRAIN_BIN not executable.
#   - missing prompt / session_id.
#   - no slug matched.
#   - `brain load <slug>` exit non-zero (project not registered, brain.db
#     missing, etc.) — this is best-effort context warming, not enforcement.

set -uo pipefail

BRAIN_BIN="${BRAIN_BIN:-$HOME/.local/bin/brain}"
[[ -x "$BRAIN_BIN" ]] || exit 0

input="$(cat 2>/dev/null || true)"
prompt="$(printf '%s' "$input" | jq -r '.prompt // empty' 2>/dev/null)"
session_id="$(printf '%s' "$input" | jq -r '.session_id // empty' 2>/dev/null)"
cwd="$(printf '%s' "$input" | jq -r '.cwd // empty' 2>/dev/null)"

[[ -n "$session_id" ]] || exit 0

# Sanitize session_id before path interpolation — defense in depth against any
# upstream session-id format change that introduces path separators or shell
# metacharacters. Claude Code controls the format today but the guarantee is
# not contractual at the hook layer.
session_id_safe="${session_id//[^a-zA-Z0-9_-]/_}"

STATE_DIR="$HOME/.claude/state"
mkdir -p "$STATE_DIR" 2>/dev/null || true

slug=""

# Trigger (a): `#contexto <slug>` hashtag. Keyword case-insensitive; slug
# strictly kebab-case (matches project.id schema; uppercase = won't resolve).
if [[ -n "$prompt" && -z "$slug" ]]; then
  slug="$(printf '%s' "$prompt" \
    | grep -oiE '#contexto[[:space:]]+[a-z0-9][a-z0-9-]*' \
    | head -1 \
    | awk '{print $2}')"
fi

# Trigger (b): `/brain-load <slug>` at prompt start (allowing leading whitespace).
if [[ -n "$prompt" && -z "$slug" ]]; then
  slug="$(printf '%s' "$prompt" \
    | sed -nE '1s|^[[:space:]]*/brain-load[[:space:]]+([a-z0-9][a-z0-9-]*).*|\1|p')"
fi

# Trigger (c): cwd path contains `/projects/<slug>/`. First-match wins; nested
# project trees (rare) take the outermost. Only fires if (a)/(b) didn't match.
if [[ -z "$slug" && -n "$cwd" ]]; then
  slug="$(printf '%s' "$cwd" \
    | grep -oE '/projects/[a-z0-9][a-z0-9-]*' \
    | head -1 \
    | sed 's|/projects/||')"
fi

[[ -n "$slug" ]] || exit 0

# Idempotency: one injection per (session, slug). session_id pre-sanitized;
# slug is already constrained by the regex char classes above.
MARKER="$STATE_DIR/brain-load-${session_id_safe}-${slug}.marker"
[[ -f "$MARKER" ]] && exit 0

# Invoke `brain load <slug>`. Non-zero exit → silent (project not in brain.db,
# brain.db not initialized, slug typo). No marker write on failure so a later
# `brain build` + re-prompt will retry.
bundle="$("$BRAIN_BIN" load "$slug" 2>/dev/null)" || exit 0
[[ -n "$bundle" ]] || exit 0

ctx="[brain-load-trigger] Warm-cached project context for slug=\"${slug}\" (C-03 bundle):

\`\`\`json
${bundle}
\`\`\`"

# Emit FIRST, mark on success. If jq fails (missing, stdout closed, etc.), the
# marker is NOT written and the next prompt retries — the retry-friendly path.
if jq -nc --arg ctx "$ctx" \
     '{hookSpecificOutput:{hookEventName:"UserPromptSubmit", additionalContext:$ctx}}' 2>/dev/null
then
  touch "$MARKER" 2>/dev/null || true
fi

exit 0
