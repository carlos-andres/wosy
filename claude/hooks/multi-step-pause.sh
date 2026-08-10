#!/usr/bin/env bash
# multi-step-pause.sh — UserPromptSubmit hook. Non-blocking.
#
# Per CSI verdict 2026-05-20 action #4:
#   [HOOK] Multi-step pause detector
#
# Scans the incoming user prompt for multi-step markers (and then / after that /
# numbered lists / first…then). On match, injects a system reminder reinforcing
# the multi-step rule in global CLAUDE.md ("list the steps, do step 1, stop").
#
# Bypass paths (silent — no nudge, no log entry):
#   - WOSY_LOOP_BYPASS=1 in the hook's environment (preferred);
#     EDC_LOOP_BYPASS=1 also honored for back-compat with older shells.
#   - Bypass phrases in the prompt: "do all of these without stopping",
#     "without stopping", "no need to stop", "don't pause", "all in one go",
#     "batch this", "whole runtime continue" (mode marker)
#
# Reads JSON event from stdin. Always exits 0 (no blocking).
# Injects context via stdout JSON: {hookSpecificOutput:{additionalContext:...}}.
# Logs each fire to a daily JSONL for trend telemetry.

set -uo pipefail

# detect-platform lib (XDG vs Library log paths). Fail-open stub keeps prior macOS path.
_dp_lib="${HOME}/.claude/hooks/lib/detect-platform.sh"
if [ -r "$_dp_lib" ]; then . "$_dp_lib"; else log_dir_for() { echo "$HOME/Library/Logs/claude-$1"; }; fi
LOG_DIR="$(log_dir_for multi-step)"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/$(date +%Y%m%d).jsonl"

input="$(cat 2>/dev/null || true)"
prompt="$(printf '%s' "$input" | jq -r '.prompt // empty' 2>/dev/null)"
session_id="$(printf '%s' "$input" | jq -r '.session_id // empty' 2>/dev/null)"

# Nothing to scan.
[ -z "$prompt" ] && exit 0

# Bypass: explicit env var. Prefer WOSY_LOOP_BYPASS; fall back to legacy
# EDC_LOOP_BYPASS so users who already set the old name don't lose the bypass.
if [ "${WOSY_LOOP_BYPASS:-${EDC_LOOP_BYPASS:-0}}" = "1" ]; then
  exit 0
fi

# Lower-case for phrase matching.
prompt_lc="${prompt,,}"

# Bypass: phrases the user uses to authorize batched execution.
case "$prompt_lc" in
  *"do all of these without stopping"*) exit 0 ;;
  *"without stopping"*) exit 0 ;;
  *"no need to stop"*) exit 0 ;;
  *"don't pause"*|*"dont pause"*) exit 0 ;;
  *"all in one go"*) exit 0 ;;
  *"batch this"*) exit 0 ;;
  *"whole runtime continue"*) exit 0 ;;
esac

# Short-circuit: if the message starts with `/<word>` and `<word>` is a real skill
# under ~/.claude/skills/<word>/SKILL.md, treat the body as ONE protocol invocation
# (not separate steps). Skill bodies frequently contain numbered protocols and
# first/then narration that misfire all detector branches (W-01 / N-05).
first_token="$(printf '%s' "$prompt" | sed -nE '1s|^[[:space:]]*/([A-Za-z0-9_-]+).*|\1|p')"
if [ -n "$first_token" ]; then
  for skill_dir in "$HOME"/.claude/skills/*/; do
    skill_name="$(basename "$skill_dir")"
    if [ "$skill_name" = "$first_token" ] && [ -f "$skill_dir/SKILL.md" ]; then
      exit 0
    fi
  done
fi

# Detection patterns.
matched_pattern=""

# 1. Connectives.
if printf '%s' "$prompt_lc" | grep -qE '\band then\b|\bafter that\b|\bfollowed by\b|\bonce that.s done\b'; then
  matched_pattern="connective"
fi

# 2. Numbered lists — a numbered list alone is ambiguous: users also number
#    ANSWERS to questions and data enumerations, which are not multi-step
#    requests. Only fire when >=2 items open with an action verb (EN or ES
#    imperative/infinitive), i.e. the list reads as a sequence of commands.
if [ -z "$matched_pattern" ]; then
  action_verbs='(add|adjust|apply|audit|build|bump|change|check|clean|commit|configure|convert|create|delete|deploy|disable|document|edit|enable|extract|find|fix|generate|implement|install|merge|migrate|move|open|push|read|refactor|remove|rename|replace|restart|revert|review|run|scan|set|setup|ship|smoke|split|test|update|upgrade|verify|write|actualiza(r)?|agrega(r)?|ajusta(r)?|anade|anadir|añade|añadir|aplica(r)?|arregla(r)?|audita(r)?|borra(r)?|busca(r)?|cambia(r)?|commitea(r)?|configura(r)?|convierte|convertir|corre(r)?|corrige|corregir|crea(r)?|despliega|desplegar|edita(r)?|ejecuta(r)?|elimina(r)?|escribe|escribir|genera(r)?|implementa(r)?|instala(r)?|lee(r)?|migra(r)?|mueve|mover|prueba|probar|quita(r)?|reemplaza(r)?|refactoriza(r)?|renombra(r)?|revisa(r)?|sube|subir|verifica(r)?)'
  verb_item_count=$(printf '%s\n' "$prompt" | grep -icE "^[[:space:]]*[0-9]+[.)][[:space:]]+${action_verbs}\b" || true)
  if [ "${verb_item_count:-0}" -ge 2 ]; then
    matched_pattern="numbered_list"
  fi
fi

# 3. first … then … (within ~200 chars of each other).
if [ -z "$matched_pattern" ]; then
  if printf '%s' "$prompt_lc" | grep -qE '\bfirst\b.{1,200}\bthen\b'; then
    matched_pattern="first_then"
  fi
fi

# No match → silent.
[ -z "$matched_pattern" ] && exit 0

# Log the fire.
prompt_len=${#prompt}
jq -nc \
  --arg ts "$(date +%Y-%m-%dT%H:%M:%S%z)" \
  --arg session "$session_id" \
  --arg pattern "$matched_pattern" \
  --argjson len "$prompt_len" \
  '{ts:$ts, session:$session, pattern:$pattern, prompt_len:$len}' \
  >> "$LOG" 2>/dev/null || true

# Inject reminder via additionalContext (non-blocking nudge to Claude).
reminder="[multi-step-pause hook] The user message contains multi-step indicators (detected: ${matched_pattern}). Per global CLAUDE.md \"Multi-step requests\" rule: open by listing the steps, complete step 1 only, stop, and wait for explicit confirmation (\"yes\", \"go\", \"continue\") before starting step 2. Silence and \"step 1 succeeded\" are NOT consent. Bypass: user phrase \"do all of these without stopping\" or env var WOSY_LOOP_BYPASS=1."

jq -nc \
  --arg ctx "$reminder" \
  '{hookSpecificOutput:{hookEventName:"UserPromptSubmit", additionalContext:$ctx}}' 2>/dev/null || true

exit 0
