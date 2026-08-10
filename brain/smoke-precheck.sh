#!/usr/bin/env bash
# smoke-precheck.sh — verify detect-platform.sh runs + branches correctly.
#
# Layer A (this file): pure-shell unit smoke. Sources the lib, calls each
# detection function in three modes (live / WOSY_FORCE_PLATFORM=macos /
# WOSY_FORCE_PLATFORM=linux), asserts the shape. No container needed — runs
# anywhere bash + uname + stat exist.
#
# Layer B (--container): opt-in alpine:3.20 run via podman. Asserts the lib
# reports platform=linux + stat_impl=gnu + log_dir=xdg on a real Linux box.
# Requires podman; documented in context.md done_when point 6.
#
# Usage:
#   bash brain/smoke-precheck.sh                # layer A only
#   bash brain/smoke-precheck.sh --container    # layer A + layer B (podman)
#
# Exit 0 if all PASS, 1 if any FAIL (CI-able).
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="${SCRIPT_DIR}/../claude/hooks/lib/detect-platform.sh"

fails=0
ok(){ printf '  \033[32mPASS\033[0m  %s\n' "$1"; }
no(){ printf '  \033[31mFAIL\033[0m  %s\n' "$1"; fails=$((fails+1)); }
eq(){ if [ "$1" = "$2" ]; then ok "$3"; else no "$3 (got=$1, want=$2)"; fi; }

# ----- preflight -----
echo "wosy precheck smoke — lib: $LIB"
echo
echo "── preflight ──"
[ -r "$LIB" ] && ok "lib readable" || { no "lib not readable at $LIB"; echo; echo "$fails FAILED ✗"; exit 1; }
bash -n "$LIB" && ok "lib bash -n clean" || no "lib bash -n FAILED"

# Source it for the rest of the run.
# shellcheck source=/dev/null
. "$LIB"

# ----- layer A: live detection -----
echo
echo "── layer A.1: live detection (no overrides) ──"
LIVE_OS="$(detect_os)"
LIVE_STAT="$(detect_stat_impl)"
LIVE_LOG="$(detect_log_dir)"
LIVE_BASH="$(detect_bash_version)"
case "$LIVE_OS" in
  macos|linux|unknown) ok "detect_os returned a valid value ($LIVE_OS)" ;;
  *) no "detect_os returned invalid: $LIVE_OS" ;;
esac
case "$LIVE_STAT" in
  bsd|gnu|unknown) ok "detect_stat_impl returned a valid value ($LIVE_STAT)" ;;
  *) no "detect_stat_impl returned invalid: $LIVE_STAT" ;;
esac
case "$LIVE_LOG" in
  library|xdg|unknown) ok "detect_log_dir returned a valid value ($LIVE_LOG)" ;;
  *) no "detect_log_dir returned invalid: $LIVE_LOG" ;;
esac
case "$LIVE_BASH" in
  [0-9]*|unknown) ok "detect_bash_version returned a valid value ($LIVE_BASH)" ;;
  *) no "detect_bash_version returned invalid: $LIVE_BASH" ;;
esac

# os/log_dir correlation (when os is known)
case "$LIVE_OS:$LIVE_LOG" in
  macos:library|linux:xdg|unknown:unknown) ok "os/log_dir correlate ($LIVE_OS → $LIVE_LOG)" ;;
  *) no "os/log_dir mismatch ($LIVE_OS / $LIVE_LOG)" ;;
esac

# log_dir_for produces a usable path
SAMPLE="$(log_dir_for canonicalize-bash)"
case "$SAMPLE" in
  "$HOME"/Library/Logs/claude-canonicalize-bash) ok "log_dir_for → library path ($SAMPLE)" ;;
  */wosy/canonicalize-bash) ok "log_dir_for → xdg path ($SAMPLE)" ;;
  *) no "log_dir_for returned unexpected path: $SAMPLE" ;;
esac

# ----- layer A.2: forced macos -----
echo
echo "── layer A.2: WOSY_FORCE_PLATFORM=macos ──"
eq "$(WOSY_FORCE_PLATFORM=macos detect_os)"        "macos"   "detect_os under force=macos"
eq "$(WOSY_FORCE_PLATFORM=macos detect_stat_impl)" "bsd"     "detect_stat_impl under force=macos"
eq "$(WOSY_FORCE_PLATFORM=macos detect_log_dir)"   "library" "detect_log_dir under force=macos"
FORCED_MAC_PATH="$(WOSY_FORCE_PLATFORM=macos log_dir_for multi-step)"
eq "$FORCED_MAC_PATH" "$HOME/Library/Logs/claude-multi-step" "log_dir_for under force=macos"

