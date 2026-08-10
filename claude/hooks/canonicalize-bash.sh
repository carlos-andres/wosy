#!/usr/bin/env bash
# canonicalize-bash.sh — PreToolUse Bash hook.
#
# Per CSI verdict 2026-05-20 action #15 + Q6–Q9 resolutions:
#   - Block non-canonical mysql invocations (no --login-path=).
#   - Nudge inline SQL / heredoc SQL toward project-scoped canonical scripts
#     under `<project>/.devwork/scripts/` (operations, not raw queries).
#   - Discovery loop: if no canonical script exists for the pattern, suggest
#     adding one so the library grows from real usage.
#   - Nudge `git log ... --grep <ticket>` patterns toward `gitfiles`.
#
# Reads JSON event from stdin per Claude Code hook contract.
# Exit 0 = allow, exit 2 = block (stderr shown to Claude).
# NOTE: exit 1 is ADVISORY only (tool proceeds) — only exit 2 actually blocks.
# Skipped silently when the command is wrapped in `rtk` (rtk handles its rewrites).

set -uo pipefail

# detect-platform lib (XDG vs Library log paths). Fail-open stub keeps prior macOS path.
_dp_lib="${HOME}/.claude/hooks/lib/detect-platform.sh"
if [ -r "$_dp_lib" ]; then . "$_dp_lib"; else log_dir_for() { echo "$HOME/Library/Logs/claude-$1"; }; fi
LOG_DIR="$(log_dir_for canonicalize-bash)"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/$(date +%Y%m%d).jsonl"

input="$(cat 2>/dev/null || true)"
command="$(printf '%s' "$input" | jq -r '.tool_input.command // empty' 2>/dev/null)"
cwd="$(printf '%s' "$input" | jq -r '.cwd // empty' 2>/dev/null)"
session_id="$(printf '%s' "$input" | jq -r '.session_id // empty' 2>/dev/null)"

[ -z "$command" ] && exit 0

# rtk-wrapped → defer to rtk's rewrite logic.
if printf '%s' "$command" | grep -qE '^\s*rtk\b'; then
  exit 0
fi

# Shared walk-up finder (factored to ~/.claude/hooks/lib/walk-up.sh). Fail-open:
# if the lib is missing, the stub returns 1 (no library found => nudge-only, no
# block — prior behavior for an unprovisioned project).
_wosy_walkup="${HOME}/.claude/hooks/lib/walk-up.sh"
if [ -r "$_wosy_walkup" ]; then
  # shellcheck source=/dev/null
  . "$_wosy_walkup"
else
  walk_up_first() { return 1; }
fi
# Nearest canonical library, priority order: .devwork/queries/library (corporate:
# .sql + q.sh) > .devwork/scripts (named ops) > .devwork/bin (runner only).
find_scripts_dir() { walk_up_first -d .devwork/queries/library .devwork/scripts .devwork/bin; }

# Per-project wosy flag reader — factored to ~/.claude/hooks/lib/wosy-flags.sh
# (shared with validate-bash.sh + write-location-warn.sh; kills the duplicated
# walk-up). Fail-open: if the lib is missing, the stub makes flag reads return
# empty (= no flags = prior pre-flag behavior: nudges only, no hard blocks).
# wosy_flag reads ${cwd:-$PWD} by default; this hook sets cwd from the event JSON.
_wosy_lib="${HOME}/.claude/hooks/lib/wosy-flags.sh"
if [ -r "$_wosy_lib" ]; then
  # shellcheck source=/dev/null
  . "$_wosy_lib"
else
  wosy_flag() { :; }
fi

log_event() {
  local kind="$1" detail="$2"
  jq -nc \
    --arg ts "$(date +%Y-%m-%dT%H:%M:%S%z)" \
    --arg session "$session_id" \
    --arg cwd "$cwd" \
    --arg kind "$kind" \
    --arg detail "$detail" \
    '{ts:$ts, session:$session, cwd:$cwd, kind:$kind, detail:$detail}' \
    >> "$LOG" 2>/dev/null || true
}

