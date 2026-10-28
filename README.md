# wosy

A working system for Claude Code: a capped global instruction file, safety hooks, a small set of subagents, a task protocol with schemas, and `brain`, a per-project knowledge store.

wosy 1 (the 1.3.x harness with `install.sh`, watchdog hooks and 15 skills) lives on tag [`v1-final`](../../tree/v1-final). This README describes 2.0.0.

## What's in this repo

```
claude/            copy into your Claude home (~/.claude)
  CLAUDE.md        global rules, capped: nothing in it needs this owner's machine or work setup
  RTK.md           optional, only if you use rtk; import it with @RTK.md
  output-styles/   Coworker: verdict first, evidence tags, ASD-STE100 prose
  rules/           path-scoped rules (browser, iOS, PHP)
  agents/          qa-gate (change-set gate), web-researcher (bounded, cited research)
  hooks/           secret_guard.py, dispatch-bash.sh + stages, validate-bash.sh,
                   skill-consistency.sh, catalogue-loader.sh, session-audit.py
  statusline/      status line and subagent status line
  settings.example.json   hook wiring and permissions to merge into your settings.json
devwork/           copy into ~/.devwork
  bin/             protocolo, encargo-check, encuadre, briefing-check, memory-check, ...
  _schema/         one JSON schema per document kind, scaffolds/, example/broken pairs
brain/             Go source of the brain CLI (see docs/README-v2.md)
docs/              README-v2.md (brain), conventions/SOURCE.md
examples/          connections.md template
```

## Install

See [INSTALL.md](INSTALL.md).

## What each part does

- **`claude/CLAUDE.md`** — verify with a command, state assumptions, short comments, secret hygiene, artifacts under `.devwork/`, read-only by default.
- **Hooks** — `secret_guard.py` redacts secrets and PII before they reach the transcript and blocks the dump paths; `dispatch-bash.sh` runs every Bash call through the stages in `dispatch-bash.stages` and fails closed; `validate-bash.sh` refuses destructive commands and every `git push`.
- **`devwork/`** — a task is a folder of markdown with frontmatter; each document kind has a schema, and every schema ships an example that passes and a broken one that must fail. `protocolo` appends events to a task ledger.
- **`brain`** — one Go binary, one SQLite store per project. It keeps where things are and how they connect, not prose.

## Build the brain binary

```bash
cd brain && go build -o "$HOME/.local/bin/brain" ./cmd/brain
brain version    # brain 0.1.0-p1
```

Go 1.21+, no CGO.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). No machine-specific paths or real secrets in commits.

## License

MIT — see [LICENSE](LICENSE).
