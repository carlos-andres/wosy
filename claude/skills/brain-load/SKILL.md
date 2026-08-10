---
name: brain-load
description: Warm-cache a project's full context into the current session in ONE call. Thin pass-through to the `brain load <project>` CLI verb (C-03 §5.2 trigger #2). Returns the 10-section bundle (identity / pipeline_stages / scope+health / connections / queries / runbooks / encyclopedia_sections / recent_tasks / recent_commits / gotchas) defined by brain/schema/bundle.schema.json. Use after `/clear` or when starting work on a project mid-session without firing the full `/interview` discovery flow.
when_to_use: User types `/brain-load <project>`, says "load context for <project>" / "warm cache <project>", or starts work on a project whose name matches a row in the per-team `brain.db`. For fuzzy discovery use `/interview` (which auto-calls this); for hashtag-driven warming the `brain-load-trigger.sh` hook (A-03 §4.2) covers `#contexto <project>` in any user message.
disable-model-invocation: true
---

# /brain-load (on-demand · pass-through · warm-cache)

Drop a project's full structured context into the session in one round trip. No discovery, no questions — just rehydration.

## When this fires

User types `/brain-load <project>` (positional project slug) or says "load context for X" / "warm cache X" where X is a known project. Three peer triggers exist for the same `brain load <project>` call:
- this slash (when the user wants warm-cache and nothing else)
- `#contexto <project>` hashtag → `brain-load-trigger.sh` hook (A-03 §4.2)
- auto-on-`/interview` (C-01 step M4 — when discovery references a known project)

## Workflow

One bash call. The argument is the project slug (matches `project.id` or `project.slug` in the per-team `brain.db`).

```bash
brain load "$PROJECT"
```

The CLI dispatches on shape (Fork 2 lock 2026-06-08): if `$PROJECT` resolves to a `project` table row, returns the 10-section JSON bundle; if not, falls through to legacy single-record warm and returns "record not found" when the id matches neither path.

## Output

A JSON object conforming to `brain/schema/bundle.schema.json`. Pipe to `jq` if the user wants a specific section:

```bash
brain load <slug> | jq .pipeline_stages    # one section
brain load <slug> | jq .recent_tasks        # active work
brain load <slug> | jq '.gotchas[] | select(.severity=="error")'
```

Inject the full JSON into the conversation as additional context. The user does NOT need it pretty-printed — Claude consumes it directly.

## Hard rules

- Pass-through only. This skill does NOT transform the bundle, summarize, or filter. Use `jq` for that.
- `--store=<path>` is optional. Without it, brain resolves the store via `BRAIN_STORE` env or the default fallback (R57). When the user is in a multi-team workspace, pass `--store=.devwork/<team>/brain.db` explicitly.
- "Project not found" is a real answer, not a failure to retry. If `brain load` returns "record not found", the project hasn't been registered yet — suggest `/wire-up` (which runs `brain init --team` + registers the project) instead of guessing slugs.
- Never invent project slugs. If the user names a project not present in `brain list --projects`, surface that gap; do not pick the closest match silently.
