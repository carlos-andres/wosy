# wosy v2 — what it is, its rules, its protocol, its uses

For any agent harness: Claude Code, Codex, OpenCode or a plain shell. wosy v2 needs no harness. A harness adapter only tells its sessions that wosy exists and points them at this file.

Status 2026-10-02. Every command below ran on that date against a fresh scratch project, or is cited from the usage protocol it comes from.

## 1. What wosy v2 is

Two parts:

- **`brain`**, one Go binary, installed once per machine (`~/.local/bin/brain`). `brain version` prints its build.
- **`brain.db`**, one SQLite store **per project**, at `<project>/.devwork/brain.db`.

It supersedes wosy 1 (the Claude Code harness of CHANGELOG 1.3.x, still under `claude/` in this repo) and the Second Brain, which is now a read-only spec.

The store keeps **where things are and how they connect**: projects, tasks, edges, gotchas, lessons, ruled-out claims, query pointers, scope entries and synonyms. It keeps no detail and no prose. Documents stay in files, and the store points at them.

## 2. Install

1. **Binary.** Build `brain/` (see the main README, "Build the brain binary from source") and put it on `PATH`.
2. **Store, once per project:**
   ```
   cd <project>
   mkdir -p .devwork
   brain init .devwork/brain.db
   printf 'hub: %s/.devwork\n' "$PWD" > .devwork/wosy.yml
   ```
3. **The logbook.** `brain pallet` refuses to answer (exit 2) until `.devwork/logbook.jsonl` holds at least one event with an `id`. Seed it with the first decision of the project (event shape in §5).
4. **Register the project:** `brain project register --id=<slug> --team=<team> --root=<project>`.

**Store resolution**, every verb except `init`: `--store=<path>`, then `$BRAIN_STORE`, then the nearest `.devwork/wosy.yml` `hub:` walking up from the working directory, then `./.brain/store.db`, which is refused when missing. Any subfolder of the project reaches its store. Verified from `<project>/src/deep`.

## 3. Rules

1. **Read before you open.** Before opening a world document, a flow or code, run `brain pallet <term>`.
2. **The store is read-only for SQL.** `brain query` accepts one `SELECT` or `PRAGMA`. Nothing writes the store except a `brain` verb.
3. **Agents only append.** An agent may run `brain note`. Every other write verb goes through the owner (§5).
4. **No credentials, anywhere.** The write verbs refuse a credential-shaped value, and the store is left unchanged.
5. **`-h` is free.** `brain <verb> -h` prints help and touches no store.
6. **A decision counts only when it is in the logbook.** Record the owner's answer verbatim before acting on it.
7. **A failed step stops the work.** Report it. Never work around it.

## 4. Read

