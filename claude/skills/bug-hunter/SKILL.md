---
name: bug-hunter
description: Adaptive bug investigation — gates the minimal-reproduction set (impact / expected / actual / steps / environment), dispatches `agent-tracer` / `agent-scout` / `agent-query` / `agent-server` per HQ-mode, and writes a cited diagnosis to a TASK inside an existing plan per C-02 (NOT a freestanding `.devwork/tasks/<id>/bug-*.md` trio — the legacy shape is files-canonical-but-orphaned; new bugs land as tasks under plans). Mirrors `/interview` shape: kickstart-aware, ask only missing fields. Loosened trigger per A-01 §1 verdict — fires on ANY stacktrace / error / regression mention / "this used to work" verb (current 5-invocation baseline is too tight). Diagnosis-only — never edits source code; the fix is a separate task in the same plan with its own gate. If no plan exists yet, runs an inline 3-question gate (objective + first task name + first task done_when) and creates the parent plan artifact directly.
when_to_use: User types `/bug-hunter [<plan-id> | <ticket>]`, pastes ANY stacktrace / error / log line / Jira-Linear-GitHub bug ticket / regression mention, OR says "I have a bug" / "investigate this regression" / "this used to work but now…" / "client / tenant / user reports X is broken". Default behavior when a bug is in the kickstart and no investigation task exists yet.
disable-model-invocation: true
---

# Bug-hunter (adaptive · diagnosis-only · output = task inside plan)

Sibling of `/interview`. Where `/interview` gates fuzzy ideas into interview records, `/bug-hunter` gates bug reports into cited diagnoses materialized as a TASK in an existing C-02 plan. Loosened trigger; task-canonical output; pause at diagnosis.

## When this fires

Loosened from the legacy 5-invocation baseline (audit A-01 verdict). Fires on:

- Explicit `/bug-hunter [<plan-id> | <ticket-id>]`
- Any pasted stacktrace / exception / error string / log line with a clear failure signal
- Any Jira / Linear / GitHub / Bitbucket bug ticket id in the kickstart
- Phrases: "I have a bug", "investigate this regression", "this used to work but now…", "X is broken", "client / tenant / user reports …", "P0 incident", "production down on …"

**Skip ONLY when**: user names a specific line to change ("rename foo to bar at file.go:42") — cheap-path territory, not investigation. Or explicit "skip investigation, just fix X".

## Output shape (NEW per A-01 + C-02)

A TASK inside an existing plan, NOT a freestanding `tasks/<id>/bug-*.md` trio. Specifically:

```
<team>/projects/<slug>/plans/<plan-id>/tasks/<bug-task-id>/
  task.yml          ← Modelo A per C-02 — implement/verify/test/maintain stages
  bug-context.md    ← minimal-reproduction set (task-folder custom file per 1.3.2 B1)
  bug-evidence.md   ← cited investigation trail
  bug-plan.md       ← diagnosis + suggested fix area (the deliverable)
```

`task.yml` is the canonical Modelo A artifact (per C-02). The three `bug-*.md` files are task-folder custom files (files-canonical 1.3.2 B1 explicitly allows this). Findings flow: dispatched evidence → `bug-evidence.md` → distilled `bug-plan.md` → mirrored into `task.yml.implement.notes` (cited) + `task.yml.implement.approach` (the fix shape).

## Workflow

### Phase A — Locate parent plan (mode triage)

Resolve which plan this bug belongs to:

- **Mode 1 — plan-id supplied**: `/bug-hunter <plan-id>` with `<plan-id>` matching `<team>/projects/<slug>/plans/<plan-id>/plan.yml` → use it.
- **Mode 2 — project-slug detected**: kickstart paths match `<team>/projects/<slug>/` → list plans under that project with `status: open`; ask user: "this bug belongs in plan `<existing-plan>` OR open a new plan (inline 3-question gate)?".
- **Mode 3 — no plan, no project**: refuse with one line — `no plan or project context — run /interview to frame scope, then create the parent plan artifact (plans/<plan-id>/plan.yml + first task) and re-run /bug-hunter`. (Personal-experiment bugs without a project are rare; force the user to think about scope.)

