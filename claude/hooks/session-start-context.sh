#!/usr/bin/env bash
# session-start-context.sh — SessionStart hook. Non-blocking context warm-up.
#
# Cold sessions in satellite repos rediscover their stack and never find the
# team hub: nothing fires on entry (brain-load-trigger needs a prompt hashtag,
# a /brain-load command, or a /projects/<slug>/ cwd — no satellite matches).
# This hook closes that gap at SessionStart by injecting, best-effort:
#
#   1. constitution  — <nearest .devwork>/constitution.md (stack/architecture).
#   2. open tasks    — task folder names under the hub's tasks/ (the hub is
#                      where cross-repo work is tracked; see wosy.yml below).
#   3. ticket match  — `git branch --show-current` matched against
#                      [A-Z]+-[0-9]+ → inject hub tasks/<ticket>/status.md,
#                      so a branch named after a ticket auto-loads its state.
#   4. brain bundle  — `brain load <slug>` startup-pack. Store and slug are
#                      resolved here (wosy.yml `project:` + first */brain.db
#                      under the hub) so no env setup is needed.
#
# wosy.yml — one small file per satellite .devwork/, the hub edge in the graph:
#   hub: /abs/path/to/<team>/.devwork    # absent → this .devwork IS the hub
#   project: <slug>                      # optional; kebab-case [a-z0-9-] only
#
# Fires only on source ∈ {startup, clear}: a resumed session keeps its context
# and a compacted one keeps its summary — re-injecting there is duplication.
#
# After emitting a brain bundle this hook touches the same marker
# brain-load-trigger.sh uses (brain-load-<sid>-<slug>.marker), so the
# UserPromptSubmit trigger does not inject the same bundle again.
#
# Reads JSON event from stdin (.session_id, .cwd, .source). Always exits 0.
# Emits stdout JSON per the SessionStart contract:
#   {hookSpecificOutput:{hookEventName:"SessionStart", additionalContext:...}}
# Zero resolved sections → no output at all.

set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0

input="$(cat 2>/dev/null || true)"
session_id="$(printf '%s' "$input" | jq -r '.session_id // empty' 2>/dev/null)"
cwd="$(printf '%s' "$input" | jq -r '.cwd // empty' 2>/dev/null)"
source_kind="$(printf '%s' "$input" | jq -r '.source // empty' 2>/dev/null)"

[ -n "$cwd" ] || exit 0
case "$source_kind" in startup|clear) ;; *) exit 0 ;; esac

# Nearest .devwork/ walking up from the session cwd (wosy-stop pattern).
devwork_root=""
_d="${cwd%/}"
while [ -n "$_d" ] && [ "$_d" != "/" ]; do
  if [ -d "$_d/.devwork" ]; then devwork_root="$_d/.devwork"; break; fi
  _d="${_d%/*}"
done
[ -n "$devwork_root" ] || exit 0

# wosy.yml: flat key:value lines only — grep/sed, no YAML parser needed.
yml_get() {  # $1=key
  [ -f "$devwork_root/wosy.yml" ] || return 0
  sed -nE "s/^[[:space:]]*$1:[[:space:]]*//p" "$devwork_root/wosy.yml" \
    | head -1 | sed -E 's/[[:space:]]+$//'
}

hub_devwork="$(yml_get hub)"
[ -n "$hub_devwork" ] && [ -d "$hub_devwork" ] || hub_devwork="$devwork_root"
hub_devwork="${hub_devwork%/}"

# Slug constrained to the project.id schema shape, same as brain-load-trigger's
# regex char class — a non-conforming value won't resolve in brain.db and would
# otherwise flow unsanitized into a marker filename.
slug="$(yml_get project)"
printf '%s' "$slug" | grep -qE '^[a-z0-9][a-z0-9-]*$' || slug=""

ctx=""

# ── (1) constitution ──────────────────────────────────────────────────
if [ -f "$devwork_root/constitution.md" ]; then
  ctx="${ctx}## Constitution ($devwork_root/constitution.md)

$(head -150 "$devwork_root/constitution.md")

"
fi

# ── (2) open tasks at the hub ─────────────────────────────────────────
if [ -d "$hub_devwork/tasks" ]; then
  open_tasks="$(find "$hub_devwork/tasks" -mindepth 1 -maxdepth 1 -type d \
    -not -name '_archive' 2>/dev/null | sort | head -10)"
  if [ -n "$open_tasks" ]; then
    ctx="${ctx}## Open task folders at hub ($hub_devwork/tasks/, first 10 alphabetically)

$(printf '%s\n' "$open_tasks" | sed 's|.*/|- |')

"
  fi
fi

# ── (3) branch ticket → hub task status ───────────────────────────────
# Task folders are kebab-lowercase by wosy convention (project.id schema), so
# the JIRA-style uppercase ticket from the branch is folded to match.
branch="$(git -C "$cwd" branch --show-current 2>/dev/null || true)"
ticket="$(printf '%s' "$branch" | grep -oE '[A-Z]+-[0-9]+' | head -1 \
  | tr '[:upper:]' '[:lower:]')"
if [ -n "$ticket" ] && [ -d "$hub_devwork/tasks/$ticket" ]; then
  if [ -f "$hub_devwork/tasks/$ticket/status.md" ]; then
    ctx="${ctx}## Branch \"$branch\" → hub task $hub_devwork/tasks/$ticket/status.md

$(head -120 "$hub_devwork/tasks/$ticket/status.md")

"
  else
    ctx="${ctx}## Branch \"$branch\" → hub task folder $hub_devwork/tasks/$ticket/ (no status.md yet)

