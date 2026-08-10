#!/usr/bin/env bash
# install-claude.sh — provision ~/.claude for the brain / WOSY system.
#
# WHY: the Claude Code agent is classifier-blocked from editing ~/.claude
# (settings.json + hooks/) — the self-modification guard. So the agent builds +
# dry-runs this; YOU run `--apply` to land the changes in your own config home.
#
# GENERAL by design: everything we need in ~/.claude is a COMPONENT. The watchdog
# is just the first; add future needs (more hooks, settings entries, lib deploys)
# as new components without rewriting the harness.
#
# SAFE: no args = DRY RUN (prints the plan, mutates nothing). Every settings write
# is JSON-validated to a temp file and the original backed up (timestamped) before
# an atomic mv. Idempotent (re-run = no-op). Reversible (`--uninstall`).
#
# Usage:
#   ./install-claude.sh                  # dry run — full plan, no changes
#   ./install-claude.sh --apply          # do it (all components) + verify
#   ./install-claude.sh --list           # list components
#   ./install-claude.sh --only=watchdog-hook [--apply]   # one component
#   ./install-claude.sh --skip-provision [--apply]       # ~/.claude only, skip brain-home
#   ./install-claude.sh --uninstall [--apply]            # reverse (keeps brain-home data)
set -euo pipefail
[[ "${BASH_VERSINFO[0]}" -ge 4 ]] || { echo "bash 4+ required (found: $BASH_VERSION). On macOS, install via Homebrew and re-run with: /opt/homebrew/bin/bash $0 $*" >&2; exit 1; }

# ---- config: source artifacts + targets ------------------------------------
# REPO_ROOT: the wosy clone this script lives in. install.sh is at the repo root;
# the canonical hook source is $REPO_ROOT/claude/hooks/watchdog.sh.
# Portability fix 2026-05-29: this used to hardcode ${HOME}/Documents/.devwork/
# which (a) only existed on the author's machine and (b) often held a STALE copy
# of watchdog.sh (the post-1.3.0 FIX-hint patch lived only at the live ~/.claude
# deploy and the repo, not at that DEVWORK staging path). Sourcing from the repo
# guarantees the install matches what was committed.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_WATCHDOG="${REPO_ROOT}/claude/hooks/watchdog.sh"

# DEVWORK: brain-home dogfood target (NOT the source for hooks). Only used by the
# brain-home-target component to provision a personal brain store + wosy.flags
# under ${HOME}/Documents/.devwork. Override via $WOSY_DEVWORK to dogfood elsewhere
# (e.g. $HOME/projects/.devwork on a machine without a Documents/ dir).
DEVWORK="${WOSY_DEVWORK:-${HOME}/Documents/.devwork}"

CLAUDE_DIR="${HOME}/.claude"
HOOKS_DIR="${CLAUDE_DIR}/hooks"
SETTINGS="${CLAUDE_DIR}/settings.json"
SETTINGS_LOCAL="${CLAUDE_DIR}/settings.local.json"
DST_WATCHDOG="${HOOKS_DIR}/watchdog.sh"
HOOK_CMD="${HOOKS_DIR}/watchdog.sh"            # absolute, matches existing hook style
MATCHER="Bash|Write|Edit|MultiEdit"

# Companion hooks added 2026-05-29: dangerous-bash blocker, inline-SQL nudger,
# multi-step-pause UserPromptSubmit hook. Repo-canonical sources; deployed to
# ~/.claude/hooks/ same as watchdog. Matchers registered idempotently.
SRC_VALIDATE_BASH="${REPO_ROOT}/claude/hooks/validate-bash.sh"
SRC_CANONICALIZE_BASH="${REPO_ROOT}/claude/hooks/canonicalize-bash.sh"
SRC_MULTI_STEP_PAUSE="${REPO_ROOT}/claude/hooks/multi-step-pause.sh"
SRC_DETECT_PLATFORM="${REPO_ROOT}/claude/hooks/lib/detect-platform.sh"
DST_VALIDATE_BASH="${HOOKS_DIR}/validate-bash.sh"
DST_CANONICALIZE_BASH="${HOOKS_DIR}/canonicalize-bash.sh"
DST_MULTI_STEP_PAUSE="${HOOKS_DIR}/multi-step-pause.sh"
DST_DETECT_PLATFORM="${HOOKS_DIR}/lib/detect-platform.sh"

# Phase III Wave B hooks (4 NEW + 1 REWRITTEN — wosy-stop existing-file overwrite),
# plus output-telemetry and edit-batch-nudge, adopted after running live unmanaged.
SRC_BRAIN_LOAD_TRIGGER="${REPO_ROOT}/claude/hooks/brain-load-trigger.sh"
SRC_BRAIN_RECONCILE="${REPO_ROOT}/claude/hooks/brain-reconcile.sh"
SRC_CONTEXT_CHECKPOINT="${REPO_ROOT}/claude/hooks/context-checkpoint.sh"
SRC_HANDOFF_VIGILANTE="${REPO_ROOT}/claude/hooks/handoff-vigilante.sh"
SRC_WOSY_STOP="${REPO_ROOT}/claude/hooks/wosy-stop.sh"
SRC_OUTPUT_TELEMETRY="${REPO_ROOT}/claude/hooks/output-telemetry.sh"
SRC_EDIT_BATCH_NUDGE="${REPO_ROOT}/claude/hooks/edit-batch-nudge.sh"
SRC_SESSION_START_CONTEXT="${REPO_ROOT}/claude/hooks/session-start-context.sh"
SRC_HANDOFF_CHECK="${REPO_ROOT}/claude/hooks/handoff-check.sh"
SRC_SESSION_AUDIT="${REPO_ROOT}/claude/hooks/session-audit.sh"
DST_BRAIN_LOAD_TRIGGER="${HOOKS_DIR}/brain-load-trigger.sh"
DST_BRAIN_RECONCILE="${HOOKS_DIR}/brain-reconcile.sh"
DST_CONTEXT_CHECKPOINT="${HOOKS_DIR}/context-checkpoint.sh"
DST_HANDOFF_VIGILANTE="${HOOKS_DIR}/handoff-vigilante.sh"
DST_WOSY_STOP="${HOOKS_DIR}/wosy-stop.sh"
DST_OUTPUT_TELEMETRY="${HOOKS_DIR}/output-telemetry.sh"
DST_EDIT_BATCH_NUDGE="${HOOKS_DIR}/edit-batch-nudge.sh"
DST_SESSION_START_CONTEXT="${HOOKS_DIR}/session-start-context.sh"
DST_HANDOFF_CHECK="${HOOKS_DIR}/handoff-check.sh"
DST_SESSION_AUDIT="${HOOKS_DIR}/session-audit.sh"

# Phase III Wave C — dead hooks (rm from disk + unwire from settings.json).
DEAD_HOOK_FILES=(inject-toc.sh skill-invocation-nudge.sh session-wrap-detect.sh)

# Phase III-A4b — brain CLI binary build target. Default GOBIN=~/.local/bin so
# `go install` lands beside (not at) the GOPATH default. Override BRAIN_GOBIN to
# install elsewhere; comp_brain-binary-build then puts brain there.
BRAIN_SRC_DIR="${REPO_ROOT}/brain"
BRAIN_GOBIN="${BRAIN_GOBIN:-${HOME}/.local/bin}"
BRAIN_BUILD_TARGET="${BRAIN_GOBIN}/brain"

# Phase II.5 + III-A5 — wosy agents. Per-file cp idempotent against repo source.
AGENTS_SRC_DIR="${REPO_ROOT}/claude/agents"
AGENTS_DST_DIR="${CLAUDE_DIR}/agents"

# Phase II Waves A-E — wosy skills. rsync --delete to mirror per skill.
# Local user-added files inside a skill dir are DELETED on sync (the repo is
# canonical); tar-backup before --apply if you have local edits to preserve.
SKILLS_SRC_DIR="${REPO_ROOT}/claude/skills"
SKILLS_DST_DIR="${CLAUDE_DIR}/skills"
# Retired skills: removed from ~/.claude/skills/ by cleanup-retired-skills. Explicit
# list only — the dst dir also holds user-owned skills, so absence-from-repo must
# never be treated as "retired" (a blanket dst sweep would delete user skills).
RETIRED_SKILLS=(intake)
STALE_PERMIT="Bash(BRAIN_BIN=/nonexistent/brain bash hooks/watchdog.sh)"

