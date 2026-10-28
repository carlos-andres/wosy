#!/usr/bin/env bash
# skill-consistency.sh — SessionStart hook. Non-blocking.
#
# CLAUDE.md names slash commands as if they were all live and all firable. Three things can make
# that false, and none of them is visible from the prose:
#
#   1. the skill is not on disk at all                     -> DANGLING
#   2. settings.json .skillOverrides sets it to "off"      -> OFF-UNMARKED
#   3. its SKILL.md sets disable-model-invocation          -> MODEL-BLIND
#
# Case 3 is the one that reads as healthy and is not: the user can type the command, the model
# cannot fire it, so a rule that says "fire /x before doing y" is an instruction the model cannot
# follow. A checker that looks only at disk and at skillOverrides reports such a token as fine.
#
# A finding is suppressed when any citing line already says so. The recognised annotations are the
# ones CLAUDE.md already uses: "OFF in settings.json"/"skillOverrides" for case 2, "Manual-only" /
# "never automatic" / "cannot self-invoke" / "user-invocable" for case 3, and "is not a skill" for a
# token the prose deliberately names as absent.
#
# LIMITS, so the next reader inherits them rather than discovering them:
#   - BUILTINS below is a literal list. Claude Code does not expose its built-in commands on disk,
#     so a newly shipped built-in reads as DANGLING until someone adds it here.
#   - Annotation is judged per TOKEN, not per line. A token annotated on any one line counts as
#     annotated everywhere. A stale incidental mention beside a correct one is therefore NOT caught.
#   - Only a backtick-delimited `/name` followed by a backtick or a space is seen, so
#     `/consolidate <id>` counts and `/usr/bin/grep` does not. The built-in skills that CLAUDE.md
#     names as bare backticked words with no leading slash are invisible to this check.
#   - It reads $CLAUDE_MD and $RULES_DIR/*.md. A citation in a project CLAUDE.md, a skill body or
#     any other file is still not examined. Each finding names its own file, so a rule-file
#     citation is told apart from a CLAUDE.md one by the `basename:line` in the finding.
#   - MODEL-BLIND reads only the first 40 lines of a SKILL.md, which is where frontmatter lives.
#     A `disable-model-invocation` key below line 40 is not seen.
#
# Fires only on source in {startup, clear}, matching session-start-context.sh: a resumed or
# compacted session already carries this context. Per the hooks reference, `source` also takes
# `resume` and `compact`, and a known defect drops additionalContext entirely on `fork`.
#
# Reads the JSON event from stdin (.source). Always exits 0. Emits stdout JSON per the SessionStart
# contract: {hookSpecificOutput:{hookEventName:"SessionStart", additionalContext:...}}
# Zero findings -> no output at all.
#
# Paths are overridable so tests/test-skill-consistency.sh can run it against fixtures.
#
# CHANGES vs the pre-2026-09-17 version, both externally visible only in the cases named:
#   - additionalContext is capped at 10000 characters, the documented hook-output limit. Past it
#     Claude Code writes the text to a file and hands over a path instead, which turns an
#     at-a-glance warning into a file to go open. The report now truncates itself and says how
#     many findings it withheld. Measured before the change: 60 findings produced 17735 chars.
#   - The per-token work no longer spawns processes. One perl pass collects every citation with
#     its line text, and the loop matches with bash's own regex engine. Needs bash 4+, and
#     fails open like the other guards when it is missing.

set -uo pipefail
shopt -u nocasematch

command -v jq   >/dev/null 2>&1 || exit 0
command -v perl >/dev/null 2>&1 || exit 0
# Associative arrays are the whole point of the single-pass rewrite. Fail open without them.
[[ "${BASH_VERSINFO[0]:-0}" -ge 4 ]] || exit 0

# The home this session is actually running in. Hardcoding $HOME/.claude made this hook read the
# wrong CLAUDE.md whenever a session ran against another config directory, which is exactly the
# drift it exists to catch. Measured 2026-09-18: in a session on ~/.test_claude it was checking
# ~/.claude. Under the default home CLAUDE_CONFIG_DIR is unset and the fallback is the same path,
# so nothing changes there. The four per-variable overrides below still win over both.
CC_HOME="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"

