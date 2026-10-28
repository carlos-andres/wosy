#!/usr/bin/env bash
# catalogue-loader.sh — SessionStart hook. srs.md A2, A3, F4 (brain-continuity-20260923, logbook 181).
# Injects a CATALOGUE generated from the wosy store: the worlds one line each, the terms that
# locate, the node families, and the read line. Nothing project-specific enters until a world
# is resolved. Hard ceiling 2,000 tokens (bytes/4): above it the hook injects nothing and says so
# on stderr, so a fat catalogue is a red control and not a silent load.
#
# Fires on source ∈ {startup, clear}. Walks up from the session cwd to the nearest .devwork/,
# follows wosy.yml `hub:` when present, resolves the store the way the binary does (cd hub
# parent, `brain query`). No .devwork, no hub, no store → exits 0 with no output.
#
# Test form: catalogue-loader.sh --print <dir>   prints the catalogue for that cwd, no JSON.
set -uo pipefail
BRAIN=${BRAIN_BIN:-$(command -v brain || echo /Users/you/.local/bin/brain)}
CEILING=2000

if [ "${1:-}" = "--print" ]; then
  cwd="${2:?dir}"; source_kind=startup; print_only=1
else
  command -v jq >/dev/null 2>&1 || exit 0
  input="$(cat 2>/dev/null || true)"
  cwd="$(printf '%s' "$input" | jq -r '.cwd // empty' 2>/dev/null)"
  source_kind="$(printf '%s' "$input" | jq -r '.source // empty' 2>/dev/null)"
  print_only=0
fi
[ -n "$cwd" ] || exit 0
case "$source_kind" in startup|clear) ;; *) exit 0 ;; esac

devwork=""; d="${cwd%/}"
while [ -n "$d" ] && [ "$d" != "/" ]; do
  [ -d "$d/.devwork" ] && { devwork="$d/.devwork"; break; }
  d="${d%/*}"
done
[ -n "$devwork" ] || exit 0
hub="$(sed -nE 's/^[[:space:]]*hub:[[:space:]]*//p' "$devwork/wosy.yml" 2>/dev/null | head -1 | sed -E 's/[[:space:]]+$//')"
[ -n "$hub" ] && [ -d "$hub" ] || hub="$devwork"
hub="${hub%/}"
[ -x "$BRAIN" ] || exit 0

q() { (cd "${hub%/.devwork}" && "$BRAIN" query "$1" 2>/dev/null | sed '1d;$d'); }
worlds="$(q "SELECT printf('  %-12s %s', id, root_path) FROM project ORDER BY id")"
[ -n "$worlds" ] || exit 0
terms="$(q "SELECT group_concat(term, ', ') FROM (SELECT DISTINCT term FROM synonym ORDER BY 1)")"
fams="$(q "SELECT group_concat(n, ' · ') FROM (SELECT count(DISTINCT to_id)||' '||substr(to_id,1,instr(to_id,':')-1)||'s' AS n FROM edge WHERE rel='about' AND instr(to_id,':')>0 GROUP BY substr(to_id,1,instr(to_id,':')-1))")"
shape="$(q "SELECT (SELECT count(*) FROM edge)||' edges · '||(SELECT count(*) FROM lesson)||' lessons · '||(SELECT count(*) FROM ruled_out)||' ruled_out · '||(SELECT count(*) FROM query_pointer)||' query pointers'")"

ctx="[catalogue-loader] wosy catalogue, generated $(date -u +%Y-%m-%dT%H:%MZ) from ${hub}/brain.db. Nothing below is project knowledge; resolve a world first (srs A3).
worlds, one line each (brain project ids):
${worlds}
terms that locate (synonym): ${terms:-none}
nodes reachable by about: ${fams:-none} · store: ${shape}
read protocol: before you open a master.md, a flow or code, run \`brain pallet <term>\` with the world id or a synonym term you were given. It returns one gated JSON pallet: the pointer set over the five layers, the five fields, and the gaps named. A term it does not know is refused (exit 1). For a ticket key or a merchant id run \`brain query \"SELECT from_id, rel, to_id FROM edge WHERE to_id='ticket:<KEY>' OR to_id='merchant:<ID>'\"\` and pallet the world it points at. Read-only SQL: \`brain query \"<SELECT>\"\`. What no query answers is for the interview."

tokens=$(( ${#ctx} / 4 ))
if [ "$tokens" -gt "$CEILING" ]; then
  echo "catalogue-loader: $tokens tokens > $CEILING ceiling (srs A2); nothing injected" >&2
  exit 0
fi
if [ "$print_only" = 1 ]; then printf '%s\n' "$ctx"; echo "-- $tokens tokens" >&2; exit 0; fi
jq -nc --arg ctx "$ctx" '{hookSpecificOutput:{hookEventName:"SessionStart", additionalContext:$ctx}}'
exit 0
