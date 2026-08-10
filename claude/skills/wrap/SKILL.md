---
name: wrap
description: Close a work session in one call against the LIVED flat layout `.devwork/tasks/<id>/`. Locates the most-recently-modified task folder, updates its `status.md` in place (the canonical 3 sections), appends a dated entry to `context.md`, writes a generalizing feedback block (Corrections / Preferences / Self-improvements) to `.devwork/_scratch/session-feedback-YYYYMMDD.md`, then emits a paste-ready kickstart/resume prompt (Mission · Working directory · Authoritative documents read order · Operating constraints · Next concrete step · Open questions · What did NOT work · Done criteria), self-audited via `/kick`'s Step-4 pass. Replaces the hand-typed session-end ritual. Targets flat `tasks/<id>/` reality — never the unadopted `plans/<plan-id>/` C-02 shape.
when_to_use: User types `/wrap` or says "wrap here", "wrap up", "wrap this session", "lets close", "cierra la sesión", or just "wrap". Manual only — the session closer that persists state and hands a fresh-start prompt.
disable-model-invocation: true
---

# Wrap — close a work session (status.md · context.md · feedback · resume prompt)

One call that persists session state to the active task folder and returns a paste-ready resume prompt. Replaces the ritual the user hand-types at every session end.

**Scope note.** This targets the flat `.devwork/tasks/<id>/` reality — the lived, adopted layout. The older `/handoff` skill targets the unadopted `plans/<plan-id>/` C-02 shape (`plan.yml` + `task.yml` + agent-scribe writes) which does not exist on disk anywhere; see `.devwork/tasks/wosy-health-audit-20260730/wosy-system-map.md`. `/wrap` never reads or writes a `plans/` or `*.yml` path.

## When this fires

`/wrap`, "wrap here", "wrap up", "wrap this session", "lets close", "cierra la sesión", "wrap". Skip only if the user explicitly says "no wrap" / "don't persist".

## Workflow

### Step 1 — Locate the active task (read-only)

Walk up from cwd to the nearest `.devwork/`. In `.devwork/tasks/`, pick the folder with the most recent mtime (`fd -t d -d 1 . tasks/` then newest, or `ls -1td tasks/*/ | head -1`). If two are plausibly active or the newest is stale, list the top few and ask which one — do not guess. Echo the chosen path before writing.

### Step 2 — Update `status.md` in place

Edit the existing `tasks/<id>/status.md`; do not clobber sections the file already carries (Phase sequence, Files changed, Quality gates, Decisions captured). Rewrite only the canonical three, keeping the exact headings: **Where we are now** (one paragraph), **Next concrete step** (one bullet), **Open questions** (bullets, labeled blocking/non-blocking, or "none"). If the file is missing, create it with just those three sections.

### Step 3 — Append a dated entry to `context.md` (append-only)

If `tasks/<id>/context.md` exists, append `## YYYY-MM-DD — <heading>` (date via `date +%Y-%m-%d`) followed by a short prose note of what this session did. Never overwrite prior entries; never lose earlier trace. If `context.md` is absent, skip — do not create it.

### Step 4 — Write the feedback block

Write to `.devwork/_scratch/session-feedback-YYYYMMDD.md` (date via `date +%Y%m%d`) so it survives session end. Three headings — **Corrections**, **Preferences**, **Self-improvements** — each a list of one-line actionable rules. Include ONLY items that generalize beyond this session (a rule the next task would also want). A heading with nothing generalizing stays empty; do not manufacture filler.

### Step 5 — Emit the paste-ready resume prompt

Fill the cached template below and print it in chat in ONE fenced block. Fields are fixed and all eight are required. Reference documents by path only — never embed their content (paths point at the live files, which stay the source of truth). Then run `/kick`'s Step-4 self-audit on the assembled prompt (specificity, scope + done_when, references-grounded, output format, format-pro); fix any WARN in place before emitting, and note the fixes in one line.

## Resume-prompt template (cached — do not hand-type the read order)

```
Resuming task: <task-id>

# Mission
<one sentence: the objective — distilled from status.md "Where we are now" + "Next concrete step">

# Working directory
<absolute cwd, e.g. /Volumes/work/Globex/Teams/Platform>

# Authoritative documents — read IN THIS ORDER (paths only; do NOT embed content)
1. .devwork/tasks/<id>/status.md    — live state (where we are / next / open questions)
2. .devwork/tasks/<id>/context.md   — append-only session log (if present)
3. .devwork/tasks/<id>/findings.md  — evidence / verdicts (if present)
4. <canonical SRS / system-map / plan the task references, by path — if any>

# Operating constraints
- <read-only vs mutation · user-gated commits/pushes · secret-redaction · tool prefs — carry from status.md / CLAUDE.md, do not reinvent>

# Next concrete step
<the single bullet from status.md "Next concrete step">

# Open questions
<bullets from status.md, or "none">

# What did NOT work
<dead ends / rejected approaches this session, so the next session does not repeat them>

# Done criteria
<falsifiable done_when for the current step>
```

## Hard rules

- **Flat layout only.** Write exclusively to `.devwork/tasks/<id>/*.md` and `.devwork/_scratch/`. Never a `plans/<plan-id>/`, `plan.yml`, or `task.yml` path — the C-02 ghost layer does not exist on disk.
- **status.md edited, not replaced.** Preserve unrelated sections; rewrite only the three canonical headings. Match the file's existing heading text exactly.
- **context.md is append-only.** `## YYYY-MM-DD — <heading>` appended; earlier entries untouched. Skip if absent — do not create it.
- **Feedback generalizes or is omitted.** Only rules the next task would also want. Empty headings are fine; no filler.
- **Resume prompt references by path, never embeds.** The live docs are the source of truth; the prompt is the gateway. Run `/kick` Step-4 audit before emitting.
- **Writes are the only mutations.** status.md (edit) · context.md (append) · the `_scratch` feedback file. No code edits, no commits, no pushes — those stay user-gated.
- **disable-model-invocation: true** — fires only on explicit `/wrap` or the wrap phrases; never auto-fires mid-task.