CLAUDE_MD="${SKILL_CONSISTENCY_CLAUDE_MD:-$CC_HOME/CLAUDE.md}"
SKILLS_DIR="${SKILL_CONSISTENCY_SKILLS_DIR:-$CC_HOME/skills}"
SETTINGS="${SKILL_CONSISTENCY_SETTINGS:-$CC_HOME/settings.json}"
RULES_DIR="${SKILL_CONSISTENCY_RULES_DIR:-$CC_HOME/rules}"

# The documented cap on a hook output string. Past it Claude Code spills to a file.
MAX_CONTEXT="${SKILL_CONSISTENCY_MAX_CONTEXT:-10000}"

input="$(cat 2>/dev/null || true)"
source_kind="$(printf '%s' "$input" | jq -r '.source // empty' 2>/dev/null)"
case "$source_kind" in startup|clear) ;; *) exit 0 ;; esac

[[ -f "$CLAUDE_MD" ]] || exit 0
[[ -d "$SKILLS_DIR" ]] || exit 0

# Filesystem paths that the token regex cannot tell apart from a command.
PATH_DENY=" tmp usr bin opt etc var dev srv home root Users private sbin lib mnt proc sys "

# Claude Code built-ins. Not on disk, therefore never DANGLING. See LIMITS above.
BUILTINS=" clear compact context config help init loop schedule run design dataviz simplify code-review security-review update-config keybindings-help claude-api fewer-permission-prompts artifact-design artifact-diagramming artifact-capabilities agents model memory review resume status cost doctor vim terminal-setup export permissions hooks mcp ide privacy-settings release-notes bug upgrade login logout add-dir "

overrides="$(jq -r '.skillOverrides // {} | to_entries[] | "\(.key)=\(.value)"' "$SETTINGS" 2>/dev/null || true)"

# One definition of what counts as a citation. Both scans interpolate it, so they cannot drift.
CITE_RE='\x60(\/[a-z][a-zA-Z0-9_-]*)(?=[\x60\s])'

# Single pass. Emits token, line number, and the full text of the citing line, unit-separated so
# a tab or a pipe inside the prose cannot split a field. The previous version re-read CLAUDE.md
# with a fresh perl for every token, and ran awk and four greps per token on top of that.
US=$'\x1f'

# Both roots, one pass. A rule file loads only for its `paths` globs, but a citation there names
# the same installed surface, so it gets the same checks. `close ARGV if eof` restarts $. per file,
# or every line number after the first file is an offset into the concatenation and names no line.
SCAN_FILES=("$CLAUDE_MD")
if [[ -d "$RULES_DIR" ]]; then
  for _rf in "$RULES_DIR"/*.md; do [[ -f "$_rf" ]] && SCAN_FILES+=("$_rf"); done
fi

citations="$(CITE_RE="$CITE_RE" perl -ne '
  BEGIN { $re = qr/$ENV{CITE_RE}/ }
  my $line = $_; chomp $line;
  my %once;
  while (/$re/g) { my $t = $1; $t =~ s{^/}{}; next if $once{$t}++; print "$t\x1f$ARGV\x1f$.\x1f$line\n" }
  close ARGV if eof;
' "${SCAN_FILES[@]}" 2>/dev/null || true)"

# A citation carries its own file now, because one token can appear in CLAUDE.md and in a rule
# file. A bare line number would name a line in whichever file the reader assumed.
declare -A CITE_LINES=() CITE_TEXT=() SEEN=()
ORDER=()
while IFS="$US" read -r tok file lineno linetext; do
  [[ -n "${tok:-}" ]] || continue
  if [[ -z "${SEEN[$tok]:-}" ]]; then SEEN["$tok"]=1; ORDER+=("$tok"); fi
  CITE_LINES["$tok"]="${CITE_LINES[$tok]:+${CITE_LINES[$tok]},}${file##*/}:$lineno"
  CITE_TEXT["$tok"]="${CITE_TEXT[$tok]:-}${linetext}"$'\n'
done <<< "$citations"

# The RULES-CITE tripwire that stood here fired on 2026-09-19 for `/mcp` in rules/browser.md, which
# is what it was built to detect. The scan above now covers $RULES_DIR, so the tripwire has no
# remaining job and a rule-file citation gets DANGLING/OFF-UNMARKED/MODEL-BLIND like any other.
if [[ "${#ORDER[@]}" -eq 0 ]]; then exit 0; fi

findings=""
finding_count=0

