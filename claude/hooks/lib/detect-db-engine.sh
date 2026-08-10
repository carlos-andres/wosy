#!/usr/bin/env bash
# shellcheck shell=bash
# detect-db-engine.sh — config-is-truth DB engine detection for a project dir.
#
# Dual use:
#   - SOURCE it:   . detect-db-engine.sh ; eng="$(detect_db_engine /path)"
#   - RUN it:      bash detect-db-engine.sh /path   # prints engine (or nothing)
#
# Built 2026-05-22 (wosy audit P2 #8) so db_engine in .devwork/wosy.flags is
# DERIVED from config, not hand-typed. Echoes: mysql | pgsql | sqlite | (empty=unknown).
#
# Precedence (app config is truth; .devwork/connections.md is the re-entry signal —
# pilot-a is provably pgsql via connections.md despite having NO .env):
#   1 .env* DB_CONNECTION   2 .env* DATABASE_URL scheme   3 docker-compose db image
#   4 .devwork/connections.md recorded engine   5 a *.sqlite file   6 unknown(empty)
#
# Reads SPECIFIC files with grep (NOT rg) on purpose: .env is usually gitignored,
# so rg would skip it. Globs (not find/fd) locate sqlite files incl. gitignored ones.
detect_db_engine() {
  local dir="${1:-$PWD}" v url f s

  # 1 + 2: env files (root + common nested app/ layout)
  for f in "$dir/.env" "$dir/.env.local" "$dir/app/.env"; do
    [ -f "$f" ] || continue
    v="$(grep -E '^[[:space:]]*DB_CONNECTION[[:space:]]*=' "$f" 2>/dev/null | head -1)"
    v="${v#*=}"; v="${v%%#*}"; v="$(printf '%s' "$v" | tr -d '[:space:]')"
    v="${v%\"}"; v="${v#\"}"; v="${v%\'}"; v="${v#\'}"
    case "${v,,}" in
      mysql|mariadb)             echo mysql;  return 0 ;;
      pgsql|postgres|postgresql) echo pgsql;  return 0 ;;
      sqlite|sqlite3)            echo sqlite; return 0 ;;
    esac
    url="$(grep -E '^[[:space:]]*DATABASE_URL[[:space:]]*=' "$f" 2>/dev/null | head -1)"
    url="${url#*=}"
    case "$url" in
      *postgres://*|*postgresql://*) echo pgsql;  return 0 ;;
      *mysql://*|*mysql2://*)        echo mysql;  return 0 ;;
      *sqlite:*|*file:*)             echo sqlite; return 0 ;;
    esac
  done

  # 3: docker-compose db image
  for f in "$dir/docker-compose.yml" "$dir/docker-compose.yaml" "$dir/compose.yml" "$dir/compose.yaml"; do
    [ -f "$f" ] || continue
    grep -qiE 'image:[[:space:]]*"?(postgres|postgis)' "$f" 2>/dev/null && { echo pgsql; return 0; }
    grep -qiE 'image:[[:space:]]*"?(mysql|mariadb|percona)' "$f" 2>/dev/null && { echo mysql; return 0; }
  done

  # 4: .devwork/connections.md (re-entry truth). Explicit 'engine:' line first; then
  #    fall back to the '### <Engine>' section header or the documented client command
  #    (fix G1 2026-05-25: corporate documents MySQL via header + `mysql --login-path`
  #    with NO explicit engine: line, so it was returning empty).
  f="$dir/.devwork/connections.md"
  if [ -f "$f" ]; then
    grep -qiE 'engine[^a-z]*(pgsql|postgres)' "$f" 2>/dev/null && { echo pgsql;  return 0; }
    grep -qiE 'engine[^a-z]*(mysql|mariadb)'  "$f" 2>/dev/null && { echo mysql;  return 0; }
    grep -qiE 'engine[^a-z]*sqlite'           "$f" 2>/dev/null && { echo sqlite; return 0; }
    # fallback: '### <Engine>' section header or documented client command. Only decide
    # when EXACTLY ONE engine is signaled — a multi-engine doc (e.g. a deploy box with
    # both mysql+pgsql) stays ambiguous -> empty, never a confident false pick.
    local have_pg='' have_my='' have_sq='' n
    grep -qiE '^#+[[:space:]]*(postgre|pgsql)|[[:space:]`]psql[[:space:]]|postgres(ql)?://' "$f" 2>/dev/null && have_pg=1
    grep -qiE '^#+[[:space:]]*(mysql|mariadb)|[[:space:]`](mysql|mariadb)[[:space:]]'        "$f" 2>/dev/null && have_my=1
    grep -qiE '^#+[[:space:]]*sqlite|[[:space:]`]sqlite3?[[:space:]]'                        "$f" 2>/dev/null && have_sq=1
    n=$(( ${have_pg:-0} + ${have_my:-0} + ${have_sq:-0} ))
    if [ "$n" = 1 ]; then
      [ -n "$have_pg" ] && { echo pgsql;  return 0; }
      [ -n "$have_my" ] && { echo mysql;  return 0; }
      [ -n "$have_sq" ] && { echo sqlite; return 0; }
    fi
  fi

  # 5: a sqlite db file (globs catch gitignored ones; bounded to common locations)
  shopt -s nullglob
  for s in "$dir"/*.sqlite "$dir"/*.sqlite3 \
           "$dir"/database/*.sqlite "$dir"/database/*.sqlite3 \
           "$dir"/storage/*.sqlite "$dir"/storage/*.sqlite3; do
    [ -f "$s" ] && { shopt -u nullglob; echo sqlite; return 0; }
  done
  shopt -u nullglob

  # 6: unknown
  return 0
}

# Run standalone when executed (not sourced).
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  detect_db_engine "${1:-$PWD}"
fi
