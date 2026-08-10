#!/usr/bin/env bash
# build-constitution.sh — bootstrap .devwork/constitution.md from project evidence.
#
# Usage: build-constitution.sh [project-root]   (default: cwd)
#
# Per the constitution-rebuild skill SKILL.md and CSI audit D9 (2026-05-20):
# - Never overwrites: writes .devwork/constitution.next.md if constitution.md exists.
# - Never emits file-count snapshots (D9 §anti-patterns).
# - Fills the skeleton from introspection sources listed in SKILL.md.

set -uo pipefail

ROOT="${1:-$PWD}"
ROOT="$(cd "$ROOT" && pwd)"   # absolutize

if [ ! -d "$ROOT" ]; then
  echo "ERROR: $ROOT is not a directory" >&2
  exit 2
fi

DEVWORK="$ROOT/.devwork"
mkdir -p "$DEVWORK"

OUT="$DEVWORK/constitution.md"
if [ -f "$OUT" ]; then
  OUT="$DEVWORK/constitution.next.md"
  echo "INFO: constitution.md exists; writing to $(basename "$OUT") for hand-merge" >&2
fi

PROJ_NAME="$(basename "$ROOT")"
TODAY="$(date +%Y-%m-%d)"

# ---- Stack introspection ----
STACK_LINES=""
add_stack() { STACK_LINES="${STACK_LINES}- $1"$'\n'; }

if [ -f "$ROOT/composer.json" ]; then
  php_ver="$(jq -r '.require."php" // empty' "$ROOT/composer.json" 2>/dev/null)"
  [ -n "$php_ver" ] && add_stack "PHP $php_ver"
  laravel_ver="$(jq -r '.require."laravel/framework" // .require."illuminate/support" // empty' "$ROOT/composer.json" 2>/dev/null)"
  [ -n "$laravel_ver" ] && add_stack "Laravel $laravel_ver"
fi

if [ -f "$ROOT/package.json" ]; then
  node_ver="$(jq -r '.engines.node // empty' "$ROOT/package.json" 2>/dev/null)"
  [ -n "$node_ver" ] && add_stack "Node $node_ver"
  for fw in next nuxt express react vue svelte; do
    fw_ver="$(jq -r ".dependencies.\"$fw\" // .devDependencies.\"$fw\" // empty" "$ROOT/package.json" 2>/dev/null)"
    [ -n "$fw_ver" ] && add_stack "${fw^} $fw_ver"
  done
fi