# ----- layer A.3: forced linux -----
echo
echo "── layer A.3: WOSY_FORCE_PLATFORM=linux ──"
eq "$(WOSY_FORCE_PLATFORM=linux detect_os)"        "linux"   "detect_os under force=linux"
eq "$(WOSY_FORCE_PLATFORM=linux detect_stat_impl)" "gnu"     "detect_stat_impl under force=linux"
eq "$(WOSY_FORCE_PLATFORM=linux detect_log_dir)"   "xdg"     "detect_log_dir under force=linux"
FORCED_LIN_PATH="$(WOSY_FORCE_PLATFORM=linux log_dir_for output-telemetry)"
case "$FORCED_LIN_PATH" in
  "${XDG_STATE_HOME:-$HOME/.local/state}/wosy/output-telemetry") ok "log_dir_for under force=linux ($FORCED_LIN_PATH)" ;;
  *) no "log_dir_for under force=linux unexpected: $FORCED_LIN_PATH" ;;
esac

# ----- layer A.4: precheck_emit_flags shape -----
echo
echo "── layer A.4: precheck_emit_flags shape ──"
EMIT="$(precheck_emit_flags)"
echo "$EMIT" | head -1 | grep -q '^# platform-detected' && ok "emit starts with marker" || no "emit missing start marker"
echo "$EMIT" | tail -1 | grep -q '^# end platform-detected' && ok "emit ends with marker" || no "emit missing end marker"
echo "$EMIT" | grep -qE '^platform=(macos|linux|unknown)$' && ok "emit has platform= line" || no "emit missing platform= line"
echo "$EMIT" | grep -qE '^stat_impl=(bsd|gnu|unknown)$'    && ok "emit has stat_impl= line" || no "emit missing stat_impl= line"
echo "$EMIT" | grep -qE '^log_dir=(library|xdg|unknown)$'  && ok "emit has log_dir= line" || no "emit missing log_dir= line"
echo "$EMIT" | grep -qE '^tool_(rg|fd|jq)=[01]$'           && ok "emit has tool_* lines" || no "emit missing tool_* lines"

# ----- layer A.5: newest_mtime against a known dir -----
echo
echo "── layer A.5: newest_mtime smoke ──"
TMPDIR_MTIME="$(mktemp -d)"
trap '[ -n "${TMPDIR_MTIME:-}" ] && rm -rf "$TMPDIR_MTIME"' EXIT
touch "$TMPDIR_MTIME/a" "$TMPDIR_MTIME/b"
LIVE_MTIME="$(newest_mtime "$TMPDIR_MTIME")"
case "$LIVE_MTIME" in
  [0-9]*) ok "newest_mtime returned an int ($LIVE_MTIME)" ;;
  *)      no "newest_mtime did not return an int: '$LIVE_MTIME'" ;;
esac
EMPTY_MTIME="$(newest_mtime "$TMPDIR_MTIME/nope")"
[ -z "$EMPTY_MTIME" ] && ok "newest_mtime empty on non-dir" || no "newest_mtime returned '$EMPTY_MTIME' on non-dir"
rm -rf "$TMPDIR_MTIME"; TMPDIR_MTIME=""

# ----- layer A done -----
echo
if [ "$fails" = 0 ]; then echo "── layer A: ALL PASS ✓"; else echo "── layer A: $fails FAILED ✗"; exit 1; fi

# ----- layer B: container check (opt-in) -----
#
# Streams the lib into the container via stdin — no volume mount required, so this
# works on any podman/docker setup regardless of host-share config. The container
# saves it to /tmp/lib.sh and bashes it, which preserves the lib's dual SOURCE/RUN
# detection ($BASH_SOURCE[0] == $0 == "/tmp/lib.sh"). Falls back to docker if
# podman is absent. Best-effort machine-start: if the user has a named podman
# machine, they start it themselves; this script does not attempt to.
if [ "${1:-}" = "--container" ]; then
  echo
  echo "── layer B: alpine:3.20 container check ──"
  RUNTIME=""
  if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
    RUNTIME=podman
  elif command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    RUNTIME=docker
  else
    no "no usable container runtime — install podman or docker, then start the daemon/machine"
    echo "$fails FAILED ✗"
    exit 1
  fi
  echo "  using runtime: $RUNTIME"
  CONTAINER_OUT="$("$RUNTIME" run -i --rm docker.io/library/alpine:3.20 sh -c '
    apk add --no-cache bash coreutils findutils >/dev/null 2>&1
    cat > /tmp/lib.sh
    bash /tmp/lib.sh
  ' < "$LIB" 2>&1)" || {
    no "$RUNTIME run failed"
    echo "$CONTAINER_OUT" | sed 's/^/    /'
    echo "$fails FAILED ✗"
    exit 1
  }
  echo "$CONTAINER_OUT" | sed 's/^/    /'
  echo "$CONTAINER_OUT" | grep -qE '^[[:space:]]+os[[:space:]]+linux$'         && ok "container reports os=linux" || no "container os mismatch"
  echo "$CONTAINER_OUT" | grep -qE '^[[:space:]]+stat_impl[[:space:]]+gnu$'    && ok "container reports stat_impl=gnu" || no "container stat_impl mismatch"
  echo "$CONTAINER_OUT" | grep -qE '^[[:space:]]+log_dir[[:space:]]+xdg$'      && ok "container reports log_dir=xdg" || no "container log_dir mismatch"
fi

echo
if [ "$fails" = 0 ]; then echo "ALL PASS ✓"; exit 0; else echo "$fails FAILED ✗"; exit 1; fi
