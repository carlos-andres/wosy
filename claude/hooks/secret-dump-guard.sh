#!/usr/bin/env bash
# secret-dump-guard.sh — PreToolUse hook for Bash. Blocks wholesale secret dumps.
#
# The deterministic form of CLAUDE.md's "Secret hygiene in command output" rule,
# which today is advisory only ("the stopgap until that hook exists"). Secrets
# that reach the transcript are exposed — the harness captures raw stdout and it
# may be cached/indexed even after deletion. Redact BEFORE the command prints.
#
# A PreToolUse hook runs BEFORE the command, so it cannot rewrite the command's
# OUTPUT (that is why true output-redaction is not hook-achievable here). What it
# CAN do deterministically is refuse the wholesale-dump commands themselves. So:
#
#   BLOCK (exit 2): dump-verb reading a real secret file (.env family, .envrc,
#     .netrc, .pgpass, .my.cnf, private keys/.pem); and a BARE `env`/`printenv`.
#   WARN (exit 0, stderr): softer leaks that are often legitimate debugging —
#     dumping config.php, `curl -v`, `git remote -v`, and piped `env|`/`printenv|`.
#
# Safe alternatives the message points at: grep key NAMES only, or mask values:
#   grep -iE '^KEY=' .env | sed -E 's/=.*/=***/'
#
# Reads JSON event from stdin. exit 0 = allow, exit 2 = block (stderr->Claude).
# NOTE: exit 1 is advisory only (tool proceeds); only exit 2 actually blocks.
# Fail-open: jq missing or no command => allow.

set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0

input="$(cat 2>/dev/null || true)"
command="$(printf '%s' "$input" | jq -r '.tool_input.command // empty' 2>/dev/null)"
[ -z "$command" ] && exit 0

block() {
  echo "BLOCKED by secret-dump-guard: $1" >&2
  echo "Secrets in the transcript are exposed (raw stdout is captured and may be cached even after deletion)." >&2
  echo "Instead, grep key NAMES only or mask values, e.g.:  grep -iE '^KEY=' .env | sed -E 's/=.*/=***/'" >&2
  exit 2
}
warn() { echo "WARN secret-dump-guard: $1 — allowed, but redact before it prints (mask values / grep key names only)." >&2; }

# Dump verbs that print a file's contents verbatim. `sed`/`awk`/`grep` are
# excluded on purpose — they are the masking/key-listing escape hatch.
dump_re='(^|[ \t;&|(])(cat|bat|head|tail|less|more|nl|xxd|od|strings)([ \t]|$)'

has_dump=0
printf '%s' "$command" | grep -qE "$dump_re" && has_dump=1

# Scrub example/sample/dist/template env files (they carry no secrets) so a
# reference to a REAL .env survives for the test below.
scrubbed="$(printf '%s' "$command" | sed -E 's/\.env\.(example|sample|dist|template)[A-Za-z0-9._-]*//g')"

secret_env=0
printf '%s' "$scrubbed" | grep -qE '(^|[ \t=:/"'"'"'])\.env([.][A-Za-z0-9_-]+)?([ \t;&|"'"'"']|$)' && secret_env=1

secret_other=0
printf '%s' "$command" | grep -qE '(\.envrc|\.netrc|\.pgpass|\.my\.cnf|(^|/)id_rsa|(^|/)id_ed25519|(^|/)id_dsa|\.pem)([ \t;&|"'"'"']|$)' && secret_other=1

# ---- BLOCK: dump-verb reading a real secret file ----
if [ "$has_dump" = 1 ] && { [ "$secret_env" = 1 ] || [ "$secret_other" = 1 ]; }; then
  block "reading a secret file (.env / .envrc / key / credential) with a dump command"
fi

# ---- BLOCK: bare `env` / `printenv` (dumps the whole environment) ----
# Strip leading VAR=val assignments and `cd … &&` so the env-RUNNER form
# (`env FOO=bar cmd`, which is not a dump) is not mistaken for a bare dump.
stripped="$(printf '%s' "$command" \
  | sed -E 's/^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*=[^ ]+[[:space:]]+)*//; s/^[[:space:]]*cd[[:space:]]+[^&]+&&[[:space:]]*//; s/^[[:space:]]*//')"
if printf '%s' "$stripped" | grep -qE '^(env|printenv)[[:space:]]*$'; then
  block "bare '${stripped%% *}' dumps the entire environment (secrets included)"
fi

# ---- WARN (allow): softer leaks that are often legitimate ----
if printf '%s' "$stripped" | grep -qE '^(env|printenv)[[:space:]]*\|'; then
  warn "piped 'env'/'printenv' still prints matched values"
fi
if [ "$has_dump" = 1 ] && printf '%s' "$command" | grep -qE '(^|[ \t=:/])config\.php([ \t;&|"'"'"']|$)'; then
  warn "dumping config.php may expose inline credentials"
fi
if printf '%s' "$command" | grep -qE '(^|[ \t;&|(])curl([ \t]|$)' && printf '%s' "$command" | grep -qE '(-v\b|--verbose\b)'; then
  warn "'curl -v' prints request headers (Authorization, cookies)"
fi
if printf '%s' "$command" | grep -qE 'git[[:space:]]+remote[[:space:]]+(-v\b|--verbose\b|show\b)'; then
  warn "'git remote -v/show' prints remote URLs that may embed tokens"
fi

exit 0
