---
kind: entry
role: The entry of the Platform umbrella. It says what this directory is, pins the interpreter rule, and loads the card. Everything else is a rule or a pointer.
root: /Users/you/work/umbrella
created: 2026-09-29
updated: 2026-09-29
source_ref: /Users/you/.devwork/tasks/brain-continuity-20260923/reference-20260929/CLAUDE.md
---

# Platform — the umbrella

This directory is the umbrella of the connectors team, not a git repo. It holds three repos as symlinks to `/Users/you/work/source/`:

| repo | what it is |
|---|---|
| `api-service` | Laravel and Nova. The API and the admin surface. |
| `worker-service` | PHP CLI. Feed ingest into the API, and export to marketplaces. |
| `dashboard-app` | Nuxt 2 SPA. The merchant-facing frontend. |

Run `git` inside the target repo. Read and edit through the `Source/` path, never through the symlink: one file reached by two paths causes repeated confusion. `inventory-feed-api/`, `hive/` and `roster.json` are out of scope until the owner names one; for `inventory-feed-api`, read `.devwork/tasks/inventory-feed-api/status.md` first.

## The interpreter rule

The umbrella has no runtime. Each repo pins its own through direnv, and direnv fires only in the owner's terminal: never in an agent shell, never under the `!` prefix. A bare `php`, `node` or `composer` runs the wrong version and breaks the repo. This is the most expensive fact here.

So an agent names the interpreter. Reach for a shim in `.devwork/bin/` first: `phpstan-tc`, `phpunit-tc`, `config-cache-tc`, `phpstan-worker`, `phpunit-worker`. When no shim covers the job, use the variable. `.claude/settings.local.json` sets the three of them:

```
$PHP_API artisan <cmd>                     # api-service
$PHP_API /opt/homebrew/bin/composer <cmd>  # composer, never bare
$PHP_WORKER RunSimple.php <args>           # worker-service
PATH=$NODE_DASHBOARD:$PATH yarn <cmd>      # dashboard-app
```

## Where to look

The card below is already in your context; its section Load on demand names every other document. The rules in `.claude/rules/` load with this file. Before a task, compose its context from the store: `brain pallet <term>`.

A session opened inside a repo does not load this file. Each repo carries its own entry at `<repo>/.claude/CLAUDE.md`.

@.devwork/context.md
