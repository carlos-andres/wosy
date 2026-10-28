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

# The per-project flag reader this hook used to source does not travel with the clean install.
# Sourcing an absent library left the flag reading empty, and empty is not "0", so the nudges
# and the `find` block were ON by accident rather than by decision. The gate is removed and the
# nudges are unconditional. The dangerous-command blocks were never gated.

# Helper: block with reason
block() {
  echo "BLOCKED by validate-bash: $1" >&2
  exit 2
}

# Pattern checks (extended regex)
# The target must be root or home itself, not a path under it: "rm -rf /tmp/x" matched the old
# substring glob (factory logbook 26, H6).
RM_ROOT='rm[[:space:]]+-(rf|fr)[[:space:]]+"?(/|/\*|~|~/|~/\*|\$HOME|\$\{HOME\})"?([[:space:];&|)]|$)'
[[ "$command" =~ $RM_ROOT ]] && block "rm -rf on root or home — refuse"
case "$command" in
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

# Any push, whatever precedes it: git -C <dir> push, /usr/bin/git push, command git push
if printf '%s' "$command" | grep -qE '(^|[[:space:];&|(])([^[:space:]]*/)?git([[:space:]]+(-C|-c)[[:space:]]+[^[:space:]]+|[[:space:]]+--[^[:space:]]+)*[[:space:]]+push([[:space:]]|$)'; then
  block "git push — the owner pushes, never an agent"
fi

# git reset --hard to a different branch's tip
if printf '%s' "$command" | grep -qE 'git\s+reset\s+--hard\s+origin/(release|main|master|production|prod)'; then
  block "git reset --hard to protected origin branch — refuse"
fi


# ---------- ssh is read-only by default ----------
# CLAUDE.md, "Direct execution patterns" and "Wosy operating rules": a prod-touching tool is
# read-only by default, and a mutation is confirmed in chat first, even when the user already said
# go. A hook cannot ask a question. Stopping the call is the only way it has to get the human asked,
# so this blocks rather than warns.
#
# Scope. The command is split on `;` and `|`, and each piece is judged from its first `ssh` token
# onward. So `cd /x && ssh prod "rm /tmp/y"` is judged, `mkdir /tmp/x && ssh prod "ls"` is not, and
# `wc -l ~/.ssh/config` is not an invocation at all because `.ssh/` carries no word boundary.
#
# Two boundaries, both deliberate and both covered by a case in tests/test-validate-bash-ssh.sh.
#
#   A pipe ends a piece. That keeps `ssh host "tail log" | grep ERROR` working, and it means a
#   mutation behind a pipe INSIDE the remote command is not seen. Recorded as a KNOWN GAP case.
#
#   A `&&` does not end a piece, so `ssh prod "ls" && rm -rf ./local` blocks on the LOCAL rm. That
#   is the conservative direction and it is preferred to missing `cd /x && ssh prod "rm ..."`.
#
# This is a deny-list and a deny-list is never complete. It names the shapes that mutate a host in
# practice. A shape it misses is a gap to add, not a licence.
SSH_MUTATES='(^|[^[:alnum:]_])(rm|rmdir|mv|cp|chmod|chown|chgrp|mkdir|touch|truncate|dd|tee|ln)[[:space:]]'
SSH_MUTATES="$SSH_MUTATES"'|(^|[^[:alnum:]_])(sed|perl)[[:space:]]+-i'
SSH_MUTATES="$SSH_MUTATES"'|(^|[^[:alnum:]_])(systemctl|service)[[:space:]].*(start|stop|restart|reload|enable|disable|mask)'
SSH_MUTATES="$SSH_MUTATES"'|(^|[^[:alnum:]_])(apt|apt-get|yum|dnf|apk|brew)[[:space:]]+(install|remove|purge|upgrade)'
SSH_MUTATES="$SSH_MUTATES"'|(^|[^[:alnum:]_])(npm|pnpm|yarn|composer|pip|pip3)[[:space:]]+(install|update|remove|uninstall)'
SSH_MUTATES="$SSH_MUTATES"'|(^|[^[:alnum:]_])(kill|pkill|killall)[[:space:]]'
SSH_MUTATES="$SSH_MUTATES"'|(^|[^[:alnum:]_])git[[:space:]]+(push|pull|reset|checkout|clean|merge|rebase|commit|stash)'
SSH_MUTATES="$SSH_MUTATES"'|(INSERT|UPDATE|DELETE|DROP|ALTER|TRUNCATE|GRANT|REVOKE)[[:space:]]'
SSH_MUTATES="$SSH_MUTATES"'|(^|[^[:alnum:]_])redis-cli[[:space:]].*(SET|DEL|FLUSHALL|FLUSHDB|EXPIRE|RENAME)'
SSH_MUTATES="$SSH_MUTATES"'|(^|[^[:alnum:]_])artisan[[:space:]]+(migrate|db:seed|[a-z]+:clear|[a-z]+:cache|queue:restart|down|up)'
SSH_MUTATES="$SSH_MUTATES"'|(^|[^[:alnum:]_])crontab[[:space:]]'