block() {
  log_event "block" "$1"
  echo "BLOCKED by canonicalize-bash: $1" >&2
  exit 2
}

nudge() {
  log_event "nudge" "$1"
  echo "nudge canonicalize-bash: $1" >&2
}

# Per-project kill switch: .devwork/wosy.flags with `wosy_enforce=0` turns this
# hook into a no-op for that project (enable/disable enforcement by project scope).
if [ "$(wosy_flag wosy_enforce)" = "0" ]; then
  exit 0
fi

# ---------- mysql: canonical-shape (AGNOSTIC + OPT-IN) ----------
# `mysql` without --login-path= is a credential-hygiene smell. NUDGE everywhere;
# hard-BLOCK only where the project opted into mysql enforcement
# (enforce_inline_sql=1 + db_engine=mysql). Keeps the rule off non-mysql projects.
if printf '%s' "$command" | grep -qE '(^|[[:space:]|&;])mysql([[:space:]]|$)'; then
  if ! printf '%s' "$command" | grep -qE '\-\-login-path='; then
    if [ "$(wosy_flag enforce_inline_sql)" = "1" ] && [ "$(wosy_flag db_engine)" = "mysql" ]; then
      block "non-canonical 'mysql' (no --login-path=) and this project enforces mysql canonical shape. Use --login-path=<alias> or .my.cnf. Opt out: wosy_enforce=0 in .devwork/wosy.flags."
    else
      nudge "non-canonical 'mysql' (no --login-path=). Prefer --login-path=<alias> or .my.cnf over credentials on the command line."
    fi
  fi
fi

# ---------- Inline SQL detection (mysql, psql, sqlite3) ----------
# Detect prose SQL passed via -e, heredoc, or quoted string.
inline_sql_detected=""
if printf '%s' "$command" | grep -qiE '(mysql|psql|sqlite3)[^|;&]*(-e[[:space:]]|-c[[:space:]]|<<-?[[:space:]]*[A-Z_]+|--command)'; then
  inline_sql_detected="exec-flag"
fi
if [ -z "$inline_sql_detected" ] && printf '%s' "$command" | grep -qiE '(SELECT|INSERT|UPDATE|DELETE)[[:space:]]+(\*|DISTINCT|FROM|INTO|SET)' ; then
  # Only count if accompanied by a SQL client invocation in the same line
  if printf '%s' "$command" | grep -qE '(mysql|psql|sqlite3|--login-path=)'; then
    inline_sql_detected="prose-sql"
  fi
fi

