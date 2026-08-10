#!/usr/bin/env bash
# smoke-integrity.sh — verify the WOSY repo ships what it claims and the install layer matches.
#
# READ-ONLY: file existence + md5 parity + install.sh dry-run + JSON validity + brain build/CLI smoke.
# Sibling of smoke-wosy.sh (enforcement) and smoke-precheck.sh (platform detect).
# Exits 0 if all checks PASS, 1 if any FAIL.
#
# Section E (brain build + CLI) is SKIPPED (not failed) when Go toolchain or brain binary absent —
# the MD/YAML work surface (sections A-D) is the must-pass core; the brain substrate (section E)
# is the should-pass-if-installed substrate.
#
# Usage: bash brain/smoke-integrity.sh     (auto-detects repo via SCRIPT_DIR)

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WOSY_REPO="${WOSY_REPO:-$(cd "$SCRIPT_DIR/.." && pwd)}"
CLAUDE_DIR="${CLAUDE_DIR:-$HOME/.claude}"
BRAIN_BIN="${BRAIN_BIN:-$HOME/.local/bin/brain}"

pass=0; fail=0; skip=0
ok()  { printf "  \033[32mPASS\033[0m  %s\n" "$1"; pass=$((pass+1)); }
no()  { printf "  \033[31mFAIL\033[0m  %s\n" "$1"; fail=$((fail+1)); }
sk()  { printf "  \033[33mSKIP\033[0m  %s\n" "$1"; skip=$((skip+1)); }
hdr() { printf "\n\033[1m== %s ==\033[0m\n" "$1"; }

[ -d "$WOSY_REPO" ] || { echo "wosy repo not found at $WOSY_REPO"; exit 2; }

hdr "A. Repo files present (MD/YAML work surface)"

for f in README.md INSTALL.md CHANGELOG.md CONTRIBUTING.md LICENSE install.sh \
         claude/CLAUDE.md claude/RTK.md claude/settings.example.json \
         docs/WOSY.md docs/conventions/SOURCE.md; do
  [ -f "$WOSY_REPO/$f" ] && ok "$f" || no "$f MISSING"
done

agent_count=$(fd -e md . "$WOSY_REPO/claude/agents" -d 1 2>/dev/null | wc -l | tr -d ' ')
[ "$agent_count" -ge 5 ] && ok "claude/agents/ has $agent_count agent files (>=5)" || no "claude/agents/ only has $agent_count files"

hook_count=$(fd -e sh . "$WOSY_REPO/claude/hooks" -d 1 2>/dev/null | wc -l | tr -d ' ')
[ "$hook_count" -ge 10 ] && ok "claude/hooks/ has $hook_count hook scripts (>=10)" || no "claude/hooks/ only has $hook_count hooks"

skill_count=$(fd 'SKILL.md' "$WOSY_REPO/claude/skills" 2>/dev/null | wc -l | tr -d ' ')
[ "$skill_count" -ge 8 ] && ok "claude/skills/ has $skill_count SKILL.md files (>=8)" || no "claude/skills/ has $skill_count SKILL.md"

hdr "B. Installed Claude layer ($CLAUDE_DIR) matches repo"

if [ ! -d "$CLAUDE_DIR" ]; then
  sk "$CLAUDE_DIR absent — install layer not deployed; parity checks skipped"
else
  for pair in "CLAUDE.md:claude/CLAUDE.md" \
              "agents/agent-scribe.md:claude/agents/agent-scribe.md" \
              "hooks/watchdog.sh:claude/hooks/watchdog.sh" \
              "skills/context/bin/check.sh:claude/skills/context/bin/check.sh"; do
    inst="${pair%%:*}"; repo="${pair##*:}"
    if [ -f "$CLAUDE_DIR/$inst" ] && [ -f "$WOSY_REPO/$repo" ]; then
      if [ "$(md5 -q "$CLAUDE_DIR/$inst" 2>/dev/null)" = "$(md5 -q "$WOSY_REPO/$repo" 2>/dev/null)" ]; then
        ok "$inst md5-matches repo"
      else
        no "$inst DIVERGED from repo $repo (re-run install.sh --apply to sync)"
      fi
    else
      no "$inst or repo counterpart missing"
    fi
  done
fi

hdr "C. install.sh syntax + dry-run"

if bash -n "$WOSY_REPO/install.sh" 2>/dev/null; then
  ok "install.sh syntax valid"
else
  no "install.sh syntax broken"
fi

if (cd "$WOSY_REPO" && timeout 30 bash install.sh >/dev/null 2>&1); then
  ok "install.sh dry-run exits 0"
else
  no "install.sh dry-run failed or hung"
fi

hdr "D. JSON validity"

if python3 -m json.tool "$WOSY_REPO/claude/settings.example.json" >/dev/null 2>&1; then
  ok "claude/settings.example.json valid JSON"
else
  no "claude/settings.example.json INVALID JSON"
fi

hdr "E. Brain substrate (Go + binary)"

if command -v go >/dev/null 2>&1; then
  go_ver=$(go version | awk '{print $3}')
  ok "Go present ($go_ver)"
  if (cd "$WOSY_REPO/brain/cmd/brain" && go build -o /tmp/brain-smoke-build . 2>/dev/null); then
    ok "brain builds from source"
    rm -f /tmp/brain-smoke-build
  else
    no "brain build failed"
  fi
else
  sk "Go toolchain absent — brain build check skipped"
fi

if [ -x "$BRAIN_BIN" ]; then
  ok "brain binary installed at $BRAIN_BIN"
  if "$BRAIN_BIN" version >/dev/null 2>&1; then
    ok "brain version exits 0"
  else
    no "brain version failed"
  fi
  if "$BRAIN_BIN" --help >/dev/null 2>&1; then
    ok "brain --help exits 0"
  else
    no "brain --help failed"
  fi
else
  sk "brain binary absent at $BRAIN_BIN — runtime checks skipped"
fi

hdr "Summary"
printf "  %d pass · %d fail · %d skip\n" "$pass" "$fail" "$skip"

[ "$fail" -eq 0 ] && exit 0 || exit 1