If user picks "open new plan", run the 3-question gate inline — objective + first task name + first task done_when; if the user shrugs on any, refuse (the plan has no falsifiable shape without them) — then create the plan artifact directly: write `plans/<plan-id>/plan.yml` via `agent-scribe` (atomic, 1 file + 1 op) and continue Phase B against it. The bug becomes the first task under the new plan.

### Phase B — Kickstart inspection (silent, pre-question)

For each `bug-context.md` field, scan the kickstart:

- **Impact** — "blocking X tenants / users", "production down", "client X reports", severity markers ("P0", "critical", "minor").
- **Expected** — "should X", "the doc says X", "before commit Y it did X", reference to a test/spec.
- **Actual** — exact error string, stacktrace, screenshot ref, "returns null / 500 / wrong number".
- **Steps** — numbered list, "to reproduce: …", "when I X then Y".
- **Environment** — branch name, commit sha, version pin, browser/OS/stack, "started after deploy on <date>".
- **Constraints** — "read-only on prod", "tenant X isolated", "don't touch table Y".
- **Open questions** — anything user flagged as "I'm not sure if".

Use prose pattern-matching, not strict regex. When in doubt, leave empty and ask.

If the kickstart names a file or stacktrace path, read it as part of Phase B (read-only, bounded — same exception `/interview` uses). The asset role classification convention applies (see `/interview` SKILL.md — `code` / `reference` / `precedent` vocab post-Wave-B-1):

| role | meaning in bug-hunter context |
|---|---|
| `code` (legacy `#work-subject`) | Code / artifact suspected as fault site. Full Read; cited as primary evidence in `bug-evidence.md`. |
| `reference` | Background — prior bug, related doc, the spec actual diverged from. Read once for context; not cited as fault evidence unless directly implicated. |
| `precedent` (legacy `#related`) | Similar prior bug, sibling regression, comparable failure mode. Skim for patterns. Cite only if a pattern matches the current symptom. |

Wrong role propagates through every dispatch — confirm classification before investigation.

### Phase C — Resume detection

If kickstart matches ANY:
- contains path to existing `<team>/projects/<slug>/plans/<plan-id>/tasks/<bug-task-id>/bug-*.md`
- starts with "resume bug" / "continuing the bug investigation" / "retomar"
- references a prior `bug-evidence.md` by path

Then SKIP the question bank — read the prior `bug-context.md` + `bug-evidence.md` + hypothesis-so-far, output a brief "what we know" frame, and ASK: "next dispatch target? (`agent-tracer <file:line>` / `agent-scout <pattern>` / `agent-query <table>` / `agent-server <alias>`)".

### Phase D — Echo capture (one short block) + lock

```
Captured from kickstart (plan=<plan-id>):
  Impact: <X> · Actual: <Y, truncated to 1 line> · Environment: <branch@sha, stack vN>
  Assets: <N> — <a code, b reference, c precedent>
  Missing: <list of unfilled minimal-reproduction fields>
ASK: lock asset classification + missing-field list?
```

Show classification + missing list BEFORE asking questions. User corrects misclassification in one keystroke ("the stacktrace is reference not code — the fault site is the migration").

### Phase E — Ask ONLY missing-field questions, one at a time

Question bank (used only for missing fields, cap 5 per cycle):

| field | question |
|---|---|
| **Impact** | "Who's affected and how badly? One tenant or all? Blocking a workflow or cosmetic? Frequency — every request or intermittent?" |
| **Expected** | "What should happen? Point to the contract — a doc, a test, prior behavior, or 'user's mental model, no written spec'." |
| **Actual** | "Exactly what happens instead? Error string verbatim, output, screenshot, or 'silent wrong result'." |
| **Steps** | "Smallest reproduction. Environment + role + data state + sequence. If you can't reproduce locally, name the smallest known-failing input set." |
| **Environment** | "Branch / commit, stack version, last known-good. If 'unknown', say so — that's a `git bisect` candidate." |
| **Constraints** | "Any non-negotiables for reproduction or fix? Read-only on prod? Tenant isolation? Tables not to touch?" |
| **PriorWork** | "Known regression? Has anyone investigated before? Linked ticket, PR comment, slack thread?" — catches duplicates + dead-end prior attempts. |

