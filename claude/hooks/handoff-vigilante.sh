#!/usr/bin/env bash
# handoff-vigilante.sh — Stop / SessionEnd / UserPromptSubmit hook. Non-blocking.
#
# Job 2 (emergency snapshot) is live and load-bearing. Job 1 (per-plan handoff
# refresh) was NEUTRALIZED (2026-07-30): the plans/<id>/plan.yml layer it wrote
# never materialized on disk, so its `-d "$PLANS_DIR"` guard made every fire a
# silent skip. Its executable block was removed; the description below is kept
# as the historical record of what it did (original in the .bak beside this
# file — restore it if a real plans/ layer is ever adopted).
#
# 1. Per-plan handoff refresh. For cwds inside a /projects/<slug>/ tree, walk
#    <project_root>/plans/*/plan.yml entries with top-level `status: open` and
#    write/overwrite plans/<plan-id>/handoff.md with a sentinel line + minimal
#    context snapshot derived from plan.yml. The /handoff skill does the full
#    mustache render against handoff.md.template on explicit invocation; this
#    silent baseline keeps the file fresh between explicit calls and lets the
#    skill detect silent-fires by the sentinel.
#
#    Sentinel contract (DRIFT-1 lock — NON-NEGOTIABLE). First line of every
#    refreshed handoff.md MUST be exactly:
#      [wosy-vigilante:mode=silent plan=<plan-id> trigger=<stop|session-end|prompt|ctx60|ctx85>]
#    No leading whitespace, no surrounding markdown. The /handoff skill greps
#    for this prefix verbatim; drift in the format breaks the silent-fire
#    detection contract.
#
# 2. Emergency session snapshot — reaches ANY cwd with a .devwork/ upward.
#    Most real sessions run outside /projects/<slug>/ trees and never produce
#    a plan.yml, so job 1 alone left sessions that died at high context — or
#    ended without a status checkpoint — with nothing recoverable. This path
#    writes <devwork>/_scratch/emergency-handoff-<sid8>-<ts>.md (the always-
#    writable scratch path) from transcript facts alone — zero model calls:
#      Stop       — poll CONTEXT_CHECK_BIN (same default as the context-
#                   checkpoint hook); pct >= 85 → snapshot, once per session
#                   via marker under ~/.claude/state/.
#      SessionEnd — only SUCCESSFUL Write/Edit count (tool_use↔tool_result
#                   correlation excludes denied); none touching */tasks/*/status.md → snapshot.
#    Snapshot fields: trigger, session id, cwd, branch + dirty count, deduped
#    list of files touched via Write/Edit, tail of the last assistant message,
#    transcript path. Sessions killed by API errors fire no hooks at all —
#    that failure mode is unreachable from the hook layer and out of scope.
#
# Trigger resolution.
#   1. $WOSY_VIGILANTE_TRIGGER set by spawner (context-checkpoint.sh sets it
#      to "ctx60" / "ctx85" when spawning at threshold).
#   2. Else derived from the hook event: Stop → "stop", SessionEnd →
#      "session-end", UserPromptSubmit → "prompt".
#   Values outside {stop,session-end,prompt,ctx60,ctx85} sanitize to "stop".
#
# UserPromptSubmit gate.
#   Registered unconditionally on UserPromptSubmit; self-gates on the prompt's
#   first slash-command token being `handoff`. The gated fire only runs the
#   per-plan refresh (job 1) — a prompt being submitted means the session is
#   alive and mid-conversation, not dying, so no emergency path.
#
# Mutual-recursion guard.
#   $WOSY_VIGILANTE_FIRED=1 in env → immediate exit 0. The /handoff skill sets
#   this before its own render so the vigilante does not double-fire and stomp
#   the skill's richer mustache render with the silent baseline.
#
# Reads JSON event from stdin (.cwd, .hook_event_name, .prompt, .session_id,
# .transcript_path). Writes no stdout — silent; artifacts are read directly.
# Always exits 0.

set -uo pipefail

# Mutual-recursion guard.
[[ "${WOSY_VIGILANTE_FIRED:-0}" == "1" ]] && exit 0

command -v jq >/dev/null 2>&1 || exit 0

