---
name: constitution-rebuild
description: Bootstrap a missing `.devwork/constitution.md` for a project — interactive. Auto-fills the mechanically-detectable sections (Stack from detect-stack.sh / Architecture from directory tree / Testing from detected framework / Quality Gates from linter configs); ASKS the user for the three judgment-heavy sections (Conventions / Do NOT Touch / Manual Notes). Writes `.devwork/constitution.next.md` for hand-merge — never overwrites an existing constitution. Use when a project's `.devwork/` exists but `constitution.md` is missing, or when `/interview` flags the gap.
when_to_use: User explicitly types `/constitution-rebuild`. Most relevant when a project's `.devwork/` exists but `constitution.md` is missing, or when `/interview` flags the gap. Interactive — Claude proposes the auto-filled sections and asks 3 questions before writing.
---

# /constitution-rebuild (interactive · 4 auto-fill + 3 user-asked · never overwrites)

Bootstrap a missing constitution file for the current project. Mechanical sections (Stack / Architecture / Testing / Quality Gates) are auto-filled from config; judgment sections (Conventions / Do NOT Touch / Manual Notes) are asked from the user. Never silent.

## When this fires

- Explicit `/constitution-rebuild` invocation.
- Soft suggestion from `/interview` when the project has `.devwork/` but no `constitution.md`.

## Output

`.devwork/constitution.md` (or `.next.md` if one already exists — never overwrites) with this skeleton:

```markdown
# Constitution: <project-name>

> Generated: <YYYY-MM-DD> by /constitution-rebuild

## Stack
<!-- AUTO-FILLED: language + version, framework + version, database, queue, search, monitoring -->

## Architecture
<!-- AUTO-FILLED: pattern name (MVC/hex/etc), directory tree (top 2-3 levels), entry points -->

## Testing
<!-- AUTO-FILLED: framework, base test class, suite names, run command -->

## Quality Gates
<!-- AUTO-FILLED: linters and static-analysis tools installed; what gates a PR -->

## Conventions
<!-- USER-ASKED: naming, type/format conventions, error handling style -->

## Do NOT Touch
<!-- USER-ASKED: preserved-on-update — legacy carve-outs the rebuilder must not regenerate -->

## Manual Notes
<!-- USER-ASKED: preserved-on-update — human-curated rules the rebuilder must not regenerate -->
```

## Workflow

### Phase A — Auto-detect (read-only, no questions)

The helper script `bin/build-constitution.sh` introspects the project for evidence and emits a PARTIAL fill covering the 4 mechanical sections only:

- `composer.json` → PHP language version + Laravel/Symfony framework + version
- `package.json` → Node version (engines field) + key framework (next/nuxt/express/react)
- `Gemfile` → Ruby version + Rails version
- `pyproject.toml` / `requirements.txt` → Python version + framework
- `go.mod` → Go version + key dependencies
- `.tool-versions` / `.nvmrc` / `.python-version` → language pins
- `docker-compose.yml` → datastore + queue + search infrastructure
- Top-level directories → directory-tree extraction (Architecture)
- `phpunit.xml` / `jest.config.*` / `pytest.ini` → Testing framework + run command
- `phpstan.neon` / `eslint.config.*` / `pyproject.toml` (ruff) / `.rubocop.yml` → Quality Gates
- `.git/config` → remote URL for the header

Invoke:

```bash
bash ~/.claude/skills/constitution-rebuild/bin/build-constitution.sh <project-root>
```

Read the resulting `.devwork/constitution.next.md` (partial — 4 sections filled, 3 empty). Echo the auto-detected facts as ONE keyed block:

```
DETECTED:
  stack:      <language X.Y · framework Z>
  arch:       <pattern · top-level dirs · entry points>
  testing:    <framework · run-cmd>
  gates:      <linters · static-analysis tools>
ASK: lock auto-detected facts as-is, or correct one? (lock / edit:<section>=<value>)
```

### Phase B — Interactive fill (3 questions, in order)

Only after the user locks Phase A. Ask one question at a time; never batch.

**Q-Conventions:**
> "Naming + format + error-handling conventions for this project. What do you want me to follow that isn't in CLAUDE.md or the existing code? (one paragraph, or `default` to leave the section empty)"

**Q-Do-NOT-Touch:**
> "Legacy carve-outs. Files / folders / patterns the rebuilder (and future Claude sessions) must NOT regenerate, refactor, or 'fix'. Vendor dirs, generated code, deprecated modules to be left alone. (bullet list, or `default`)"

**Q-Manual-Notes:**
> "Any human-curated rules, gotchas, or anchors you want preserved verbatim across rebuilds? Quoted commands, named owners, links to private docs. (paragraph or list, or `default`)"

`default` answers leave the section as an empty placeholder comment — the section header survives so future rebuilds re-prompt.

### Phase C — Merge + write

Fold the Phase A auto-fill and Phase B answers into the full template. Write atomically:

- No existing `.devwork/constitution.md` → write `.devwork/constitution.md` directly.
- Existing `.devwork/constitution.md` → write `.devwork/constitution.next.md` and report: "constitution.next.md ready for hand-merge against existing constitution.md". Never overwrite.

If the existing constitution carries content in `## Do NOT Touch` or `## Manual Notes`, PRESERVE it verbatim in the new file (preserved-on-update sections). Phase B answers append to (not replace) the preserved content.

### Phase D — Return ≤3-line summary

1. **Constitution at:** `.devwork/constitution.md` (or `.next.md` for hand-merge).
2. **Filled:** 4 auto + 3 user. Empty sections (`default` answers): `<list>` or "none".
3. **Next step:** review and edit if needed; `/interview` no longer flags the gap.

## Hard rules

- Never auto-fire from any hook. Explicit user trigger only.
- Never overwrite an existing `constitution.md`. Write `.next.md` for hand-merge.
- Never auto-fill Conventions / Do NOT Touch / Manual Notes. These are JUDGMENT sections — ask the user.
- Never write file-count snapshots like "Total PHP files (3,470)" — they go stale within a week and turn the constitution into a doc-as-database.
- Never invent tech-team rules. Tech-team-specific guidance belongs in `## Manual Notes` and only gets there from the user's Phase B answer.
- Preserve `## Do NOT Touch` and `## Manual Notes` content verbatim across rebuilds — they are preserved-on-update.
- One question at a time in Phase B. No batching, no menus. `default` is always an accepted answer.
- **Reconsider the skeleton if the first 3 generated constitutions get totally rewritten by the user** — that's the signal the template is wrong, not the input. Stop generating until the skeleton is fixed.