# Stale dev-time permits accumulated during brain build. Each is either (a) a script
# that no longer exists or (b) a one-off /tmp probe from a single session. The
# install-watchdog.sh entries reference a path on the author's machine (DEVWORK)
# that won't exist on a fresh clone — cleanup is idempotent (jq filter is a no-op
# Idempotent: cleanup-dev-permits is a no-op if the entries are absent. New
# machines will see "no stale dev permits present".
DEAD_PERMITS=(
  "Bash(bash -n ${HOME}/Documents/.devwork/brain/hooks/install-watchdog.sh)"
  "Bash(bash ${HOME}/Documents/.devwork/brain/hooks/install-watchdog.sh)"
  "Bash(sqlite3 /tmp/t.db \".tables\")"
  "Bash(sqlite3 /tmp/t.db \"SELECT value FROM meta WHERE key='schema_version'\")"
  "Bash(sqlite3 /tmp/t.db \"SELECT count\\(*\\) FROM sqlite_master WHERE type='table'\")"
)

STORE_PATH="${DEVWORK}/brain/store.db"
WOSY_FLAGS="${DEVWORK}/wosy.flags"
BRAIN_BIN="${BRAIN_BIN:-$(command -v brain || echo "${HOME}/.local/bin/brain")}"
LOCAL_BIN_DIR="${HOME}/.local/bin"

# wosy-metrics CLI: per-session metrics computed on demand from transcripts.
# Deployed without the .sh suffix so it reads as a command, not a hook.
SRC_WOSY_METRICS="${REPO_ROOT}/claude/bin/wosy-metrics.sh"
WOSY_METRICS_TARGET="${LOCAL_BIN_DIR}/wosy-metrics"

# Component registry — order matters (provision target before arming the hook).
# Phase III ordering rules:
#   - brain-binary-build BEFORE brain-home-target (brain-home calls $BRAIN_BIN init)
#   - cleanup-dead-hooks BEFORE any new hook wires (no orphan settings refs)
#   - Phase III hook + matcher pairs after the legacy 4-hook block
#   - Bulk deploys (agents, skills) after individual hook components
ALL_COMPONENTS=(
  brain-binary-build
  brain-home-target
  detect-platform-lib            platform-precheck
  cleanup-dead-hooks
  watchdog-hook                  watchdog-matcher
  validate-bash-hook             validate-bash-matcher
  canonicalize-bash-hook         canonicalize-bash-matcher
  multi-step-pause-hook          multi-step-pause-matcher
  wosy-stop-hook                 wosy-stop-matcher
  brain-load-trigger-hook        brain-load-trigger-matcher
  brain-reconcile-hook           brain-reconcile-matcher
  context-checkpoint-hook        context-checkpoint-matchers
  handoff-vigilante-hook         handoff-vigilante-matchers
  output-telemetry-hook          output-telemetry-matcher
  edit-batch-nudge-hook          edit-batch-nudge-matcher
  handoff-check-hook             handoff-check-matcher
  session-start-context-hook     session-start-context-matcher
  session-audit-hook             session-audit-matcher
  wosy-agents-deploy
  wosy-skills-deploy             cleanup-retired-skills
  wosy-metrics-deploy
  cleanup-stale-permit           cleanup-dev-permits
)
declare -A COMP_DESC=(
  [brain-binary-build]="build brain CLI + install to ${BRAIN_GOBIN}/brain via GOBIN (Phase III-A4b: 13 functional verbs)"
  [brain-home-target]="brain store + wosy.flags(enforce_kb_scribe) at ~/Documents/.devwork  [NOT ~/.claude]"
  [detect-platform-lib]="copy detect-platform.sh -> ~/.claude/hooks/lib/  (OS / stat / log_dir detection)"
  [platform-precheck]="write detected platform flags into brain-home wosy.flags (idempotent; closes audit D-3 + D-4)"
  [cleanup-dead-hooks]="remove 3 Phase III-killed hooks (inject-toc, skill-invocation-nudge, session-wrap-detect) + unwire from settings.json"
  [watchdog-hook]="copy watchdog.sh -> ~/.claude/hooks/"
  [watchdog-matcher]="register PreToolUse matcher in settings.json"
  [validate-bash-hook]="copy validate-bash.sh -> ~/.claude/hooks/  (hard-block dangerous bash)"
  [validate-bash-matcher]="register PreToolUse Bash matcher for validate-bash.sh"
  [canonicalize-bash-hook]="copy canonicalize-bash.sh -> ~/.claude/hooks/  (nudge/block inline SQL)"
  [canonicalize-bash-matcher]="register PreToolUse Bash matcher for canonicalize-bash.sh"
  [multi-step-pause-hook]="copy multi-step-pause.sh -> ~/.claude/hooks/  (multi-step pause nudge)"
  [multi-step-pause-matcher]="register UserPromptSubmit hook for multi-step-pause.sh"
  [wosy-stop-hook]="copy wosy-stop.sh -> ~/.claude/hooks/  (Phase III B-4 REWRITE; stale-task + _scratch nudges)"
  [wosy-stop-matcher]="register Stop hook for wosy-stop.sh"
  [brain-load-trigger-hook]="copy brain-load-trigger.sh -> ~/.claude/hooks/  (Phase III B-1; warm-cache brain bundle)"
  [brain-load-trigger-matcher]="register UserPromptSubmit hook for brain-load-trigger.sh"
  [brain-reconcile-hook]="copy brain-reconcile.sh -> ~/.claude/hooks/  (Phase III B-2; thin-invoke brain reconcile)"
  [brain-reconcile-matcher]="register SessionEnd hook for brain-reconcile.sh"
  [context-checkpoint-hook]="copy context-checkpoint.sh -> ~/.claude/hooks/  (Phase III B-3; ctx60/85 nudges)"
  [context-checkpoint-matchers]="register PostToolUse + SubagentStop hooks for context-checkpoint.sh (dual-trigger DRIFT-7)"
  [handoff-vigilante-hook]="copy handoff-vigilante.sh -> ~/.claude/hooks/  (Phase III B-5; silent per-plan handoff refresh)"
  [handoff-vigilante-matchers]="register Stop + SessionEnd + UserPromptSubmit hooks for handoff-vigilante.sh"
  [output-telemetry-hook]="copy output-telemetry.sh -> ~/.claude/hooks/  (Stop-hook telemetry: oversized prose / tables / prose SQL)"
  [output-telemetry-matcher]="register Stop hook for output-telemetry.sh"
  [edit-batch-nudge-hook]="copy edit-batch-nudge.sh -> ~/.claude/hooks/  (per-file edit-count nudge: batch sequential single-line edits)"
  [edit-batch-nudge-matcher]="register PostToolUse Edit|Write matcher for edit-batch-nudge.sh"
  [handoff-check-hook]="copy handoff-check.sh -> ~/.claude/hooks/  (advisory: handoff writes citing nonexistent local paths)"
  [handoff-check-matcher]="register PostToolUse Edit|Write matcher for handoff-check.sh"
  [session-start-context-hook]="copy session-start-context.sh -> ~/.claude/hooks/  (hub-resolving context warm-up at SessionStart; closes the cold-session entry gap)"
  [session-start-context-matcher]="register SessionStart hook for session-start-context.sh"
  [session-audit-hook]="copy session-audit.sh -> ~/.claude/hooks/  (SessionEnd scorecard; TSV append removed — wosy-metrics owns per-session metrics)"
  [session-audit-matcher]="register SessionEnd hook for session-audit.sh"
  [wosy-agents-deploy]="copy claude/agents/*.md -> ~/.claude/agents/  (Phase II.5 + III-A5: agent-scribe + agent-brain-builder)"
  [wosy-skills-deploy]="rsync claude/skills/*/ -> ~/.claude/skills/  (repo-canonical; retired skills excluded)"
  [cleanup-retired-skills]="remove retired skill dirs (${RETIRED_SKILLS[*]}) from ~/.claude/skills/  (explicit list; user skills untouched)"
  [wosy-metrics-deploy]="copy claude/bin/wosy-metrics.sh -> ~/.local/bin/wosy-metrics  (per-session metrics CLI; replaces the sessions.tsv hook append)"
  [cleanup-stale-permit]="drop dead BRAIN_BIN=/nonexistent permit from settings.local.json"
  [cleanup-dev-permits]="drop 6 stale dev-time permits (vanished install-watchdog.sh, /tmp probes)"
)

