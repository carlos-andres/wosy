#!/bin/bash
# wosy-metrics.sh — answers "which hooks/skills actually fire, and which are dead?"
# Reads ~/.claude/projects/*/*.jsonl (hook_success/error attachments carry .command;
# Skill tool_use + <command-name> tags carry skills), ~/.claude/state/*.marker, and
# ~/Library/Logs/claude-*/<YYYYMMDD>.jsonl. No new logging required.

set -euo pipefail

PROJECTS_DIR="$HOME/.claude/projects"
STATE_DIR="$HOME/.claude/state"
LOGS_DIR="$HOME/Library/Logs"
SETTINGS="$HOME/.claude/settings.json"
SKILLS_DIR="$HOME/.claude/skills"

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(dirname "$(dirname "$SCRIPT_DIR")")
# Deployed copies (~/.local/bin) can't derive the repo from $0; install.sh pins the
# REPO_ROOT line above at deploy time, WOSY_REPO is the manual override. Empty after
# both fallbacks = origin column shows "?" instead of guessing "user".
[ -d "$REPO_ROOT/claude/hooks" ] || REPO_ROOT="${WOSY_REPO:-}"
[ -n "$REPO_ROOT" ] && [ -d "$REPO_ROOT/claude/hooks" ] || REPO_ROOT=""

usage() {
  cat <<'EOF'
Usage: wosy-metrics.sh [--days N] [--adoption [N]] [--json]

Default mode    fire counts per component (hook/skill) over the last N days
                (--days, default 30), one row per evidence source.
--adoption [N]  zero-fire components over the last N days (default 14) =
                retire candidates. Universe: hooks in ~/.claude/settings.json
                + skills in ~/.claude/skills/. Origin column marks wosy
                (present in repo claude/hooks|skills) vs user.
--json          machine-readable output instead of the table.

Caveat: SessionEnd hooks (and PreToolUse hooks that pass silently) leave no
transcript trace — a zero there means "no evidence", not "dead".
EOF
}

command -v jq >/dev/null 2>&1 || { echo "wosy-metrics: jq is required but not on PATH" >&2; exit 1; }

DAYS=30
ADOPTION=0
ADOPTION_DAYS=14
JSON=0

while [ $# -gt 0 ]; do
  case "$1" in
    --days)
      shift
      case "${1:-}" in (*[!0-9]*|'') echo "wosy-metrics: --days needs a number" >&2; exit 1;; esac
      DAYS=$1 ;;
    --adoption)
      ADOPTION=1
      case "${2:-}" in (''|*[!0-9]*) : ;; (*) ADOPTION_DAYS=$2; shift ;; esac ;;
    --json) JSON=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "wosy-metrics: unknown option: $1" >&2; usage >&2; exit 1 ;;
  esac
  shift
done