Cap 5 per cycle. If genuinely >5 fields empty, the bug is not yet investigatable — surface that and ask user to gather more before continuing.

### Phase F — Resolve `<bug-task-id>` + create the task

- `<bug-task-id>`: from the bug ticket id (e.g. `proj-1409`) OR kebab-case of `actual` headline capped at 40 chars (e.g. `null-on-tenant-switch`). Pattern: `^[a-z0-9][a-z0-9-]{1,63}$`.
- Collision pre-check (per agent-scribe contract):
  ```bash
  PLAN_DIR="<team>/projects/<slug>/plans/<plan-id>"
  test -d "${PLAN_DIR}/tasks/<bug-task-id>" && echo "exists" || echo "ok"
  ```
  On collision: ask user — append to existing investigation OR start fresh with a different id?

Create the task folder + task.yml + bug-context.md via 2 parallel agent-scribe dispatches:

```
Agent(subagent_type="agent-scribe", prompt="Write to <PLAN_DIR>/tasks/<bug-task-id>/task.yml. Content: <Modelo-A yaml>")
Agent(subagent_type="agent-scribe", prompt="Write to <PLAN_DIR>/tasks/<bug-task-id>/bug-context.md. Content: <minimal-reproduction yaml-frontmatter + body>")
```

`task.yml` shape:

```yaml
id: <bug-task-id>
plan_id: <plan-id>
created: <YYYY-MM-DD>
updated: <YYYY-MM-DD>
status: in-progress
# task.yml.references[].role uses the C-02 schema enum (target/adjacent/background) — NOT
# the Wave B kickstart vocab (code/reference/precedent). Map at write time:
#   kickstart code      → schema target
#   kickstart reference → schema adjacent
#   kickstart precedent → schema background
references: <list of assets with role=target|adjacent|background per the task.yml schema enum>
implement:
  status: in-progress
  done_when: "diagnosis written at bug-plan.md (confidence HIGH or MEDIUM with ruled-out alternatives)"
  approach: "investigate per bug-context.md; dispatch agent-tracer/agent-scout/agent-query/agent-server per HQ-mode dispatch table"
  notes: "minimal reproduction captured at bug-context.md"
verify:
  status: pending
  done_when: "hypothesis cited with >=1 ruled-out alternative; bug-plan.md reviewed"
  method: inspection
test:
  status: pending
  done_when: "reproduction step in bug-context.md confirmed deterministic by agent-reviewer or user"
  framework: <detected-or-none>
  last_result: unrun
maintain:
  status: pending
  done_when: "if root cause confirmed, encyclopedia_delta captures the failure mode + detection hint; runbook_delta captures the diagnostic procedure"
  gotchas: []
history:
  - ts: <ISO-8601-Z>
    actor: claude
    event: created
    notes: "spun from bug-hunter kickstart"
  - ts: <ISO-8601-Z>
    actor: claude
    event: stage-started
    stage: implement
```

`bug-context.md` minimal-reproduction set (YAML frontmatter + optional body):

```yaml
---
id: <bug-task-id>
plan_id: <plan-id>
created: <YYYY-MM-DD>
status: investigating
source: bug-hunter
impact: <verbatim from user where possible>
expected: <contract reference>
actual: <error/output exactly>
steps: <minimal reproduction>
environment:
  branch: <git branch>
  commit: <sha-or-unknown>
  stack: <language/framework pin>
  last_known_good: <commit/date/version or unknown>
assets:
  - path: <abs-or-relative-path-or-url>
    role: <code|reference|precedent>
    note: <one line>
constraints: [<security/tenant/read-only rules>]
open_questions: [<surfaced, unresolved — empty list ok>]
---
```

