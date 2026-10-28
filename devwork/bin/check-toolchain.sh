#!/opt/homebrew/bin/bash
# Toolchain inventory for the continuity work.
#
# Every row ties a tool to a need this corpus measured, not to a preference. A tool with no
# measured need does not belong in this file. Presence is checked with `command -v` only: nothing
# here invokes a database client, because `hooks/db-client-gate.sh` denies that by default.
#
# Exit 0 when every REQUIRED tool resolves. Exit 1 otherwise, after printing the install line.
set -uo pipefail

# name | brew formula | tier | why, tied to a measurement
ROWS=(
  "jq|jq|required|Reads the 873 JSONL transcripts and every judge event log. Used by every instrument here."
  "rg|ripgrep|required|The search path for the whole corpus. The bare grep in a Claude call is ugrep with ignore-file semantics, so rg is the one with predictable flags."
  "fd|fd|required|File discovery. The corpus miner walks 27 project directories with it."
  "python3|python@3.14|required|Every checker under bin/protocol/ is python3 or bash. check-closures.py, check-ledger.py, check-decides.py."
  "sqlite3|sqlite|required|The archaeologist's index. 873 transcripts indexed once answers 'was this command already run' in one query instead of 36 reconstruction calls. Keg-only: confirm it is on PATH."
  "pandoc|pandoc|required|Markdown to AST. The closure checker produced a false positive on 2026-09-16 because a quoted strike-through inside backticks parsed as a real closure. An AST makes that a type check instead of a regex guess."
  "difft|difftastic|required|Structural diff. Inserting text into dist/schema.md on 2026-09-16 moved every line below it and silently broke every schema.md:N citation. difft reports value changes without line-move noise."
  "lychee|lychee|candidate|External URL checker only. Measured 2026-09-16: it finds 0 links in BRUJULA.md because this corpus cites paths in backticked code spans, not markdown link syntax. It is NOT the path checker. pandoc reads code spans and is."
  "check-jsonschema|check-jsonschema|useful|Turns the 8-key frontmatter envelope from 144 lines of bash regex into a declared schema. done_when 3 needs the validation layer under 500 lines and it sits at 1574."
  "watchexec|watchexec|useful|Runs the startup chain on save. CLAUDE.md states hooks are deterministic and advice is not, and the chain went red once today for a citation added without declaring it."
  "yq|yq|useful|Frontmatter is YAML. The 8-key envelope check reads it."
  "qsv|qsv|useful|The hole catalogue and the coverage table are CSV-shaped and large."
  "sd|sd|useful|Literal find-and-replace. sed BSD flags differ from GNU and that has cost repairs here."
  "delta|git-delta|useful|Readable diffs when reviewing a document change."
  "bat|bat|useful|Paged reads with syntax, cheaper than dumping a file into context."
  "hyperfine|hyperfine|useful|Required by the measurement rule: measure the old version warm, first run discarded. The 7.05s per-turn baseline was taken that way."
  "tokei|tokei|useful|Line counts by language. done_when 3 measures the validation layer and wc -l over a glob is fragile."
  "fzf|fzf|useful|Interactive selection when the conductor must pick from many task folders."
  "uv|uv|useful|Fast python environments if any instrument grows a dependency."
  "duckdb|duckdb|candidate|Queries JSONL in place with SQL, no index build. The alternative to sqlite3 for one-off archaeology."
  "fastgron|fastgron|useful|C++ reimplementation of gron. On the 873-file corpus the size difference matters, so the archaeologist calls this one."
  "gron|gron|useful|Flattens JSON to greppable lines. Makes rg usable on transcripts without writing a jq expression first."
  "mlr|miller|candidate|Record-stream processing. The hole catalogue is a record stream."
  "jd|jd|candidate|Structural JSON diff. difft covers text, this covers the judge event logs."
  "ast-grep|ast-grep|candidate|Structural search in code, for when rg patterns stop being enough."
)

printf '%-10s %-8s %-14s %s\n' TOOL STATUS FORMULA WHY
printf '%s\n' "------------------------------------------------------------------------"
missing_required=(); missing_optional=(); fail=0
for row in "${ROWS[@]}"; do
  IFS='|' read -r name formula tier why <<< "$row"
  if command -v "$name" >/dev/null 2>&1; then
    status="OK"
  else
    status="MISSING"
    case "$tier" in
      required) missing_required+=("$formula"); fail=1 ;;
      *)        missing_optional+=("$formula") ;;
    esac
  fi
  printf '%-10s %-8s %-14s %s\n' "$name" "$status" "$formula" "${why:0:96}"
done

echo
if [ ${#missing_required[@]} -gt 0 ]; then
  echo "REQUIRED missing — the design depends on these:"
  echo "  brew install ${missing_required[*]}"
fi
if [ ${#missing_optional[@]} -gt 0 ]; then
  echo "OPTIONAL missing — install what the next measurement asks for, not all of it:"
  echo "  brew install ${missing_optional[*]}"
fi
[ $fail -eq 0 ] && echo "TOOLCHAIN OK — every required tool resolves." || echo "TOOLCHAIN INCOMPLETE"
exit $fail
