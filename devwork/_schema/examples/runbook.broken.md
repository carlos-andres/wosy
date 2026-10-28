---
# yaml-language-server: $schema=/Users/you/.devwork/_schema/runbook.schema.json
role: How a document of this .devwork is recreated under the standard, from which sources, who generates its frontmatter, who validates it, and what closes the step.
root: /Users/you/work/umbrella/.devwork
para: Any session, personal or corporate, recreates a house document the same as the last one, without inventing where to look or which fields it carries.
created: 2026-09-29
updated: 2026-09-29
source_ref: /Users/you/.devwork/tasks/brain-continuity-20260923/plan.yml
---

# Runbook — recreate a document

One dialect, one gate, one ledger. Ruled in `/Users/you/.devwork/tasks/brain-continuity-20260923/logbook.jsonl` entries 145, 187, 192, 193, 195 and 196. This runbook names the steps; the rulings hold the reasons.

## 1. Which contract applies

`kind` picks the schema. Every schema lives in `/Users/you/.devwork/_schema/` and every one carries an `examples/<schema>.example.md` that must pass and an `examples/<schema>.broken.md` that must fail.

| the document | `kind` | schema |
|---|---|---|
| `projects/<world>/master.md`, `architecture.md`, `connections.md` | `master`, `architecture`, `connections` | `world-doc.schema.json` |
| `tasks/<id>/context.md`, the single task document | `task-context` | `task-doc.schema.json` |
| `tasks/<id>/logbook.jsonl`, the task ledger, one event per line | none, JSONL | `logbook-event.schema.json` |
| `runbooks/<name>.md` | `runbook` | `runbook.schema.json` |
| the root `context.md` | `context-card` | `context-card.schema.json`, written in phase B |

Keys are English. `role` is one line of prose. `para` is the only field a human writes, in English. `additionalProperties` is false everywhere, so a `tipo:` key fails. Documents marked KEEP-AS-REFERENCE (`flows/`, `schema/`, `specs/`, `runbooks/` before 2026-09-29, `queries/`, `reports/`, `research/`, `interview/`) are read and cited, never edited.

## 2. Where each field comes from

The frontmatter is composed, not authored. Read `decision-badge-contract-20260925.md` section 6c.3 for the derivation.

- `kind`, `created`, `updated`: the convention and the clock. Bump `updated` on every write.
- `project_id`, `task_id`, `root`: the folder, as `brain` derives them. A task with no derivable world keeps its gap; do not invent one.
- `role`: what a session opened on this document is for. One line.
- `para`: Calo's words. Ask, do not paraphrase.
- `source_ref`: the document this one recreates, or the generator that produced it. The current file stays as reference until phase F.
- `state`, `where_we_are`, `next_step`, `done_when`, `waiting_on`: the living state, same names as `status.schema.json`. `blocked` requires `waiting_on`.
- `ledger`: the absolute path of the task's `logbook.jsonl`.

## 3. The steps

1. Read the current document in full. It is the reference, not the target.
2. Write the new document under the hub first: `/Users/you/.devwork/tasks/<task>/sql/<name>.md`.
3. Extract and validate the frontmatter, with its control in the same block:

```
S=/Users/you/.devwork/_schema; X=$(mktemp -d)
yq --front-matter=extract -o=json '.' <file>.md > $X/fm.json
check-jsonschema --schemafile $S/<kind>.schema.json $X/fm.json            # must exit 0
jq 'del(.para)' $X/fm.json > $X/bad.json
check-jsonschema --schemafile $S/<kind>.schema.json $X/bad.json           # must exit 1
```

4. Run the rendered check of the body. A world document carries the six sections Identity, Pipeline, Scope, Gotchas, Connections, Pipeline conventions / cuidados. A task document carries What it is, Why, What for, Where we are, What is next. Count them with `rg -c '^## '`.
5. Write a self-verifying applier under `sql/`, rehearse it on a temp copy with `--target`, and hand Calo the `!` line. An agent never writes to `/Volumes/data`, to `~/.local/bin` or to a Claude home.
6. Record the step in the task ledger before the write that depends on it. A ruling by Calo goes in verbatim.
7. Move the task in `plan.yml` with the `yq -i` line the file carries, and run its gate.

## 4. What does not go in the document

A value that wosy stores as a route, a relation, a conclusion or an identification does not get restated. The document carries the frame; wosy carries where things are and how they connect (`decision-what-wosy-stores-20260925.md` section 4). A dated snapshot belongs in a task, never in wosy.

## 5. The step is closed when

The schema exits 0 on the new file and 1 on its control, the rendered check counts every section, the applier's `--verify` reports the umbrella copy byte-identical to the hub copy, and the ledger carries the entry.
