# shellcheck shell=bash
# wosy-flags.sh — shared helper for reading per-project .devwork/wosy.flags.
#
# SOURCE this (do not execute). It defines wosy_flag(). Every caller must fail
# open if it cannot source this lib, e.g.:
#
#   _wosy_lib="${HOME}/.claude/hooks/lib/wosy-flags.sh"
#   if [ -r "$_wosy_lib" ]; then . "$_wosy_lib"; else wosy_flag() { :; }; fi
#
# A stub `wosy_flag() { :; }` makes every flag read return empty, which equals
# "no flags file present" = no project-scoped enforcement = prior behavior.
#
# Factored 2026-05-22 (wosy audit P2 #7) from canonicalize-bash.sh so validate-bash.sh
# and write-location-warn.sh read the same flags the same way (kill the dup walk-up).
#
# wosy_flag <key> [start_dir]
#   Walk UP from start_dir (default: ${cwd:-$PWD}) to the nearest .devwork/wosy.flags,
#   echo the trimmed, comment-stripped value for <key>, or nothing if file/key absent.
#   Fail-open by design: absent file => empty => caller treats as "no enforcement".
wosy_flag() {
  local key="$1" dir="${2:-${cwd:-$PWD}}" f line
  while [ -n "$dir" ] && [ "$dir" != "/" ]; do
    f="$dir/.devwork/wosy.flags"
    if [ -f "$f" ]; then
      line="$(grep -E "^[[:space:]]*${key}[[:space:]]*=" "$f" 2>/dev/null | head -1)"
      line="${line#*=}"; line="${line%%#*}"
      printf '%s' "$line" | tr -d '[:space:]'
      return 0
    fi
    dir="$(dirname "$dir")"
  done
}
