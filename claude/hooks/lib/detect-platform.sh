#!/usr/bin/env bash
# shellcheck shell=bash
# detect-platform.sh — per-machine capability detection for the wosy hook chain.
#
# Dual use:
#   - SOURCE it:   . detect-platform.sh ; os="$(detect_os)" ; ld="$(log_dir_for canonicalize-bash)"
#   - RUN it:      bash detect-platform.sh   # prints the status table (precheck_report)
#
# Built 2026-05-29 (precheck-layer ticket) to close D-3 + D-4 from the
# publish-readiness audit by replacing the binary "macOS-only vs Linux-supported"
# decision with per-machine detection + branching. Sibling of detect-stack.sh
# (project-stack axis) and detect-db-engine.sh (db-engine axis).
#
# Detection axes:
#   - os          → macos | linux | unknown   (via `uname -s`)
#   - stat_impl   → bsd | gnu | unknown       (via `stat --version` probe)
#   - log_dir     → library | xdg | unknown   (derived from os; separable for XDG-on-macOS)
#   - bash_major  → integer | unknown         (via ${BASH_VERSINFO[0]})
#   - tool_<name> → 1 | 0                     (via `command -v`, for rg/fd/eza/gh/httpie/direnv/jq/rtk)
#
# Helpers (used by branching hooks):
#   - log_dir_for <suffix>   → echoes the right base log dir for a hook
#                              (e.g. log_dir_for canonicalize-bash → $HOME/Library/Logs/claude-canonicalize-bash on macOS,
#                              ${XDG_STATE_HOME:-$HOME/.local/state}/wosy/canonicalize-bash on linux)
#   - newest_mtime <dir>     → echoes the newest mtime epoch int among files in <dir>
#                              (collapses the find|xargs|stat pipeline to find -printf on GNU)
#
# Hot-path note: detect_os / detect_stat_impl / detect_log_dir each cache their
# result in a module-level var (`__WOSY_OS` / `__WOSY_STAT_IMPL` / `__WOSY_LOG_DIR`)
# so a hook process that calls log_dir_for and newest_mtime pays one fork+exec total,
# not three per call. log_dir_for and newest_mtime read the cache directly (no
# nested `$()` subshell) once it is primed. WOSY_FORCE_* overrides bypass the cache.
#
# Test affordance: setting WOSY_FORCE_PLATFORM=linux (or =macos) overrides detect_os,
# detect_stat_impl, and (transitively) detect_log_dir + log_dir_for + newest_mtime
# so the same hook code can be exercised on the maintainer's mac without a real
# Linux box. Used by smoke-precheck.sh.
#
# Fail-open everywhere: any uncertainty echoes `unknown`. Hooks that source this lib
# stub the functions to safe defaults when the lib is missing (see hook-stub pattern
# in canonicalize-bash.sh / wosy-flags.sh header).

# Module-level caches — populated lazily on first call. Safe because platform
# doesn't change mid-process. Hot-path callers (log_dir_for, newest_mtime) read
# these directly after priming, avoiding nested `$()` subshell forks.
__WOSY_OS=""
__WOSY_STAT_IMPL=""
__WOSY_LOG_DIR=""

detect_os() {
  if [ -n "${WOSY_FORCE_PLATFORM:-}" ]; then
    case "$WOSY_FORCE_PLATFORM" in
      macos|linux|unknown) echo "$WOSY_FORCE_PLATFORM"; return 0 ;;
    esac
  fi
  if [ -z "$__WOSY_OS" ]; then
    case "$(uname -s 2>/dev/null)" in
      Darwin) __WOSY_OS=macos ;;
      Linux)  __WOSY_OS=linux ;;
      *)      __WOSY_OS=unknown ;;
    esac
  fi
  echo "$__WOSY_OS"
}

detect_stat_impl() {
  if [ -n "${WOSY_FORCE_PLATFORM:-}" ]; then
    case "$WOSY_FORCE_PLATFORM" in
      macos)   echo bsd; return 0 ;;
      linux)   echo gnu; return 0 ;;
      unknown) echo unknown; return 0 ;;
    esac
  fi
  if [ -z "$__WOSY_STAT_IMPL" ]; then
    # GNU coreutils stat supports --version; BSD stat does not.
    if stat --version 2>/dev/null | grep -q 'GNU coreutils'; then
      __WOSY_STAT_IMPL=gnu
    elif command -v stat >/dev/null 2>&1; then
      __WOSY_STAT_IMPL=bsd
    else
      __WOSY_STAT_IMPL=unknown
    fi
  fi
  echo "$__WOSY_STAT_IMPL"
}

detect_log_dir() {
  if [ -n "${WOSY_FORCE_LOG_DIR:-}" ]; then
    case "$WOSY_FORCE_LOG_DIR" in
      library|xdg|unknown) echo "$WOSY_FORCE_LOG_DIR"; return 0 ;;
    esac
  fi
  if [ -z "$__WOSY_LOG_DIR" ]; then
    case "$(detect_os)" in
      macos)   __WOSY_LOG_DIR=library ;;
      linux)   __WOSY_LOG_DIR=xdg ;;
      *)       __WOSY_LOG_DIR=unknown ;;
    esac
  fi
  echo "$__WOSY_LOG_DIR"
}