if [ -f "$ROOT/Gemfile" ]; then
  ruby_ver="$(rg -oE 'ruby ["'\''][^"'\'']+' "$ROOT/Gemfile" 2>/dev/null | head -1 | sed -E "s/ruby [\"']//")"
  [ -n "$ruby_ver" ] && add_stack "Ruby $ruby_ver"
  rails_ver="$(rg -oE "gem ['\"]rails['\"][^,]*,\s*['\"][^'\"]+" "$ROOT/Gemfile" 2>/dev/null | head -1)"
  [ -n "$rails_ver" ] && add_stack "Rails (Gemfile: $rails_ver)"
fi

if [ -f "$ROOT/go.mod" ]; then
  go_ver="$(rg '^go ' "$ROOT/go.mod" 2>/dev/null | head -1 | awk '{print $2}')"
  [ -n "$go_ver" ] && add_stack "Go $go_ver"
fi

if [ -f "$ROOT/pyproject.toml" ]; then
  py_ver="$(rg 'python\s*=\s*' "$ROOT/pyproject.toml" 2>/dev/null | head -1 | sed -E 's/.*=\s*//')"
  [ -n "$py_ver" ] && add_stack "Python $py_ver (pyproject.toml)"
fi
if [ -f "$ROOT/.python-version" ]; then
  add_stack "Python $(cat "$ROOT/.python-version") (.python-version)"
fi
if [ -f "$ROOT/.nvmrc" ]; then
  add_stack "Node $(cat "$ROOT/.nvmrc") (.nvmrc)"
fi
if [ -f "$ROOT/.tool-versions" ]; then
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    add_stack "$line (.tool-versions)"
  done < "$ROOT/.tool-versions"
fi

# Infra from docker-compose
if [ -f "$ROOT/docker-compose.yml" ] || [ -f "$ROOT/docker-compose.yaml" ]; then
  compose_file="$ROOT/docker-compose.yml"
  [ -f "$ROOT/docker-compose.yaml" ] && compose_file="$ROOT/docker-compose.yaml"
  services="$(rg -oE '^\s{2,4}(\w[\w-]*):' "$compose_file" 2>/dev/null | sed -E 's/^\s+//; s/://' | sort -u | tr '\n' ',' | sed 's/,$//')"
  [ -n "$services" ] && add_stack "Infra (docker-compose): $services"
fi

[ -z "$STACK_LINES" ] && STACK_LINES="- (no language/framework manifest detected — fill manually)"$'\n'

# ---- Architecture: directory tree (top 2 levels), entry points ----
TREE="$(eza -D --level=2 "$ROOT" 2>/dev/null | head -40 || ls -d "$ROOT"/*/ 2>/dev/null | head -20)"

ENTRY_POINTS=""
add_entry() { ENTRY_POINTS="${ENTRY_POINTS}- \`$1\`"$'\n'; }
[ -f "$ROOT/artisan" ] && add_entry "artisan (Laravel)"
[ -f "$ROOT/manage.py" ] && add_entry "manage.py (Django)"
[ -f "$ROOT/bin/rails" ] && add_entry "bin/rails (Rails)"
[ -f "$ROOT/main.go" ] && add_entry "main.go (Go)"
[ -f "$ROOT/index.js" ] && add_entry "index.js"
[ -f "$ROOT/server.js" ] && add_entry "server.js"
[ -f "$ROOT/app.py" ] && add_entry "app.py"
[ -z "$ENTRY_POINTS" ] && ENTRY_POINTS="- (no canonical entry detected — fill manually)"$'\n'

# ---- Testing ----
TEST_LINES=""
add_test() { TEST_LINES="${TEST_LINES}- $1"$'\n'; }
[ -f "$ROOT/phpunit.xml" ] && add_test "PHPUnit (\`phpunit.xml\`)"
[ -f "$ROOT/phpunit.xml.dist" ] && add_test "PHPUnit (\`phpunit.xml.dist\`)"
[ -d "$ROOT/tests/Pest" ] || rg -lq "pest" "$ROOT/composer.json" 2>/dev/null && add_test "Pest"
[ -f "$ROOT/jest.config.js" ] || [ -f "$ROOT/jest.config.ts" ] && add_test "Jest"
[ -f "$ROOT/vitest.config.js" ] || [ -f "$ROOT/vitest.config.ts" ] && add_test "Vitest"
[ -f "$ROOT/pytest.ini" ] || rg -lq '\[tool.pytest' "$ROOT/pyproject.toml" 2>/dev/null && add_test "pytest"
[ -d "$ROOT/spec" ] && [ -f "$ROOT/Gemfile" ] && add_test "RSpec"
[ -z "$TEST_LINES" ] && TEST_LINES="- (no test framework detected — fill manually)"$'\n'

# ---- Quality gates ----
QG_LINES=""
add_qg() { QG_LINES="${QG_LINES}- $1"$'\n'; }
[ -f "$ROOT/phpstan.neon" ] || [ -f "$ROOT/phpstan.neon.dist" ] && add_qg "PHPStan"
[ -f "$ROOT/psalm.xml" ] && add_qg "Psalm"
[ -f "$ROOT/.php-cs-fixer.php" ] || [ -f "$ROOT/.php-cs-fixer.dist.php" ] && add_qg "php-cs-fixer"
[ -f "$ROOT/eslint.config.js" ] || [ -f "$ROOT/.eslintrc.json" ] || [ -f "$ROOT/.eslintrc.cjs" ] && add_qg "ESLint"
[ -f "$ROOT/.prettierrc" ] || [ -f "$ROOT/.prettierrc.json" ] && add_qg "Prettier"
rg -lq '\[tool.ruff' "$ROOT/pyproject.toml" 2>/dev/null && add_qg "ruff"
rg -lq '\[tool.mypy' "$ROOT/pyproject.toml" 2>/dev/null && add_qg "mypy"
[ -z "$QG_LINES" ] && QG_LINES="- (no static-analysis config detected — fill manually)"$'\n'

# ---- Header / git remote ----
REMOTE="$(git -C "$ROOT" remote get-url origin 2>/dev/null || echo '(no git remote)')"
# Credentialed HTTPS remotes (https://user:TOKEN@host/…) would leak the secret into the committed doc; keep the username, drop the secret.
REMOTE="$(printf '%s' "$REMOTE" | sed -E 's#(://[^/:@]+):[^@/]+@#\1@#')"

# ---- Write the constitution ----
cat > "$OUT" <<EOF
# Constitution: $PROJ_NAME

> Generated: $TODAY by \`/constitution-rebuild\`
> Origin: $REMOTE

## Stack

$STACK_LINES

## Architecture

**Top-level layout:**

\`\`\`
$TREE
\`\`\`

**Entry points:**

$ENTRY_POINTS

## Conventions

<!-- Auto-introspection cannot reliably derive naming/error conventions; fill manually. -->
- (naming conventions — to fill)
- (error handling style — to fill)
- (type / format rules — to fill)

## Testing

$TEST_LINES

## Quality Gates

$QG_LINES

## Do NOT Touch

<!-- Preserved on update: legacy carve-outs the rebuilder must not regenerate. -->

## Manual Notes

<!-- Preserved on update: human-curated rules the rebuilder must not regenerate. -->
EOF

echo "Wrote: $OUT"