# ---- modes -----------------------------------------------------------------
APPLY=0; VERB=install; SKIP_PROVISION=0; ONLY=""
for a in "$@"; do
  case "$a" in
    --apply)          APPLY=1 ;;
    --uninstall)      VERB=uninstall ;;
    --skip-provision) SKIP_PROVISION=1 ;;
    --only=*)         ONLY="${a#--only=}" ;;
    --list)           printf '%-22s %s\n' "COMPONENT" "DESCRIPTION"
                      for c in "${ALL_COMPONENTS[@]}"; do printf '%-22s %s\n' "$c" "${COMP_DESC[$c]}"; done; exit 0 ;;
    -h|--help)        sed -n '2,30p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "unknown arg: $a (try --help / --list)" >&2; exit 2 ;;
  esac
done
MODE=$([ "$APPLY" = 1 ] && echo APPLY || echo DRY-RUN)

# ---- shared helpers --------------------------------------------------------
say()  { printf '%s\n' "$*"; }
step() { printf '\n\033[1m• %s\033[0m\n' "$*"; }
backup() { cp "$1" "$1.bak.$(date +%Y%m%d-%H%M%S)"; }
do_or_show() {  # $1=desc ; $2.. = cmd (run only under --apply)
  local desc="$1"; shift
  if [ "$APPLY" = 1 ]; then say "  [apply] $desc"; "$@"
  else                     say "  [dry]   would: $desc"; fi
}
write_json_atomic() {  # $1=target ; new JSON on stdin ; validate -> backup -> atomic mv
  local target="$1" tmp; tmp="$(mktemp)"; cat > "$tmp"
  jq -e . "$tmp" >/dev/null 2>&1 || { rm -f "$tmp"; say "  ✗ invalid JSON, refusing to write $target"; return 1; }
  backup "$target"; mv "$tmp" "$target"
}
matcher_present() { jq -e --arg c "$HOOK_CMD" 'any((.hooks.PreToolUse // [])[]; ((.hooks // [])[]?.command) == $c)' "$SETTINGS" >/dev/null 2>&1; }
stale_present()   { [ -f "$SETTINGS_LOCAL" ] && jq -e --arg p "$STALE_PERMIT" '((.permissions.allow // []) | index($p)) != null' "$SETTINGS_LOCAL" >/dev/null 2>&1; }

# ---- components: each is comp_<name> <action>, action ∈ install|uninstall|verify
comp_brain-home-target() {
  case "$1" in
    install)
      if [ -f "$STORE_PATH" ]; then say "  ✓ store exists: $STORE_PATH";
      else do_or_show "brain init store at $STORE_PATH" "$BRAIN_BIN" init "$STORE_PATH"; fi
      if [ -f "$WOSY_FLAGS" ] && grep -q '^enforce_kb_scribe=1' "$WOSY_FLAGS" 2>/dev/null; then
        say "  ✓ enforce_kb_scribe=1 already in $WOSY_FLAGS"
      elif [ -f "$WOSY_FLAGS" ]; then
        do_or_show "append enforce_kb_scribe=1 to $WOSY_FLAGS" \
          bash -c 'printf "\nenforce_kb_scribe=1   # install-claude.sh dogfood\n" >> "$1"' _ "$WOSY_FLAGS"
      else
        do_or_show "create $WOSY_FLAGS (wosy_enforce=1, enforce_kb_scribe=1)" \
          bash -c 'cat > "$1" <<EOF
# wosy enforcement flags — brain-home dogfood (install-claude.sh)
# Absent file = fail-open. Remove this file to disable enforcement here.
wosy_enforce=1
enforce_kb_scribe=1
EOF' _ "$WOSY_FLAGS"
      fi ;;
    uninstall) say "  ↺ data left intact. To disable: rm \"$WOSY_FLAGS\"" ;;
    verify)
      [ -f "$STORE_PATH" ] && say "  store present ............ PASS" || say "  store present ............ (after --apply)"
      grep -q '^enforce_kb_scribe=1' "$WOSY_FLAGS" 2>/dev/null && say "  enforce_kb_scribe=1 ..... PASS" || say "  enforce_kb_scribe=1 ..... (after --apply)" ;;
  esac
}

comp_watchdog-hook() {
  case "$1" in
    install)
      if [ -f "$DST_WATCHDOG" ] && cmp -s "$SRC_WATCHDOG" "$DST_WATCHDOG"; then say "  ✓ installed + identical (no copy)";
      else
        [ -f "$DST_WATCHDOG" ] && say "  ! differs — existing copy will be backed up"
        do_or_show "mkdir -p $HOOKS_DIR" mkdir -p "$HOOKS_DIR"
        { [ -f "$DST_WATCHDOG" ] && do_or_show "backup $DST_WATCHDOG" backup "$DST_WATCHDOG"; } || true
        do_or_show "cp watchdog.sh -> $DST_WATCHDOG" cp "$SRC_WATCHDOG" "$DST_WATCHDOG"
        do_or_show "chmod +x $DST_WATCHDOG" chmod +x "$DST_WATCHDOG"
      fi ;;
    uninstall) [ -f "$DST_WATCHDOG" ] && do_or_show "rm $DST_WATCHDOG" rm -f "$DST_WATCHDOG" || say "  ✓ already absent" ;;
    verify) [ -x "$DST_WATCHDOG" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}

comp_watchdog-matcher() {
  case "$1" in
    install)
      if matcher_present; then say "  ✓ matcher already registered";
      else
        say "  + add: { matcher:\"$MATCHER\", command:\"$HOOK_CMD\" }"
        if [ "$APPLY" = 1 ]; then
          jq --arg cmd "$HOOK_CMD" --arg m "$MATCHER" \
            '.hooks //= {} | .hooks.PreToolUse //= [] | .hooks.PreToolUse += [{matcher:$m, hooks:[{type:"command", command:$cmd}]}]' \
            "$SETTINGS" | write_json_atomic "$SETTINGS" && say "  [apply] matcher added (backup written)"
        else say "  [dry]   settings.json gains 1 matcher (backup first)"; fi
      fi ;;
    uninstall)
      if matcher_present; then
        if [ "$APPLY" = 1 ]; then
          jq --arg c "$HOOK_CMD" '.hooks.PreToolUse |= ( map(.hooks |= map(select(.command != $c))) | map(select((.hooks // []) | length > 0)) )' \
            "$SETTINGS" | write_json_atomic "$SETTINGS" && say "  [apply] matcher removed (backup written)"
        else say "  [dry]   would remove watchdog matcher"; fi
      else say "  ✓ no matcher to remove"; fi ;;
    verify) matcher_present && say "  matcher registered ...... PASS" || say "  matcher registered ...... (after --apply)" ;;
  esac
}

# ---- generic helpers for companion hooks -----------------------------------
# Idempotent file deploy: copy src → dst with backup if dst differs.
deploy_hook() {  # $1=src $2=dst $3=label
  local src="$1" dst="$2" label="$3"
  if [ -f "$dst" ] && cmp -s "$src" "$dst"; then say "  ✓ installed + identical (no copy)"; return; fi
  [ -f "$dst" ] && say "  ! differs — existing copy will be backed up"
  do_or_show "mkdir -p $HOOKS_DIR" mkdir -p "$HOOKS_DIR"
  { [ -f "$dst" ] && do_or_show "backup $dst" backup "$dst"; } || true
  do_or_show "cp $label -> $dst" cp "$src" "$dst"
  do_or_show "chmod +x $dst" chmod +x "$dst"
}

# Idempotent matcher registration. $1=cmd path, $2=event (PreToolUse|UserPromptSubmit),
# $3=matcher string (empty for events without matcher, e.g. UserPromptSubmit).
matcher_present_for() {  # $1=cmd $2=event
  local cmd="$1" evt="$2"
  jq -e --arg c "$cmd" --arg e "$evt" \
    'any((.hooks[$e] // [])[]; ((.hooks // [])[]?.command) == $c)' \
    "$SETTINGS" >/dev/null 2>&1
}
register_hook() {  # $1=cmd $2=event $3=matcher
  local cmd="$1" evt="$2" m="$3"
  if matcher_present_for "$cmd" "$evt"; then say "  ✓ matcher already registered"; return; fi
  say "  + add: { event:\"$evt\"${m:+, matcher:\"$m\"}, command:\"$cmd\" }"
  if [ "$APPLY" = 1 ]; then
    if [ -n "$m" ]; then
      jq --arg cmd "$cmd" --arg m "$m" --arg e "$evt" \
        '.hooks //= {} | .hooks[$e] //= [] | .hooks[$e] += [{matcher:$m, hooks:[{type:"command", command:$cmd}]}]' \
        "$SETTINGS" | write_json_atomic "$SETTINGS" && say "  [apply] matcher added (backup written)"
    else
      jq --arg cmd "$cmd" --arg e "$evt" \
        '.hooks //= {} | .hooks[$e] //= [] | .hooks[$e] += [{hooks:[{type:"command", command:$cmd}]}]' \
        "$SETTINGS" | write_json_atomic "$SETTINGS" && say "  [apply] hook added (backup written)"
    fi
  else say "  [dry]   settings.json gains 1 hook (backup first)"; fi
}
unregister_hook() {  # $1=cmd $2=event
  local cmd="$1" evt="$2"
  if ! matcher_present_for "$cmd" "$evt"; then say "  ✓ no matcher to remove"; return; fi
  if [ "$APPLY" = 1 ]; then
    jq --arg c "$cmd" --arg e "$evt" \
      '.hooks[$e] |= ( map(.hooks |= map(select(.command != $c))) | map(select((.hooks // []) | length > 0)) )' \
      "$SETTINGS" | write_json_atomic "$SETTINGS" && say "  [apply] matcher removed (backup written)"
  else say "  [dry]   would remove $cmd from $evt"; fi
}