for name in "${ORDER[@]}"; do
  [[ "$PATH_DENY" == *" $name "* ]] && continue
  [[ "$BUILTINS"  == *" $name "* ]] && continue

  cites="${CITE_LINES[$name]}"
  ctx="${CITE_TEXT[$name]}"

  skill_md="$SKILLS_DIR/$name/SKILL.md"
  on_disk=0; [[ -f "$skill_md" ]] && on_disk=1

  override=""
  case "$overrides" in
    *"$name=off"*)                 override=off ;;
    *"$name=user-invocable-only"*) override=user-invocable-only ;;
    *"$name=name-only"*)           override=name-only ;;
  esac

  # Frontmatter only. mapfile is a builtin, so this costs no process.
  model_blind=0
  if [[ "$on_disk" = 1 ]]; then
    head_lines=()
    mapfile -t -n 40 head_lines < "$skill_md" 2>/dev/null || true
    for hl in "${head_lines[@]}"; do
      [[ "$hl" == disable-model-invocation:* ]] && { model_blind=1; break; }
    done
  fi

  ann_absent=0; [[ "$ctx" == *"is not a skill"* ]] && ann_absent=1
  ann_off=0;    [[ "$ctx" =~ (OFF\ in\ settings\.json|skillOverrides) ]] && ann_off=1
  shopt -s nocasematch
  ann_manual=0; [[ "$ctx" =~ (manual-only|manual\ \`|never\ automatic|cannot\ self-invoke|user-invocable|not\ a\ skill) ]] && ann_manual=1
  shopt -u nocasematch

  if   [[ "$on_disk" = 0 && "$ann_absent" = 0 ]]; then
    findings="$findings  DANGLING      /$name  cited at $cites — no $SKILLS_DIR/$name/SKILL.md and not a known built-in.
"
    finding_count=$((finding_count + 1))
  elif [[ "$override" = off && "$ann_off" = 0 ]]; then
    findings="$findings  OFF-UNMARKED  /$name  cited at $cites — settings.json .skillOverrides sets it to \"off\". No citing line says so.
"
    finding_count=$((finding_count + 1))
  elif [[ "$model_blind" = 1 && "$ann_manual" = 0 && "$ann_off" = 0 ]]; then
    findings="$findings  MODEL-BLIND   /$name  cited at $cites — SKILL.md sets disable-model-invocation, so the user can type it and the model cannot fire it. No citing line says so.
"
    finding_count=$((finding_count + 1))
  fi
done

[[ -n "$findings" ]] || exit 0

# One section over both roots. Every finding names its own file, so the count needs no split.
report="[skill-consistency] $finding_count finding(s) against $CLAUDE_MD and $RULES_DIR/*.md
$findings
Each one is drift between the rules file and what is actually installed. Repair the prose or the
frontmatter, not this hook. Sources read: $CLAUDE_MD, $RULES_DIR/*.md, $SKILLS_DIR/*/SKILL.md,
$SETTINGS .skillOverrides."

# Respect the documented output cap. Past it Claude Code writes the text to a file and hands over
# a path, which turns an at-a-glance warning into an errand. Truncating on a finding boundary and
# saying how many were withheld keeps the first findings actionable and the count honest.
if [[ "${#report}" -gt "$MAX_CONTEXT" ]]; then
  total=$((finding_count + ${tcount:-0}))
  # Drop one line at a time until the kept text AND its truncation notice both fit. Reserving a
  # fixed head-room does not work: the notice carries counts, so its own length is not known
  # until the cut is made. Measured failing that way at 10203 characters against a 10000 cap.
  kept="$report"
  notice=""
  while [[ -n "$kept" ]]; do
    kept="${kept%$'\n'*}"
    shown=0
    while IFS= read -r _kl; do
      case "$_kl" in
        "  DANGLING"*|"  OFF-UNMARKED"*|"  MODEL-BLIND"*|"  RULES-CITE"*) shown=$((shown + 1)) ;;
      esac
    done <<< "$kept"
    notice="
  ... TRUNCATED. Showing $shown of $total finding(s). A hook output string is capped at
  $MAX_CONTEXT characters, and past that Claude Code writes it to a file and hands over a path
  instead of showing it. For the full list, run the hook by hand with
  SKILL_CONSISTENCY_MAX_CONTEXT=999999 and source startup."
    [[ $(( ${#kept} + ${#notice} )) -le "$MAX_CONTEXT" ]] && break
  done
  report="$kept$notice"
fi

jq -nc --arg ctx "$report" \
  '{hookSpecificOutput:{hookEventName:"SessionStart", additionalContext:$ctx}}'
exit 0