input="$(cat 2>/dev/null || true)"
event_name="$(printf '%s' "$input" | jq -r '.hook_event_name // "Stop"' 2>/dev/null)"
cwd="$(printf '%s' "$input" | jq -r '.cwd // empty' 2>/dev/null)"
[[ -n "$cwd" ]] || cwd="$PWD"
[[ -d "$cwd" ]] || exit 0

# UserPromptSubmit self-gate — only fire on explicit `/handoff` slash.
if [[ "$event_name" == "UserPromptSubmit" ]]; then
  prompt="$(printf '%s' "$input" | jq -r '.prompt // empty' 2>/dev/null)"
  first_token="$(printf '%s' "$prompt" | sed -nE '1s|^[[:space:]]*/([A-Za-z0-9_-]+).*|\1|p')"
  [[ "$first_token" == "handoff" ]] || exit 0
fi

# Trigger resolution. Spawner-provided env wins; else derive from the hook
# event so SessionEnd snapshots are not mislabeled as plain Stops.
trigger="${WOSY_VIGILANTE_TRIGGER:-}"
if [[ -z "$trigger" ]]; then
  case "$event_name" in
    SessionEnd)       trigger="session-end" ;;
    UserPromptSubmit) trigger="prompt" ;;
    *)                trigger="stop" ;;
  esac
fi
case "$trigger" in
  stop|session-end|prompt|ctx60|ctx85) ;;
  *)                                   trigger="stop" ;;
esac

# ── Job 1: per-plan handoff refresh — NEUTRALIZED 2026-07-30 ──
# Removed. This job refreshed plans/<plan-id>/handoff.md for cwds inside a
# /projects/<slug>/ tree, but that plans/ layer never existed on disk, so the
# `-d "$PLANS_DIR"` guard skipped it on every fire (0 files ever carried its
# sentinel). Dropped so the hook stops reading cwd and walking a dead branch on
# every Stop/SessionEnd/prompt. Full original is in
# handoff-vigilante.sh.bak.20260730-124508 — restore it verbatim if a real
# plans/ layer is ever adopted. Job 2 (emergency snapshot) below is untouched.

# ── Job 2: emergency session snapshot ─────────────────────────────────
# Stop|SessionEnd only — a UserPromptSubmit fire means the session is alive,
# and spawner-relayed ctx60/ctx85 events were already nudged by their spawner.
case "$event_name" in
  Stop|SessionEnd) ;;
  *) exit 0 ;;
esac

# Nearest .devwork/ walking up from the session cwd.
devwork_root=""
_d="${cwd%/}"
while [[ -n "$_d" && "$_d" != "/" ]]; do
  if [[ -d "$_d/.devwork" ]]; then devwork_root="$_d/.devwork"; break; fi
  _d="${_d%/*}"
done
[[ -n "$devwork_root" ]] || exit 0

session_id="$(printf '%s' "$input" | jq -r '.session_id // empty' 2>/dev/null)"
session_id_safe="${session_id//[^a-zA-Z0-9_-]/_}"
transcript_path="$(printf '%s' "$input" | jq -r '.transcript_path // empty' 2>/dev/null)"