comp_validate-bash-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_VALIDATE_BASH" "$DST_VALIDATE_BASH" "validate-bash.sh" ;;
    uninstall) [ -f "$DST_VALIDATE_BASH" ] && do_or_show "rm $DST_VALIDATE_BASH" rm -f "$DST_VALIDATE_BASH" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_VALIDATE_BASH" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_validate-bash-matcher() {
  case "$1" in
    install)   register_hook   "$DST_VALIDATE_BASH" PreToolUse "Bash" ;;
    uninstall) unregister_hook "$DST_VALIDATE_BASH" PreToolUse ;;
    verify)    matcher_present_for "$DST_VALIDATE_BASH" PreToolUse && say "  matcher registered ...... PASS" || say "  matcher registered ...... (after --apply)" ;;
  esac
}
comp_canonicalize-bash-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_CANONICALIZE_BASH" "$DST_CANONICALIZE_BASH" "canonicalize-bash.sh" ;;
    uninstall) [ -f "$DST_CANONICALIZE_BASH" ] && do_or_show "rm $DST_CANONICALIZE_BASH" rm -f "$DST_CANONICALIZE_BASH" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_CANONICALIZE_BASH" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_canonicalize-bash-matcher() {
  case "$1" in
    install)   register_hook   "$DST_CANONICALIZE_BASH" PreToolUse "Bash" ;;
    uninstall) unregister_hook "$DST_CANONICALIZE_BASH" PreToolUse ;;
    verify)    matcher_present_for "$DST_CANONICALIZE_BASH" PreToolUse && say "  matcher registered ...... PASS" || say "  matcher registered ...... (after --apply)" ;;
  esac
}
comp_multi-step-pause-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_MULTI_STEP_PAUSE" "$DST_MULTI_STEP_PAUSE" "multi-step-pause.sh" ;;
    uninstall) [ -f "$DST_MULTI_STEP_PAUSE" ] && do_or_show "rm $DST_MULTI_STEP_PAUSE" rm -f "$DST_MULTI_STEP_PAUSE" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_MULTI_STEP_PAUSE" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_multi-step-pause-matcher() {
  case "$1" in
    install)   register_hook   "$DST_MULTI_STEP_PAUSE" UserPromptSubmit "" ;;
    uninstall) unregister_hook "$DST_MULTI_STEP_PAUSE" UserPromptSubmit ;;
    verify)    matcher_present_for "$DST_MULTI_STEP_PAUSE" UserPromptSubmit && say "  hook registered ......... PASS" || say "  hook registered ......... (after --apply)" ;;
  esac
}

comp_detect-platform-lib() {
  case "$1" in
    install)
      do_or_show "mkdir -p $HOOKS_DIR/lib" mkdir -p "$HOOKS_DIR/lib"
      deploy_hook "$SRC_DETECT_PLATFORM" "$DST_DETECT_PLATFORM" "detect-platform.sh"
      ;;
    uninstall) [ -f "$DST_DETECT_PLATFORM" ] && do_or_show "rm $DST_DETECT_PLATFORM" rm -f "$DST_DETECT_PLATFORM" || say "  ✓ already absent" ;;
    verify)    [ -f "$DST_DETECT_PLATFORM" ] && say "  lib present ............. PASS" || say "  lib present ............. (after --apply)" ;;
  esac
}

# platform-precheck: sources the detect-platform lib, emits a `# platform-detected`
# flag block, and writes it into brain-home WOSY_FLAGS. Idempotent — strips any
# prior block (sed range delete) before appending. Hand-edits OUTSIDE the marker
# block survive. Dry-run prints the block that WOULD be written, indented.
comp_platform-precheck() {
  case "$1" in
    install)
      if [ ! -r "$SRC_DETECT_PLATFORM" ]; then say "  ✗ lib missing at $SRC_DETECT_PLATFORM"; return 1; fi
      # Refuse to operate on a malformed file: a `# platform-detected` start marker
      # without its matching `# end platform-detected` would cause the sed range to
      # delete to EOF, silently wiping content the user appended below the block.
      if [ -f "$WOSY_FLAGS" ] && grep -q '^# platform-detected' "$WOSY_FLAGS" && ! grep -q '^# end platform-detected' "$WOSY_FLAGS"; then
        say "  ✗ $WOSY_FLAGS has '# platform-detected' but no '# end platform-detected' — refusing to operate (would delete to EOF). Fix the file by hand."
        return 1
      fi
      local emit; emit="$(. "$SRC_DETECT_PLATFORM" && precheck_emit_flags)"
      if [ "$APPLY" = 1 ]; then
        [ -f "$WOSY_FLAGS" ] && backup "$WOSY_FLAGS"
        local tmp; tmp="$(mktemp)"
        if [ -f "$WOSY_FLAGS" ]; then
          sed '/^# platform-detected/,/^# end platform-detected/d' "$WOSY_FLAGS" > "$tmp"
        fi
        printf '\n%s\n' "$emit" >> "$tmp"
        mv "$tmp" "$WOSY_FLAGS"
        say "  [apply] platform block written to $WOSY_FLAGS"
      else
        say "  [dry]   would write platform block (idempotent strip+append) to $WOSY_FLAGS:"
        printf '%s\n' "$emit" | sed 's/^/        /'
      fi
      ;;
    uninstall)
      if [ -f "$WOSY_FLAGS" ] && grep -q '^# platform-detected' "$WOSY_FLAGS"; then
        if [ "$APPLY" = 1 ]; then
          backup "$WOSY_FLAGS"
          local tmp; tmp="$(mktemp)"
          sed '/^# platform-detected/,/^# end platform-detected/d' "$WOSY_FLAGS" > "$tmp"
          mv "$tmp" "$WOSY_FLAGS"
          say "  [apply] platform block stripped from $WOSY_FLAGS"
        else
          say "  [dry]   would strip platform block from $WOSY_FLAGS"
        fi
      else
        say "  ✓ no platform block to remove"
      fi
      ;;
    verify)
      if [ -f "$WOSY_FLAGS" ] && grep -q '^# platform-detected' "$WOSY_FLAGS"; then
        say "  platform block present ... PASS"
      else
        say "  platform block present ... (after --apply)"
      fi
      ;;
  esac
}

comp_cleanup-stale-permit() {
  case "$1" in
    install)
      if stale_present; then
        if [ "$APPLY" = 1 ]; then
          jq --arg p "$STALE_PERMIT" '.permissions.allow |= map(select(. != $p))' \
            "$SETTINGS_LOCAL" | write_json_atomic "$SETTINGS_LOCAL" && say "  [apply] stale permit removed (backup written)"
        else say "  [dry]   would remove dead permit: $STALE_PERMIT"; fi
      else say "  ✓ no stale permit present"; fi ;;
    uninstall) say "  ↺ cleanup is not reversed (it only removed dead config)" ;;
    verify)
      if ! stale_present; then say "  stale permit gone ....... PASS"
      elif [ "$APPLY" = 1 ]; then say "  stale permit gone ....... FAIL (still present)"
      else say "  stale permit gone ....... (after --apply)"; fi ;;
  esac
}

# Counts dead-permit entries currently in settings.local.json. Iterates DEAD_PERMITS
# via jq --args so escaping (\(*\) etc.) is preserved literally. jq syntax: --args
# must be the LAST flag before positional args — it acts as the separator, NOT a
# regular boolean flag (otherwise filter+file get treated as positionals too).
dead_permits_present() {
  [ -f "$SETTINGS_LOCAL" ] || { echo 0; return; }
  jq '[.permissions.allow // [] | .[] | select(IN($ARGS.positional[]))] | length' \
    "$SETTINGS_LOCAL" --args "${DEAD_PERMITS[@]}" 2>/dev/null || echo 0
}

