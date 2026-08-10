# WOSY — Workflow + Knowledge System

> A per-project knowledge + workflow substrate for Claude Code: agnostic schema, single brain binary, PreToolUse watchdog hook, opt-in enforcement flags, and a suite of skills that drive a gated protocol.

## What's in this repo

```
claude/                 deploy targets — copied to ~/.claude/
  CLAUDE.md             user-global instructions (the operator law)
  RTK.md                Rust Token Killer companion
  settings.example.json reference hook wiring + permissions (template, NOT auto-deployed)
  agents/               subagent role definitions
  hooks/                PreToolUse / Stop / SessionEnd hooks
    lib/                shared helpers (detect-stack, detect-db-engine, detect-platform, walk-up, wosy-flags)
  skills/               active slash-skills (/wire-up, /interview, /handoff, …)
brain/                  Go source for the `brain` CLI binary
  cmd/brain/            CLI entry point
  internal/             cli · store · guard · detect · promoter · connections
  schema/               record ontology (umbrella · project · task · artifact · decision · spec · edge) + check.py
  hooks/                brain-side hook protocol docs
docs/
  WOSY.md               the operator manual (read first)
  conventions/SOURCE.md provenance convention for .devwork/data/
reference-flags/        sample wosy.flags profiles
  brain-home-minimal.flags
  full-enforcement.flags
examples/               agnostic templates (per-project CLAUDE.md, connections.md)
install.sh              deploy script (repo → live ~/.claude/ + ~/.local/bin/)
INSTALL.md              step-by-step installation guide
CONTRIBUTING.md         contribution guidelines
LICENSE                 MIT
```

## Install

See **[INSTALL.md](INSTALL.md)** for the full step-by-step guide (prerequisites, build, dry-run, apply, verify, troubleshooting).

Quick start:

```bash
git clone <this-repo> ~/Documents/GitHub/wosy
cd ~/Documents/GitHub/wosy
./install.sh            # dry run — prints the plan, mutates nothing
./install.sh --apply    # land changes
```

`install.sh` is the source of truth for the deploy. Repo is canonical; live state is derived. Edit here, then `./install.sh --apply` to push changes to `~/.claude/` and `~/.local/bin/`.

## Two layers (read this before the architecture)

WOSY ships two layers you interact with at different rhythms.

- **Work surface** — MD + YAML artifacts you author every day: specs, decisions, plans, status, task folders, hooks, skills, agents, `CLAUDE.md`. This is everything under `claude/` and `.devwork/`. If you're writing wosy as a user, this is where you spend 100% of your time.
- **Substrate** — the `brain` Go binary + sqlite store that enforces routing, validates schemas, and persists records under the hood. This is everything under `brain/`. You build it once at install time (`go build`); after that it runs silently as the engine behind the work surface.

The work surface is what you write. The substrate is what runs. Both are wosy — but a contributor touching the surface rarely needs to touch the substrate, and vice versa.

## How wosy works (in one paragraph)

The `brain` binary is the structural backbone — sqlite-backed, schema-validated, one record per entity, traversable edges. The PreToolUse `watchdog.sh` hook (Claude Code shim) routes Bash/Write/Edit/MultiEdit/NotebookEdit through `brain guard` to classify and block by per-project policy. Policy is opt-in via `.devwork/wosy.flags` — absent file means no enforcement (fail-open). Skills (`/wire-up`, `/interview`, `/handoff`, `/consolidate`, `/query`, `/graphify`, `/hq-orchestrator`, `/context`, `/constitution-rebuild`) drive a gated workflow protocol; task kickoff itself is artifacts-first — create the task folder / plan files directly, no skill required. Hook libs (`detect-stack.sh`, `detect-db-engine.sh`, `walk-up.sh`, `wosy-flags.sh`) are the single source of truth for capability discovery.

Read `docs/WOSY.md` for the full operator manual.

## Build the brain binary from source

```bash
cd brain/cmd/brain && go build -o $HOME/.local/bin/brain .
$HOME/.local/bin/brain version    # → brain 0.1.0-p1
$HOME/.local/bin/brain selftest   # → selftest OK
```

Go 1.21+ required. No CGO. Cross-platform.

## Versioning

This repo is the canonical source of changes. Versions live in `docs/WOSY.md` §13 changelog. Each fix gets its own commit; CHANGELOG.md tracks the user-facing summary per release.

## Contributing

See **[CONTRIBUTING.md](CONTRIBUTING.md)**. In short: open an issue before non-trivial PRs, run `bash brain/smoke-integrity.sh` (repo + install integrity) and `bash brain/smoke-wosy.sh` (enforcement) before submitting, no machine-specific paths or real secrets in commits.

## License

MIT — see [LICENSE](LICENSE).