### Phase G — Dispatch investigation (HQ mode)

Dispatch order per captured shape (matches Wave A `/wire-up` agent dispatch idiom):

| Captured shape | First dispatch | Second dispatch (conditional) |
|---|---|---|
| Stacktrace + file paths | `agent-tracer` from topmost user-code frame | `agent-scout` for recent commits touching the trace |
| Symptom, no clear entry point | `agent-scout` for files matching error string | `agent-tracer` once entry point identified |
| Data-state suspicion (wrong number, missing row) | `agent-query` against schema-validated tables | `agent-tracer` for write-path to that table |
| Remote-only repro (only fails on prod) | `agent-server` read-only inspect of logs/config | `agent-scout` locally for the code path |
| Regression with last-known-good commit | `agent-scout` `git log` between known-good and HEAD | `agent-tracer` for the suspect commit's diff |

Append findings to `bug-evidence.md` after each dispatch (one section per dispatch, dated, with cited rows). Main session writes a one-line recap; the full output stays in the artifact.

After each dispatch, refine the hypothesis paragraph at the bottom of `bug-evidence.md`. Hypothesis sharpens as evidence accumulates.

`bug-evidence.md` format:

```markdown
# Evidence trail — <bug-task-id>

## <YYYY-MM-DD HH:MM> — agent-tracer <entry-point>
- `path/file.ext:LINE` — does X (cited snippet if essential)
- `path/file.ext:LINE` — calls Y, returns Z
- `git log -S <symbol>` — introduced in commit `<sha>` on `<date>` by `<author>`
- `<query result>` — N rows, schema-validated against .devwork/schema/<table>.md

## Hypothesis after first sweep
<one paragraph, refined as evidence accumulates>
```

No claim without a cited artifact. No "looks like" without `file:line`. No "probably" without commit / row / log.

### Phase H — Write `bug-plan.md` (the diagnosis)

When hypothesis is HIGH confidence OR 3+ dispatches have not raised confidence above MEDIUM (a "no clear root cause" outcome is a valid plan — tells the user where to look next).

`bug-plan.md` shape:

```markdown
# Bug plan — <bug-task-id>

## Symptom
<one line, derived from bug-context.md actual>

## Reproduction (confirmed)
<minimal steps that produce actual deterministically, environment locked>

## Evidence trail
<reference: bug-evidence.md — list top 3-5 cited rows here for quick read>

## Root cause hypothesis
**Confidence:** HIGH | MEDIUM | LOW
<one paragraph: what is broken, why, causal chain from input to error>

## Why not <alternative-hypothesis-1>
<one line ruling out with cited evidence>

## Why not <alternative-hypothesis-2>
<one line ruling out with cited evidence>

## Suggested fix area
- File(s) to change: `<path>`, `<path>` — NOT code, just paths
- Approach: <one paragraph — what shape the fix takes, no implementation>
- Risk surface: <what else touches this code path; regression risk>
- Estimated effort: <S | M | L — derivation required (e.g. "M because the fix touches 3 files in the auth middleware path, each ~50 lines, plus 2 tests") OR explicit `estimate` label>

## done_when
<falsifiable test the fix must pass — provable by a command or repro step>

## Rollback
<how to revert if the fix regresses — one file / feature flag / migration revert>

## Open questions
<bullets, or "none">
```

Mandatory: at least one `Why not <alternative>` ruled out with cited evidence. Single-hypothesis plans are weaker than plans that show the elimination.

### Phase I — Mirror to task.yml + return + pause

