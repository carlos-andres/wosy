---
# yaml-language-server: $schema=/Users/you/.devwork/_schema/context-card.schema.json
kind: context-card
scope: workspace
role: The card of the Platform umbrella. It tells a fresh session where it is, which world it works in, and which gate stops which command.
root: /Users/you/work/umbrella
repos: [worker-service, api-service, dashboard-app]
runtime:
  api-service: /opt/homebrew/opt/php@8.3/bin/php
  worker-service: /opt/homebrew/opt/php@7.4/bin/php
  dashboard-app: /Users/you/.nvm/versions/node/v18.20.8/bin
brain:
  store: /Users/you/work/umbrella/.devwork/connectors/brain.db
  worlds: [feed-alpha, collectors, clients]
boards: [TICKET, BOARD]
branches: feature/<TICKET> to release, hotfix/<TICKET> to production; develop, release, production
gates:
  - secret_guard.py blocks a command whose output would print a credential, and a bare mysql, mariadb, psql or redis-cli. Its denial names the safe form. Read it, do not retry.
  - validate-bash.sh runs first; hooks/dispatch-bash.sh chains both.
  - a commit, a push and a PR wait for the owner.
read_only:
  - the production and staging databases
  - every staging-* and prod-* host
  - anything under /srv on a server
created: 2026-09-29
updated: 2026-09-29
source_ref: /Users/you/.devwork/tasks/brain-continuity-20260923/reference-20260929/context.md
---

# Platform — the umbrella card

## What this umbrella makes

Data integration for Shopfront (TC), a listing and powersports inventory marketplace. One shared schema: `shopfront` on staging and production, `tower_shopfront` on the local clone. Three worlds, one per direction:

- **Inbound**: inventory data, mostly listings, arrives from many sources (XML, JSON, CSV, FTP, API) and vendors. `worker-service` validates and standardizes it into one qualified inventory row in TC, with its legs: images and files. Laravel Nova, in `api-service`, holds the configuration and the mappings. Worlds `collectors` (the generic engine) and `feed-alpha` (two named feeds).
- **Outbound**: the qualified inventory of TC is exported in several formats to merchants and vendors. `api-service` enqueues the export; `worker-service` delivers it, with data standardization where the export requires it. World `clients`.
- **Frontend**: `dashboard-app`, the merchant-facing SPA on the public API of `api-service`.

Each world has a frame at `.devwork/connectors/projects/<world>/master.md`. Reach it, do not copy it.

## Runtimes

The interpreters are in the frontmatter under `runtime`. direnv pins them in the owner's terminal and never in an agent shell, so name the absolute path or the variable. A bare `php` or `node` breaks. Prefer a `.devwork/bin/` shim to either. Read `runtime`, never a version written in prose.

## Reaching data and servers

`.devwork/connections.md` is the source of truth for hosts, aliases and routes. It carries credentials, so never `cat` it. Pull one alias with `rg -n -o --replace 'HIT' '<pattern>' .devwork/connections.md`. The credentials already live in `~/.mylogin.cnf` and `~/.ssh/config`.

- SQL runs through `mysqlp <profile> < query.sql`, the binary at `~/.local/bin/mysqlp`, reachable from anywhere. It validates the profile, then runs `mysql --defaults-group-suffix=_<profile> --login-path=<profile>` with any option you add. An agent writes a self-contained `.sql` file under `.devwork/` and hands the owner the `!` line. The line carries the absolute path of the file, so no relative path breaks and no credential reaches the transcript.
- The production profiles set `safe-updates`, and its side effect truncates a `SELECT` at 1000 rows in silence. A bulk read sends `SET SQL_SELECT_LIMIT=DEFAULT;` first.
- Before you write SQL: read `.devwork/queries/library/INDEX.md`, then `.devwork/schema/<table>.md`. Never invent a column.
- Verify an SSH alias with `ssh -G <alias>` before you call it missing.

## Branches and releases

Three branches, one flow. `develop` takes the normal workload. `release` takes every task. `production` takes release candidates and hotfixes. The lifecycle is three weeks. At mid-cycle every developer freezes: they send their work from `develop` to `release`, and QA runs on staging only. The branch pattern is in the frontmatter under `branches`. `origin/HEAD` reads `develop` in all three repos, and `develop` is not the target.

A patch by `scp` to production is an exception on `worker-service` only, because the owner owns that repo and its servers. It is never the default.

## The store

The store holds where things are and how they connect, never the detail. Before a task, compose its context: `brain pallet <term>`, from this root. Load one world only when the work touches it: `brain load <world>`. Query read-only: `brain query "<SELECT>"`. Record a trap: `brain note <world> "<text>" --severity=warn`. Read the blocking traps before you touch a pipeline:

```
brain query "SELECT project_id, body_md FROM gotcha WHERE severity='error'"
```

## Load on demand

| you need | read |
|---|---|
| the frame of a world: pipeline, scope, gotchas, connections | `.devwork/connectors/projects/<world>/master.md` |
| the shared-table contract between repos, and the stack | `.devwork/architecture.md` |
| a class, command or job by name | `.devwork/flows/`, then `rg` for the symbol |
| a table's columns | `.devwork/schema/<table>.md` |
| a ready SQL | `.devwork/queries/library/INDEX.md` |
| the layout of this `.devwork/` and the full conventions | `.devwork/harness.md`, section Layout |
| a repo's rules and runtime | `<repo>/.devwork/context.md` |
| the state of one task | `.devwork/tasks/<id>/context.md`, then its `logbook.jsonl` |
| how to recreate a document under the standard | `.devwork/runbooks/recreate-a-document.md` |
| drafts and throwaway files | `.devwork/_scratch/`, the always-writable path |