comp_cleanup-dev-permits() {
  case "$1" in
    install)
      local n; n="$(dead_permits_present)"
      if [ "${n:-0}" -gt 0 ]; then
        if [ "$APPLY" = 1 ]; then
          jq '.permissions.allow |= map(select(IN($ARGS.positional[]) | not))' \
            "$SETTINGS_LOCAL" --args "${DEAD_PERMITS[@]}" \
            | write_json_atomic "$SETTINGS_LOCAL" \
            && say "  [apply] removed $n stale dev permit(s) (backup written)"
        else
          say "  [dry]   would remove $n stale dev permit(s):"
          for p in "${DEAD_PERMITS[@]}"; do say "          - $p"; done
        fi
      else say "  ✓ no stale dev permits present"; fi ;;
    uninstall) say "  ↺ cleanup is not reversed (it only removed dead config)" ;;
    verify)
      local n; n="$(dead_permits_present)"
      if [ "${n:-0}" = 0 ]; then say "  dev permits gone ......... PASS"
      elif [ "$APPLY" = 1 ]; then say "  dev permits gone ......... FAIL ($n still present)"
      else say "  dev permits gone ......... (after --apply)"; fi ;;
  esac
}

# ============================================================================
# Phase III components (added Wave D extension).
# ----------------------------------------------------------------------------
# Brain binary build: build the brain CLI from ${BRAIN_SRC_DIR} via go install
# with GOBIN=${BRAIN_GOBIN} so the binary lands at ${BRAIN_BUILD_TARGET} (not at
# the GOPATH default of $GOPATH/bin/). Fork 4 lock = 13 functional verbs.
# Idempotency: skips build only if the existing binary's `brain help` output
# reports the post-A4b surface — the reduced verb table after the doctor/audit/
# find/append/detect/gate/selftest/show verbs were killed — AND the binary is newer than
# every build input under brain/ — the surface check alone can't see Go source
# changes that keep the verb table identical. Build error → non-fatal
# (returns 0); operator sees the error and can re-run --only=brain-binary-build.
brain_binary_post_a4b() {  # exit 0 = post-A4b (no killed verbs in help)
  [ -x "$1" ] || return 1
  # Capture help separately so a non-zero exit from `brain help` (very old
  # binaries where the subcommand doesn't exist) returns 1 rather than
  # falsely passing the "no killed verbs found" grep miss.
  local out; out="$("$1" help 2>&1)" || return 1
  ! printf '%s' "$out" | grep -qE '^\s+(doctor|audit|find|append|detect|gate|selftest|show)\s'
}
brain_binary_fresh() {  # exit 0 = binary mtime >= every *.go/go.mod/go.sum under brain/
  [ -e "$1" ] || return 1
  local newer
  newer="$(find "$BRAIN_SRC_DIR" \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) -newer "$1" 2>/dev/null | head -1)"
  [ -z "$newer" ]
}
# A `brain` earlier in PATH than the build target (typically ~/go/bin from a
# manual `go install`) silently shadows every rebuild this script lands — the
# operator keeps running stale code. Warn only when the PATH winner is a
# different file with different content; identical copies or no hit stay silent.
brain_path_shadow_warn() {
  local resolved
  resolved="$(command -v brain 2>/dev/null || true)"
  [ -n "$resolved" ] || return 0
  [ "$resolved" = "$BRAIN_BUILD_TARGET" ] && return 0
  [ -f "$BRAIN_BUILD_TARGET" ] || return 0
  cmp -s "$resolved" "$BRAIN_BUILD_TARGET" && return 0
  say "  ! PATH shadow: 'brain' resolves to $resolved — not the install target $BRAIN_BUILD_TARGET, and contents differ"
  say "    fix: rm \"$resolved\"  (stale copy), or reorder PATH so $BRAIN_GOBIN comes first"
}

comp_brain-binary-build() {
  case "$1" in
    install)
      if ! command -v go >/dev/null 2>&1; then
        say "  ✗ go not in PATH — skip; install Go ≥1.21 and re-run --only=brain-binary-build --apply"
        return 0
      fi
      if [ ! -d "$BRAIN_SRC_DIR" ]; then say "  ✗ brain source missing at $BRAIN_SRC_DIR"; return 0; fi
      if brain_binary_post_a4b "$BRAIN_BUILD_TARGET"; then
        if brain_binary_fresh "$BRAIN_BUILD_TARGET"; then
          say "  ✓ brain already at post-A4b surface and newer than brain/ source"
          return 0
        fi
        say "  ↺ brain/ source newer than binary — rebuilding despite post-A4b surface"
      fi
      local cur_ver=""
      [ -x "$BRAIN_BUILD_TARGET" ] && cur_ver="$("$BRAIN_BUILD_TARGET" version 2>&1 | head -1)"
      if [ "$APPLY" = 1 ]; then
        mkdir -p "$BRAIN_GOBIN"
        if (cd "$BRAIN_SRC_DIR" && GOBIN="$BRAIN_GOBIN" go install ./cmd/brain); then
          local new_ver; new_ver="$("$BRAIN_BUILD_TARGET" version 2>&1 | head -1)"
          say "  [apply] built brain -> $BRAIN_BUILD_TARGET"
          say "          before: ${cur_ver:-<absent>}"
          say "          after:  $new_ver"
        else
          say "  ✗ go install FAILED — leaving existing binary in place"
          return 0
        fi
      else
        say "  [dry]   would: cd $BRAIN_SRC_DIR && GOBIN=$BRAIN_GOBIN go install ./cmd/brain"
        [ -n "$cur_ver" ] && say "  [dry]   current binary: $cur_ver"
      fi ;;
    uninstall) [ -f "$BRAIN_BUILD_TARGET" ] && do_or_show "rm $BRAIN_BUILD_TARGET" rm -f "$BRAIN_BUILD_TARGET" || say "  ✓ already absent" ;;
    verify)
      if brain_binary_post_a4b "$BRAIN_BUILD_TARGET"; then
        say "  brain post-A4b ........... PASS"
      elif [ -x "$BRAIN_BUILD_TARGET" ]; then
        say "  brain post-A4b ........... FAIL (killed verbs still present)"
      else
        say "  brain post-A4b ........... (after --apply)"
      fi
      brain_path_shadow_warn ;;
  esac
}

# ----------------------------------------------------------------------------
# Cleanup-dead-hooks: remove 3 Phase III-killed hooks from ~/.claude/hooks/
# AND unwire them from settings.json across ALL event types. Idempotent:
# safe re-run reports "already clean". Sequenced BEFORE new hook installs so
# settings.json never holds an orphan dead-hook ref alongside new wires.
comp_cleanup-dead-hooks() {
  local events=(PreToolUse PostToolUse SubagentStop Stop SessionEnd SessionStart UserPromptSubmit)
  case "$1" in
    install)
      local removed=0 unwired=0
      for dead in "${DEAD_HOOK_FILES[@]}"; do
        local dst="$HOOKS_DIR/$dead"
        if [ -f "$dst" ]; then
          do_or_show "rm $dst" rm -f "$dst"
          removed=$((removed + 1))
        fi
        for evt in "${events[@]}"; do
          if matcher_present_for "$dst" "$evt"; then
            unregister_hook "$dst" "$evt"
            unwired=$((unwired + 1))
          fi
        done
      done
      if [ "$removed" = 0 ] && [ "$unwired" = 0 ]; then
        say "  ✓ already clean (no dead hooks on disk or in settings.json)"
      else
        say "  cleanup: removed=$removed unwired=$unwired"
      fi ;;
    uninstall)
      say "  ↺ cleanup is not reversed (Phase III killed these hooks intentionally)" ;;
    verify)
      local leftover=0 still_wired=0
      for dead in "${DEAD_HOOK_FILES[@]}"; do
        local dst="$HOOKS_DIR/$dead"
        [ -f "$dst" ] && leftover=$((leftover + 1))
        for evt in "${events[@]}"; do
          matcher_present_for "$dst" "$evt" && still_wired=$((still_wired + 1))
        done
      done
      if [ "$leftover" = 0 ] && [ "$still_wired" = 0 ]; then
        say "  dead hooks gone .......... PASS"
      else
        say "  dead hooks gone .......... FAIL (leftover=$leftover wired=$still_wired)"
      fi ;;
  esac
}

