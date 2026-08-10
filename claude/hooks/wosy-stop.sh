#!/usr/bin/env bash
# wosy-stop.sh — Stop hook. Non-blocking session-wrap nudges.
#
# REWRITE of the c0ed684 capability-probe band-aid that worked around the
# removed `brain audit` verb (4f91eba). The _scratch/ accretion check is a
# pure-shell mtime scan in the same shape as the stale-tasks scan — no brain
# dependency, no probe.
#
# What this hook surfaces (all advisory; never blocks):
#   1. stale tasks      — .devwork/tasks/<id>/ folders untouched >$STALE_DAYS
#                          days → nudge /consolidate.
#   2. stale _scratch/  — .md files under .devwork/_scratch/ older than
#                          $STALE_DAYS days → nudge "promote or archive".
#   3. untracked work   — session created persistent .devwork/ files but
#                          touched no tasks/<id>/ folder → nudge to create
#                          the task folder directly (artifacts-first).
#
# What this hook does NOT do (deferred / orthogonal):
#   - encyclopedia_delta + runbook_delta apply: brain-reconcile.sh (d508a79)
#     fires on SessionEnd and consumes the maintain.* surface. No
#     double-application here.
#   - task followups[] surface: task.yml has no followups[] field (only
#     history[], gotchas[], encyclopedia_delta, runbook_delta). Deferred until
#     the schema gains an actionable follow-up surface.
#   - patterns appearing 3+ times: undefined operationally (across what
#     window? what counts as a "pattern"?). Deferred until a queryable
#     definition is locked.
#
# Scan scope (in order):
#   1. The nearest .devwork/ walking up from the session cwd (stdin .cwd).
#   2. $WOSY_SCAN_ROOTS — colon-separated extra roots; explicit opt-in for
#      cross-project sweeps. Each entry is -d-guarded.
# No implicit $HOME-wide fallbacks: scanning trees unrelated to the session
# nags about sibling repos the session cannot act on.
#
# Nudges (1) and (2) print once per session via marker files — Stop fires on
# every turn, and repeating an identical advisory each turn buries it.
#
# Reads JSON event from stdin (.transcript_path, .session_id, .cwd).
# Always exits 0.

set -euo pipefail

# detect-platform lib (BSD vs GNU find/stat for newest-mtime). Fail-open stub
# keeps the prior macOS pipeline.
_dp_lib="${HOME}/.claude/hooks/lib/detect-platform.sh"
if [ -r "$_dp_lib" ]; then . "$_dp_lib"; else
  newest_mtime() { find "$1" -maxdepth 2 -type f -print0 2>/dev/null | xargs -0 stat -f '%m' 2>/dev/null | sort -nr | head -1; }
fi

# Capture stdin once — Stop-hook input JSON.
INPUT="$(cat 2>/dev/null || true)"

STALE_DAYS=7
now=$(date +%s)
stale_threshold=$(( now - STALE_DAYS * 86400 ))

SESSION_ID="$(printf '%s' "$INPUT" | jq -r '.session_id // empty' 2>/dev/null)"
SESSION_CWD="$(printf '%s' "$INPUT" | jq -r '.cwd // empty' 2>/dev/null)"

STATE_DIR="$HOME/.claude/state"
mkdir -p "$STATE_DIR" 2>/dev/null || true
SESSION_ID_SAFE="${SESSION_ID//[^a-zA-Z0-9_-]/_}"

# Markers/counters are per-session and sessions end; reap old ones so state/
# stays flat. Covers this hook's own markers plus the other per-session
# accumulators no one else reaps (brain-load, context-checkpoint counters +
# ctx markers, context-window, handoff-vigilante).
find "$STATE_DIR" -maxdepth 1 \( \
  -name 'wosy-stop-*.marker' -o \
  -name 'retro-intake-*.marker' -o \
  -name 'brain-load-*.marker' -o \
  -name 'context-checkpoint-*' -o \
  -name 'context-window-*' -o \
  -name 'handoff-vigilante-*.marker' \
  \) -mtime +7 -delete 2>/dev/null || true

# once_per_session <check>: true the first time per session+check, false after.
# Call it only when there is something to print, so the marker isn't burned on
# an empty scan. No session_id → no dedup key → always true (fail-open).
once_per_session() {
  [ -n "$SESSION_ID_SAFE" ] || return 0
  local m="$STATE_DIR/wosy-stop-$1-${SESSION_ID_SAFE}.marker"
  [ -f "$m" ] && return 1
  : > "$m"
}

# Nearest .devwork/ walking up from the session cwd (trailing slash stripped
# so the probe paths and devwork_root stay canonical).
devwork_root=""
_d="${SESSION_CWD%/}"
while [ -n "$_d" ] && [ "$_d" != "/" ]; do
  if [ -d "$_d/.devwork" ]; then devwork_root="$_d/.devwork"; break; fi
  _d="${_d%/*}"
done

# handoff-vigilante writes emergency-handoff snapshots into _scratch/ that the
# accretion scan below then nags about; archive (never delete) aged ones to break the loop.
if [ -n "$devwork_root" ]; then
  _emerg_archive="$devwork_root/_archive/emergency-snapshots"
  mkdir -p "$_emerg_archive" 2>/dev/null || true
  find "$devwork_root/_scratch" -maxdepth 1 -type f -name 'emergency-handoff-*.md' -mtime +14 -exec mv {} "$_emerg_archive"/ \; 2>/dev/null || true