if [ -n "$inline_sql_detected" ]; then
  scripts_dir="$(find_scripts_dir || true)"

  # Opt-in HARD BLOCK (per-project flag + provision-gated + engine-aware).
  # Fires ONLY when enforce_inline_sql=1 AND a real query library exists AND the
  # command isn't already routed through the runner / a .sql file. Requires the
  # library to be present, so it can never self-lockout an unprovisioned project.
  if [ -n "$scripts_dir" ] && [ "$(wosy_flag enforce_inline_sql)" = "1" ]; then
    case "$scripts_dir" in
      */queries/library|*/scripts)
        if ! printf '%s' "$command" | grep -qE 'q\.sh|\.sql([[:space:]"'\'']|$)|\.read[[:space:]]'; then
          eng="$(wosy_flag db_engine)"
          if [ -z "$eng" ]; then
            # config-is-truth fallback: derive the engine from project config when
            # db_engine is not hand-set in wosy.flags. Fail-open: if the detector lib
            # is missing, eng stays empty -> generic any-client matcher (prior behavior).
            _db_lib="${HOME}/.claude/hooks/lib/detect-db-engine.sh"
            [ -r "$_db_lib" ] && { . "$_db_lib"; eng="$(detect_db_engine "${cwd:-$PWD}")"; }
          fi
          case "$eng" in
            mysql)  client_re='mysql' ;;
            pgsql)  client_re='psql' ;;
            sqlite) client_re='sqlite3' ;;
            *)      client_re='(mysql|psql|sqlite3)' ;;
          esac
          if printf '%s' "$command" | grep -qE "(^|[^[:alnum:]_])${client_re}([^[:alnum:]_]|$)"; then
            proj_dw="${scripts_dir%/queries/library}"; proj_dw="${proj_dw%/scripts}"
            block "inline ${eng:-SQL} and this project sets enforce_inline_sql=1. Route via ${proj_dw}/bin/q.sh <file.sql>, or add a parameterized .sql under ${scripts_dir}. Opt out in ${proj_dw}/wosy.flags (wosy_enforce=0)."
          fi
        fi
        ;;
    esac
  fi

  if [ -n "$scripts_dir" ]; then
    available="$(ls -1 "$scripts_dir" 2>/dev/null | head -20 | tr '\n' ' ' || true)"
    case "$scripts_dir" in
      */queries/library)
        # Corporate shape: .sql files + q.sh runner. Find the runner path.
        proj_devwork="${scripts_dir%/queries/library}"
        runner=""
        if [ -x "$proj_devwork/bin/q.sh" ]; then
          runner="$proj_devwork/bin/q.sh"
        elif [ -f "$proj_devwork/bin/q.sh" ]; then
          runner="bash $proj_devwork/bin/q.sh"
        fi
        if [ -n "$available" ]; then
          if [ -n "$runner" ]; then
            nudge "inline SQL detected (${inline_sql_detected}). Canonical query library at ${scripts_dir}. Existing .sql files: ${available%% }. Runner: ${runner} <file.sql> [DEALER_ID] [SOURCE_ID] [FEED_NAME]. If one matches your intent, call it via the runner instead of running raw SQL. If none match, extend the library with a new parameterized .sql file (header convention in ${proj_devwork}/queries/README.md)."
          else
            nudge "inline SQL detected (${inline_sql_detected}). Canonical query library at ${scripts_dir}. Existing .sql files: ${available%% }. If one matches your intent, read it and adapt; if none match, extend the library."
          fi
        else
          nudge "inline SQL detected (${inline_sql_detected}). Query library dir exists but is empty: ${scripts_dir}. Consider seeding it with a parameterized .sql file (see ${proj_devwork}/queries/README.md if present)."
        fi
        ;;
      */scripts)
        if [ -n "$available" ]; then
          nudge "inline SQL detected (${inline_sql_detected}). Canonical scripts library at ${scripts_dir}. Existing operations: ${available%% }. If one matches your intent, call it instead of running raw SQL. If none match, propose adding a new script."
        else
          nudge "inline SQL detected (${inline_sql_detected}). Scripts dir exists but is empty: ${scripts_dir}. Consider canonicalizing this as ${scripts_dir}/<operation-name> — TSV default, --md flag for human-read."
        fi
        ;;
      */bin)
        # Runner location only — no first-class library. Surface what's there.
        nudge "inline SQL detected (${inline_sql_detected}). Runner directory at ${scripts_dir} (no /scripts or /queries/library). Available executables: ${available%% }. Check if one accepts your intent before running raw SQL."
        ;;
    esac
  else
    proj_root="${cwd:-$PWD}"
    nudge "inline SQL detected (${inline_sql_detected}). No canonical library found walking up from ${proj_root}. Discovery loop: should this become .devwork/queries/library/<name>.sql (parameterized query) or .devwork/scripts/<name> (named operation)? See nearest CLAUDE.md or .devwork/queries/README.md for project conventions."
  fi
fi

# ---------- gitfiles integration ----------
# Catch git log ... --grep <ticket-prefix> and suggest gitfiles.
if printf '%s' "$command" | grep -qE 'git[[:space:]]+log\b.*--grep' || \
   printf '%s' "$command" | grep -qE 'git[[:space:]]+log\b.*\|[[:space:]]*grep'; then
  if [ -x "$HOME/.local/bin/gitfiles" ]; then
    nudge "ticket-scoped 'git log' detected. For 'what files did tickets X–Y touch', prefer 'gitfiles <ticket-prefix>' (~/.local/bin/gitfiles) — supports --paths, --with-commits, --branch, --repos. Multi-repo: 'gitfiles-umbrella'."
  fi
fi

exit 0
