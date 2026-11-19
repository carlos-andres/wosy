---
paths:
  - "**/.devwork/**"
---

# .devwork

- Write to the umbrella `.devwork/`, never to a repository's own `.devwork/`. Work under `/Users/you/work/source` writes to `/Users/you/work/source/.devwork/`. Work under `/Users/you/work/umbrella` writes to `/Users/you/work/umbrella/.devwork/`. Outside both, use the nearest `.devwork/`, and ask before you create one.
- A repository's own `.devwork/` holds repo-level cards only (`context.md`, `constitution.md`, `flows/`). Read it for orientation and never put a task, a plan, a spec, a query or a scratch file there.
- `.devwork/` is the convention. The toolchain knows no other name, so recognize a different one and never add a second.

## Context artifacts

A context artifact carries state across the loss of a context window. Write it for the model that loads it next, not for a person.

- Record what you ruled out and why. A dead end that nobody records gets walked again.
- Name the open questions. An unknown that nobody names becomes an assumption.
- The artifact is the source of truth. When the user asks for its content, render a view. Do not rewrite the artifact to match the view.
