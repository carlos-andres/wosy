# Brain registry — the single entry point ("work on X")

The registry is **per-installation** — your `~/.devwork/brain/store.db` (or whatever your `$WOSY_DEVWORK/brain/store.db` resolves to) holds umbrella + project records as you onboard them. There is no shipped registry; this document describes the model and how to grow yours.

## Concept

Resolve a name → its umbrella → its working set. **Capability is owned once at the umbrella, referenced by projects.** This avoids re-detection per project and prevents capability drift (project says "mysql"; umbrella says "pgsql" — the umbrella wins, and the discrepancy surfaces).

A minimal registry has one umbrella and one project. A grown registry has multiple umbrellas (personal lab vs corporate), each with multiple projects (`<name>-<short>`) that the REHYDRATOR can warm into the working set.

## Schema shape (per umbrella, per project)

| field | umbrella | project |
|---|---|---|
| `id` | kebab-case identifier (e.g. `corporate-umbrella`) | kebab-case (e.g. `<project-name>-<short>`) |
| `location` | absolute path to the workspace root | absolute path to the project dir |
| `capability` | `{stack, db_engine, connections, schema, queries_library, runner}` | inherited from umbrella |
| `state` | `onboarded` / `reserved` (record on first touch) / `deferred` | same |
| `store` | path to the brain store under `<workspace>/.devwork/brain/store.db` | same |

## Capability — detected, owned once per umbrella

When `/wire-up` runs on a project, it detects stack + DB engine via `~/.claude/hooks/lib/detect-stack.sh` and `detect-db-engine.sh`. The capability presence flags (connections, schema, queries/library, runner) come from `brain detect <path>`. The umbrella owns the canonical capability; projects inherit it.

## How a store is born for a new project

```bash
brain init <project>/.devwork/brain/store.db
brain detect <project> | jq '{...umbrella...}' | brain import --store=… -
brain import --store=… <project-record.json>     # validated on the way in
```

After this:
- `brain show <project-id>` produces a one-shot human card
- `brain load <project-id>` produces the warm working set for REHYDRATION

## Runner contracts (when the project uses a `q.sh` SQL runner)

The WATCHDOG routes inline DB ops to the project's `bin/q.sh` runner when one exists. Runner signatures differ by engine:

- **pgsql:** `q.sh queries/library/<NAME>.sql -v KEY=value` (psql `-v` substitution)
- **mysql:** `q.sh queries/library/<NAME>.sql [POSITIONAL_ARGS...]` (mysql `-v` means verbose; positional args used instead)
- **sqlite:** `q.sh queries/library/<NAME>.sql` (parameters via `.parameter` or query-side placeholders)

The runner is project-side, not wosy-side. Document its signature in your project's `CLAUDE.md` or `.devwork/scripts/README.md` so subsequent sessions don't re-derive it.

## Notes

- **`brain doctor`** reports dangling edges + self-loops (exit 1 if any) — run before a wrap.
- **`brain doctor --integrity`** checks the logical-sha pin (`expected_logical_sha` in your `.devwork/wosy.flags`) — provisioned-then-enforced; absence is fail-open.
- **Ledger / external record-keeping integration.** If a project already has an external records system (`ledger.yml`, an issue tracker, a spreadsheet), brain can import from it; conversely, brain records can be exported. "Unify, don't reinvent."
