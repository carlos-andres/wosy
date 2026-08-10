# Contributing to wosy

Thanks for taking a look. wosy is small, opinionated, and shipped from a single repo — the code is also the documentation. Read `docs/WOSY.md` (the operator manual) and `README.md` before opening a PR.

## Before you open an issue

- **Search existing issues** — wosy's surface is small; many "this doesn't work" reports have been seen.
- **Run the smoke test** — `bash brain/smoke-wosy.sh`. If it fails, paste the full output in the issue.
- **State your environment** — macOS version, bash version (`bash --version`), `brew --version`, `go version` if relevant.

## Before you open a PR

1. **Open an issue first** for anything beyond a typo, comment cleanup, or one-line bug fix. A 10-minute conversation in an issue saves an hour of PR rework.
2. **Run the test suites:**
   - `cd brain && go test ./...` — the brain Go suite
   - `bash brain/smoke-wosy.sh` — the watchdog enforcement smoke
3. **Add or update tests** for any behavior change. The brain `guard` package especially — every classifier rule has at least one positive and one regression case.
4. **No machine-specific paths** — no `/Users/<name>/...`, no `~/Documents/<personal-folder>/`. Use `$HOME`, `$WOSY_DEVWORK`, or relative paths.
5. **No real secrets, tokens, hostnames** in code or commits. The `reference-flags/` and `examples/` files use `<PLACEHOLDER>` patterns — match that convention.

## Code conventions

### Bash

- Shebang: `#!/usr/bin/env bash` (portable).
- Hooks that are advisory: `set -uo pipefail` (no `-e` — they're fail-open by design).
- Hooks that are gates or scripts that must be correct end-to-end: `set -euo pipefail`.
- Quote variables: `"$var"`, not `$var`. Same for command substitutions: `"$(cmd)"`.
- Prefer `printf '%s' "$var"` over `echo "$var"` when the value might start with `-`.
- Use `[[ ]]` for tests; reserve `[ ]` for POSIX-portable lib helpers.
- Bash 4+ features are fine (`declare -A`, `${var,,}`, `<(...)`); they're a stated prerequisite.

### Hook exit codes

The Claude Code PreToolUse contract:

- `exit 0` — allow the tool call.
- `exit 2` — **block** the tool call. Stderr is shown to Claude.
- `exit 1` — **advisory only.** Tool call proceeds. Use only for log/warn paths that should never block.

Get this wrong and a "block" silently allows the action. The hook headers document this contract explicitly.

### Go

- `gofmt`-clean, `go vet`-clean.
- One package per directory; no `internal/` packages with multiple unrelated concerns.
- Tests live next to the code (`*_test.go`).
- New classifier rules go through `brain guard`'s `Decision.FixHint` — don't add per-tool branching to the bash shim.

### Markdown / docs

- Follow the existing voice (terse, citation-grounded, present tense).
- No emoji unless explicitly requested by the maintainer.
- Update `CHANGELOG.md` for any user-visible change. One line is fine.
- Examples use `<UPPERCASE_PLACEHOLDERS>`.

## Commit messages

- One commit per logical change.
- Subject line ≤ 70 chars, present tense ("add X", "fix Y", "remove Z" — not "added"/"fixed").
- Body explains the *why*, not the *what* (the diff shows what).
- Reference the issue (`Fixes #123`) when applicable.

## License

By contributing, you agree your contributions are licensed under the MIT License (see `LICENSE`).
