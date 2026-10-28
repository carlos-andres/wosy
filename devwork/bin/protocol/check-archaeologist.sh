#!/opt/homebrew/bin/bash
# Does each recovery command in the graph actually answer its hole type?
#
# `_scratch/grafo-continuidad-20260916.md` assigns a command to five hole types. Nothing had ever
# tested that running it returns the answer. Written 2026-09-16.
#
# Every row carries BOTH a case that must hit and a case that must miss, in the same block. A
# command that always returns something answers nothing.
set -uo pipefail
D=/Users/you/.devwork
ok=0; bad=0
# The miss pattern is generated at run time. A literal written here would be found in this
# very file, and the control would report a false hit against itself. Measured 2026-09-16.
NOPE="absent-$(date +%s)-$$"
row(){ # name | hit-cmd | miss-cmd
  local name="$1" hitc="$2" missc="$3" h m
  h=$(eval "$hitc" 2>/dev/null | wc -l | tr -d ' ')
  m=$(eval "$missc" 2>/dev/null | wc -l | tr -d ' ')
  if [ "$h" -gt 0 ] && [ "$m" -eq 0 ]; then
    printf '%-20s found=%-5s absent=%-5s DISCRIMINA\n' "$name" "$h" "$m"; ok=$((ok+1))
  elif [ "$h" -eq 0 ]; then
    printf '%-20s found=%-5s absent=%-5s FALLA - no halla lo que existe\n' "$name" "$h" "$m"; bad=$((bad+1))
  else
    printf '%-20s found=%-5s absent=%-5s FALLA - responde a lo inexistente\n' "$name" "$h" "$m"; bad=$((bad+1))
  fi
}

row file_discovery \
  "fd -H -t f '^BRUJULA\.md\$' $D" \
  "fd -H -t f '^zzz-no-existe-20260916\.md\$' $D"

row content_search \
  "rg -l 'CLOSURES OK' $D/bin" \
  "rg -l \"\$NOPE\" $D/bin"

row transcript_memory \
  "duckdb -csv -noheader -c \"SELECT json_extract_string(c,'\\\$.input.file_path') FROM (SELECT unnest(json_extract(json,'\\\$.message.content[*]')) c FROM read_ndjson_objects('/Users/you/.claude/projects/**/*.jsonl')) WHERE json_extract_string(c,'\\\$.input.file_path') LIKE '%BRUJULA%' LIMIT 3\"" \
  "duckdb -csv -noheader -c \"SELECT json_extract_string(c,'\\\$.input.file_path') FROM (SELECT unnest(json_extract(json,'\\\$.message.content[*]')) c FROM read_ndjson_objects('/Users/you/.claude/projects/**/*.jsonl')) WHERE json_extract_string(c,'\\\$.input.file_path') LIKE '%\$NOPE%' LIMIT 3\""

# The archaeologist's job is to ANSWER the question, not to make the answer good. An earlier version
# of this row asserted `CHAIN MATCHES`, so a genuinely red chain read as a broken command. Measured
# 2026-09-16 when the chain went red and this control blamed the tool.
# The graph assigned `git status --porcelain` to state_probe. The hub is not a git repository,
# measured 2026-09-16, so that command answers nothing here. The startup chain is the real probe.
row state_probe \
  "cd $D/tasks/handoff-protocol-impl-20260915 && bash bin/run-chain.sh | rg 'CHAIN (MATCHES|DOES NOT MATCH)'" \
  "cd $D/tasks/handoff-protocol-impl-20260915 && bash bin/run-chain.sh | rg \"\$NOPE\""

echo
printf 'discriminan %s - fallan %s\n' "$ok" "$bad"
[ "$bad" -eq 0 ] && echo "ARCHAEOLOGIST OK" || echo "ARCHAEOLOGIST BROKEN"
exit $([ "$bad" -eq 0 ] && echo 0 || echo 1)