After `bug-plan.md` is written:
- Update `task.yml.implement.notes` with: `diagnosis at bug-plan.md (confidence=<x>; root cause: <one-line>)` via agent-scribe.
- **HIGH confidence**: update `task.yml.implement.status: done` (diagnosis is the deliverable; investigation closed). Update `task.yml.verify.status: pending` and `verify.evidence: "see bug-evidence.md"`. Append `task.yml.history` event: `{ts, actor:claude, event:stage-done, stage:implement, notes:"diagnosis confidence=HIGH"}`.
- **MEDIUM confidence**: EXPLICITLY keep `task.yml.implement.status: in-progress` (investigation incomplete — alternative hypotheses not fully ruled out; needs more dispatches). Do NOT advance `verify.status`. Append `task.yml.history` event: `{ts, actor:claude, event:stage-started, stage:implement, notes:"diagnosis confidence=MEDIUM — investigation continues"}` (informational, not stage-done). The handoff-vigilante (Phase III) reads `implement.status` to pick the active task — MEDIUM-confidence bugs stay in the queue as in-progress.
- **LOW confidence**: same as MEDIUM (status stays `in-progress`). Append history with `notes:"diagnosis confidence=LOW — pivot to alternative hypothesis or escalate"`.

Return ≤4-line summary:

1. **Plan at:** `<PLAN_DIR>/tasks/<bug-task-id>/bug-plan.md` — `<root cause one-liner>` (confidence: HIGH | MEDIUM | LOW).
2. **Evidence trail:** `bug-evidence.md` — N cited rows across M dispatches.
3. **Task at:** `<PLAN_DIR>/tasks/<bug-task-id>/task.yml` (status=in-progress if MEDIUM, done if HIGH).
4. **Next step:** approve plan and start fix as a NEW task in the same plan / rework hypothesis / abandon.

Then PAUSE per the multi-step rule. The FIX is a separate task with its own gate — directly create a sibling `tasks/<fix-task-id>/task.yml` under the same plan-id (its own done_when, via `agent-scribe`). The user explicitly confirms before any source file is edited.

## Hard rules

- **No source edits.** This skill NEVER touches code in the repo under investigation. Edits are limited to `<PLAN_DIR>/tasks/<bug-task-id>/{task.yml,bug-context.md,bug-evidence.md,bug-plan.md}` and `.devwork/_scratch/`.
- **No destructive commands.** No `rm`, `drop`, force-push, branch delete. Investigation is read-only by default. `agent-server` runs `read-only` per its frontmatter.
- **No real credentials in artifacts.** Placeholders (`<tenant-id>`, `<customer-uuid>`, `<connstr>`) in `bug-context.md` / `bug-plan.md`.
- **Forensic citation throughout.** Every `bug-evidence.md` row cites `file:line` / row / log line / commit sha. No "looks like" without an artifact.
- **Calibrate estimates.** Effort and risk magnitudes get derivations or explicit `estimate` labels.
- **One step = one user "go".** After `bug-plan.md` is written, pause. The fix is a NEW task; don't begin it without explicit approval.
- **Don't propose a fix before diagnosis is written.** Plan goes in `bug-plan.md` first; if user says "just fix it now", confirm they want to skip the diagnosis gate before editing source.
- **Don't ask all questions at once.** Even with an empty kickstart, ask one at a time.
- **Don't invent reproduction steps.** Unable to reproduce = valid plan output ("needs sample data"), not a guessed reproduction.
- **Don't claim a root cause without ruling out at least one alternative.** `Why not X` with cited evidence is the rigor floor.
- **Don't read >50 lines into main context.** HQ-mode applies — dispatch `agent-tracer` / `agent-scout` for actual file reads.
- **Don't paste the plan back into chat.** The 4-bullet summary is the reply; `bug-plan.md` is the artifact.
- **Don't write to legacy `.devwork/tasks/<id>/bug-*.md`.** New bugs land as tasks under plans per C-02; legacy paths are dead for new work (existing folders stay per 1.3.2 B1 files-canonical).
- **disable-model-invocation: true** — manual user trigger only (matches /handoff + /consolidate + /interview siblings). Auto-dispatched only on EXPLICIT kickstart bug-shape detection at session entry; never re-entry mid-session without user lock.

## Companions

`rationale.md` — provenance, design choices, worked examples from pre-Wave-D legacy shape. NOT auto-loaded; preserved for git-history continuity. Open on demand.