detect_bash_version() {
  if [ -n "${BASH_VERSINFO[0]:-}" ]; then
    echo "${BASH_VERSINFO[0]}"
  else
    echo unknown
  fi
}

detect_optional_tool() {
  local name="$1"
  [ -n "$name" ] || { echo 0; return 0; }
  if command -v "$name" >/dev/null 2>&1; then echo 1; else echo 0; fi
}

# log_dir_for <hook-suffix> — single source of truth for hook log-dir paths.
# Each hook calls with its own suffix (canonicalize-bash / multi-step / output-telemetry / etc.).
# Falls back to macOS library path on unknown — safe default on author machine.
# Hot-path: primes the __WOSY_LOG_DIR cache once via detect_log_dir, then reads it
# directly. Avoids the nested $(detect_log_dir) → $(detect_os) subshell chain on
# subsequent calls within the same process.
log_dir_for() {
  local suffix="$1" impl
  [ -n "$suffix" ] || { echo ""; return 0; }
  if [ -n "${WOSY_FORCE_LOG_DIR:-}" ]; then
    impl="$WOSY_FORCE_LOG_DIR"
  elif [ -n "${WOSY_FORCE_PLATFORM:-}" ]; then
    # Force-platform always implies a log_dir without populating the cache.
    case "$WOSY_FORCE_PLATFORM" in
      macos)   impl=library ;;
      linux)   impl=xdg ;;
      *)       impl=unknown ;;
    esac
  else
    [ -z "$__WOSY_LOG_DIR" ] && detect_log_dir >/dev/null
    impl="$__WOSY_LOG_DIR"
  fi
  case "$impl" in
    xdg)     echo "${XDG_STATE_HOME:-$HOME/.local/state}/wosy/$suffix" ;;
    *)       echo "$HOME/Library/Logs/claude-$suffix" ;;
  esac
}

# newest_mtime <dir> — echoes the newest file mtime (epoch) under <dir>, maxdepth 2.
# Replaces the pipeline in wosy-stop.sh:33-34 with a stat-impl-aware single call.
# Empty echo when the dir is empty / unreadable.
# Hot-path: primes the __WOSY_STAT_IMPL cache once, then reads directly.
newest_mtime() {
  local dir="$1" impl
  [ -n "$dir" ] && [ -d "$dir" ] || { echo ""; return 0; }
  if [ -n "${WOSY_FORCE_PLATFORM:-}" ]; then
    case "$WOSY_FORCE_PLATFORM" in
      macos)   impl=bsd ;;
      linux)   impl=gnu ;;
      *)       impl=unknown ;;
    esac
  else
    [ -z "$__WOSY_STAT_IMPL" ] && detect_stat_impl >/dev/null
    impl="$__WOSY_STAT_IMPL"
  fi
  case "$impl" in
    gnu)
      # GNU find emits mtime directly — no xargs+stat needed.
      find "$dir" -maxdepth 2 -type f -printf '%T@\n' 2>/dev/null \
        | sort -nr | head -1 | cut -d. -f1
      ;;
    *)
      # BSD fallback (or unknown — keep prior macOS behavior).
      find "$dir" -maxdepth 2 -type f -print0 2>/dev/null \
        | xargs -0 stat -f '%m' 2>/dev/null | sort -nr | head -1
      ;;
  esac
}

# precheck_report — human-readable status table to stdout.
precheck_report() {
  local os stat_impl log_dir bash_major
  os="$(detect_os)"
  stat_impl="$(detect_stat_impl)"
  log_dir="$(detect_log_dir)"
  bash_major="$(detect_bash_version)"
  printf '=== wosy precheck ===\n'
  printf '  %-14s %s\n' 'os'         "$os"
  printf '  %-14s %s\n' 'stat_impl'  "$stat_impl"
  printf '  %-14s %s\n' 'log_dir'    "$log_dir"
  printf '  %-14s %s\n' 'bash_major' "$bash_major"
  printf '  %-14s %s\n' 'log_sample' "$(log_dir_for canonicalize-bash)"
  printf '\n  optional tools:\n'
  local t
  for t in rg fd jq eza gh httpie direnv rtk; do
    printf '  %-14s %s\n' "tool_$t" "$(detect_optional_tool "$t")"
  done
  if [ -n "${WOSY_FORCE_PLATFORM:-}" ]; then
    printf '\n  (WOSY_FORCE_PLATFORM=%s active — overriding live detection)\n' "$WOSY_FORCE_PLATFORM"
  fi
}

# precheck_emit_flags — bash key=value lines to stdout, bracketed for idempotent rewrite.
# Consumed by the install.sh `platform-precheck` component when --apply'd.
precheck_emit_flags() {
  printf '# platform-detected (managed by install.sh --only=platform-precheck --apply — do not hand-edit)\n'
  printf 'platform=%s\n'   "$(detect_os)"
  printf 'stat_impl=%s\n'  "$(detect_stat_impl)"
  printf 'log_dir=%s\n'    "$(detect_log_dir)"
  printf 'bash_major=%s\n' "$(detect_bash_version)"
  local t
  for t in rg fd jq eza gh httpie direnv rtk; do
    printf 'tool_%s=%s\n' "$t" "$(detect_optional_tool "$t")"
  done
  printf '# end platform-detected\n'
}

# Run standalone when executed (not sourced).
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  precheck_report
fi
