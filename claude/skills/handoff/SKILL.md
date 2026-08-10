---
name: handoff
description: Render the plan-level `handoff.md` for an active plan by substituting `claude/skills/handoff/templates/handoff.md.template` against the live `plan.yml` + most-recent `updated` `task.yml` (status ∈ pending/in-progress/blocked). Per C-02 Rule 2 + A-01 §1 verdict, /handoff is the renderer for the placeholder handoff.md shipped at plan creation at `<team>/projects/<slug>/plans/<plan-id>/handoff.md`. Three triggers — session-end hook / `/context >60%` hook (both Phase III) / explicit `/handoff [<plan-id>]` (this skill). Writes via `agent-scribe` (atomic, replaces the placeholder). Returns the paste-ready resume-prompt block extracted from the rendered handoff.md so the user can paste it into a fresh Claude Code session. Manual only — never auto-fires from THIS skill; the hook-driven triggers in Phase III invoke the same renderer.
when_to_use: User types `/handoff` or `/handoff <plan-id>`, says "end the session" / "save state for tomorrow" / "I want a fresh-start prompt" / "package this for tomorrow" / "I'm out of context, summarize for restart". Manual only from this skill — Phase III hooks (`handoff-vigilante.sh`) invoke the same render path on session-end + `/context >60%`.
disable-model-invocation: true
---

# Handoff (vigilante renderer · 18 placeholders · paste-ready resume)

The renderer for the plan-level handoff.md placeholder shipped at plan creation. Reads structured YAML (plan.yml + task.yml), substitutes 18 placeholders from `claude/skills/handoff/templates/handoff.md.template`, writes back via `agent-scribe`, extracts the resume-prompt block for chat output. Three triggers converge here; the rendering is identical.

## When this fires

User types `/handoff` (mode A — auto-detect active plan) or `/handoff <plan-id>` (mode B — explicit plan). Phase III hooks fire the SAME render path: `handoff-vigilante.sh` on session-end and on `/context >60%`. Skip only when the user names a one-shot edit or explicitly says "no handoff".

## Inputs

- `<plan-id>` (optional). If supplied, locate at `<team>/projects/<slug>/plans/<plan-id>/`.
- If absent, scan `<team>/projects/*/plans/*/plan.yml` with `status: open` — exactly one → use it; multiple → list + ask; zero → refuse with `no open plan — create one first (plans/<plan-id>/plan.yml + first task per C-02), then re-run /handoff`.

## Workflow

### Phase A — Locate (read-only)

- Walk up from cwd to find the nearest `<team>/` shape (matches `<team>/projects/<slug>/`).
- Resolve `<plan-id>` per above.
- Resolve `<slug>` from the parent path segment `<team>/projects/<slug>/plans/<plan-id>/`.
- Resolve the ACTIVE task: scan `tasks/*/task.yml`, pick the one with the most recent `updated` AND `status ∈ {pending, in-progress, blocked}`. If multiple match the same `updated`, pick lexicographically smallest `task-id` (stable). If NONE match, the plan is effectively idle — render with `{{active_task.*}}` placeholders as `_(no active task)_`.

Echo one block (no questions yet):

```
HANDOFF:
  plan:     <team>/projects/<slug>/plans/<plan-id>/plan.yml
  active:   tasks/<task-id>/task.yml (status=<x>, updated=<y>)
  template: claude/skills/handoff/templates/handoff.md.template (18 placeholders)
  output:   <team>/projects/<slug>/plans/<plan-id>/handoff.md (REPLACE placeholder)
ASK: lock / edit:active-task=<other-task-id>?
```

User can override the active task once. Otherwise `lock`.

### Phase B — Render (mechanical substitution)

Load the template at `claude/skills/handoff/templates/handoff.md.template`. Substitute 18 placeholders per the C-02 §4 rendering rules (loop open/close tags `{{#...}}` / `{{/...}}` are loop control, NOT counted as placeholders — only the data tokens count):

| # | placeholder | source |
|---|---|---|
| 1 | `{{plan.id}}` | `plan.yml id` |
| 2 | `{{plan.status}}` | `plan.yml status` |
| 3 | `{{plan.status_notes}}` | `plan.yml status_notes` (or `_(none)_` if absent) |
| 4 | `{{plan.links.ticket}}` | `plan.yml links.ticket` (or empty string) |
| 5 | `{{plan.links.branch}}` | `plan.yml links.branch` (or empty string) |
| 6 | `{{active_task.id}}` | active task's `id` (or `_(no active task)_`) |
| 7 | `{{active_task.status}}` | active task's `status` (or `_(idle)_`) |
| 8 | `{{today}}` | UTC date `YYYY-MM-DD` |
| 9 | `{{project.slug}}` | parent path segment `<slug>` |
| 10 | `{{plan_yml_path}}` | absolute path to plan.yml |
| 11 | `{{active_task_yml_path}}` | absolute path to active task.yml (or empty if idle) |
| 12 | `{{next_step_from_active_task}}` | first stage in `[implement, verify, test, maintain]` of the active task with `status != done`; value = that stage's `done_when` (single line — if multi-line, take first line up to 200 chars, append `…` if truncated). If all 4 stages done OR no active task → `_(no next step — consider /consolidate)_` |
| 13 | `{{#plan.references}}` loop control | iterate `plan.yml references[]`; per-item line shape: `` - `{{path}}` — {{role}} `` (NO leading `$`); empty list → replace the entire `{{#plan.references}}…{{/plan.references}}` block with `_(none)_` |
| 14 | `{{path}}` | loop-var inside `{{#plan.references}}` — `references[i].path` |
| 15 | `{{role}}` | loop-var inside `{{#plan.references}}` — `references[i].role` |
| 16 | `{{#plan.open_questions.unresolved}}` loop control | iterate `plan.yml open_questions[]` filtered to `resolved != true`; per-item line: `- {{question}}  ({{raised_by}})`; empty → replace block with `_(none)_` |
| 17 | `{{question}}` | loop-var inside open_questions filter — `open_questions[i].question` |
| 18 | `{{raised_by}}` | loop-var inside open_questions filter — `open_questions[i].raised_by` |

