#!/usr/bin/env bash
# context-checkpoint.sh — PostToolUse + SubagentStop hook. Non-blocking.
#
# Phase III Wave B-3 (DRIFT-7 lock; A-03 §4.4; C-02 Rule 1 "60%/85%").
#
# Tier-branched context-pressure nudge. Polls `/context --silent` (III-A0
# `~/.claude/skills/context/bin/check.sh --silent`, ships `pct=<N>`), parses
# pct, emits one of three nudges per (session, threshold):
#   pct <60   : no-op
#   60 ≤ pct <85 : nudge ("approaching boundary") + best-effort spawn of
#                  handoff-vigilante.sh to refresh per-plan handoff
#   pct ≥85    : HARD-STOP directive ("wrap up, /handoff, /clear before
#                continuing") + spawn handoff-vigilante.sh
#
# Dual-trigger per DRIFT-7:
#   - PostToolUse with N-counter = 10 (cheap floor for incremental drift).
#   - SubagentStop ALWAYS (a single subagent return can blow context past
#     a threshold in one event; N-counter would miss it).
#
# Idempotency:
#   - N-counter at ~/.claude/state/context-checkpoint-<sess>.counter
#   - per-threshold marker at .../context-checkpoint-<sess>-ctx{60,85}.marker
#     (fires-once-per-threshold-per-session; once ctx85 fires, ctx60 also
#     gets implicitly suppressed by the elif ordering below)
#
# Self-loop guard: the check.sh sub-call is NOT a tool invocation, so it
# does not re-fire PostToolUse. The N-counter increments per HOOK fire,
# not per subprocess. No infinite-loop risk.
#
# Reads JSON event from stdin (.session_id, .hook_event_name). Always exits 0.
# Emits stdout per Claude Code hook contract:
#   {hookSpecificOutput:{hookEventName:<event>, additionalContext:<text>}}

set -uo pipefail

CHECK_BIN="${CONTEXT_CHECK_BIN:-$HOME/.claude/skills/context/bin/check.sh}"
[[ -x "$CHECK_BIN" ]] || exit 0

VIGILANTE_BIN="$HOME/.claude/hooks/handoff-vigilante.sh"

input="$(cat 2>/dev/null || true)"
session_id="$(printf '%s' "$input" | jq -r '.session_id // empty' 2>/dev/null)"
event_name="$(printf '%s' "$input" | jq -r '.hook_event_name // "PostToolUse"' 2>/dev/null)"
[[ -n "$session_id" ]] || exit 0
session_id_safe="${session_id//[^a-zA-Z0-9_-]/_}"

STATE_DIR="$HOME/.claude/state"
mkdir -p "$STATE_DIR" 2>/dev/null || true
COUNTER="$STATE_DIR/context-checkpoint-${session_id_safe}.counter"

# Decide whether THIS fire actually polls check.sh.
should_check=0
case "$event_name" in
  SubagentStop|PostAgent)
    # Always poll on subagent return — one return can blow context past a
    # threshold in a single event.
    should_check=1
    ;;
  *)
    # Default = PostToolUse path. N-counter floor of 10 keeps per-tool
    # overhead negligible while still catching gradual drift.
    n=$(cat "$COUNTER" 2>/dev/null || echo 0)
    n=$((n + 1))
    printf '%d' "$n" > "$COUNTER" 2>/dev/null || true
    if (( n % 10 == 0 )); then
      should_check=1
    fi
    ;;
esac

[[ "$should_check" == "1" ]] || exit 0

# Poll the context-skill. Single line: `pct=N`. Silent fail on any non-zero
# exit (no transcript, no usage record, model unrecognized — see check.sh).
pct_line="$("$CHECK_BIN" --silent 2>/dev/null)" || exit 0
pct="${pct_line#pct=}"
# Strip anything past the digits — defense against trailing newline / whitespace
# from a future check.sh emit; current shipping `printf 'pct=%d\n'` already
# trims via the integer regex below, but the explicit trim is one less
# silent-no-op when the line format drifts.
pct="${pct%%[^0-9]*}"
[[ "$pct" =~ ^[0-9]+$ ]] || exit 0

# emit_nudge — fire-once-per-threshold guard. Spawns handoff-vigilante.sh
# in background if it exists (Wave B-5 lands it; this hook is defensive).
# Marker touched only on successful jq emit (same pattern as B-1).
emit_nudge() {
  local label="$1" body="$2"
  local marker="$STATE_DIR/context-checkpoint-${session_id_safe}-${label}.marker"
  [[ -f "$marker" ]] && return 0

  # Emit FIRST. Marker + side-effects (vigilante spawn) only on successful
  # emit so a transient jq failure leaves clean state for retry — same
  # invariant as brain-load-trigger.sh (B-1 / f7d7a98). Spawning vigilante
  # before the emit would double-fire on retry (vigilante writes handoff.md,
  # a real side-effect).
  if jq -nc --arg ev "$event_name" --arg ctx "$body" \
       '{hookSpecificOutput:{hookEventName:$ev, additionalContext:$ctx}}' \
       2>/dev/null
  then
    touch "$marker" 2>/dev/null || true
    if [[ -x "$VIGILANTE_BIN" ]]; then
      # Backgrounded — pipefail does not propagate through `&`. `disown`
      # detaches the child from the shell job table so the parent's exit 0
      # does not SIGHUP it under `huponexit`. WOSY_VIGILANTE_TRIGGER tells
      # the vigilante which threshold spawned it (B-5 contract; vigilante
      # embeds the value in the DRIFT-1 sentinel line).
      printf '%s' "$input" | WOSY_VIGILANTE_TRIGGER="$label" "$VIGILANTE_BIN" >/dev/null 2>&1 &
      disown 2>/dev/null || true
    fi
  fi
}

if (( pct >= 85 )); then
  emit_nudge "ctx85" "[context-checkpoint] HARD STOP — context at ${pct}%. The next step MUST be to (1) wrap the current micro-task to a clean point (commit if uncommitted; leave nothing half-applied), (2) invoke /handoff to capture session state into the per-plan handoff.md, (3) \`/clear\` before continuing. Do NOT start new work. This nudge fires once per session at the 85% boundary."
elif (( pct >= 60 )); then
  emit_nudge "ctx60" "[context-checkpoint] context at ${pct}% — approaching the boundary. handoff-vigilante is refreshing the per-plan handoff in the background. Finish the current step, then consider a clean checkpoint + /handoff before opening new scope. This nudge fires once per session at the 60% boundary."
fi

exit 0
