# Installing wosy

This guide walks through installing wosy on macOS via Homebrew. The installer is bash-heavy and targets macOS, but most pieces are POSIX-compatible.

## Prerequisites

| Tool | Version | Why |
|---|---|---|
| **bash** | ≥ 4.0 | `install.sh` uses `declare -A` (associative arrays). macOS ships bash 3.2 at `/bin/bash`; install a newer bash via Homebrew. |
| **jq** | any recent | Required by `install.sh` for JSON validation of `~/.claude/settings.json`. |
| **fd** | any recent | Used by `claude/hooks/lib/detect-stack.sh` for project-stack detection. Optional for install but required at runtime by `/wire-up` and related skills. |
| **Go** | ≥ 1.21 | Required to build the `brain` binary from source. No CGO. |
| **Claude Code** | current | The wosy hooks attach to Claude Code's PreToolUse/Stop/SessionEnd events. |

### One-shot Homebrew install

```bash
brew install bash jq fd go
```

Optional but recommended (referenced by the operator-level `CLAUDE.md` tooling preferences):

```bash
brew install ripgrep eza gh httpie direnv coreutils gnu-sed
```

## Install

### 1. Clone the repo

```bash
git clone <repo-url> ~/Documents/GitHub/wosy
cd ~/Documents/GitHub/wosy
```

The repo path is your choice — `install.sh` derives its source paths from `$SCRIPT_DIR`, not from `$HOME`.

### 2. Build the brain binary

```bash
cd brain/cmd/brain
go build -o "$HOME/.local/bin/brain" .
"$HOME/.local/bin/brain" version    # → brain X.Y.Z
"$HOME/.local/bin/brain" selftest   # → selftest OK
cd -                                # return to repo root
```

Add `$HOME/.local/bin` to your `$PATH` if it isn't already (most shells do this via `~/.profile` or `~/.zshrc`).

### 3. Dry-run the installer

```bash
./install.sh
```

No arguments means **dry run** — prints the full plan, mutates nothing. Review the planned changes to `~/.claude/settings.json` and `~/.claude/hooks/`.

### 4. Apply

```bash
./install.sh --apply
```

This:
- Provisions `~/Documents/.devwork/` (the brain-home target — overridable via `$WOSY_DEVWORK`).
- Copies `claude/hooks/watchdog.sh` → `~/.claude/hooks/`.
- Registers the PreToolUse matcher in `~/.claude/settings.json` (atomic write with timestamped backup).
- Cleans up any stale dev permits from prior installs.

The installer is **idempotent** — re-running is a no-op when nothing has changed.

### 5. Verify

```bash
bash brain/smoke-wosy.sh
```

Expected: `ALL PASS ✓`. The smoke runs the installed hook through `brain guard` against a set of known-good and known-blocked tool-input shapes.

## Installer flags

| Flag | What it does |
|---|---|
| `./install.sh` | Dry run — print plan, no changes |
| `./install.sh --apply` | Land all components |
| `./install.sh --list` | List install components with descriptions |
| `./install.sh --only=<component> [--apply]` | Install or dry-run one component |
| `./install.sh --skip-provision [--apply]` | Skip the brain-home provisioning step |
| `./install.sh --uninstall [--apply]` | Reverse — keeps the brain-home data store intact |

## What lands where

| Path | Owner | Contents |
|---|---|---|
| `~/.claude/hooks/watchdog.sh` | install.sh | The PreToolUse classifier shim |
| `~/.claude/settings.json` | install.sh (one matcher entry) | Hook matcher + permission list — installer touches one key, backs up the rest |
| `~/Documents/.devwork/brain/store.db` | install.sh + brain | The brain SQLite store (override location with `$WOSY_DEVWORK`) |
| `~/Documents/.devwork/wosy.flags` | install.sh | Enforcement profile for the brain-home itself |
| `~/.local/bin/brain` | you (via `go build`) | The brain binary |
| `~/.claude/CLAUDE.md` | you (separate copy) | Operator-level instructions — see `claude/CLAUDE.md` for the canonical content; copy what you want |
| `~/.claude/skills/<name>/SKILL.md` | you (separate copy) | The wosy skills — see `claude/skills/` |
| `~/.claude/agents/<name>.md` | you (separate copy) | The wosy subagents — see `claude/agents/` |
| `~/.claude/output-styles/coworker.md` | you (separate copy) | The Coworker output style — see `claude/output-styles/` and the Configuration section below |