# ----------------------------------------------------------------------------
# Phase III Wave B hooks — 5 hook components + 5 matcher components.
# Each hook component uses the existing deploy_hook helper. Matchers use
# register_hook/unregister_hook from the companion-hooks block above. The
# context-checkpoint and handoff-vigilante hooks fire on MULTIPLE events;
# their matcher components register all events as a single unit.
comp_wosy-stop-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_WOSY_STOP" "$DST_WOSY_STOP" "wosy-stop.sh" ;;
    uninstall) [ -f "$DST_WOSY_STOP" ] && do_or_show "rm $DST_WOSY_STOP" rm -f "$DST_WOSY_STOP" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_WOSY_STOP" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_wosy-stop-matcher() {
  case "$1" in
    install)   register_hook   "$DST_WOSY_STOP" Stop "" ;;
    uninstall) unregister_hook "$DST_WOSY_STOP" Stop ;;
    verify)    matcher_present_for "$DST_WOSY_STOP" Stop && say "  hook registered ......... PASS" || say "  hook registered ......... (after --apply)" ;;
  esac
}

comp_output-telemetry-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_OUTPUT_TELEMETRY" "$DST_OUTPUT_TELEMETRY" "output-telemetry.sh" ;;
    uninstall) [ -f "$DST_OUTPUT_TELEMETRY" ] && do_or_show "rm $DST_OUTPUT_TELEMETRY" rm -f "$DST_OUTPUT_TELEMETRY" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_OUTPUT_TELEMETRY" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_output-telemetry-matcher() {
  case "$1" in
    install)   register_hook   "$DST_OUTPUT_TELEMETRY" Stop "" ;;
    uninstall) unregister_hook "$DST_OUTPUT_TELEMETRY" Stop ;;
    verify)    matcher_present_for "$DST_OUTPUT_TELEMETRY" Stop && say "  hook registered ......... PASS" || say "  hook registered ......... (after --apply)" ;;
  esac
}

comp_edit-batch-nudge-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_EDIT_BATCH_NUDGE" "$DST_EDIT_BATCH_NUDGE" "edit-batch-nudge.sh" ;;
    uninstall) [ -f "$DST_EDIT_BATCH_NUDGE" ] && do_or_show "rm $DST_EDIT_BATCH_NUDGE" rm -f "$DST_EDIT_BATCH_NUDGE" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_EDIT_BATCH_NUDGE" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_edit-batch-nudge-matcher() {
  case "$1" in
    install)   register_hook   "$DST_EDIT_BATCH_NUDGE" PostToolUse "Edit|Write" ;;
    uninstall) unregister_hook "$DST_EDIT_BATCH_NUDGE" PostToolUse ;;
    verify)    matcher_present_for "$DST_EDIT_BATCH_NUDGE" PostToolUse && say "  hook registered ......... PASS" || say "  hook registered ......... (after --apply)" ;;
  esac
}

comp_handoff-check-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_HANDOFF_CHECK" "$DST_HANDOFF_CHECK" "handoff-check.sh" ;;
    uninstall) [ -f "$DST_HANDOFF_CHECK" ] && do_or_show "rm $DST_HANDOFF_CHECK" rm -f "$DST_HANDOFF_CHECK" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_HANDOFF_CHECK" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_handoff-check-matcher() {
  case "$1" in
    install)   register_hook   "$DST_HANDOFF_CHECK" PostToolUse "Edit|Write" ;;
    uninstall) unregister_hook "$DST_HANDOFF_CHECK" PostToolUse ;;
    verify)    matcher_present_for "$DST_HANDOFF_CHECK" PostToolUse && say "  hook registered ......... PASS" || say "  hook registered ......... (after --apply)" ;;
  esac
}

comp_session-start-context-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_SESSION_START_CONTEXT" "$DST_SESSION_START_CONTEXT" "session-start-context.sh" ;;
    uninstall) [ -f "$DST_SESSION_START_CONTEXT" ] && do_or_show "rm $DST_SESSION_START_CONTEXT" rm -f "$DST_SESSION_START_CONTEXT" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_SESSION_START_CONTEXT" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_session-start-context-matcher() {
  case "$1" in
    install)   register_hook   "$DST_SESSION_START_CONTEXT" SessionStart "" ;;
    uninstall) unregister_hook "$DST_SESSION_START_CONTEXT" SessionStart ;;
    verify)    matcher_present_for "$DST_SESSION_START_CONTEXT" SessionStart && say "  hook registered ......... PASS" || say "  hook registered ......... (after --apply)" ;;
  esac
}

comp_session-audit-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_SESSION_AUDIT" "$DST_SESSION_AUDIT" "session-audit.sh" ;;
    uninstall) [ -f "$DST_SESSION_AUDIT" ] && do_or_show "rm $DST_SESSION_AUDIT" rm -f "$DST_SESSION_AUDIT" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_SESSION_AUDIT" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_session-audit-matcher() {
  case "$1" in
    install)   register_hook   "$DST_SESSION_AUDIT" SessionEnd "" ;;
    uninstall) unregister_hook "$DST_SESSION_AUDIT" SessionEnd ;;
    verify)    matcher_present_for "$DST_SESSION_AUDIT" SessionEnd && say "  hook registered ......... PASS" || say "  hook registered ......... (after --apply)" ;;
  esac
}

comp_brain-load-trigger-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_BRAIN_LOAD_TRIGGER" "$DST_BRAIN_LOAD_TRIGGER" "brain-load-trigger.sh" ;;
    uninstall) [ -f "$DST_BRAIN_LOAD_TRIGGER" ] && do_or_show "rm $DST_BRAIN_LOAD_TRIGGER" rm -f "$DST_BRAIN_LOAD_TRIGGER" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_BRAIN_LOAD_TRIGGER" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_brain-load-trigger-matcher() {
  case "$1" in
    install)   register_hook   "$DST_BRAIN_LOAD_TRIGGER" UserPromptSubmit "" ;;
    uninstall) unregister_hook "$DST_BRAIN_LOAD_TRIGGER" UserPromptSubmit ;;
    verify)    matcher_present_for "$DST_BRAIN_LOAD_TRIGGER" UserPromptSubmit && say "  hook registered ......... PASS" || say "  hook registered ......... (after --apply)" ;;
  esac
}

comp_brain-reconcile-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_BRAIN_RECONCILE" "$DST_BRAIN_RECONCILE" "brain-reconcile.sh" ;;
    uninstall) [ -f "$DST_BRAIN_RECONCILE" ] && do_or_show "rm $DST_BRAIN_RECONCILE" rm -f "$DST_BRAIN_RECONCILE" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_BRAIN_RECONCILE" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_brain-reconcile-matcher() {
  case "$1" in
    install)   register_hook   "$DST_BRAIN_RECONCILE" SessionEnd "" ;;
    uninstall) unregister_hook "$DST_BRAIN_RECONCILE" SessionEnd ;;
    verify)    matcher_present_for "$DST_BRAIN_RECONCILE" SessionEnd && say "  hook registered ......... PASS" || say "  hook registered ......... (after --apply)" ;;
  esac
}

comp_context-checkpoint-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_CONTEXT_CHECKPOINT" "$DST_CONTEXT_CHECKPOINT" "context-checkpoint.sh" ;;
    uninstall) [ -f "$DST_CONTEXT_CHECKPOINT" ] && do_or_show "rm $DST_CONTEXT_CHECKPOINT" rm -f "$DST_CONTEXT_CHECKPOINT" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_CONTEXT_CHECKPOINT" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_context-checkpoint-matchers() {
  case "$1" in
    install)
      register_hook "$DST_CONTEXT_CHECKPOINT" PostToolUse ""
      register_hook "$DST_CONTEXT_CHECKPOINT" SubagentStop "" ;;
    uninstall)
      unregister_hook "$DST_CONTEXT_CHECKPOINT" PostToolUse
      unregister_hook "$DST_CONTEXT_CHECKPOINT" SubagentStop ;;
    verify)
      matcher_present_for "$DST_CONTEXT_CHECKPOINT" PostToolUse  && say "  PostToolUse hook ......... PASS" || say "  PostToolUse hook ......... (after --apply)"
      matcher_present_for "$DST_CONTEXT_CHECKPOINT" SubagentStop && say "  SubagentStop hook ........ PASS" || say "  SubagentStop hook ........ (after --apply)" ;;
  esac
}

