# wosy

A working system for Claude Code: a capped global instruction file, safety hooks, a Subagent classifier, a task protocol with schemas and a phase gate, three method skills, and `brain`, a per-project knowledge store.

wosy 1 (the 1.3.x harness with `install.sh`, watchdog hooks and 15 skills) lives on tag [`v1-final`](../../tree/v1-final). The last 2.x state is on tag [`v2.1.0`](../../tree/v2.1.0). This README describes 3.0.0.

## What's in this repo

```
claude/            copy into your Claude home (~/.claude)
  CLAUDE.md        global rules, capped: nothing in it needs this owner's machine or work setup
  RTK.md           optional, only if you use rtk; import it with @RTK.md
  output-styles/   Coworker: verdict first, evidence tags, ASD-STE100 prose
  rules/           path-scoped rules (browser, code, .devwork, iOS, PHP, SQL)
  agents/          scout, extractor, archaeologist, web-researcher, builder, qa-gate
  skills/          build-method, interview, harness-work
  hooks/           secret_guard.py + classifier.tsv, dispatch-bash.sh + stages, validate-bash.sh,
                   gate.py, orchestrator.py, anchor.py, badge.py, dispatch-record.py,
                   qa-record.py, round-record.py, budget-record.py, skill-consistency.sh,
                   catalogue-loader.sh, session-audit.py
  statusline/      status line and subagent status line
  settings.example.json   hook wiring and permissions to merge into your settings.json
devwork/           copy into ~/.devwork
  bin/             protocolo, plan-graph, doc-gate, task-lint, telemetry.py, encargo-check,
                   encuadre, briefing-check, memory-check, ctx, gen-status.py, ...
  _schema/         one JSON schema per document kind, scaffolds/, example/broken pairs
  AGENTS.md        pointer for agents of other tools
brain/             Go source of the brain CLI (see docs/README-v2.md)
docs/              README-v2.md (brain), conventions/SOURCE.md
examples/          connections.md template
```

## Install

See [INSTALL.md](INSTALL.md). A 2.1.0 install is not upgraded by copying 3.0.0 over it: read the upgrade note in [CHANGELOG.md](CHANGELOG.md).

## What each part does

- **`claude/CLAUDE.md`** — verify with a command, state assumptions, secret hygiene, read-only by default, verification before done, and how to dispatch subagents.
- **The classifier** — `claude/hooks/classifier.tsv` holds one row per kind of work: the agent, the model and the effort. `secret_guard.py` enforces it on every subagent dispatch, and every agent a row names ships under `claude/agents/`.
- **The safety hooks** — `secret_guard.py` redacts secrets and PII before they reach the transcript and blocks the dump paths. `dispatch-bash.sh` runs every Bash call through the stages in `dispatch-bash.stages` and fails closed. `validate-bash.sh` refuses destructive commands and every `git push`.
- **The task hooks** — they act only in a session bound to a task, except rules P90 and P91 of `orchestrator.py`, which judge every session. `anchor.py` binds the session and loads the task. `gate.py` holds writes and questions to the phase the task is in. `orchestrator.py` checks the steps of the build method. `badge.py`, `dispatch-record.py`, `qa-record.py` and `round-record.py` give each subagent the task badge and record dispatches, test runs and question rounds.
- **The skills** — `interview` opens a task and ends in a `context.md` the owner freezes. `build-method` is the protocol of a main session that builds plan nodes through subagents and re-runs every proof. `harness-work` holds the rules for work on the harness itself.
- **`devwork/`** — a task is a folder of markdown with frontmatter; each document kind has a schema, and every schema ships an example that passes and a broken one that must fail. `protocolo` appends events to a task ledger, binds a session, loads a task and writes the handoff. `plan-graph` checks a plan and runs its proofs. `doc-gate` validates a document against the schema of its kind.
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