TMP=$(mktemp -d "${TMPDIR:-/tmp}/wosy-metrics.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

# First two dash-segments identify a component family; markers and log dirs
# carry suffixes (session id, project, ctx60) the hook basename does not.
family() { printf '%s\n' "$1" | cut -d- -f1-2; }

# Emits "kind<TAB>component<TAB>source" lines, one per observed fire.
collect_fires() {
  days=$1
  out=$2
  cutoff_epoch=$(( $(date +%s) - days * 86400 ))
  cutoff_iso=$(jq -rn --argjson e "$cutoff_epoch" '$e | todate')
  cutoff_ymd=$(printf '%s' "$cutoff_iso" | cut -c1-4,6-7,9-10)
  : > "$out"

  # Transcripts. grep prefilter keeps jq off the ~99% of lines with no signal.
  find "$PROJECTS_DIR" -name '*.jsonl' -type f -mtime -"$days" -print0 2>/dev/null \
    | xargs -0 grep -h -e '"hook_' -e '"Skill"' -e '<command-name>' 2>/dev/null \
    | jq -Rr --arg cut "$cutoff_iso" '
        fromjson? // empty
        | select((.timestamp // "") >= $cut)
        | (
            ( select(.type == "attachment")
              | .attachment | select(type == "object" and .command != null)
              | "hook\t\(.command | sub(".*/"; "") | sub("\\.sh$"; ""))\ttranscript" ),
            ( select(.type == "assistant")
              | .message.content[]?
              | select(.type == "tool_use" and .name == "Skill")
              | "skill\t\(.input.skill // .input.command // "unknown")\tskill-tool" ),
            ( select(.type == "user")
              | ( .message.content
                  | if type == "string" then . else ([.[]? | .text? // ""] | join(" ")) end )
              | scan("<command-name>/([a-zA-Z0-9:_-]+)</command-name>")
              | "skill\t\(.[0])\tslash-cmd" )
          )
      ' >> "$out" || true

  # Slash-command tags include built-ins (/clear, /model); only installed skills count.
  if [ -s "$out" ]; then
    awk -F'\t' -v sd="$SKILLS_DIR" '
      $3 == "slash-cmd" { if (system("test -d \"" sd "/" $2 "\"") != 0) next }
      { print }
    ' "$out" > "$out.f" && mv "$out.f" "$out"
  fi

  # State markers: the only trace for hooks that print nothing to the session
  # (context-checkpoint ctx60/85, brain-load-trigger, vigilante emergency path).
  find "$STATE_DIR" -maxdepth 1 -name '*.marker' -type f -mtime -"$days" 2>/dev/null \
    | while IFS= read -r m; do
        b=$(basename "$m" .marker)
        comp=$(printf '%s\n' "$b" \
          | sed -E 's/-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}//')
        printf 'hook\t%s\tstate-marker\n' "$comp"
      done >> "$out"

  # Per-hook jsonl logs under ~/Library/Logs/claude-<name>/<YYYYMMDD>.jsonl.
  for d in "$LOGS_DIR"/claude-*/; do
    [ -d "$d" ] || continue
    comp=$(basename "$d" | sed 's/^claude-//')
    for f in "$d"*.jsonl; do
      [ -f "$f" ] || continue
      ymd=$(basename "$f" .jsonl)
      case "$ymd" in
        [0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9])
          if [ "$ymd" -ge "$cutoff_ymd" ]; then
            awk -v comp="$comp" 'END { for (i = 0; i < NR; i++) print "hook\t" comp "\tlibrary-log" }' "$f"
          fi ;;
      esac
    done
  done >> "$out"
}

# "kind<TAB>component<TAB>source" -> "kind<TAB>component<TAB>source<TAB>count"
aggregate() {
  sort "$1" | uniq -c \
    | awk '{ c=$1; sub(/^ *[0-9]+ /, ""); print $0 "\t" c }' \
    | sort -t"$(printf '\t')" -k4,4rn
}

if [ "$ADOPTION" -eq 0 ]; then
  collect_fires "$DAYS" "$TMP/fires.tsv"
  aggregate "$TMP/fires.tsv" > "$TMP/counts.tsv"

  if [ "$JSON" -eq 1 ]; then
    jq -Rn --argjson days "$DAYS" '
      { days: $days,
        components: [ inputs | split("\t")
          | { kind: .[0], component: .[1], source: .[2], fires: (.[3] | tonumber) } ] }
    ' < "$TMP/counts.tsv"
  else
    echo "wosy-metrics — fires per component, last $DAYS days"
    echo
    printf '%-7s %-38s %-14s %6s\n' KIND COMPONENT SOURCE FIRES
    printf '%-7s %-38s %-14s %6s\n' ----- --------- ------ -----
    if [ -s "$TMP/counts.tsv" ]; then
      while IFS="$(printf '\t')" read -r kind comp src n; do
        printf '%-7s %-38s %-14s %6s\n' "$kind" "$comp" "$src" "$n"
      done < "$TMP/counts.tsv"
    else
      echo "(no fires found)"
    fi
    echo
    echo "sources: transcript=~/.claude/projects jsonl · state-marker=~/.claude/state ·"
    echo "         library-log=~/Library/Logs/claude-* · skill-tool/slash-cmd=transcript"
    echo "note: one component can show several rows (one per source); rows are not deduped."
  fi
  exit 0
fi

# --adoption -------------------------------------------------------------

collect_fires "$ADOPTION_DAYS" "$TMP/fires.tsv"
aggregate "$TMP/fires.tsv" > "$TMP/counts.tsv"

# Hook universe from settings.json: "basename<TAB>events" (events comma-joined).
jq -r '
  .hooks // {} | to_entries[] | .key as $ev | .value[] | .hooks[]?
  | select(.command | endswith(".sh"))
  | "\(.command | sub(".*/"; "") | sub("\\.sh$"; ""))\t\($ev)"
' "$SETTINGS" 2>/dev/null | sort -u \
  | awk -F'\t' '
      { if ($1 in ev) ev[$1] = ev[$1] "," $2; else { ev[$1] = $2; order[++n] = $1 } }
      END { for (i = 1; i <= n; i++) print order[i] "\t" ev[order[i]] }
    ' > "$TMP/hooks.tsv"

: > "$TMP/skills.tsv"
for d in "$SKILLS_DIR"/*/; do
  [ -d "$d" ] || continue
  name=$(basename "$d")
  case "$name" in .*) continue ;; esac
  printf '%s\n' "$name" >> "$TMP/skills.tsv"
done

hook_fires() {
  fam=$(family "$1")
  awk -F'\t' -v fam="$fam" '
    $1 == "hook" { split($2, p, "-"); f = p[1]; if (p[2] != "") f = f "-" p[2]
                   if (f == fam) s += $4 }
    END { print s + 0 }
  ' "$TMP/counts.tsv"
}

skill_fires() {
  awk -F'\t' -v name="$1" '$1 == "skill" && $2 == name { s += $4 } END { print s + 0 }' "$TMP/counts.tsv"
}

: > "$TMP/adoption.tsv"
while IFS="$(printf '\t')" read -r comp events; do
  n=$(hook_fires "$comp")
  if [ -z "$REPO_ROOT" ]; then origin='?'
  elif [ -f "$REPO_ROOT/claude/hooks/$comp.sh" ]; then origin=wosy
  else origin=user; fi
  note=-
  case ",$events," in
    *,SessionEnd,*) case "$events" in SessionEnd) note="SessionEnd-only: untraceable" ;; esac ;;
  esac
  printf 'hook\t%s\t%s\t%s\t%s\t%s\n' "$comp" "$origin" "$n" "$events" "$note" >> "$TMP/adoption.tsv"
done < "$TMP/hooks.tsv"

while IFS= read -r name; do
  n=$(skill_fires "$name")
  if [ -z "$REPO_ROOT" ]; then origin='?'
  elif [ -d "$REPO_ROOT/claude/skills/$name" ]; then origin=wosy
  else origin=user; fi
  printf 'skill\t%s\t%s\t%s\t-\t-\n' "$name" "$origin" "$n" >> "$TMP/adoption.tsv"
done < "$TMP/skills.tsv"

if [ "$JSON" -eq 1 ]; then
  jq -Rn --argjson days "$ADOPTION_DAYS" '
    { days: $days,
      components: [ inputs | split("\t")
        | { kind: .[0], component: .[1], origin: .[2], fires: (.[3] | tonumber),
            events: (if .[4] == "-" then null else .[4] end),
            note:   (if .[5] == "-" then null else .[5] end) } ] }
  ' < "$TMP/adoption.tsv"
else
  echo "wosy-metrics --adoption — components with ZERO fires in the last $ADOPTION_DAYS days"
  echo
  printf '%-7s %-26s %-6s %-30s %s\n' KIND COMPONENT ORIGIN EVENTS NOTE
  printf '%-7s %-26s %-6s %-30s %s\n' ----- --------- ------ ------ ----
  zero=0
  while IFS="$(printf '\t')" read -r kind comp origin n events note; do
    if [ "$n" -eq 0 ]; then
      zero=$((zero + 1))
      printf '%-7s %-26s %-6s %-30s %s\n' "$kind" "$comp" "$origin" "$events" "$note"
    fi
  done < "$TMP/adoption.tsv"
  [ "$zero" -eq 0 ] && echo "(none — every registered component fired at least once)"
  echo
  echo "active components (fires > 0):"
  while IFS="$(printf '\t')" read -r kind comp origin n events note; do
    [ "$n" -gt 0 ] && printf '  %-7s %-26s %-6s %5s fires\n' "$kind" "$comp" "$origin" "$n"
  done < "$TMP/adoption.tsv"
  echo
  echo "caveat: SessionEnd-only hooks leave no transcript trace; silent-pass PreToolUse"
  echo "hooks may too. Zero = no evidence, not proof of death."
fi
