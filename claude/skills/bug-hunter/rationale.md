# `/bug-hunter` — design rationale

Companion to `SKILL.md`. NOT auto-loaded. Open on demand when "why is the skill shaped this way?" matters more than "how do I invoke it?".

The rationale is wosy-internal. Where an external system informed a decision, that's named — but every choice ultimately reasons from a wosy principle already in `claude/CLAUDE.md`.

## 1. Why a separate skill instead of extending `/interview`

`/interview` gates fuzzy ideas into specs — forward design, output to `.devwork/specs/<id>.md`. `/bug-hunter` gates bug reports into diagnoses — backward triage, output to `.devwork/tasks/<id>/bug-*.md`. The intake shape is identical (kickstart-aware adaptive questioning); the cognitive frame and output shape are not.

Specializing produces:
- A field set tuned to bug triage (Impact / Expected / Actual / Steps / Environment) instead of the universal task spec (Role / Headline / Context / Inputs / Output / Constraints / done_when / Rollback).
- An evidence-trail artifact (`bug-evidence.md`) that doesn't fit in a spec at all.
- A plan output that explicitly defers the fix — a spec doesn't make sense for "diagnose this, then we'll decide".

Mirroring the `/interview` flow (Step 1 silent scan → Step 2 echo + lock → Step 3 ask missing → Step 4 write artifact → Step 5 short summary) inherits the validated adaptive-intake behavior without restating it.

## 2. Why the minimal-reproduction set is these five fields

Impact, Expected, Actual, Steps, Environment. Each gates a different downstream decision:

| field | gates | failure mode if missing |
|---|---|---|
| Impact | Whether to investigate at all, and at what priority. | Investigation burns dispatches on a non-issue, or stalls a real outage behind a cosmetic ticket. |
| Expected | The contract the bug violates. Without it, "wrong" is undefined. | Fix targets the symptom instead of the contract — re-breaks on the next requirement change. |
| Actual | The diff against Expected. The thing to make go away. | "Looks right by inspection" verification (banned by the CLAUDE.md "Verification before declaring done" rule). |
| Steps | Reproducibility. Without it, fix is unverifiable. | The fix lands and no one knows if it worked. `done_when` becomes "trust me". |
| Environment | Regression triage. Enables `git bisect`, last-known-good comparison. | `agent-tracer` dispatches against the wrong commit; evidence cites code that wasn't there when the bug shipped. |

Skipping any of them shifts cost downstream, usually amplified.

## 3. Why three artifacts (`bug-context`, `bug-evidence`, `bug-plan`) instead of one

Different lifecycles, different audiences, different write rates.

- **`bug-context.md`** — written once at intake; rarely edited after. Read by humans, by future-Claude on resume, by `/handoff` snapshot.
- **`bug-evidence.md`** — appended after each subagent dispatch. Grows monotonically. Audit trail; the medical record of the investigation.
- **`bug-plan.md`** — the deliverable. Written when hypothesis stabilizes. Read by the user to decide approve / rework / abandon.

One combined file would force every dispatch to re-edit a fixed-section structure (which the deprecated `consolidated.md` convention died trying to do — 2% adoption). Three files respect the 1.3.2 B1 lock: task folders are files-canonical and accept any custom file the work needs.

## 4. Why no `state.json` and no rework counter

Across three repos sampled (PilotB, pilot-a, Globex), brain-task-record adoption was 0% / 9% / 21%. Plain markdown wins by adoption.

A `state.json` per investigation would re-introduce the brain-task-record shape this project already rejected. A rework counter would be a schema field with a discipline-dependent update rule — the same shape `consolidated.md` had at 2% adoption.

If rework happens (user pushes back on a hypothesis): write `bug-rework-1.md` in the task folder with the user's pushback verbatim, dispatch new investigation, append to `bug-evidence.md`, refine `bug-plan.md`. No schema. No counter. The files in the folder are the history.

## 5. Why HQ-mode dispatch instead of reading files directly

`claude/CLAUDE.md` "HQ orchestrator" mandates: main session doesn't read >50 lines or edit code when exploratory. Bug investigation is the canonical exploratory shape — reading the trace, related code, schema, logs, all of which routinely exceed 50 lines per file.

Dispatching `agent-tracer` / `agent-scout` / `agent-query` / `agent-server` keeps main context lean enough for the multi-turn investigation, and lets each subagent specialize (tracer caches flow maps, scout returns paths-with-notes, query reads schema before any SQL).

The cheap-path override (CLAUDE.md) still applies — if the user names a specific `file:line` to inspect, read it directly.

## 6. Why forensic citation is mandatory throughout

