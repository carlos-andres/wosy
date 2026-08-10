---
name: interview
description: Adaptive discovery interview for any task — large or small. Reads the kickstart, applies a mechanical triage gate (trivial / standard / heavy), detects resume-shape kickstarts and skips the question flow, auto-warms the project bundle when the kickstart references a known project slug (C-03 §5.2 trigger #3), asks ONLY missing fields with per-asset role classification (code / reference / precedent), and exits via one of three locked branches (intake-now / spec-only-park / just-do-it). Writes a unified interview record to `.devwork/interview/<id>.md` validating against `template.schema.json`. Universal kickoff — fire before any task artifact is created for anything not already framed.
when_to_use: User says "interview mode", "let's define context", "discovery", "I have an idea, help me frame it", or "before we start need to think this through". Default behavior even for small asks when a kickstart is supplied. Fires before the task artifact is created whenever the topic isn't already framed.
disable-model-invocation: true
---

# Interview (adaptive · triage-gated · resume-aware · universal kickoff)

Discovery before execution. One record per interview at `.devwork/interview/<id>.md`. The frontmatter is the contract (validates against `template.schema.json`); the markdown body is optional. Three modes, picked mechanically:

| tier | mechanical rule | flow | typical exit |
|---|---|---|---|
| **trivial** | `chars < 300` AND `paths == 0` AND `paste_blocks == 0` | echo capture · skip question bank · pick exit | `just-do-it` |
| **standard** | `300 ≤ chars < 2000` OR `1 ≤ paths ≤ 3` OR `has_paste_block` | M2 per-asset role · ask only missing fields · pick exit | `spec-only-park` or `intake-now` |
| **heavy** | `chars ≥ 2000` OR `paths > 3` OR has phase keywords (`phase` / `stage` / `step 1` / `step 2` / `fase` / `etapa`) | full M2 + question bank + open_questions · pick exit | `intake-now` |

User override: `tier: <x>` in the kickstart wins over the rule. Otherwise Claude does NOT subjectively decide tier — the rule is a numeric pre-filter.

## When this fires

Trigger phrases — explicit `/interview`, "interview mode", "let's define context", "discovery", "I have an idea help me frame it", "before we start I need to think this through", or any fuzzy multi-paragraph topic with no `.devwork/interview/<id>.md` yet. **Skip ONLY when** the user has named a one-shot edit on a specific file/line, a commit hash to revert, or said "skip interview" / "just do it" / "this is a one-liner".

## Workflow

### Phase A — M4 resume probe (silent, pre-triage)

Detect resume-shape kickstart BEFORE triage. Match ANY of:
- contains `PROJECT:` AND `SESSION:` AND `DATE:` headers (handoff dump shape)
- starts with `resume` / `continuing` / `retomo` / `continúo` (case-insensitive)
- contains a path to a prior `.devwork/interview/<id>.md` OR a prior `plans/<plan-id>/handoff.md`

On match: skip Phases B/D entirely. Read the referenced prior file (read-only, bounded). Output one block:

```
RESUME:
  prior:   <prior interview-id or plan-id · file path>
  status:  <one sentence from prior status / status_notes>
  next:    <next concrete step from prior — verbatim if stated>
ASK: ready to continue with `<next>`? (yes / edit)
```

On `yes`: append `history` event to the prior record (`event: resumed`) and stop. Main session takes over. No new record written.

### Phase B — M1 triage gate (mechanical)

Count `chars`, `paths`, `paste_blocks`, and scan for `phase_keywords`. Apply the table above → emit `tier`. Honor explicit `tier: <x>` override in the kickstart.

### Phase C — Auto warm (C-03 §5.2 trigger #3)

Scan the kickstart for paths matching `<team>/projects/<slug>/`. For each detected slug, invoke `/brain-load <slug>` (which calls `brain load <slug>`) BEFORE proceeding. The returned bundle injects as additional context — Claude consumes the 10 sections directly. If `brain load` returns "record not found", do NOT guess slugs; surface the gap to the user and continue without warm cache.

### Phase D — Per-tier flow

#### D.trivial — minimal echo

Capture `subject` (one line, verbatim from kickstart) + `exit=just-do-it`. No question bank. Skip M2. Skip open_questions. Write record. Return 3-bullet summary.

#### D.standard — M2 + minimal questions

For each path / URL in the kickstart WITHOUT explicit role tag (no `#code` / `#reference` / `#precedent` / no surrounding-prose verb), ask ONCE per asset using M2 shape:

```
I see <N> assets without an explicit role:
  - <path-1>  (role?)
  - <path-2>  (role?)
For each, pick: code (I may modify) · reference (consult-only) · precedent (prior decision/work).
Or answer `default-all=reference` (safest default).
```

Legacy hashtag aliases accepted: `#work-subject` → `code` · `#references` → `reference` · `#related` → `precedent` · `#source` → `code`.

Then ask ONLY missing-field questions from the bank below (cap at 3 total). Pick exit per Phase E.

#### D.heavy — full M2 + question bank + open_questions

M2 as above. Then ask only missing-field questions from the bank (cap at 5). Capture `open_questions[]` for anything raised but unresolved. Pick exit per Phase E.

##### Question bank (used only for missing fields)

| field | question shape |
|---|---|
| `subject` | "One sentence — what is this about?" |
| `objective` | "Describe done in one sentence — what is true after this ships that isn't true today?" (reject verbs-without-outcomes) |
| `requirement` | "What concrete artifacts must be produced? Files, schemas, commits, PRs." |
| `done_when` | "How will we know it shipped — a condition provable by a command?" |
| `constraints` | "Any non-negotiables? Stack pins, tables not to touch, security, schedule." |
| `rollback` | "If this ships and breaks something, how do we undo? Revert one file, feature flag, migration rollback?" |
| `deferred` | "What's explicitly OUT for this pass — things we'll do later or never?" (ask on every heavy-tier interview) |

Cap: 3 questions (standard) / 5 questions (heavy) per turn-cycle. If genuinely >5 fields empty on heavy, the task is too large for one interview — suggest splitting first.

### Phase E — M3 exit branch (mandatory · single question)

At the END of every interview (except resume), prompt ONE of three exits:

| exit | meaning | next |
|---|---|---|
| `intake-now` | open a task under an existing plan (or a fresh plan) — the plan/task artifact is created directly against this record | suggest creating `plans/<plan-id>/plan.yml` + first `task.yml` from this record |
| `spec-only-park` | save the record, no task created (default for standard with low signal) | record at `.devwork/interview/<id>.md` only |
| `just-do-it` | execute inline, no task, no spec (default for trivial) | proceed directly to main-session work |

Default per tier: trivial → `just-do-it` · standard → `spec-only-park` · heavy → `intake-now`. User can override.

### Phase F — Write the record

Resolve `<id>`:
- If the kickstart names a ticket (e.g. `PROJ-1409`, `TICKET-1234`), use it (preserve case).
- Else kebab-case the `subject` noun phrase, capped at 40 chars.

Locate `.devwork/`:
- Walk up from cwd. If found, write `<.devwork>/interview/<id>.md`.
- If not found, ask once: "no `.devwork/` found — create at `<cwd>` or write inline?" (per global CLAUDE.md "Artifact location" rule).

Write the record (frontmatter shape in §Output below). Validate against `template.schema.json` if present. On validation failure under NUDGE mode, log to stderr and continue; under HOOK mode (post-promotion), block and surface the error.

Under enforced flags (`.devwork/wosy.flags` `enforce_kb_scribe=1`): also `brain import` the record as a `task` record (per C-03 §5.1 — `task` absorbs the legacy `spec` use case; `decision` and `spec` record types are KILLED, do not attempt to import as `spec`). Optional under nudge mode.

### Phase G — Return ≤4-line summary

1. **Record at:** `.devwork/interview/<id>.md` — `<subject>` (tier=`<tier>`, exit=`<exit>`).
2. **Next step:** per exit branch (intake-now → suggest creating the plan/task artifact from this record; spec-only-park → record stays parked; just-do-it → proceed to work).
3. **Open questions:** `<N>` (or "none").
4. **Warm cache:** `<project>` bundle loaded (or "none — no project slug in kickstart").

Do not paste the record back. The file is the artifact.

## Output (frontmatter contract)

```yaml
---
# === IDENTITY ===
id: <kebab-case>
created: <YYYY-MM-DD>
updated: <YYYY-MM-DD>
status: draft           # draft | locked | superseded
tier: <trivial|standard|heavy>
schema_version: 1
project: <slug-or-empty>
session_origin: <session-id-or-this-conversation>
doc_kind: interview
supersedes: []

# === THE ASK ===
subject: |
  <one sentence — verbatim or paraphrased from kickstart>
objective: |               # required: standard + heavy
  <one paragraph — what is true after this ships>
requirement: |             # required: heavy
  <concrete artifacts to produce>
done_when: |               # required: standard + heavy
  <falsifiable condition provable by a command>

# === THE MATERIAL ===
code:                      # required: heavy (≥1 item)
  - path: <abs-or-relative-path>
    role: <target|adjacent>
    note: <one line — why it's here>
references:                # optional
  - source: <path-or-URL>
    kind: <doc|code-example|pattern|api-spec|sibling-project|data>
    note: <one line>
precedents:                # optional
  - kind: <decision|interview|pr|incident|conversation>
    ref: <id-or-URL>
    note: <one line>

# === THE GUARDRAILS ===
constraints: []            # required: heavy (may be empty list)
deferred: []
rollback: |
  <how to undo>
open_questions: []

# === THE EXIT ===
exit: <intake-now|spec-only-park|just-do-it>   # required: all tiers
warm:                      # optional — populated by Phase C
  project: <slug>
  cache_version: <ULID>
---

# <Optional markdown body — only when prose adds signal the frontmatter can't carry>
```

## Hard rules

- Tier classification is MECHANICAL (numeric rule). Claude does not subjectively decide tier; user can override with explicit `tier: <x>`.
- M4 resume PREEMPTS triage. Resume kickstarts never run the question bank — they output the next-step frame and stop on `yes`.
- M2 asset-role question fires once per UNTAGGED asset on standard + heavy. Tagged assets (hashtag or surrounding-prose verb) skip the question. `default-all=reference` is the safest user shortcut.
- Auto warm runs BEFORE the question bank. The bundle changes what's "missing" — never ask a question the bundle already answers.
- M3 exit is MANDATORY (except on resume). Default by tier; user can override; never silently picked.
- Cap questions: 0 (trivial) · 3 (standard) · 5 (heavy). Genuinely >5 empty fields = task too large; suggest splitting.
- Don't invent answers. Missing fields are written as `_(not specified)_` or omitted (schema enforces per-tier requireds).
- Don't read files outside the kickstart's named paths during the interview — except the M4 prior file. Discovery happens at work time, not here.
- Don't summarize what the user said. Move to the next missing question.
- Don't paste the record back. Four bullets is the whole reply.
- Don't migrate existing `.devwork/specs/<id>.md` records — they keep their old shape. The new template applies to new interviews only.
- Storage is `.devwork/interview/<id>.md`. Never `.devwork/specs/` (killed per C-02/C-03). Never `.devwork/decisions/` (killed — absorbed by `plan.yml.open_questions[]`).
- Under enforced flags: `brain import` the record as `task` type. Under nudge mode: validation failures log but do not block.