fi

IFS=':' read -r -a _extra_roots <<< "${WOSY_SCAN_ROOTS:-}"

# Loops expand the array as ${arr[@]+"${arr[@]}"} — a bare "${arr[@]}" on an
# empty array is an unbound-variable error under set -u on bash <4.4.
list_task_dirs() {
  if [ -n "$devwork_root" ] && [ -d "$devwork_root/tasks" ]; then
    find "$devwork_root/tasks" -mindepth 1 -maxdepth 1 -type d -not -name '_archive' 2>/dev/null
  fi
  for root in ${_extra_roots[@]+"${_extra_roots[@]}"}; do
    [[ -n "$root" && -d "$root" ]] || continue
    find "$root" -maxdepth 5 -type d -path '*/.devwork/tasks/*' -not -path '*/_archive/*' 2>/dev/null
  done
}

list_scratch_dirs() {
  if [ -n "$devwork_root" ] && [ -d "$devwork_root/_scratch" ]; then
    printf '%s\n' "$devwork_root/_scratch"
  fi
  for root in ${_extra_roots[@]+"${_extra_roots[@]}"}; do
    [[ -n "$root" && -d "$root" ]] || continue
    find "$root" -maxdepth 5 -type d -path '*/.devwork/_scratch' -not -path '*/_archive/*' 2>/dev/null
  done
}

# ── (1) stale tasks scan ──────────────────────────────────────────────
stale=()
while IFS= read -r task_dir; do
  [[ -d "$task_dir" ]] || continue
  newest="$(newest_mtime "$task_dir")"
  [[ -n "$newest" ]] || continue
  if (( newest < stale_threshold )); then
    stale+=("$task_dir")
  fi
done < <(list_task_dirs | sort -u)

if (( ${#stale[@]} > 0 )) && once_per_session stale-tasks; then
  echo "wosy: ${#stale[@]} task folder(s) untouched >${STALE_DAYS}d — consider /consolidate:" >&2
  for s in "${stale[@]}"; do
    rel="${s#"$HOME"/}"
    echo "  - $rel" >&2
  done
fi

# ── (2) stale _scratch/ accretion scan ────────────────────────────────
# Pure shell — `brain audit` was removed; this is its replacement. Count .md
# files under each _scratch/ dir older than $STALE_DAYS days (portable
# `-mtime +N` across BSD + GNU find).
scratch_msgs=()
while IFS= read -r scratch_dir; do
  [[ -d "$scratch_dir" ]] || continue
  stale_n=$(find "$scratch_dir" -maxdepth 3 -type f -name '*.md' \
              -mtime +${STALE_DAYS} 2>/dev/null | wc -l | tr -d ' ')
  if (( stale_n > 0 )); then
    rel="${scratch_dir#"$HOME"/}"
    scratch_msgs+=("wosy: _scratch/ accretion at $rel — ${stale_n} .md file(s) >${STALE_DAYS}d (promote to research/runbooks or archive)")
  fi
done < <(list_scratch_dirs | sort -u)

if (( ${#scratch_msgs[@]} > 0 )) && once_per_session stale-scratch; then
  printf '%s\n' "${scratch_msgs[@]}" >&2
fi

# ── (3) untracked-work nudge ──────────────────────────────────────────
# Marker filename keeps the legacy "retro-intake" prefix: the reaper glob
# above and markers already on disk depend on it.
# Detect sessions that produced persistent `.devwork/` artifacts but never
# touched a `.devwork/tasks/<id>/` folder. Once-per-session via marker file.
{
  TRANSCRIPT="$(printf '%s' "$INPUT" | jq -r '.transcript_path // empty' 2>/dev/null)"
  [ -n "$TRANSCRIPT" ] && [ -f "$TRANSCRIPT" ] && [ -n "$SESSION_ID" ] || exit 0

  RETRO_MARKER="$STATE_DIR/retro-intake-${SESSION_ID_SAFE}.marker"
  [ -f "$RETRO_MARKER" ] && exit 0

  # Collect .devwork/ file_paths from Write/Edit tool_use calls this session.
  paths=$(jq -r '
    select(.type=="assistant")
    | .message.content[]?
    | select(.type=="tool_use" and (.name=="Write" or .name=="Edit"))
    | (.input.file_path // empty)
  ' "$TRANSCRIPT" 2>/dev/null | grep -E '/\.devwork/' || true)

  [ -z "$paths" ] && exit 0

  # Classify: in_tasks (tasks/<id>/) vs persistent_outside (anywhere else
  # under .devwork/ except _scratch/).
  in_tasks=$(printf '%s\n' "$paths" | grep -cE '/\.devwork/tasks/[^/]+/' || true)
  persistent_outside=$(printf '%s\n' "$paths" \
    | grep -vE '/\.devwork/_scratch/' \
    | grep -vE '/\.devwork/tasks/' \
    | grep -cE '/\.devwork/' || true)

  if [ "${persistent_outside:-0}" -ge 2 ] && [ "${in_tasks:-0}" -eq 0 ]; then
    echo "wosy: untracked work — session created ${persistent_outside} persistent file(s) under .devwork/ without touching any tasks/<id>/ folder. Create .devwork/tasks/<id>/ with status.md (plus context.md / scope.md if useful) to track this work." >&2
    : > "$RETRO_MARKER"
  fi
} || true

exit 0