Substitution is mustache-style but the rendering owner (this skill) handles loops and empty-section fallback. No external template engine — Claude renders by string replacement.

**Section emptiness rule** (C-02 §4): if a `{{#section}}…{{/section}}` block has zero items after filter, replace the entire block (open tag to close tag inclusive) with `_(none)_`.

### Phase C — Write via agent-scribe (atomic dispatch)

Write the rendered content to `<team>/projects/<slug>/plans/<plan-id>/handoff.md` via one atomic dispatch (1 file + 1 op per agent-scribe contract):

```
Agent(subagent_type="agent-scribe",
      prompt="Write to <team>/projects/<slug>/plans/<plan-id>/handoff.md. Content: <rendered handoff content>. Overwrite the existing placeholder.")
```

The placeholder file is the one shipped at plan creation; this Write replaces it. Scribe will accept the overwrite since plan-level handoff.md is in the C-02 ALLOW list (not KB-protected — vigilante-managed).

### Phase D — Extract + return paste-ready resume

From the rendered handoff.md, extract the block under `## Resume prompt (paste-ready)` (everything between that heading and the next `## ` or EOF). Echo it to chat verbatim, fenced as a code block. Add ≤3 prose lines around it:

1. **Handoff written:** `<absolute path to handoff.md>`.
2. **Active task next step:** `<next_step_from_active_task one-liner>`.
3. **To resume in a fresh session:** paste the block below.

```
<resume block, verbatim from handoff.md ## Resume prompt section>
```

That's the entire return. No further summary, no full handoff repaste.

## Hook integration (Phase III — context for future me)

Two Phase III hooks invoke THIS skill's render path with no user interaction:

- `handoff-vigilante.sh` on `Stop` event: fires when the session ends OR when `/context` reports >60% used. Runs the same Phase A locate + Phase B render + Phase C write, but skips Phase D's chat output (silent vigilante write). On `/context >85%`, the hook also forces a `/clear` after the write.
- **Vigilante signal contract** (Phase III hook ↔ this skill — Claude cannot read shell env vars from a prompt-language skill, so the hook MUST signal via either a sentinel prepended to the prompt context OR a flag file Claude can Read): chosen contract is the prompt-sentinel — the hook prepends `[wosy-vigilante:mode=silent plan=<plan-id> trigger=<stop|session-end|prompt|ctx60|ctx85>]` as the first line of the additionalContext block. This skill scans the kickstart for that sentinel on entry; on match, skip Phase A's `ASK: lock / edit:active-task=…?` prompt (auto-pick the most-recently-updated qualifying task), skip Phase D's chat output, write the file and exit silently. The hook surfaces its own banner to chat AFTER this skill returns.

Until Phase III ships, neither the hook nor the sentinel exist — manual `/handoff` is the only entry, and Phase A's `ASK:` prompt always fires.

## Hard rules

- **Renderer only.** Substitution + write. Never invent fields not in plan.yml/task.yml.
- **One file + one op via agent-scribe.** The handoff.md write is atomic; never write plan.yml or task.yml from this skill.
- **18 placeholders, no more.** If the template gains placeholders later, this skill must be updated; don't silently leave unknown `{{x}}` in the output.
- **Empty-section fallback is `_(none)_`** for the 2 loop blocks. Don't render empty `## Open questions` headers without content.
- **`{{next_step_from_active_task}}` truncates at 200 chars** with trailing `…` if multi-line `done_when`. The full `done_when` lives in task.yml; the handoff is the gateway, not the source of truth.
- **No source code reads.** Read only plan.yml + task.yml + the template. No grep, no glob, no following references.
- **Vigilante mode is silent.** When the kickstart starts with the `[wosy-vigilante:mode=silent …]` sentinel (prepended by `handoff-vigilante.sh` in Phase III), write the file and exit — no chat output, no user prompts. The hook surfaces its own banner to chat afterward.
- **Resume-block extraction is verbatim.** Don't paraphrase the `## Resume prompt` section; copy it character-for-character to chat.
- **disable-model-invocation: true** — never auto-fire from this skill. Hooks invoke the renderer directly (Phase III).
