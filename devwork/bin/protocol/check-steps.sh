#!/opt/homebrew/bin/bash
# Does a handover satisfy the six steps that produce it?
#
# The six steps live in `_scratch/pasos-handover-20260916.md`, each with a done_when. Nothing ever
# executed them against a document, so a handover could be written by the procedure and never
# measured by it. Written 2026-09-16 after a document passed four checkers and still failed to
# transfer.
#
# Known limit, stated because the tool cannot close it: S1 checks that every path WRITTEN resolves.
# It cannot see a referent the author never wrote as a path. That is the defect that motivated this
# runner and it needs an L1-scoped referent check, which is a different instrument.
set -uo pipefail
D=/Users/you/.devwork
DOC="${1:-}"
[ -n "$DOC" ] || { echo "usage: check-steps.sh <document.md>"; exit 2; }
[ -f "$DOC" ] || { echo "FATAL: no such document $DOC"; exit 2; }
pass=0; fail=0; advis=0

row(){ # id | verdict | class | detail
  printf '%-4s %-9s %-11s %s\n' "$1" "$2" "$3" "$4"
  case "$2" in PASS) pass=$((pass+1));; FAIL) fail=$((fail+1));; *) advis=$((advis+1));; esac
}

# Absolute paths the document commits to, with any :line suffix stripped.
paths(){ pandoc -f markdown -t json "$DOC" \
  | jq -r '[.. |objects| select(.t=="Code") | .c[1]] | map(select(test("^/Users/"))) | unique | .[]' \
  | sed 's/:[0-9-]*$//'; }

# S1 - every cited path resolves now
cited=0; dead=0
while read -r p; do
  [ -n "$p" ] || continue
  cited=$((cited+1))
  # A glob is a pattern, not a pointer. Resolve its longest literal prefix as a directory.
  case "$p" in
    *[*?]*) base="${p%%[*?]*}"; base="${base%/*}"
            [ -d "$base" ] || { dead=$((dead+1)); deadlist="${deadlist:-}$p "; } ;;
    *)      [ -e "$p" ]      || { dead=$((dead+1)); deadlist="${deadlist:-}$p "; } ;;
  esac
done < <(paths)
if [ "$dead" -eq 0 ]; then
  row S1 PASS structural "$cited absolute paths cited, 0 dead"
else
  row S1 FAIL structural "$dead of $cited paths are dead: ${deadlist}"
fi

# S1b - step 1 asks that the artifact set be NAMED, and a bare noun names nothing
if python3 "$D/bin/protocol/check-referents.py" "$DOC" >/dev/null 2>&1; then
  row S1b PASS structural "every artifact phrase in L1 carries a path in its clause"
else
  ref=$(python3 "$D/bin/protocol/check-referents.py" "$DOC" 2>/dev/null || true)
  n=$(printf '%s\n' "$ref" | rg -c UNRESOLVED || echo 0)
  row S1b FAIL structural "$n artifact phrase(s) in L1 resolve to no path"
fi

# S2 - advisory by design: the predictor is backward-looking and only advises a young task
slug=$(basename "$(dirname "$DOC")")
pivot=$(rg -m1 '^fecha:' "$DOC" | rg -o '[0-9]{4}-[0-9]{2}-[0-9]{2}' || true)
if [ -z "$pivot" ] || ! command -v duckdb >/dev/null 2>&1; then
  row S2 ADVISORY corpus "skipped: needs a fecha: in frontmatter and duckdb on PATH"
else
  cand=$(duckdb -csv -noheader -c "
    WITH t AS (SELECT unnest(json_extract(json,'\$.message.content[*]')) AS c
               FROM read_ndjson_objects('/Users/you/.claude/projects/**/*.jsonl'))
    SELECT DISTINCT json_extract_string(c,'\$.input.file_path') AS f FROM t
    WHERE json_extract_string(c,'\$.name') IN ('Read','Edit','Write')
      AND f LIKE '%${slug}%' LIMIT 10" 2>/dev/null | sed '/^$/d')
  n=$(printf '%s\n' "$cand" | sed '/^$/d' | wc -l | tr -d ' ')
  unnamed=0
  while read -r f; do
    [ -n "$f" ] || continue
    rg -qF "$f" "$DOC" || unnamed=$((unnamed+1))
  done < <(printf '%s\n' "$cand")
  row S2 ADVISORY corpus "$n files prior sessions read, $unnamed not named here - justify or exclude"
fi

# S3 - step 3 owns the first act alone, so this reads the L1 verdict and never the whole load
# block. Completeness of L1 to L7 is check-load-block.py's own exit code, a separate instrument.
lb=$(python3 "$D/bin/protocol/check-load-block.py" "$DOC" 2>/dev/null || true)
if printf '%s\n' "$lb" | rg -q '^L1 +RUNNABLE'; then
  row S3 PASS structural "L1 carries an absolute command with no placeholder"
else
  row S3 FAIL structural "L1 carries no runnable absolute command"
fi

# Decoupling S3 removed the only completeness assertion from this runner. Reported, never failed:
# turning it red here would re-couple step 3 and step 5 through the same missing L6.
if printf '%s\n' "$lb" | rg -q 'LOAD BLOCK OK'; then
  row LB ADVISORY structural "load block L1-L7 complete, per check-load-block.py"
else
  miss=$(printf '%s\n' "$lb" | rg -c 'MISSING' || echo 0)
  row LB ADVISORY structural "$miss load-block line(s) missing - run check-load-block.py for the list"
fi

# S4 - the weakest step: the runner cannot know which holes the document creates, only whether it
# wrote any recovery route at all.
routes=0; have=""
for t in duckdb fd rg run-chain; do
  rg -qwF "$t" "$DOC" && { routes=$((routes+1)); have="${have}$t "; }
done
if [ "$routes" -gt 0 ]; then
  row S4 PASS textual "$routes of 4 recovery routes written: $have"
else
  row S4 FAIL textual "no recovery command written for any hole type"
fi

# S5 - asking the human is the one hole no tool recovers, so it must be a section
if rg -q 'only the owner|solo el due|L6\.' "$DOC"; then
  row S5 PASS structural "an owner-only block is present"
else
  row S5 FAIL structural "no owner-only block - the one hole no tool can recover"
fi

# S6 - the ruled-out register is carried by pointer, never restated
reg=""
while read -r p; do
  [ -f "$p" ] || continue
  rg -q '^#+ *Ruled out' "$p" && { reg="$p"; break; }
done < <(paths)
if [ -n "$reg" ]; then
  row S6 PASS structural "ruled-out register reached by pointer: $reg"
else
  row S6 FAIL structural "no cited file carries a '## Ruled out' register"
fi

echo
printf 'pass %s - fail %s - advisory %s\n' "$pass" "$fail" "$advis"
[ "$fail" -eq 0 ] && echo "STEPS OK" || echo "STEPS BROKEN"
exit $([ "$fail" -eq 0 ] && echo 0 || echo 1)