> **Note:** `install.sh` currently only deploys the watchdog hook + settings matcher. Skills, agents, and operator CLAUDE.md are not auto-deployed in this version. Copy them manually from `claude/skills/`, `claude/agents/`, and `claude/CLAUDE.md` into your `~/.claude/` tree.
>
> The repo also ships `claude/settings.example.json` — a reference showing the full hook wiring. It uses `${HOME}` placeholders. If you want to use it as a starter for a fresh `~/.claude/settings.json`:
>
> ```bash
> cp claude/settings.example.json ~/.claude/settings.json
> sed -i.bak "s|\${HOME}|$HOME|g" ~/.claude/settings.json
> # Then copy any other hooks you want active from claude/hooks/ into ~/.claude/hooks/
> ```
>
> See `docs/WOSY.md` for the full operator manual.

## Configuration

### Output style

`claude/output-styles/coworker.md` is the Coworker style — a controlled-English response format built on ASD-STE100 Issue 9. Copy it and point `settings.json` at it:

```bash
mkdir -p ~/.claude/output-styles
cp claude/output-styles/coworker.md ~/.claude/output-styles/
```

```json
{ "outputStyle": "Coworker" }
```

**The value is the style's frontmatter `name` field, not its filename.** `coworker.md` declares `name: Coworker`, so `settings.json` must say `Coworker` with the capital C. Claude Code builds its style map keyed by `name` and falls back to the filename only when `name` is absent. A case mismatch loads no style at all, and it reports no warning and writes no log entry — the session simply runs unstyled.

Verify with `/output-style` in an interactive session. It must report `Coworker`. To confirm the rules actually reached the system prompt, ask the session whether it carries a phrase unique to the file, and run the same probe against a name that does not exist. A probe that answers yes in both arms proves nothing.

### Brain-home location

Default: `~/Documents/.devwork/`. Override:

```bash
export WOSY_DEVWORK="$HOME/projects/.devwork"   # in your shell profile
./install.sh --apply
```

### Scan roots for stale-task nudges

The Stop hook (`wosy-stop.sh`) scans for `.devwork/tasks/` folders untouched for >7 days and nudges to `/consolidate` them. Tell it where your projects live:

```bash
# in ~/.zshrc or ~/.bashrc
export WOSY_SCAN_ROOTS="$HOME/projects:$HOME/work"
```

When unset, the hook falls back to `$HOME/projects` and `$HOME/Documents` — silently skipping anything that doesn't exist.

### Per-project enforcement

wosy enforcement is opt-in via `.devwork/wosy.flags` in each project. Two starter profiles ship in `reference-flags/`:

| File | Profile |
|---|---|
| `brain-home-minimal.flags` | Minimal — for the brain-home itself |
| `full-enforcement.flags` | Full — inline-SQL block + KB-scribe routing |

Copy one into your project's `.devwork/` and adjust per the operator manual (`docs/WOSY.md` §6).

## Uninstall

```bash
./install.sh --uninstall --apply
```

This removes the watchdog hook + settings matcher. The brain-home data store at `$WOSY_DEVWORK` is **preserved** — delete manually if you want a clean slate:

```bash
rm -rf "${WOSY_DEVWORK:-$HOME/Documents/.devwork}"
```

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `declare -A: command not found` | macOS system bash (3.2) used to run install.sh | Install Homebrew bash + re-run: `/opt/homebrew/bin/bash ./install.sh --apply` |
| `precondition: jq MISSING` | jq not installed | `brew install jq` |
| `precondition: source hook MISSING` | Running install.sh from outside the repo | `cd` into the repo root and re-run |
| `brain: command not found` | Binary not on `$PATH` | Confirm `~/.local/bin` is in `$PATH`, or use `BRAIN_BIN=/abs/path/to/brain ./install.sh --apply` |
| Hooks "silently do nothing" | `~/.claude/settings.json` matcher missing | Re-run `./install.sh --apply`; check `bash brain/smoke-wosy.sh` output |

## Updating wosy

```bash
cd ~/Documents/GitHub/wosy
git pull
cd brain/cmd/brain && go build -o "$HOME/.local/bin/brain" . && cd -
./install.sh --apply
bash brain/smoke-wosy.sh   # confirm
```