comp_handoff-vigilante-hook() {
  case "$1" in
    install)   deploy_hook "$SRC_HANDOFF_VIGILANTE" "$DST_HANDOFF_VIGILANTE" "handoff-vigilante.sh" ;;
    uninstall) [ -f "$DST_HANDOFF_VIGILANTE" ] && do_or_show "rm $DST_HANDOFF_VIGILANTE" rm -f "$DST_HANDOFF_VIGILANTE" || say "  ✓ already absent" ;;
    verify)    [ -x "$DST_HANDOFF_VIGILANTE" ] && say "  hook executable ......... PASS" || say "  hook executable ......... (after --apply)" ;;
  esac
}
comp_handoff-vigilante-matchers() {
  case "$1" in
    install)
      register_hook "$DST_HANDOFF_VIGILANTE" Stop ""
      register_hook "$DST_HANDOFF_VIGILANTE" SessionEnd ""
      register_hook "$DST_HANDOFF_VIGILANTE" UserPromptSubmit "" ;;
    uninstall)
      unregister_hook "$DST_HANDOFF_VIGILANTE" Stop
      unregister_hook "$DST_HANDOFF_VIGILANTE" SessionEnd
      unregister_hook "$DST_HANDOFF_VIGILANTE" UserPromptSubmit ;;
    verify)
      matcher_present_for "$DST_HANDOFF_VIGILANTE" Stop             && say "  Stop hook ................ PASS" || say "  Stop hook ................ (after --apply)"
      matcher_present_for "$DST_HANDOFF_VIGILANTE" SessionEnd       && say "  SessionEnd hook .......... PASS" || say "  SessionEnd hook .......... (after --apply)"
      matcher_present_for "$DST_HANDOFF_VIGILANTE" UserPromptSubmit && say "  UserPromptSubmit hook .... PASS" || say "  UserPromptSubmit hook .... (after --apply)" ;;
  esac
}

# ----------------------------------------------------------------------------
# Wosy agents bulk-deploy. Per-file cp; backup-on-differs. Repo is canonical.
# WARN: existing ~/.claude/agents/<name>.md NOT in the repo is left untouched
# (one-way sync TO ~/.claude, not a mirror).
comp_wosy-agents-deploy() {
  case "$1" in
    install)
      if [ ! -d "$AGENTS_SRC_DIR" ]; then say "  ✗ source missing: $AGENTS_SRC_DIR"; return 0; fi
      do_or_show "mkdir -p $AGENTS_DST_DIR" mkdir -p "$AGENTS_DST_DIR"
      local synced=0 skipped=0
      for src in "$AGENTS_SRC_DIR"/*.md; do
        [ -f "$src" ] || continue
        local bn dst
        bn=$(basename "$src")
        dst="$AGENTS_DST_DIR/$bn"
        if [ -f "$dst" ] && cmp -s "$src" "$dst"; then
          skipped=$((skipped + 1))
        else
          { [ -f "$dst" ] && do_or_show "backup $dst" backup "$dst"; } || true
          do_or_show "cp $bn -> agents/" cp "$src" "$dst"
          synced=$((synced + 1))
        fi
      done
      say "  agents: synced=$synced skipped-identical=$skipped" ;;
    uninstall)
      for src in "$AGENTS_SRC_DIR"/*.md; do
        [ -f "$src" ] || continue
        local dst="$AGENTS_DST_DIR/$(basename "$src")"
        [ -f "$dst" ] && do_or_show "rm $dst" rm -f "$dst"
      done ;;
    verify)
      local count=0 drift=0
      for src in "$AGENTS_SRC_DIR"/*.md; do
        [ -f "$src" ] || continue
        count=$((count + 1))
        local dst="$AGENTS_DST_DIR/$(basename "$src")"
        { [ -f "$dst" ] && cmp -s "$src" "$dst"; } || drift=$((drift + 1))
      done
      if [ "$drift" = 0 ]; then say "  agents synced ............ PASS ($count/$count)"
      else say "  agents synced ............ FAIL ($drift/$count drifted)"; fi ;;
  esac
}

# ----------------------------------------------------------------------------
# Wosy skills bulk-deploy. Per-skill rsync --delete (one-way sync TO ~/.claude;
# any local file inside an existing skill dir is DELETED on sync — repo wins).
# Compares dir-content sha256 to short-circuit identical dirs. Pre-existing
# skills in ~/.claude/skills/ NOT in the repo are left untouched. RETIRED_SKILLS
# are skipped even if still present in the repo (cleanup-retired-skills owns them).
is_retired_skill() { local n; for n in "${RETIRED_SKILLS[@]}"; do [ "$n" = "$1" ] && return 0; done; return 1; }
skill_dir_hash()   { (cd "$1" && find . -type f -exec shasum -a 256 {} \; 2>/dev/null | sort | shasum -a 256 | awk '{print $1}'); }
comp_wosy-skills-deploy() {
  case "$1" in
    install)
      if ! command -v rsync >/dev/null 2>&1; then say "  ✗ rsync missing — install rsync and re-run"; return 0; fi
      if [ ! -d "$SKILLS_SRC_DIR" ]; then say "  ✗ source missing: $SKILLS_SRC_DIR"; return 0; fi
      do_or_show "mkdir -p $SKILLS_DST_DIR" mkdir -p "$SKILLS_DST_DIR"
      local synced=0 skipped=0
      for src_dir in "$SKILLS_SRC_DIR"/*/; do
        [ -d "$src_dir" ] || continue
        local bn dst src_h dst_h
        bn=$(basename "$src_dir")
        is_retired_skill "$bn" && continue
        dst="$SKILLS_DST_DIR/$bn"
        src_h=$(skill_dir_hash "$src_dir")
        dst_h=""; [ -d "$dst" ] && dst_h=$(skill_dir_hash "$dst")
        if [ "$src_h" = "$dst_h" ]; then
          skipped=$((skipped + 1))
        else
          do_or_show "rsync --delete $bn/" rsync -a --delete "$src_dir" "$dst/"
          synced=$((synced + 1))
        fi
      done
      say "  skills: synced=$synced skipped-identical=$skipped" ;;
    uninstall)
      for src_dir in "$SKILLS_SRC_DIR"/*/; do
        [ -d "$src_dir" ] || continue
        local bn; bn=$(basename "$src_dir")
        is_retired_skill "$bn" && continue
        local dst="$SKILLS_DST_DIR/$bn"
        [ -d "$dst" ] && do_or_show "rm -rf $dst" rm -rf "$dst"
      done ;;
    verify)
      local count=0 ok_names=() missing=() drifted=()
      for src_dir in "$SKILLS_SRC_DIR"/*/; do
        [ -d "$src_dir" ] || continue
        local bn dst
        bn=$(basename "$src_dir")
        is_retired_skill "$bn" && continue
        count=$((count + 1))
        dst="$SKILLS_DST_DIR/$bn"
        if [ ! -d "$dst" ]; then missing+=("$bn"); continue; fi
        if [ "$(skill_dir_hash "$src_dir")" = "$(skill_dir_hash "$dst")" ]; then ok_names+=("$bn")
        else drifted+=("$bn"); fi
      done
      if [ "${#missing[@]}" = 0 ] && [ "${#drifted[@]}" = 0 ]; then
        say "  skills synced ............ PASS ($count/$count: ${ok_names[*]-})"
      else
        local detail=""
        [ "${#missing[@]}" -gt 0 ] && detail="missing: ${missing[*]}"
        [ "${#drifted[@]}" -gt 0 ] && detail="${detail:+$detail; }drifted: ${drifted[*]}"
        say "  skills synced ............ FAIL — $detail"
      fi ;;
  esac
}

# ----------------------------------------------------------------------------
# Cleanup-retired-skills: remove RETIRED_SKILLS dirs from ~/.claude/skills/.
# Mirrors cleanup-dead-hooks: explicit names only, idempotent, not reversed on
# uninstall. Never sweeps the dst dir — it also holds user-owned skills.