| need | command | returns |
|---|---|---|
| the routes for a term (a project id or a synonym) | `brain pallet <term>` | one JSON pallet gated by `pallet-brief.schema.json`: a `brief` with five fields (`de_que_trata`, `en_que_nos_quedamos`, `para_que`, `porque`, `que_sigue`), the rows it used (`built_from`), and the gaps it found. Exit 1 unknown term, 2 no logbook, 3 over budget (`--budget=N`) |
| one fact | `brain query "<SELECT>"` (add `--json` for JSON) | rows |
| a library query, ready for the database client | `encuadre sql <query> NAME=value ...` (owner's home tool) | a self-contained `.sql` with its values inlined, for `mysqlp`; `q.sh` is retired |
| one record or one section of it | `brain get --<type>=<id> [--section=<f>]`, e.g. `--task=<task-id>` | JSON; "record not found" for a project known only by `project register` |
| a project bundle | `brain load <id>` | the warm-up bundle |
| the live schema | `brain schema --tables` | DDL |

**What a pallet holds.** A pallet holds pointers, not content. For the worlds it reaches, it lists:
- the runbooks, `runbooks/<world>/*.md` plus any top-level runbook that names the term;
- the schema cards of the world's scope tables;
- the query pointers into the library.

The world's runbooks and schema cards were added in commit e563f8d. Its gotchas, lessons and ruled-out rows are the ones whose **text names the term**, not every row filed under the project. On 2026-10-02, `brain pallet a123-irv` returned 2 of the 195 live a123-irv gotchas. When the work needs a project's own rows, ask the store directly:

```
brain query "SELECT id, severity, body_md FROM gotcha WHERE project_id='<id>' AND superseded_by IS NULL"
```

Read the blocking ones before touching a pipeline: add `AND severity='error'`.

## 5. Write and update

| verb | who | command |
|---|---|---|
| note (append a gotcha) | the agent | `brain note <project> "<text>" --source=<path or ref> [--severity=info\|warn\|error]` |
| confirm, or supersede, a fact | at the close, through the logbook | `brain confirm <gotcha\|lesson>:<id> [--contradicted=<successor>]` |
| create a record | the owner | `brain import <file.json>` |
| replace one section | the owner | `brain set --<type>=<id> --section=<f> --input=<v>` |
| register a project | the owner | `brain project register ...` |
| apply task deltas | the owner | `brain reconcile <project>` |

**"The owner" means a self-verifying applier.** The agent writes a script with three modes: `--verify`, red before the change; a control inside it that must fail; and `--revert`. The owner runs it. A superseded row stays in the store, and `pallet` and `load` skip it.

**The logbook** is `<task>/logbook.jsonl`, one event per line, append-only. Shape:

```
{"id":"<ts>-<n>","ts":"<UTC ISO>","type":"decision|verification|correction","project":"<id>","task_id":"<task>","pointer":{"kind":"path","value":"<path>"},"text":"<what happened, citing earlier ids>"}
```

Write it with `jq -nc`, gate it with `check-jsonschema --schemafile logbook-event.schema.json`, then append it. A verification event that re-checked a store row says `CONFIRMS gotcha:<id>`, and the close stamps only those rows.

## 6. The session protocol

1. **Receive.** Run the verify of the briefing you were handed, write the verdict (GREEN or RED) into it and into the logbook. On RED, stop.
2. **Read** through §4.
3. **Work and decide.** Ask the owner whatever is the owner's to decide. Log every answer (§3.6).
4. **Write** through §5.
5. **Close.** The owner picks the moment. The close appends a final event, gates it, stamps the `CONFIRMS` rows and seals the ledger. Any failure writes nothing.

Steps 1 and 5 use the owner's protocol tools (`protocolo`, `briefing-check`, `gen-context`). Those live in the owner's home hub (`~/.devwork/bin`), not in this repo. They are plain python3 and read no harness transcript. The full step-by-step protocol, with every command: `~/.devwork/tasks/factory-redesign-20260911/protocol-wosy-v2-20261001.md`.

## 7. Uses

- **Start of a task:** `brain pallet <world>`, then the project's `error` gotchas.
- **A trap found mid-work:** `brain note <project> "<what breaks and how to see it>" --source=<file:line>`.
- **"Was this already tried?"** `brain query "SELECT claim, negative_evidence FROM ruled_out WHERE project_id='<id>'"`.
- **A ticket or a dealer:** `brain query "SELECT from_id, rel, to_id FROM edge WHERE to_id='ticket:<KEY>'"`, then pallet the world it points at.

## 8. What an adapter does

An adapter is per harness and lives outside the binary. It does three things:

1. Puts `brain` on the session's `PATH`.
2. Loads, at session start, the rule "read with `brain pallet` before opening anything" and the list of worlds, in the harness's own instruction file (Codex and OpenCode: `AGENTS.md`).
3. Points at this README.

The Claude Code adapter exists and is not changed by v2: a SessionStart hook, `catalogue-loader.sh`, plus `CLAUDE.md`. Codex and OpenCode each write their own installation from this file.

## 9. Known limits on 2026-10-02

- **No verb prints the catalogue yet.** The decision is `AGENTS.md` plus a `brain` verb (factory-redesign logbook 61). Until then an adapter lists the worlds itself, for example with `brain query "SELECT id FROM project"`.
- **The session brief stays out of v2.** It is the hand-written briefing the protocol receives in §6.1, and its design is still open.
- **Leftovers from the v1 harness:** `internal/detect` defaults to `~/.claude/hooks/lib` and no package imports it; `brain build` names a Claude agent file; `brain guard` takes a tool-call shape and cites `CLAUDE.md`. None of them is on the read or write path above.
- **Not yet proven agnostic in practice:** no Codex or OpenCode session has run against a store so far. That test is deferred by the owner.