ssh_tails="$(printf '%s' "$command" | awk '
  { n = split($0, s, /;|\|/)
    for (i = 1; i <= n; i++)
      if (match(s[i], /(^|[ \t;&(])ssh[ \t]/)) print substr(s[i], RSTART) }')"

if [ -n "$ssh_tails" ]; then
  while IFS= read -r ssh_tail; do
    [ -n "$ssh_tail" ] || continue
    if printf '%s' "$ssh_tail" | grep -qE "$SSH_MUTATES"; then
      block "ssh would mutate the remote host, and a prod-touching tool is read-only by default. Get the user's confirmation, then ask them to run it themselves with a leading '!' in the prompt."
    fi
    # A redirection into a real file is a mutation. `2>/dev/null`, `>/dev/null` and `2>&1` are not,
    # so they are removed before the test rather than special-cased inside it.
    ssh_tail_nr="$(printf '%s' "$ssh_tail" | sed -E 's/[0-9]*>>?[[:space:]]*\/dev\/null//g; s/[0-9]*>&[0-9]//g')"
    case "$ssh_tail_nr" in
      *">"*) block "ssh would redirect into a file on the remote host, and a prod-touching tool is read-only by default. Get the user's confirmation, then ask them to run it themselves with a leading '!' in the prompt." ;;
    esac
  done <<SSH_TAILS
$ssh_tails
SSH_TAILS
fi
# Warn but allow: rm -rf in current directory (relative path)
if printf '%s' "$command" | grep -qE 'rm\s+-rf\s+\.\b'; then
  echo "WARN validate-bash: rm -rf in cwd — allowed but verify scope" >&2
fi

# ---------- Convention nudges (non-blocking) ----------
# CLAUDE.md prescribes rg over grep, fd over find, Read over cat, Edit over sed,
# THE PREFERENCE NUDGES WERE REMOVED ON 2026-09-18, and the measurement is why.
#
# Over the 20,571 unique commands of the 21,032-command corpus this hook spoke on 2,902 and
# BLOCKED 35. The other 2,867 were four nudges: prefer Edit over `sed -i` (1,477), prefer Read
# over `cat` (553), prefer `rg` over `grep` (450), prefer Write over `echo >` (303).
#
# All four are already instructed in writing by the CLAUDE.md this install ships, and the prose
# travels. A runtime reminder that repeats shipped prose on 13.9% of every Bash call is noise,
# and computing the first word of every command to produce it cost a `sed`, an `awk` and several
# `grep` calls on EVERY call, nudging or not.
#
# What the hook keeps is the 0.17% it alone catches: 32 of those 35 blocks are matched by no
# other hook in the chain. They are above this line and they are unconditional.
#
# If the nudges are ever wanted back, the argument has to be evidence that a runtime reminder
# changes behaviour where the prose does not. A usage count is not that evidence.

exit 0