# An empty / "." / ".." / slash-bearing entry in RETIRED_SKILLS would make
# "$SKILLS_DST_DIR/$name" escape or BECOME the skills dir and rm -rf it —
# abort loudly instead: that's a maintenance error in this script, not state.
assert_safe_retired_skill_name() {
  case "$1" in
    ''|.|..|*/*)
      echo "cleanup-retired-skills: unsafe RETIRED_SKILLS entry '$1' — refusing to touch $SKILLS_DST_DIR (fix RETIRED_SKILLS in install.sh)" >&2
      exit 2 ;;
  esac
}

comp_cleanup-retired-skills() {
  case "$1" in
    install)
      local removed=0 name
      for name in "${RETIRED_SKILLS[@]}"; do
        assert_safe_retired_skill_name "$name"
        local dst="$SKILLS_DST_DIR/$name"
        if [ -d "$dst" ]; then
          do_or_show "rm -rf $dst (retired skill)" rm -rf "$dst"
          removed=$((removed + 1))
        fi
      done
      if [ "$removed" = 0 ]; then
        say "  ✓ already clean (no retired skills in $SKILLS_DST_DIR)"
      else
        say "  cleanup: removed=$removed retired skill(s)"
      fi ;;
    uninstall)
      say "  ↺ cleanup is not reversed (retired skills left the repo intentionally)" ;;
    verify)
      local leftover=() name
      for name in "${RETIRED_SKILLS[@]}"; do
        assert_safe_retired_skill_name "$name"
        if [ -d "$SKILLS_DST_DIR/$name" ]; then leftover+=("$name"); fi
      done
      if [ "${#leftover[@]}" = 0 ]; then
        say "  retired skills gone ...... PASS (${RETIRED_SKILLS[*]})"
      else
        say "  retired skills gone ...... FAIL — still present: ${leftover[*]}"
      fi ;;
  esac
}

# ----------------------------------------------------------------------------
# wosy-metrics deploy: claude/bin/wosy-metrics.sh -> ~/.local/bin/wosy-metrics.
# The source script may land in the repo after this component does; a missing
# source is a SKIP with a re-run hint, never a hard failure.
# The deployed copy lives outside the repo, so its $0-derived REPO_ROOT is wrong;
# pin that line to this repo's path at install time. Verify must compare against
# the same transformation or it would FAIL forever against the raw source.
render_wosy_metrics() { sed "s|^REPO_ROOT=.*|REPO_ROOT=\"${REPO_ROOT}\"|" "$SRC_WOSY_METRICS"; }
comp_wosy-metrics-deploy() {
  case "$1" in
    install)
      if [ ! -f "$SRC_WOSY_METRICS" ]; then
        say "  ✗ source missing: $SRC_WOSY_METRICS — skipping; re-run --only=wosy-metrics-deploy --apply once it exists"
        return 0
      fi
      local rendered; rendered="$(mktemp)"
      render_wosy_metrics > "$rendered"
      if [ -f "$WOSY_METRICS_TARGET" ] && cmp -s "$rendered" "$WOSY_METRICS_TARGET"; then
        rm -f "$rendered"; say "  ✓ installed + identical (no copy)"; return 0
      fi
      [ -f "$WOSY_METRICS_TARGET" ] && say "  ! differs — existing copy will be backed up"
      do_or_show "mkdir -p $LOCAL_BIN_DIR" mkdir -p "$LOCAL_BIN_DIR"
      { [ -f "$WOSY_METRICS_TARGET" ] && do_or_show "backup $WOSY_METRICS_TARGET" backup "$WOSY_METRICS_TARGET"; } || true
      do_or_show "install wosy-metrics.sh (REPO_ROOT pinned) -> $WOSY_METRICS_TARGET" cp "$rendered" "$WOSY_METRICS_TARGET"
      do_or_show "chmod +x $WOSY_METRICS_TARGET" chmod +x "$WOSY_METRICS_TARGET"
      rm -f "$rendered" ;;
    uninstall) [ -f "$WOSY_METRICS_TARGET" ] && do_or_show "rm $WOSY_METRICS_TARGET" rm -f "$WOSY_METRICS_TARGET" || say "  ✓ already absent" ;;
    verify)
      if [ ! -f "$SRC_WOSY_METRICS" ]; then
        say "  wosy-metrics deployed .... SKIP (source missing: claude/bin/wosy-metrics.sh)"
      elif [ -x "$WOSY_METRICS_TARGET" ] && render_wosy_metrics | cmp -s - "$WOSY_METRICS_TARGET"; then
        say "  wosy-metrics deployed .... PASS (wosy-metrics)"
      else
        say "  wosy-metrics deployed .... (after --apply)"
      fi ;;
  esac
}

# ---- which components to run -----------------------------------------------
selected=()
if [ -n "$ONLY" ]; then
  printf '%s\n' "${ALL_COMPONENTS[@]}" | grep -qx "$ONLY" || { echo "no such component: $ONLY (--list)" >&2; exit 2; }
  selected=("$ONLY")
else
  for c in "${ALL_COMPONENTS[@]}"; do
    [ "$c" = brain-home-target ] && [ "$SKIP_PROVISION" = 1 ] && continue
    selected+=("$c")
  done
fi

# ---- preconditions ---------------------------------------------------------
say "install-claude.sh — $VERB / $MODE   (components: ${selected[*]})"
step "preconditions"
ok=1
command -v jq >/dev/null 2>&1 && say "  ✓ jq" || { say "  ✗ jq MISSING"; ok=0; }
{ command -v "$BRAIN_BIN" >/dev/null 2>&1 || [ -x "$BRAIN_BIN" ]; } && say "  ✓ brain: $BRAIN_BIN" || say "  ! brain missing (watchdog fails open until present): $BRAIN_BIN"
[ -f "$SRC_WATCHDOG" ] && say "  ✓ source hook: $SRC_WATCHDOG" || { say "  ✗ source hook MISSING: $SRC_WATCHDOG"; ok=0; }
{ [ -f "$SETTINGS" ] && jq -e . "$SETTINGS" >/dev/null 2>&1; } && say "  ✓ valid settings.json" || { say "  ✗ settings.json missing/invalid"; ok=0; }
# Ensure ~/.local/bin/ exists when brain is targeted there but missing.
# Portability fix 2026-05-29: fresh user accounts lack this dir; without it the
# `brain` install + watchdog fail-open contract both work, but the user has no
# obvious place to drop the binary.
if [ ! -d "$LOCAL_BIN_DIR" ]; then
  if [ "$APPLY" = 1 ]; then
    mkdir -p "$LOCAL_BIN_DIR" && say "  ✓ created $LOCAL_BIN_DIR (for brain binary)"
  else
    say "  ! ${LOCAL_BIN_DIR} missing — will create on --apply"
  fi
else
  say "  ✓ ${LOCAL_BIN_DIR} present"
fi
[ "$ok" = 1 ] || { say ""; say "PRECONDITIONS FAILED — aborting."; exit 1; }

# ---- run -------------------------------------------------------------------
for c in "${selected[@]}"; do
  step "[$c] ${COMP_DESC[$c]} ($MODE)"
  "comp_$c" "$VERB"
done

# ---- verify ----------------------------------------------------------------
step "verify"
for c in "${selected[@]}"; do "comp_$c" verify; done
# behavioral probe via the real shim (read-only; brain guard does not mutate)
shim="$DST_WATCHDOG"; [ -x "$shim" ] || shim="$SRC_WATCHDOG"
probe() { printf '%s' "$1" | BRAIN_BIN="$BRAIN_BIN" bash "$shim" >/dev/null 2>&1; echo $?; }
rcb="$(probe "{\"tool_name\":\"Write\",\"tool_input\":{\"file_path\":\"${DEVWORK}/brain/_probe.json\"},\"cwd\":\"${DEVWORK}/brain\"}")"
rcs="$(probe "{\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"${DEVWORK}/brain/main.go\"},\"cwd\":\"${DEVWORK}/brain\"}")"
rco="$(probe "{\"tool_name\":\"Write\",\"tool_input\":{\"file_path\":\"/tmp/x.json\"},\"cwd\":\"/tmp\"}")"
pf() { [ "$1" = "$2" ] && echo PASS || echo "FAIL(exit=$1 want $2)"; }
say "  source .go flows free .... $(pf "$rcs" 0)"
say "  /tmp fail-open ........... $(pf "$rco" 0)"
if [ "$VERB" = install ] && [ "$APPLY" = 1 ] && [ "$SKIP_PROVISION" = 0 ]; then
  # Claude Code PreToolUse contract: exit 2 = block (exit 1 is advisory only,
  # tool proceeds). Earlier versions of this verify and watchdog.sh used exit 1
  # incorrectly; the shim now exits 2 and so does this check.
  say "  R10 KB write -> BLOCK ..... $(pf "$rcb" 2)"
else
  say "  R10 KB write probe ....... exit=$rcb (BLOCKs only once provisioned + applied)"
fi

# ---- footer ----------------------------------------------------------------
say ""
if [ "$APPLY" = 1 ] && [ "$VERB" = install ]; then
  say "DONE. Restart your Claude Code session so the PreToolUse hook loads."
  say ""
  say "Tips:"
  say "  • Set WOSY_SCAN_ROOTS in your shell profile for stale-task nudges:"
  say "      export WOSY_SCAN_ROOTS=\"\$HOME/projects:\$HOME/work\""
  say "  • See INSTALL.md for the full reference (configuration, troubleshooting, update flow)."
elif [ "$APPLY" = 1 ]; then
  say "DONE ($VERB)."
else
  say "DRY RUN — nothing changed. Re-run with --apply to execute."
fi