It's already a wosy rule (`claude/CLAUDE.md` "Communication discipline + numeric calibration" → "Forensic citation discipline" bullet). The reason it's non-negotiable in bug-hunter specifically: a diagnosis is a load-bearing claim. The fix that follows is built on it. An uncited "I think it's the cache invalidation" produces fixes that target hallucinated root causes. With `path/file.ext:LINE — does X` plus a quoted snippet, the user can spot-check the chain in seconds.

## 7. Why the fix is always a separate task

Two reasons, both from existing wosy rules:

1. **Multi-step pause rule** — one user "go" = one step. Approving the diagnosis is one decision; approving the fix is another. Conflating them violates the rule and skips a gate the user might want.
2. **Verification before declaring done** — the fix's `done_when` is a different falsifiable test from the diagnosis's `confidence: HIGH`. Splitting them keeps each gate verifiable on its own terms.

External input: a `/bug → bug-hunter → implementer → reviewer → test-writer → verifier` pipeline from an external multi-agent harness. Wosy keeps the pause-between-stages discipline; rejects the `state.json` sole-writer orchestrator machinery (conflicts with 1.3.2 B1 lock). The chain is the existing wosy ladder: `/interview` → direct plan/task artifact creation → `/bug-hunter` → manual fix → `/consolidate`.

## 8. Why "no source edits" is a hard rule

Role separation. Same reason `agent-scout` is read-only, `agent-reviewer` is read-only, `agent-server` defaults to read-only. The investigator role has a different mandate from the implementer role; conflating them means one mistake (a too-eager edit during investigation) breaks both gates.

Safety property: the user can invoke `/bug-hunter` on a production repo or a teammate's branch without worrying that the skill will mutate anything outside `.devwork/tasks/<id>/`. The blast radius is bounded by design.

## 9. Why mirror `/interview` rather than design fresh

1. **Cognitive economy** — users who know `/interview` already know how `/bug-hunter` works. The Step 1–5 shape is shared; only the field set and output differ.
2. **Validated patterns** — `/interview`'s adaptive-intake (silent scan, ask only missing, asset role classification, hashtag conventions) survived audit. Reusing the shape inherits those wins.
3. **Skill family coherence** — `/interview`, `/bug-hunter` (and future `/feature-scope`, `/refactor-scope`) share a family resemblance. Each gates a specific input shape into the same downstream pipeline.

## 10. External input — kept vs rejected

External system: a multi-agent harness from an unrelated project.

**Kept (adapted to wosy):**
- Diagnosis-only role separation (no source edits).
- Minimal-reproduction field set (Impact / Expected / Actual / Steps / Environment).
- Confidence-graded hypothesis (HIGH / MEDIUM / LOW).
- "Why not <alternative>" elimination requirement.
- Companion rationale file pattern (this file).

**Rejected (conflicts with wosy locks):**
- `state.json` sole-writer orchestrator — same shape as the brain-task-record empirically rejected by 1.3.2 B1.
- Fixed `devdoc-format` consolidation template — re-introduces the discipline-dependent template `consolidated.md` died at (2% adoption).
- `rm -rf .claude/runs/<TICKET>/` after consolidation — wosy keeps `_archive/` for audit; discarding-by-default loses queryability.
- `[[wikilink]]` shared-context auto-loader — wosy's CLAUDE.md already eager-loads; the wikilink resolver would duplicate.
- Full pipeline orchestration machinery (rework counters, schema-versioned state, sole-writer rules).

## 11. Worked example (placeholder)

To be filled with the first real `/bug-hunter` run. Until then: read the "Adaptive scope examples" block in `SKILL.md` for shape-illustrative cases.

## 12. Open questions for future evolution

- **Should `/feature-scope` exist as a sibling?** Same shape (gated intake → cited evidence of impact → suggested approach + done_when). Different input (feature ask vs bug report). Defer until a feature-shaped gate is felt as missing.
- **Confidence grading rigor.** HIGH / MEDIUM / LOW is coarse. Does wosy want a calibration table ("HIGH means: cited file:line + reproduces deterministically + alternative ruled out")? Or is the coarseness a feature? Lean: coarse for now, refine if confidence inflation appears.
- **Integration with `agent-query` for data-state bugs.** `agent-query` is the dispatch shape, but should there be a `/bug-hunter --data` flag that biases the dispatch order toward query-first? Lean: no flag — let the captured Actual shape determine the order.
- **Rework patterns.** Convention worth naming? `bug-rework-1.md`, `bug-rework-<date>.md`, or just append to `bug-evidence.md` under a "Rework <N>" heading? Lean: append to `bug-evidence.md`; rework is more evidence, not a new artifact class.