# Deduped Write/Edit targets from the transcript (tool_use entries live in
# assistant-message content arrays; `[]?` skips string-content lines).
# Each tool_use.id is correlated with its tool_result: only is_error==true marks
# a denial; absent or non-true is treated as success (ambiguous → counts as touched).
touched=""
if [[ -n "$transcript_path" && -f "$transcript_path" ]]; then
  touched="$(jq -rn '
      reduce (inputs | .message.content[]? | objects) as $c ({u: {}, f: {}};
        if $c.type == "tool_use" and ($c.name == "Write" or $c.name == "Edit")
           and ($c.id | type) == "string"
        then .u[$c.id] = ($c.input.file_path // "")
        elif $c.type == "tool_result" and $c.is_error == true
             and ($c.tool_use_id | type) == "string"
        then .f[$c.tool_use_id] = true
        else . end)
      | . as $s | $s.u | to_entries[]
      | select($s.f[.key] != true)
      | .value | select(. != "")' "$transcript_path" 2>/dev/null \
    | sort -u)"
fi

pct=""
if [[ "$event_name" == "Stop" ]]; then
  # High-context emergency: same poll target as context-checkpoint. Marker
  # checked BEFORE polling so the per-turn Stop overhead after the first
  # snapshot is a single stat. A missing or failing check binary disables
  # this branch entirely — accepted trade-off of "never block, fail silent".
  [[ -n "$session_id" ]] || exit 0
  CHECK_BIN="${CONTEXT_CHECK_BIN:-$HOME/.claude/skills/context/bin/check.sh}"
  [[ -x "$CHECK_BIN" ]] || exit 0
  STATE_DIR="$HOME/.claude/state"
  marker="$STATE_DIR/handoff-vigilante-${session_id_safe}-emergency.marker"
  [[ -f "$marker" ]] && exit 0
  pct_line="$("$CHECK_BIN" --silent 2>/dev/null)" || exit 0
  pct="${pct_line#pct=}"
  pct="${pct%%[^0-9]*}"
  [[ "$pct" =~ ^[0-9]+$ ]] || exit 0
  (( pct >= 85 )) || exit 0
  reason="context at ${pct}% on Stop — past the wrap-up boundary"
else
  # Checkpoint-less death: work happened (Write/Edit recorded) but no task
  # status.md was updated, so nothing on disk says where the session left off.
  [[ -n "$touched" ]] || exit 0
  printf '%s\n' "$touched" | grep -q '/tasks/[^/]*/status\.md$' && exit 0
  reason="session ended with file writes but no tasks/*/status.md checkpoint"
fi

scratch_dir="$devwork_root/_scratch"
mkdir -p "$scratch_dir" 2>/dev/null || exit 0

sid8="$(printf '%s' "${session_id_safe:-unknown}" | cut -c1-8)"
[[ -n "$sid8" ]] || sid8="unknown"
ts="$(date -u +%Y%m%dT%H%M%SZ)"
snapshot="$scratch_dir/emergency-handoff-${sid8}-${ts}.md"

git_branch="$(git -C "$cwd" branch --show-current 2>/dev/null || true)"
git_dirty="$(git -C "$cwd" status --porcelain 2>/dev/null | grep -c . || true)"
git_dirty="${git_dirty:-0}"

# Tail of the last assistant text message — usually holds the freshest
# "where we are" statement.
last_msg=""
if [[ -n "$transcript_path" && -f "$transcript_path" ]]; then
  last_line="$(jq -c 'select(.type=="assistant")
      | select(([.message.content[]? | select(.type=="text") | .text] | join("")) != "")' \
      "$transcript_path" 2>/dev/null | tail -1)"
  if [[ -n "$last_line" ]]; then
    last_msg="$(printf '%s' "$last_line" \
      | jq -r '[.message.content[]? | select(.type=="text") | .text] | join("\n")' 2>/dev/null \
      | tail -40)"
  fi
fi

tmp="$snapshot.tmp.$$"
{
  # Deliberately NOT the DRIFT-1 plan-sentinel shape: no plan= field, because
  # no plan exists here. Consumers must key on mode=, not the bare prefix.
  printf '[wosy-vigilante:mode=emergency trigger=%s]\n\n' "$trigger"
  printf '# Emergency handoff snapshot — %s\n\n' "$ts"
  printf 'Auto-written by handoff-vigilante from transcript facts (no model involved).\n'
  printf 'Reason: %s.\n\n' "$reason"
  printf '**Session:** %s  \n' "${session_id:-—}"
  printf '**Cwd:** %s  \n' "$cwd"
  printf '**Branch:** %s (%s dirty files)  \n' "${git_branch:-—}" "$git_dirty"
  printf '**Transcript:** %s\n\n' "${transcript_path:-—}"
  printf '## Files touched this session (Write/Edit, deduped)\n\n'
  if [[ -n "$touched" ]]; then
    printf '%s\n' "$touched" | sed 's/^/- /'
  else
    printf '_(none recorded)_\n'
  fi
  printf '\n## Tail of last assistant message\n\n'
  if [[ -n "$last_msg" ]]; then
    printf '%s\n' "$last_msg"
  else
    printf '_(no assistant text found in transcript)_\n'
  fi
} > "$tmp" 2>/dev/null && mv "$tmp" "$snapshot" 2>/dev/null
[[ -f "$tmp" ]] && rm -f "$tmp" 2>/dev/null

# Marker only after a successful Stop-path write so a transient write failure
# leaves clean state for the next Stop to retry.
if [[ "$event_name" == "Stop" && -f "$snapshot" ]]; then
  mkdir -p "$STATE_DIR" 2>/dev/null || true
  touch "$marker" 2>/dev/null || true
fi

exit 0