"
  fi
fi

# ── (4) brain startup-pack ────────────────────────────────────────────
BRAIN_BIN="${BRAIN_BIN:-$HOME/.local/bin/brain}"
if [ -n "$slug" ] && [ -x "$BRAIN_BIN" ]; then
  # Explicit --store because a cold session has no BRAIN_STORE env (unlike
  # brain-load-trigger, which leans on brain's own resolution). find, not a
  # glob — the hub path comes from wosy.yml and may contain glob metachars.
  # First match alphabetically when a hub holds several team stores.
  store="$(find "$hub_devwork" -mindepth 2 -maxdepth 2 -type f -name brain.db 2>/dev/null \
    | sort | head -1)"
  if [ -n "$store" ]; then
    bundle="$("$BRAIN_BIN" load "$slug" --store="$store" 2>/dev/null)" || bundle=""
    if [ -n "$bundle" ]; then
      ctx="${ctx}## Brain startup-pack (slug=$slug, store=$store)

\`\`\`json
$bundle
\`\`\`

"
      # Suppress brain-load-trigger's duplicate injection for this session+slug.
      if [ -n "$session_id" ]; then
        session_id_safe="${session_id//[^a-zA-Z0-9_-]/_}"
        mkdir -p "$HOME/.claude/state" 2>/dev/null || true
        touch "$HOME/.claude/state/brain-load-${session_id_safe}-${slug}.marker" 2>/dev/null || true
      fi
    fi
  fi
fi

# ── (4b) brain CLI cheat-sheet (kills the --help/--version re-check tax) ─
# Static reference. Two footguns cost 44 re-checks in the audit: `brain <sub>
# --help` RUNS the sub (--help is a positional), and read-side cmds refuse the
# implicit store fallback (R57). Verified against `brain --help` 2026-07-30.
if [ -x "$BRAIN_BIN" ]; then
  cheat_store="${store:-}"
  [ -n "$cheat_store" ] || cheat_store="$(find "$hub_devwork" -mindepth 2 -maxdepth 2 -type f -name brain.db 2>/dev/null | sort | head -1)"
  store_line=""
  [ -n "$cheat_store" ] && store_line="
- This workspace's store: \`$cheat_store\` (pass \`--store=\` to it for read-side cmds)."
  ctx="${ctx}## brain CLI cheat-sheet (avoid the --help/--version re-check tax)

- Version: \`brain version\` — NOT \`brain --version\` (errors 'unknown command').
- Help: top-level \`brain --help\` works; do NOT run \`brain <sub> --help\` — there \`--help\` is a positional and the subcommand RUNS (e.g. \`brain init --help\` migrates the store).
- Store (R57): read-side cmds (\`list\`/\`get\`/\`report\`/\`import\`/\`set\`/\`schema\`) refuse the implicit fallback — pass \`--store=<path>\` or set \`BRAIN_STORE\`. Only \`load\`/\`note\`/\`query\`/\`schema --tables\` follow the \`.devwork/wosy.yml\` \`hub:\` pointer.${store_line}
- Rehydrate: \`brain load <slug>\`. Read SQL: \`brain query \"<SELECT …>\"\` (single SELECT/PRAGMA). Lists: \`brain list [records|todos] [--where status=open]\`. One section: \`brain get --<type>=<id> [--section=<f>]\`. Gotcha: \`brain note <project> \"<text>\" [--severity=info|warn|error]\`.
- Array append (no \`brain append\`): \`get --section=<f>\` → splice → \`set --section=<f> --input=<full-array-json>\`.
- Record types: umbrella · project · task · artifact · edge.

"
fi

# ── (5) recent _scratch findings at the hub ───────────────────────────
# _scratch/ is the universal-write path, so mid-task findings land there and
# a fresh session never sees them. Surface the 3 newest from the last 7 days.
# `find -exec ls -t` gives a portable mtime sort (no GNU stat, no mapfile —
# this hook must run under /bin/bash 3.2 with a minimal PATH).
if [ -d "$hub_devwork/_scratch" ]; then
  recent_md="$(find "$hub_devwork/_scratch" -maxdepth 1 -type f -name '*.md' -mtime -7 \
    -exec ls -t {} + 2>/dev/null | head -3)"
  if [ -n "$recent_md" ]; then
    scratch_lines=""
    while IFS= read -r f; do
      [ -f "$f" ] || continue
      # First markdown heading, else first non-empty line, capped so one
      # runaway draft cannot blow the section budget.
      h="$(grep -m1 '^#' "$f" 2>/dev/null || true)"
      [ -n "$h" ] || h="$(grep -m1 -v '^[[:space:]]*$' "$f" 2>/dev/null || true)"
      h="$(printf '%s' "$h" | cut -c1-100)"
      scratch_lines="${scratch_lines}- ${f##*/} — ${h:-(empty)}
"
    done <<EOF
$recent_md
EOF
    if [ -n "$scratch_lines" ]; then
      ctx="${ctx}## Recent _scratch findings at hub ($hub_devwork/_scratch/, 3 newest of last 7 days)

${scratch_lines}
"
    fi
  fi
fi

[ -n "$ctx" ] || exit 0

ctx="[session-start-context] Wosy context auto-resolved for this session (devwork=$devwork_root, hub=$hub_devwork):

$ctx"

jq -nc --arg ctx "$ctx" \
  '{hookSpecificOutput:{hookEventName:"SessionStart", additionalContext:$ctx}}' 2>/dev/null

exit 0
