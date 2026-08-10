#!/usr/bin/env bash
# validate-bash.sh — Pre-tool-use hook to block dangerous commands
#
# Reads JSON event from stdin per Claude Code hook contract.
# Exit 0 = allow, exit 2 = block (with stderr message shown to Claude).
# NOTE: exit 1 is ADVISORY only (tool proceeds) — only exit 2 actually blocks.
#
# Blocks:
#   - rm -rf / or near-root paths
#   - dd of= on disk devices
#   - mkfs / fdisk / parted (partitioning)
#   - chmod / chown -R on / or ~
#   - curl | bash / wget | sh patterns
#   - git push --force on protected branches (release, main, master, production)
#   - git reset --hard origin/* outside the current branch

set -euo pipefail

input="$(cat)"

# Extract the command from the event JSON.
# Hook contract: { "tool_name": "Bash", "tool_input": { "command": "..." } }
command="$(printf '%s' "$input" | jq -r '.tool_input.command // empty')"

# Not a bash invocation? Allow.
if [ -z "$command" ]; then
  exit 0
fi

# cwd for per-project flag walk-up. Fail-open if jq is unavailable.
cwd="$(printf '%s' "$input" | jq -r '.cwd // empty' 2>/dev/null || true)"

# Per-project wosy flag reader (shared lib). Fail-open: stub => flags read empty
# => prior behavior. Gates ONLY the convention nudges below; the dangerous-command
# blocks stay unconditional regardless of any flag.
_wosy_lib="${HOME}/.claude/hooks/lib/wosy-flags.sh"
if [ -r "$_wosy_lib" ]; then
  # shellcheck source=/dev/null
  . "$_wosy_lib"
else
  wosy_flag() { :; }
fi

# Helper: block with reason
block() {
  echo "BLOCKED by validate-bash: $1" >&2
  exit 2
}

# Pattern checks (extended regex)
case "$command" in
  *"rm -rf /"*|*"rm -rf /*"*|*"rm -rf ~"*|*"rm -fr /"*|*'rm -rf $HOME'*|*'rm -fr $HOME'*|*'rm -rf ${HOME}'*|*'rm -fr ${HOME}'*)
    block "rm -rf on root or home — refuse" ;;
  *"dd of=/dev/"*|*"dd "*" of=/dev/"*)
    block "dd to disk device — refuse" ;;
  *"mkfs."*|*"mkfs "*)
    block "filesystem creation — refuse" ;;
  *"fdisk "*|*"parted "*|*"sgdisk "*)
    block "partitioning — refuse" ;;
  *"chmod -R 777 /"*|*"chmod -R 777 ~"*)
    block "chmod -R 777 on root or home — refuse" ;;
  *"chown -R "*" /"|*"chown -R "*" ~")
    block "chown -R on root or home — refuse" ;;
esac

# Pipe-to-shell pattern
if printf '%s' "$command" | grep -qE '(curl|wget)[^|]*\|\s*(bash|sh|zsh)\b'; then
  block "pipe-to-shell pattern — refuse"
fi

# git push --force on protected branches
if printf '%s' "$command" | grep -qE 'git\s+push\s+(.*--force|.*-f\b).*\b(release|main|master|production|prod)\b'; then
  block "git push --force on protected branch — refuse"
fi

# git reset --hard to a different branch's tip
if printf '%s' "$command" | grep -qE 'git\s+reset\s+--hard\s+origin/(release|main|master|production|prod)'; then
  block "git reset --hard to protected origin branch — refuse"
fi

# Warn but allow: rm -rf in current directory (relative path)
if printf '%s' "$command" | grep -qE 'rm\s+-rf\s+\.\b'; then
  echo "WARN validate-bash: rm -rf in cwd — allowed but verify scope" >&2
fi

# ---------- Convention nudges (non-blocking) ----------
# CLAUDE.md prescribes rg over grep, fd over find, Read over cat, Edit over sed,
# Write over `echo > file`. Audit (2026-05-15) showed drift: grep=322, find=143,
# cat=140, sed=104 over 10 days. Print a one-line nudge so the next attempt
# defaults to the right tool. Exit 0 — does not block.

# Convention nudges honor the per-project kill switch: wosy_enforce=0 in the
# nearest .devwork/wosy.flags disables them (the find->BLOCK included). The
# dangerous-command blocks above are UNCONDITIONAL and already ran. Fail-open:
# absent flags => nudges on (prior behavior). Also skipped when wrapped by rtk.
if [ "$(wosy_flag wosy_enforce)" != "0" ] && printf '%s' "$command" | grep -qvE '^\s*rtk\b'; then

  # First word of the command (after env-var assignments / cd && )
  head_cmd=$(printf '%s' "$command" | sed -E 's/^[[:space:]]*([A-Z_]+=[^ ]+[[:space:]]+)*//; s/^[[:space:]]*cd[[:space:]]+[^&]+&&[[:space:]]*//; s/^[[:space:]]*//' | awk '{print $1}')

  case "$head_cmd" in
    grep)
      echo "nudge validate-bash: prefer 'rg <pattern>' over grep (respects .gitignore, faster). Allowed." >&2 ;;
    find)
      # 'find . -name X' or 'find <path>' — promoted from nudge to BLOCK 2026-05-20 per CSI verdict #3.
      # Exception: non-search uses like 'find ... -delete', 'find ... -exec', '-print0' xargs pipelines
      # (D1 evidence: find is reached for ~50% of the time despite fd being installed; the habit gap
      # only closes with a hard stop. RTK still wraps these — rtk-prefixed commands skip this block).
      if printf '%s' "$command" | grep -qE '(-delete|-exec\b|-execdir\b|-print0)'; then
        echo "nudge validate-bash: 'find' with -delete/-exec is non-search; allowed. For pure search use 'fd'." >&2
      else
        block "find used for search — use 'fd <pattern>' instead (sane syntax, .gitignore-aware). Promoted from nudge per CSI verdict 2026-05-20."
      fi ;;
    cat)
      # `cat file | something` is fine (pipe usage); flag standalone `cat file` for inspection
      if printf '%s' "$command" | grep -qvE '\|'; then
        echo "nudge validate-bash: prefer Read tool over standalone 'cat <file>' for inspection. Allowed." >&2
      fi ;;
    sed)
      echo "nudge validate-bash: prefer Edit tool over 'sed -i' for file mutations. Allowed." >&2 ;;
    echo)
      # `echo > file` or `echo >> file` — Write tool is the right call
      if printf '%s' "$command" | grep -qE '>>?\s*[^&|]+\.[a-zA-Z]'; then
        echo "nudge validate-bash: prefer Write tool over 'echo > file' for content authoring. Allowed." >&2
      fi ;;
  esac
fi

exit 0
