# shellcheck shell=bash
# walk-up.sh — shared "walk up the tree" path finder for hooks.
#
# SOURCE it; defines walk_up_first(). Callers fail open if it's missing:
#   _wu="${HOME}/.claude/hooks/lib/walk-up.sh"
#   if [ -r "$_wu" ]; then . "$_wu"; else walk_up_first() { return 1; }; fi
#
# Factored 2026-05-22 (wosy audit P3 #15) from find_scripts_dir / find_toc /
# find_graph — they shared the walk-up SHAPE, differing only in what they match.
#
# walk_up_first <test-flag> <relpath...>
#   Walk UP from ${cwd:-$PWD}; at each ancestor, test each <relpath> in order with
#   [test-flag] (-d dir / -f file); echo the first absolute match and return 0.
#   Return 1 if nothing matches up to /. Priority is nearest-ancestor-first, then
#   arg-order within a level — preserves the exact semantics of the originals.
walk_up_first() {
  local flag="$1"; shift
  local dir="${cwd:-$PWD}" rel
  while [ -n "$dir" ] && [ "$dir" != "/" ]; do
    for rel in "$@"; do
      if [ "$flag" "$dir/$rel" ]; then
        printf '%s/%s' "$dir" "$rel"
        return 0
      fi
    done
    dir="$(dirname "$dir")"
  done
  return 1
}
