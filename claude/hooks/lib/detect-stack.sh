#!/usr/bin/env bash
# shellcheck shell=bash
# detect-stack.sh — config-is-truth primary-stack detection for a project dir.
#
# Dual use:
#   - SOURCE it:  . detect-stack.sh ; s="$(detect_stack /path)"
#   - RUN it:     bash detect-stack.sh /path   # prints the primary stack
#
# Built 2026-05-22 (wosy wire-up optimization) so the /wire-up A2 DETECT stack axis
# is DETERMINISTIC and reproducible for testing, instead of re-derived by hand each
# run. Echoes one of: swift-xcode swift-spm php-laravel php-plain php-legacy rust go
# android browser-ext ruby elixir java-maven dotnet deno node python static none.
#
# Recurses (bounded depth, vendor/node_modules excluded) so a nested marker like
# pilot-a's app/composer.json (depth 2) is found — the exact case a root-only scan misses.
# Pairs with detect-db-engine.sh (the db axis). Remote/build axes stay agent judgment
# (they need reading schemes / package scripts — not reliably scriptable).
#
# Requires fd (the project's standard finder). If fd is absent, echoes nothing so the
# caller falls back to manual detection.
detect_stack() {
  local dir="${1:-$PWD}" m composer cdir
  command -v fd >/dev/null 2>&1 || { return 0; }

  # Exclude build output + archives so a project's CURRENT stack is detected, never a
  # built/archived artifact. _archive/.archive_devwork added 2026-05-25 (fix G14: kreata's
  # archived *.php under _archive/headless/ were outranking its live static site).
  _ff() { fd --no-ignore -H -t f -E vendor -E node_modules -E .git -E .build -E _archive -E .archive_devwork --max-depth 4 -g "$1" "$dir" 2>/dev/null | head -1; }
  _fd() { fd --no-ignore -H -t d -E vendor -E node_modules -E .git -E .build -E _archive -E .archive_devwork --max-depth 4 -g "$1" "$dir" 2>/dev/null | head -1; }

  # Priority order mirrors /wire-up A2 (first match wins).
  [ -n "$(_fd '*.xcodeproj')" ] || [ -n "$(_fd '*.xcworkspace')" ] && { echo swift-xcode; return 0; }

  composer="$(_ff 'composer.json')"
  if [ -n "$composer" ]; then
    cdir="$(dirname "$composer")"
    if [ -f "$cdir/artisan" ]; then echo php-laravel; return 0; fi
    # Orphan composer (no artisan): a Vite config or a package.json declaring
    # vite as a dep means JS is the live stack — composer artifacts are dormant.
    # Ohana case 2026-05-27: vendored composer/ + vite.config.js + active Vite build
    # falsely reported php-plain on a pure-frontend project. Vite signal wins.
    pkg="$(_ff 'package.json')"
    if [ -n "$(_ff 'vite.config.*')" ] || { [ -n "$pkg" ] && grep -q '"vite"' "$pkg" 2>/dev/null; }; then
      echo node; return 0
    fi
    echo php-plain; return 0
  fi
  [ -n "$(_ff '*.php')" ]            && { echo php-legacy; return 0; }
  [ -n "$(_ff 'Package.swift')" ]   && { echo swift-spm;  return 0; }
  [ -n "$(_ff 'Cargo.toml')" ]      && { echo rust;       return 0; }
  [ -n "$(_ff 'go.mod')" ]          && { echo go;         return 0; }
  [ -n "$(_ff 'build.gradle')" ] || [ -n "$(_ff 'build.gradle.kts')" ] && { echo android; return 0; }

  # browser-ext BEFORE node (extensions also carry package.json).
  m="$(_ff 'manifest.json')"
  if [ -n "$m" ] && grep -q 'content_scripts\|manifest_version' "$m" 2>/dev/null; then echo browser-ext; return 0; fi

  [ -n "$(_ff 'Gemfile')" ]         && { echo ruby;       return 0; }
  [ -n "$(_ff 'mix.exs')" ]         && { echo elixir;     return 0; }
  [ -n "$(_ff 'pom.xml')" ]         && { echo java-maven; return 0; }
  [ -n "$(_ff '*.csproj')" ] || [ -n "$(_ff '*.sln')" ]       && { echo dotnet; return 0; }
  [ -n "$(_ff 'deno.json')" ] || [ -n "$(_ff 'deno.jsonc')" ] && { echo deno;   return 0; }
  [ -n "$(_ff 'package.json')" ]    && { echo node;       return 0; }
  [ -n "$(_ff 'pyproject.toml')" ] || [ -n "$(_ff 'requirements.txt')" ] && { echo python; return 0; }
  [ -n "$(_ff 'index.html')" ]      && { echo static;     return 0; }

  echo none
}

# Run standalone when executed (not sourced).
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  detect_stack "${1:-$PWD}"
fi
