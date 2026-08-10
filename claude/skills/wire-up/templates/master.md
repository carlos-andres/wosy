# {{project_name}} — master

> Hand-filled by the user during `/wire-up` Tier 4 mini-workshop. `brain build` reads this file to extract `pipeline_stages`, `scope_entry`, `gotcha` rows into brain.db. Do not delete sections — leave empty if not yet known. Replace `{{...}}` placeholders.

---

## Identity

- **Project slug**: `{{project_slug}}`
- **Team**: `{{team}}`
- **Display name**: {{project_display_name}}
- **Status**: active / paused / archived (pick one)
- **Created**: {{date}}

## Pipeline

> Ordered list of stages this project moves data/work through. Each stage gets ONE line: `N. <stage-name> — <one-sentence description> [STATUS]` where STATUS is one of `green` (healthy), `yellow` (degraded), `red` (broken).

1. {{stage-name}} — {{one-sentence description}} [green/yellow/red]
2. ...

## Scope

> What this project touches. Group by kind. `agent-brain-builder` infers kind from the bullet prefix.

- **Files**:
  - `{{path/to/file.ext}}` — {{role}} [green/yellow/red]
- **Imports** (external libs / packages):
  - `{{lib@version}}` — [green/yellow/red]
- **External** (URLs, APIs, S3 buckets):
  - `{{https://... or s3://...}}` — [green/yellow/red]
- **Feeds** (inbound data):
  - `{{feed-id}}` — {{cadence}} [green/yellow/red]

## Gotchas

> Durable warnings. Format: `severity: <short description>`. Severity ∈ info / warn / error. Each gotcha gets ONE paragraph.

- **warn**: {{describe one trap that future-you will fall into}}
- **info**: {{describe a non-obvious convention worth knowing}}
- **error**: {{describe a bug pattern that recurs}}

## Connections

> DB / SSH / API endpoints. Format: `alias (kind): target [creds: <ref>]`.

- `{{alias}}` (`mysql`/`postgres`/`sqlite`/`redis`/`ssh`/`http`): `{{host:port/db or URL}}` [creds: `1password://...`]

## Pipeline conventions / cuidados

> Free-form prose. Anything that doesn't fit a structured field above but a fresh-context Claude needs to know. `agent-brain-builder` does NOT extract from this section into brain.db (it's reference for humans + main-session reads).

{{free-form notes}}
