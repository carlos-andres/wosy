# CHANGELOG

All notable changes to the WOSY system. Reverse-chronological. One line per change is fine — detail lives in commit messages.

The deep changelog (binary shas, gate status, test counts) lives in `docs/WOSY.md` §13.

## Unreleased — 2026-06-11

### Added
- **install.sh `brain` PATH-shadow warning** — `brain-binary-build` verify now warns when `command -v brain` resolves to a different file than `~/.local/bin/brain` with differing content (production case: a stale `~/go/bin/brain` from a manual `go install` shadowing every rebuild). Names both paths and suggests the fix (rm the stale copy or reorder PATH); silent when identical or absent.
- **install.sh `wosy-metrics-deploy` component** — deploys `claude/bin/wosy-metrics.sh` → `~/.local/bin/wosy-metrics` (executable). SKIPs with a re-run hint while the source script hasn't landed.
- **install.sh `session-audit-hook` + `session-audit-matcher` components** — `session-audit.sh` was live-only (registered in `settings.json` but never deployed by the installer); now repo-canonical like every other hook.

### Removed
- **`sessions.tsv` writer killed** (`claude/hooks/session-audit.sh`). The SessionEnd hook appended one TSV row per session to `<cwd>/.devwork/metrics/sessions.tsv` — zero readers existed anywhere, three columns were always `null`, and the cwd-relative path scattered divergent copies across projects. Both existing TSVs archived to their `.devwork/_archive/sessions.tsv.20260611`. Per-session metrics move to `wosy-metrics` (computed on demand from transcripts). The hook keeps its scorecard responsibility (tool-use tallies + hotspots).
- **`/intake` skill retired** (`claude/skills/intake/` deleted). Evidence: 0 fires across the full corporate trial — explicitly skipped every time ("interview record + research already cover that"). Replacement is artifacts-first and already shipped in the hook layer: `wosy-stop.sh` detects sessions that produce ≥2 persistent `.devwork/` files without touching any `tasks/<id>/` folder and nudges direct creation of the task artifact (`tasks/<id>/status.md` + optional `context.md` / `scope.md`). Live templates relocated to their consumers: `claude/skills/handoff/templates/` now holds `handoff.md.template` (read by `/handoff`) and `{plan,task}.yml.schema.json` (read by `agent-scribe` nudge-mode validation). `install.sh` gains the `cleanup-retired-skills` component (removes `RETIRED_SKILLS` dirs from `~/.claude/skills/`, idempotent, not reversed on uninstall) and `wosy-skills-deploy` now verifies per skill name, skipping retired ones.

### Changed
- **Protocol-wide shift: mandatory skills → mandatory artifacts.** Skills keep `disable-model-invocation: true`, but the protocol verifies ARTIFACTS (task folder / plan.yml exists), not that a skill fired. Touched: `claude/skills/bug-hunter/SKILL.md` (`/intake mode B` dispatches replaced by the inline 3-question gate — objective + task name + done_when — followed by direct plan/task creation via `agent-scribe`), `claude/skills/handoff/SKILL.md` (refusal now instructs creating the plan artifact directly; template path updated), `claude/skills/interview/SKILL.md` (`intake-now` exit keeps its name but suggests direct artifact creation), `claude/CLAUDE.md` EDC toolbelt, `claude/skills/constitution-rebuild/SKILL.md`, `docs/WOSY.md`, `README.md`, `brain/README.md`, `claude/agents/agent-scribe.md` (schema paths), `brain/internal/guard/guard_test.go` (comment only). The `wosy-stop.sh` nudge text no longer mentions any skill; marker filenames keep the legacy `retro-intake-*` prefix (reaper-glob + on-disk compatibility).

## 1.3.8 — 2026-06-08

### Added
- **`brain/smoke-integrity.sh`** — new repo-integrity + install-parity harness. Sibling of `smoke-wosy.sh` (enforcement) and `smoke-precheck.sh` (platform detect). Verifies: top-level files present, `~/.claude/` install matches repo via md5, `install.sh` syntax + dry-run, `settings.example.json` JSON validity, brain builds from source, brain CLI exits 0. Section E (brain build + CLI) skips cleanly when Go toolchain or brain binary absent — sections A–D are the must-pass core. Runs in ~2s. Audit provenance: `.devwork/tasks/wosy-audit-2026-06-08/`.
- **README "Two layers" section** — explicit work-surface (MD/YAML you author daily — `claude/`, `.devwork/`) vs substrate (`brain` Go binary + sqlite store — `brain/`) distinction. Heads off the recurring confusion where the Go presence reads as foreign to a contributor who only touches the MD/YAML surface.

### Changed
- **CONTRIBUTING line in README** — pre-submit checklist now names BOTH smokes: `brain/smoke-integrity.sh` (repo + install) and `brain/smoke-wosy.sh` (enforcement).

## 1.3.7 — 2026-05-29

### Added
- **Universal SCRIBE routing rule in `claude/CLAUDE.md`** (new `## SCRIBE routing` section). Any read/write/edit of `.devwork/**.{md,yml,yaml,jsonl}` MUST dispatch `agent-scribe`, regardless of subfolder. Task folders and `_scratch/` previously allowed direct Write/Edit (1.3.2 B1 watchdog policy); the new rule makes scribe routing universal at the instruction layer. Watchdog technical layer unchanged — the files-canonical lock holds; scribe still routes internally per its escape table (brain CLI for KB records, Write for `_scratch/` drafts, Write/Edit for task-folder files). `claude/agents/agent-scribe.md` description + body intro updated to reflect always-dispatch.

### Fixed
- **Skill template standardized to derived template across 11 `SKILL.md`.** Empirical pattern extraction via Python `re`-based scan of all 11 files produced an agnostic template grounded in gold data, not external models. Frontmatter: `name` / `description` / `when_to_use` (universal across 11/11), `disable-model-invocation` (7/11 — manual-only skills). Body H2 order: `When this fires` → `Inputs` → `Workflow` (with `### Phase N` / `### Step N` allowed) → `Output` → optional `Role & boundaries` / `Caveats` / `Companions` / `Limits` → `Hard rules`. Canonical heading names match empirical majority ("Hard rules" not "Rules"; "When this fires" 8/11; "Inputs" preferred over `Required input` / `Input shape`). Files touched: 11 × `claude/skills/<name>/SKILL.md`.
- **graphify frontmatter cleanup** — non-conforming `trigger: /graphify` field stripped (1/11 occurrence in gold data; not on Anthropic's recognized field list).
- **De-decoration pass across 17 files** (11 SKILL.md + 6 agents + 3 companions + CLAUDE.md). Cut: audit-citation narrative ("Evidence from 2026-05-21 audit (D10)"), rhetorical asides ("The graph is the map. Your job is to be the guide."), meta-commentary ("Symmetric to the existing rule"), Anthropic-name-drop justifications, DEPRECATED tombstones. Kept verbatim: output templates inside fenced blocks, tables, paths, command snippets, hard-rule bullet lists.
- **hq-orchestrator dispatch table** — `agent-scribe` row added; was missing in source but already listed in `claude/CLAUDE.md` Available-agents line. Drift sync.
- **bug-hunter scope examples** — Anti-patterns section folded into Hard rules (gold data: anti-patterns are hard rules in negative form). "The principle" rationale section cut (audit citations migrated to imperatives in Hard rules and Rationale).
- **interview/SKILL.md "The principle" + "Migration notes"** sections cut (pure narration). Behavioral content distilled into one-line opener + a single "Don't migrate existing specs" hard rule.
- **CLAUDE.md ceremony trims**: "Anthropic's highest-leverage practice:" preamble, "Trust without verification is how recommendations fall into the dune" rhetorical close, "Each rule builds on a principle already in this file" intro, "Symmetric to the existing rule" meta-commentary, "The cost of one sentence is small. The cost of doubling the spend without warning is bigger" rhetorical contrast pair, deprecated `~~Shipped-marker convention~~` tombstone. Net: 150 → 167 lines (−ceremony / +SCRIBE rule).
- **`/bug-hunter` added to Skill toolbelt** in `claude/CLAUDE.md` (was missing despite being shipped in 1.3.4).

### Tests
- `go test ./...` in `brain/`: **125/125 PASS** across 9 packages (zero Go edits this release).
- `bash brain/smoke-wosy.sh`: **ALL PASS ✓** (no classifier-shape regression from prose-only edits).
- Python ceremony detector (post-edit verification): 6 hits across 21 files. 2 actionable → trimmed (rationale.md "Users vote with their tools" aphorism; bug-hunter SKILL.md "forensic-citation discipline established in" wrapper). 4 KEEP (description discovery metadata, false-positive rationale section heading, load-bearing policy-lock refs, vendor sponsorship line).
- Empirical re-audit: heading vocabulary canonicalized across 11/11 skills; frontmatter conforming 11/11.
- Subagent review pass (`agent-reviewer` dispatched against the diff): 9 findings — 3 actionable applied (rationale.md cross-ref to renamed CLAUDE.md section; hq-orchestrator `When this loads` semantics restored; agent-scribe "Don't propose alternatives" + R11 isolation note re-added to Hard rules). 6 documented overrides / accepted variants (Output-before-Workflow ordering in 4 SKILLs where workflow steps fill the output template; agent-scribe dispatch table sync; LOW agent-body docs trims).

### Live-vs-source drift (advisory)
- `~/.claude/skills/` and `~/.claude/agents/` are live directories. Sync command: `cp -r claude/skills/ ~/.claude/skills/ && cp -r claude/agents/ ~/.claude/agents/`.
- `~/.claude/CLAUDE.md` is the user's personal overlay with archived lessons + RTK import + MEMORY pointer + an older SCRIBE block. The Option B canonical-split (per `audits/2026-05-29-claude-md-live-sync-strategy.md`) was not executed before 1.3.7. Until then, the new universal-SCRIBE rule from this release does NOT auto-merge — it must be hand-merged into the personal `~/.claude/CLAUDE.md` SCRIBE section (replace the older block).

### Deferred (1.3.8+ candidates)
- Option B canonical-split execution (one-time user action to reshape `~/.claude/CLAUDE.md` into personalization + `@canonical.md` import).
- Description-prose trim across the 11 skills (carried from 1.3.6 Deferred).
- `.claude/rules/<name>.md` path-scoped rule files (carried).
- Worked example for `bug-hunter/rationale.md` Section 11 (placeholder until first real `/bug-hunter` invocation).
- `hq-orchestrator` skill body re-evaluation — now that the core rule is auto-loaded via CLAUDE.md, the skill body is purely a deep reference. Move to `audits/` / `docs/` as a reference doc? (carried).
- Standing deferrals (master §8): `bigtex` / `atw` onboards; v3 harness re-run; PilotB empirically dormant.

## 1.3.6 — 2026-05-29

### Added
- **`when_to_use` frontmatter field across all 11 skills** — Anthropic-published pattern (separate field from `description`, both count toward the 1536-char skill-listing cap). Per-skill content derived from existing trigger-phrase prose in each `description`; description text unchanged (additive only — a description trim is a future round so we don't conflate adding the field with rewriting copy). Files touched: `claude/skills/{bug-hunter,consolidate,constitution-rebuild,context,graphify,handoff,hq-orchestrator,intake,interview,query,wire-up}/SKILL.md`.
- `audits/2026-05-29-claude-md-live-sync-strategy.md` — Stage F decision doc. Picks **Option B** (split personalization, `@canonical.md` import) over (A) manual hand-paste (current broken state) and (C) generated overlay (over-engineering). One-time user action to reshape `~/.claude/CLAUDE.md`; subsequent syncs become `cp wosy/claude/CLAUDE.md ~/.claude/canonical.md`. Personalization (SCRIBE routing block, archived wosy-lesson sections, `@RTK.md` import) lives in the personal file, never touched by sync.

### Fixed
- **FixHint regression: `brainHelpTail` was promoted to a constant in 1.3.2 ("consistent across all 5 block classes") but only applied to `inline_sql`.** The 4 other classes (kb_write, bash_kb_write, destructive_kb, inline_ssh) had no tail. Restored design intent per `internal/guard/guard_test.go:299` comment ("end with the brain --help promotion tail for the 4 brain-CLI classes; inline_ssh is the exception"). `brainHelpTail` now appended to kb_write / bash_kb_write / destructive_kb (3 added). inline_ssh deliberately stays without (connections alias is the right answer, not brain). This was shadow-promotion regression, not test drift — the constant existed and the comment claimed it was used; the implementation simply lost it during compaction. Same shape as the 2026-05-28 `WOSY_SCRIBE_BYPASS` lesson: verify the implementation against the comment, not the comment against the implementation.
- **FixHint substring shapes corrected to match test spec.** `kb_write`: `KB→brain set/import` → `KB→brain set, brain import` (so "brain import" appears as a substring, not glued via `/`). `bash_kb_write`: `KB→brain set/import` → `KB→brain set/append, brain import` (the bash redirect semantics include append, which the test expects). `destructive_kb`: `Task folders ... user-owned — not protected.` → `Use brain CLI to snapshot; task folders ... user-owned — not protected.` + `brainHelpTail` (the test asserts a "brain" substring; the route arg mentioned brain but the FixHint didn't — separate fields).
- **`disable-model-invocation: true` added to 3 more skills**: `hq-orchestrator`, `wire-up`, `context`. Pattern continues from 1.3.5 (consolidate / constitution-rebuild / handoff / graphify). Rationale: hq-orchestrator's core conductor rule is already auto-loaded via CLAUDE.md when `.devwork/` is present — the skill body is deep reference only; context is pure on-demand utility (user-typed `/context` or explicit question); wire-up is high-cost pre-flight that should require explicit user intent (trigger-phrase prose remains as user-typed reminders, no longer auto-routes). All other behaviors preserved.

### Tests
- `go test ./...` in `brain/`: **125 / 0** — up from 121/4. The 4 `TestClassify_FixHint` subtests carried over from 1.3.3/1.3.4 all pass after the FixHint regression fix above. Touched files: `internal/guard/guard.go` lines 691, 700, 731.
- `bash brain/smoke-wosy.sh`: **13/14 PASS, 1 FAIL** — same drift as 1.3.5 ("hook not installed/identical"). Source hooks remain correct from 1.3.5; live sync is the explicit last step per user constraint ("installation only when we finish"). Post-sync expected: 14/14 PASS.
- Binary sha changed (Go edits in 1.3.6): `internal/guard/guard.go` modified, brain binary needs rebuild on next install. Shim sha unchanged (no hook edits in 1.3.6).

### Live-vs-source drift (advisory)
- Same advisory as 1.3.5 — Stage F now has a strategy doc (`audits/2026-05-29-claude-md-live-sync-strategy.md`) but the user has not yet executed the one-time split. Until then, the existing "do not overwrite live CLAUDE.md" rule still applies.
- New for 1.3.6: 14 skill files have updated frontmatter (`when_to_use` × 11 + `disable-model-invocation` × 3). Live sync of those is included in Stage J.

### Deferred (1.3.7+ candidates)
- **Description-prose trim** across the 11 skills. The trigger-phrase content currently lives in both `description` and `when_to_use` (additive add was the right call for 1.3.6; the trim is a focused future round).
- **`.claude/rules/<name>.md` path-scoped rule files** — promising home for the 23-line SCRIBE routing block currently living in the personalization half of `~/.claude/CLAUDE.md`. Carried over from 1.3.5 Deferred.
- **CLAUDE.md compaction pass 2** — explored in Stage E. No clean cuts found that preserve the operational signal: every section's bullets do distinct work; the HQ orchestrator section's 4 cheap-path conditions are OR'd triggers, each different. Forcing cuts loses signal. Defer with empirical reason (matches the 1.3.5 lesson "if a 'rule' lives only in docs, it's not a rule — it's a wish": chasing a number isn't a real improvement). Revisit only when a duplication actually surfaces.
- **`hq-orchestrator` skill body re-evaluation.** Now that `disable-model-invocation: true` is set and the core rule is auto-loaded via CLAUDE.md, the skill body is purely a deep reference. Question for a future round: should it stay as a skill, or move to `audits/` / `docs/` as a reference doc?
- **graphify three-file pattern behavioral verification** — still gated on first real `/graphify` invocation post-1.3.5 refactor. Carried.
- Standing deferrals (master §8): `bigtex` / `atw` onboards; v3 harness re-run; PilotB empirically dormant.

## 1.3.5 — 2026-05-29

### Added
- **Anthropic Claude Code best-practices alignment** — full system audit (11 skills + 6 agents + 12 hooks + CLAUDE.md) against `code.claude.com/docs/en/*` published guidance, then surgical alignment edits. The user's direction: an external toolkit was input data in 1.3.4 round 1; in 1.3.5 round 2 the lens widens to wosy reaching the next level, with Anthropic's published practices joining the input set.
- `audits/2026-05-29-round-2-best-practices-absorption.md` — full round-2 audit (trigger, method, decision table, kept/rejected with reason, deferrals).
- `audits/2026-05-29-claude-md-reduction.md` — companion audit preserving the verbatim content of the 4 archived CLAUDE.md sections (Capability-aware tooling + wire-up lessons, Wosy 1.3.2 intake B1 policy, Wosy lessons from net-new mining, Wosy lessons from real test-drive). Every non-duplicated rule was either compacted into a bullet in the live file or preserved here.
- `claude/skills/graphify/reference.md` (185 lines) — full CLI surface extracted from the bloated SKILL.md: every `--flag` with default and use case, per-subcommand flag tables.
- `claude/skills/graphify/pipeline.md` (1109 lines) — full implementation detail extracted from SKILL.md: Steps 0–9 with substeps, subagent prompt template, cache logic, `--update` merge logic, vocab expansion, MCP / watch / hook / claude-install subcommand detail.

### Fixed
- **CLAUDE.md size: 227 → 150 lines.** Anthropic published target is <200 lines per CLAUDE.md ("Longer files consume more context and reduce adherence"). Cuts: Shipped-marker DEPRECATED tombstone (−7), HQ orchestrator session-end tail (−10), Browser inspection hard-rules restatement (−5), Skill toolbelt archived-skills + heartbeat bookkeeping (−3), Multi-step rhetorical scaffolding (−3), Capability-aware tooling block (−12, compacted to 1 bullet under Tooling preferences), Wosy 1.3.2 section (−10, compacted to 1 bullet under Artifact location), two Wosy lesson blocks (−24 net, 2 non-duplicated rules extracted into Communication discipline). Total: −77 lines, every non-duplicated rule preserved.
- **graphify SKILL.md size: 1157 → 89 lines.** Anthropic published cap is ≤500 lines body. Refactored into three-file pattern (SKILL.md = standing instructions, reference.md = CLI, pipeline.md = implementation detail). Frontmatter preserved; behavior preserved. Pure restructure.
- **graphify description tightened** — old wording "Use when user asks any question about a codebase" was dangerously vague (would auto-fire on every codebase discussion, loading the body unnecessarily). Now requires explicit `/graphify` invocation OR an existing `graphify-out/` directory the user references by name.
- **`disable-model-invocation: true`** added to 4 side-effect-only skills: `consolidate`, `constitution-rebuild`, `handoff`, `graphify`. Each explicitly states "manual only" in prose; the frontmatter flag is the Anthropic-published enforcement mechanism so the prose has force.
- **Subagent model pinning**: `agent-query` and `agent-server` moved from `sonnet` to `haiku` per Anthropic's published "Control costs by routing tasks to faster, cheaper models like Haiku" guidance. Both agents read 1–2 files and either run SQL or format SSH output — no inference required.
- **agent-tracer description** rewritten to distinguish from `agent-scout` (cross-file tracing vs single-pattern discovery). Anthropic: "Write detailed descriptions: Claude uses the description to decide when to delegate." Boundary was previously only documented in `agent-scout` body, which the orchestrator does not read when choosing the tracer.
- **agent-tracer Step 4 `[y/n]` UX ghost removed.** Subagents cannot receive interactive replies (they run to completion per Anthropic spec). Replaced with deterministic "cache when ≥3 files" rule.
- **`watchdog.sh` brain subprocess wrapped in `timeout 5`.** Without it, a hung brain binary would stall Claude Code for the full 600s PreToolUse budget (Anthropic-published timeout). Timeout exit falls through to empty stdout → `action=allow` (fail-open) — same shape as the existing missing-binary guard.
- **`output-telemetry.sh:17` misleading comment fixed.** Was: "can be hard-converted to `exit 1` on violation (true block)". Per Anthropic hooks contract, only `exit 2` blocks — `exit 1` is a non-blocking error. Comment now states the correct semantics.
- **`bug-hunter` SKILL.md auto-fire heuristic removed** — the speculative "Also fire automatically when a kickstart contains: an error message + a file path + a recent commit" line had no `disable-model-invocation` guard and would auto-load 290 lines on false positives. Trigger phrase block + skip rule retained.
- **Cross-reference repairs in `bug-hunter` SKILL.md and `CLAUDE.md`** (from the Phase E `/simplify` quality gate): `agent-scribe` added to the CLAUDE.md available-agents list (was missing); "Four rules" → "Six rules" in Communication discipline section header (count drifted after the 2 extracted bullets were appended); `audits/` paths corrected to `../audits/` (relative to `claude/CLAUDE.md` the file lives one directory up); 2 `bug-hunter` citations rewritten to point at the surviving inline locations / `/interview` skill, since the named CLAUDE.md anchors they cited were archived.

### Tests
- `bash brain/smoke-wosy.sh`: **13/14 PASS, 1 FAIL** — the failure is "hook not installed/identical". Smoke compares the source `claude/hooks/` against the installed `~/.claude/hooks/`; 1.3.5 edited `watchdog.sh` (added `timeout 5` wrapper) and `output-telemetry.sh` (corrected misleading comment) in source WITHOUT syncing the install. The source hooks themselves are correct — `timeout` is non-breaking (falls through to fail-open on timeout, same shape as the existing missing-binary guard); the output-telemetry change is a comment fix (no behavioral change). Sync command: `cp $REPO_ROOT/claude/hooks/watchdog.sh $REPO_ROOT/claude/hooks/output-telemetry.sh ~/.claude/hooks/`. Post-sync expected: 14/14 PASS. Derivation: smoke re-run at 2026-05-29 after all Phase D edits + Phase E fixes.
- `go test ./...` in `brain/`: **121/4** — same 4 pre-existing failures from 1.3.4 carry over unchanged. All in `guard_test.go:374` (`TestClassify_FixHint/*`). Zero Go edits in 1.3.5. Cleanup flagged in Deferred.
- Binary sha unchanged: `312d197b128e30d1c8bfe1baa1b31f20a40d364a` (no brain CLI edits).
- Shim sha unchanged: `04c41f3893ce6c018fd1c33010d9e56805fa0cec` (no hook *logic* edits — watchdog timeout is a wrapper around the existing brain call, not a contract change; output-telemetry change is a comment fix).

### Live-vs-source drift (advisory)
- Same drift situation as 1.3.4 — `~/.claude/CLAUDE.md` is a regular file copy with personalized content (SCRIBE routing block, RTK.md / MEMORY.md imports, project-specific identifiers from past session lessons). Round 2's cuts apply only to canonical source. Live sync strategy is deferred per `audits/2026-05-29-round-2-best-practices-absorption.md` §6.
- `~/.claude/skills/graphify/` is the OLD 1156-line file. The new 89-line SKILL.md + companions (`reference.md`, `pipeline.md`) exist only in the source repo. Manual copy command: `cp -r $REPO_ROOT/claude/skills/graphify ~/.claude/skills/` (will overwrite).
- `~/.claude/hooks/watchdog.sh` and `~/.claude/hooks/output-telemetry.sh` need manual sync (drives the smoke 1 FAIL above). Sync command: `cp $REPO_ROOT/claude/hooks/{watchdog,output-telemetry}.sh ~/.claude/hooks/`.

### Deferred (1.3.6+ candidates)
- **`when_to_use` frontmatter field across all 11 skills.** Anthropic-published pattern (separate field appended to description, both count toward the 1536-char skill-listing cap). Substantive structural change; deferred to a focused round.
- **`.claude/rules/<name>.md` path-scoped rule files.** Promising home for the 23-line SCRIBE routing block currently in user's drifted live copy.
- **`TestClassify_FixHint` text-format drift** — 4 failing subtests pre-existing from 1.3.3. Decision needed: update tests to match current FixHint output OR restore the longer-form FixHint.
- **`hq-orchestrator` and `wire-up` `disable-model-invocation` review** — both have manual-only semantics but didn't receive the flag in 1.3.5.
- **`context` skill `disable-model-invocation` review** — pure-bash passthrough; candidate for the flag.
- **Live sync strategy for `~/.claude/CLAUDE.md`** — source is now 150 lines; live is 239 lines with personalization. Sync mechanics need a decision.
- **Second compaction pass on CLAUDE.md.** 150 lines is under cap but not minimal. Sections like HQ orchestrator (35 lines) could compact further.
- **graphify three-file pattern behavioral verification** — first real `/graphify` invocation post-refactor is the smoke test.
- Standing deferrals (master §8): `bigtex` / `atw` onboards; v3 harness re-run; PilotB empirically dormant.

## 1.3.4 — 2026-05-29

### Added
- **Communication discipline + numeric calibration** rules in `claude/CLAUDE.md` (4 rules in one new dated section). Closes pre-existing gaps in wosy's prose discipline: anti-hedging on decided stances · calibrate before claiming any number · surface conflicts and pick by evidence · surface atypical token cost upfront. Each rule reasons from a wosy principle already in the file (forensic citation, schema-before-queries, 1.3.2 B1 macro-lock, `check_context` reactive complement). Source analysis: an external multi-agent system evaluated as input data, kept the gap-closing rules, rejected the project-specific scaffolding (numeric cost-band tables, person-hours estimation framework, team-facing glossary discipline).
- `claude/skills/bug-hunter/` — new skill, sibling of `/interview`. Adaptive bug-investigation gate: kickstart-aware intake of the minimal-reproduction set (impact / expected / actual / steps / environment), HQ-mode dispatch of `agent-tracer` / `agent-scout` / `agent-query` / `agent-server`, three artifacts written files-canonical to `.devwork/tasks/<id>/bug-{context,evidence,plan}.md`. Diagnosis only — never edits source code; fix is a separate task with its own gate. Includes companion `rationale.md` (not auto-loaded) documenting design choices and external-source provenance.
- `audits/2026-05-29-communication-calibration.md` — full audit for the CLAUDE.md additions (trigger, decision, implementation, verification, deferrals, live-vs-source drift strategy).
- `audits/2026-05-29-bug-hunter-skill.md` — full audit for the new skill (gap analysis, design choices grounded in wosy principles, rejected pipeline machinery, deferrals).

### Fixed
- None this release. Additive only.

### Tests
- `bash brain/smoke-wosy.sh`: **14/14 PASS** — re-verified post-1.3.4 changes (full block of watchdog enforcement + fail-open + validate-bash hard-blocks all pass). No classifier-shape regression from the prose-only 1.3.4 deliverables. Derivation: smoke run at 2026-05-29 against `claude/CLAUDE.md` + `claude/skills/bug-hunter/` additions.
- `go test ./...` in `brain/`: **121/4 (4 failed)** — pre-existing failures, not introduced by 1.3.4. All 4 failures are in `guard_test.go:374` (`TestClassify_FixHint/*` subtests) — text-format drift between expected FixHint strings (`"brain import"`, `"brain set/append"`) and the actual FixHint output (`"Drafts→_scratch/ · Tasks→tasks/<id>/ · KB→brain set/import"`). 1.3.4 ships zero Go edits — the divergence is from the 1.3.3 baseline. Flagged for cleanup in 1.3.5+ (see Deferred).
- Binary sha unchanged: `312d197b128e30d1c8bfe1baa1b31f20a40d364a` (no brain CLI source edits).
- Shim sha unchanged: `04c41f3893ce6c018fd1c33010d9e56805fa0cec` (no hook edits).
- Prose-rule and skill-file additions are verified-by-inspection. First behavioral confirmation comes on first real `/bug-hunter` invocation.

### Live-vs-source drift (advisory, not blocking)
- Preflight confirmed `~/.claude/CLAUDE.md` is a regular file copy that has drifted from `claude/CLAUDE.md` in the repo (carries user-personalized RTK.md / MEMORY.md / project blocks). Both `~/.claude/skills/` and `~/.claude/agents/` are live directories, not symlinks. The 1.3.4 deliverables ship as paste-ready additions (the new CLAUDE.md section + the `bug-hunter/` skill subtree) — auto-sync NOT performed to preserve personalization. See `audits/2026-05-29-communication-calibration.md` §5 and `audits/2026-05-29-bug-hunter-skill.md` §5 for the recommended manual sync steps.

### Deferred (1.3.5+ candidates)
- **`TestClassify_FixHint` text-format drift.** 4 subtests in `brain/internal/guard/guard_test.go:374` expect FixHint strings containing `"brain import"` and `"brain set/append"`, but the actual FixHint output is `"Drafts→_scratch/ · Tasks→tasks/<id>/ · KB→brain set/import"`. Pre-existing as of the 1.3.4 verification run (zero Go edits this release). Pick by evidence: the actual output reads more usefully — likely the test assertions are stale. Suggested 1.3.5 fix: update test expectations to match the current FixHint shape, OR if the test reflects intent, restore the longer-form FixHint that names all three verbs. Investigation needed before deciding.
- **Working-rules consolidation pass.** The dated-lesson blocks (lines 137+ of `claude/CLAUDE.md`) have grown to ~9 sections post-2026-05-20. A mid-version compaction (consolidate overlapping lessons, demote fully-absorbed rules to a dated archive) would reduce always-loaded surface. Hold until two more dated blocks accumulate.
- **`/feature-scope` sibling skill** to `/bug-hunter` and `/interview` — same adaptive-intake shape, different inputs (feature ask) and outputs (impact map). Hold until felt as missing; premature creation repeats the 2% `consolidated.md` adoption anti-pattern.
- **Pre-output calibration hook.** A future `pre-output-check.sh` could grep agent output for unanchored magnitude phrases ("about", "~", "roughly N", "should take N hours") and warn before send. Collect failure cases first before tooling.
- **Confidence-grading calibration table for `/bug-hunter`.** HIGH/MEDIUM/LOW is coarse — possible refinement once confidence inflation appears in real runs.
- **`bug-hunter` → `agent-reviewer` integration.** Natural pre-fix gate, currently user-initiated. Defer until a real case shows the value.
- Standing deferrals (master §8): `bigtex` / `atw` onboards; v3 harness re-run from plain Terminal (inventory O.6); PilotB empirically dormant per 1.3.2 discovery.

## 1.3.3 — 2026-05-29

### Added
- `brain schema [--type=<name>]` verb. No flag → lists 7 public record types one per line (excludes `_base` internal $ref base). `--type=<name>` → dumps raw embedded JSON schema for that type. Closes the 4-fumble cluster of `brain schema --type=task` across Globex subagents (2026-05-29 brain --help mining). Source: `brain/schema.go` (new file, exported helpers wrapping the existing `//go:embed schema/*.schema.json`) + `brain/internal/cli/cli_schema.go` (new file, handler).
- `claude/agents/agent-scribe.md` Step 0 — schema-discovery block after `brain --help`. Tells the scribe to run `brain schema --type=<recType>` before any `brain set` / `append` / `import` on an unfamiliar type, so the allowed `--section` values (G3) are known upfront. Kills the "REJECTED (unknown section)" round-trip.
- `audits/2026-05-29-brain-schema-verb.md` — full audit trail (trigger evidence, decision rationale, fixes, verification matrix, 1.3.4 deferrals).

### Fixed
- `brain/internal/cli/cli.go` — top-of-Run usage error now lists `schema` so users typing `brain` with no args see the full surface.
- `brain/internal/cli/cli.go` — `helpText` "Read / discover:" section gets the schema row, mirroring `docs/WOSY.md` §4.

### Tests
- 5 new `internal/cli` cases in `cli_schema_test.go`: `TestSchema_NoFlag_ListsTypes`, `TestSchema_TypeTask_DumpsJSON`, `TestSchema_AllPublicTypes_OK` (table-driven across all 7), `TestSchema_UnknownType_Fails`, `TestSchema_BaseType_Allowed`.
- `go test ./...`: **125/125 PASS** across 9 packages (was 120 at 1.3.2; +5 new schema cases).
- `bash brain/smoke-wosy.sh`: **ALL PASS ✓** (14/14 — no classifier-shape change, additive verb only).
- New binary sha: `312d197b128e30d1c8bfe1baa1b31f20a40d364a` (was `c30659000…` at 1.3.2).
- Shim sha unchanged: `04c41f3893ce6c018fd1c33010d9e56805fa0cec` (no watchdog edit).

### Deferred (1.3.4+ candidates)
- **`brain create` 1-fumble case.** Different shape from `schema` — agents reach for `create` when they mean `import` (which takes a JSON record on stdin). Two options: (a) alias `create → import`, (b) leave alone since the FIX hint already routes them via `brain --help` → import. Hold for more usage data; 1 fumble is not yet a pattern.
- Standing deferrals (master §8): `bigtex` / `atw` onboards; v3 harness re-run from plain Terminal (inventory O.6); PilotB empirically dormant per 1.3.2 discovery.

## 1.3.2 — 2026-05-29 (later)

### Locked
- **Intake brain-vs-files canonical drift: Option B1 (Pure files-canonical).** Empirical lock based on 3-repo discovery (pilot-a dogfood 1/11 = 9% brain task adoption; Globex 3/14 = 21%; PilotB dormant). User vote: files-canonical. Brain task records remain opt-in for cross-cutting indexing, not the canonical store.

### Fixed
- `brain/internal/guard/guard.go` — removed `reKBTaskFile` rule from `isKBPath` AND from `isKBTarget`. `.devwork/tasks/<id>/{status,context,scope}.md` AND custom names like `findings.md`, `verification.md`, `inventory.md` now flow free. Destructive ops on task folders also flow free (user owns those files; brain store still protected). Evidence: `audits/2026-05-29-intake-policy-execution.md` §3.
- `brain/internal/guard/guard.go` — added `/tasks/` to `kbExcludeSegments` so the widened kbExt rule branch matches B1 policy across all `.md/.yml/.yaml/.jsonl` files under any task folder.
- `brain/internal/guard/guard.go` — removed `status.md`, `context.md`, `consolidated.md` from `kbBase`. status.md/context.md were task-folder canonical names (files-canonical now); consolidated.md was the shipped-marker convention (2% adoption in pilot-a evidence — broken signal, deprecated).
- `claude/agents/agent-scribe.md` — sharpened the `WOSY_SCRIBE_BYPASS` reference from "is fiction" to "was removed from brain guard 2026-05-29" — now literally accurate after the 1.3.1 POC removal.

### Added
- `brain/internal/guard/guard.go` — `Decision.FixHint string` field. Watchdog shim now relays it verbatim. Closes 2026-05-28 D-1 (FIX text into JSON contract). Per-class hints populated at 5 block sites (kb_write, inline_sql, inline_ssh, destructive_kb, bash_kb_write).
- `brain/internal/guard/guard.go` — `brainHelpTail` const — SHADOW-ADOPTED remediation. Appended to 4 of 5 class hints (inline_ssh excluded — connections.md is the right answer, not brain). Evidence: brain --help discoverability research found 5 invocations corpus-wide, all wosy-dev or scripted; FIX hint drove 7/18 sessions → brain write-verb usage.
- `claude/agents/agent-scribe.md` — new Step 0 "Discover the brain CLI surface (mandatory on first dispatch)" with `brain --help` invocation. Pairs with the FIX-hint promotion: hook promotes it, scribe runbook primes it.
- `claude/agents/agent-scribe.md` — escape table gets a new "Task-folder file" row + explicit "NOT KB-protected" list naming `.devwork/tasks/<id>/**` plus the human-edited root-level docs (`connections.md`, `INDEX.md`, `BACKLOG.md`).
- `audits/2026-05-29-intake-policy-execution.md` — full audit trail (discovery findings + policy lock + execution + verification matrix).

### Removed
- `claude/hooks/watchdog.sh` — per-tool `case` statement (13 lines). Replaced with a 2-line JSON `.fix_hint` relay. Single-source-of-truth contract now lives in `guard.go`'s `block()` calls.
- `brain/internal/guard/guard.go` — `reKBTaskFile` var declaration (no longer used).
- `brain/internal/guard/guard_test.go` — `consolidated.md at .devwork root blocked` cases (no longer blocks per kbBase removal; replaced with `state.yml` / `STATUS.md` regression guards).

### Tests
- 13 new guard cases (8 task-folder positives, 1 task-folder destructive allow, 4 kbBase exclusion/regression edits).
- `TestClassify_FixHint`: 5-case suite verifying fix_hint presence + substring contracts (`brain set`, `brain import`, `_scratch/`, `q.sh`, `brain promote`, `connections.md`, `brain --help`).
- `go test ./...`: **120/120 PASS** across 9 packages (was 107 at 1.3.1; +13 new guard cases).
- `bash brain/smoke-wosy.sh`: **ALL PASS ✓** (14/14).
- New binary sha: `c30659009b6823a40503fdca2528d9bf9589f2ee` (was `2d5548bb…` at 1.3.1).
- New shim sha: `04c41f3893ce6c018fd1c33010d9e56805fa0cec` (was `026c11cf…` at 1.3.1).

### Deferred (1.3.3 candidates)
- **`brain schema` verb decision.** brain --help research surfaced 4 fumbles of `brain schema --type=task` across Globex subagents. Three options pending user lock: (1) add as real verb dumping JSON schema for a record type, (2) alias to existing `detect`/`doctor`, (3) document as gap. Source: `brain/cmd/brain/main.go` + `brain/schema/`.
- v3 harness re-run from plain Terminal (inventory O.6 — still open).

## 1.3.1 — 2026-05-29

### Fixed
- `brain/internal/guard/guard.go` — kbBase basename rule (`status.md`, `context.md`, `consolidated.md`, `STATUS.md`, `state.yml`, `ledger.yml`, `plan.yml`) now honors `kbExcludeSegments`. Pre-fix bug: `.devwork/_scratch/status.md` blocked under enforced flags even though `_scratch/` is the documented universal-write path. Evidence: PROJ-1414 transcripts (b8b62bab) showed scribe loops; user fell back to `session-tracker.md` to dodge the basename pattern.
- `brain/internal/guard/guard.go` — scribe-poc-b widened kbExt rule now requires depth ≥ 2 below `.devwork/`. Pre-fix bug: root-level `.devwork/connections.md`, `.devwork/INDEX.md`, `.devwork/BACKLOG.md` blocked even though they're human-edited operational docs (not brain records). Deadlock: Write blocked AND `brain set` errors because they're not brain records. Root-level files now flow free; explicit `kbBase` entries (consolidated.md, STATUS.md, etc.) still block at root via the unchanged kbBase rule. Nested subfolder rule unchanged (decisions/, specs/, plans/, etc. still block).
- `brain/internal/guard/guard.go` — removed the `WOSY_SCRIBE_BYPASS` env-var POC (scribe-poc-b). Dead code: Claude Code subagent tool calls don't propagate env vars set via `Bash export …` across to subsequent Write/Edit calls. The 2026-05-28 audit said the env var "doesn't exist in any hook" — true for the bash shims, but the Go source DID have it. Removing closes the gap and makes the agent-scribe.md "no bypass exists" claim literally true. Evidence: audits/2026-05-29-classifier-portability-fixes.md §3C.
- `install.sh` — `SRC_WATCHDOG` is now script-relative (`$SCRIPT_DIR/claude/hooks/watchdog.sh`), not hardcoded `~/Documents/.devwork/brain/hooks/watchdog.sh`. The previous path (a) only existed on the author's machine and (b) was a stale pre-1.3.0 copy without the FIX hint. A fresh clone + `./install.sh --apply` now sources the canonical hook from the repo itself.
- `install.sh` — precondition check ensures `~/.local/bin/` exists (creates under `--apply`). Fresh user accounts don't have it; `BRAIN_BIN` default was assuming it.
- `claude/hooks/wosy-stop.sh` — scan roots reordered: `$WOSY_SCAN_ROOTS` first, then `$HOME/projects`, `$HOME/Documents`, `$HOME/Work` (last for legacy back-compat). Previous behavior hardcoded `$HOME/Work` first, operator-specific. Fresh machines without `~/Work/` got no useful default.
- `brain/smoke-wosy.sh` — `SRC` fallback derived from `$SCRIPT_DIR`; `$PROJ` overridable via `$WOSY_SMOKE_PROJECT`; pilot-a enforcement block updated (covers full-profile flags + 3 new 1.3.1 classifier probes).
- Removed stale `brain/hooks/watchdog.sh` (1.7K, no FIX hint). Canonical is `claude/hooks/watchdog.sh`. `brain/hooks/PROTOCOL.md` updated to point at the canonical location.

### Added
- `claude/skills/interview/SKILL.md` — `#reference` / `#source` / `#related` hashtag convention for asset role classification at kickstart inspection (Step 1). Adds `assets:` field to the unified task template with `path` / `role` / `note` per asset. Classification echoed in Step 2 so users can correct a misclassified asset before any tool call. Evidence: 3 unprompted user uses of "as reference" in PROJ-1414 transcripts (fc6ea9c0 12:22, 12:35, 13:48) — the vocabulary is the user's own.

### Removed
- `brain/internal/guard/guard.go` — `WOSY_SCRIBE_BYPASS` POC (see above).
- `brain/internal/guard/guard_test.go` — `TestClassify_ScribeBypass` (no longer applicable).
- `brain/hooks/watchdog.sh` — stale duplicate (see above).

### Tests
- 11 new guard cases (7 positive: _scratch/status.md allow, _scratch/<id>/context.md allow, _scratch/state.yml allow, connections.md root allow, INDEX.md root allow, BACKLOG.md root allow; 4 regression guards: consolidated.md/STATUS.md/state.yml at root still block, decisions/*.md still block).
- `go test ./...`: **107/107 PASS** across 9 packages (was 96/96 at 1.3.0; +11 new cases, -1 removed scribe-bypass test).
- `brain doctor --integrity` (pilot-a): logical sha matches pin.
- `bash brain/smoke-wosy.sh`: **ALL PASS ✓** (14/14, including 3 new 1.3.1 classifier-fix probes).
- New binary sha: `2d5548bbad20af2bc80e55f010cf64c42f1d4629` (was `d9296f74…`).

### Deferred (next round)
- `/intake` brain-vs-files canonical drift. SKILL.md documents `status.md` / `context.md` / `scope.md` as plain files at `.devwork/tasks/<id>/`, but the classifier's `reKBTaskFile` rule blocks them. Three competing stores for one ticket (brain task record, task folder files, `_scratch/<id>/` fallback). Needs a one-line policy in `/intake`'s SKILL.md naming brain as canonical. Source: `claude/skills/intake/SKILL.md`.
- Move watchdog `FIX:` hint text into `brain guard`'s JSON response (2026-05-28 D-1 carry-over). Today's shim case statement compensates. Source: `brain/internal/guard/`.
- Extend the watchdog `FIX:` hint with a 4th option: "or rename the file if filename matches a kbBase pattern". The 2026-05-29 classifier fix removes most cases this would help; lower priority.
- `brain --help` discoverability (1.3.0 shipped it; 2 net-new sessions show zero usage — either the loop didn't take root or existing habits filled in first). Unverified; not actionable this round.

## 1.3.0 — 2026-05-28

### Repo
- Initialized canonical repo at `~/Documents/GitHub/wosy/` (was: scattered across `~/.claude/` + `~/Documents/.devwork/`).
- Imported live state as baseline (2026-05-28).

### Fixed
- `claude/agents/agent-scribe.md` — removed hallucinated `WOSY_SCRIBE_BYPASS=1` mechanism (no such env var exists in any **bash hook**; the Go-side POC was discovered + removed in 1.3.1 closing the gap completely); rewrote tool flow to use brain CLI (`brain set/append/import`) + `.devwork/_scratch/` for drafts. Evidence: 2026-05-28 Globex/Platform session — scribe dispatched 5×, blocked 3× while citing the phantom env var.
- `claude/hooks/watchdog.sh` — appended a supplementary "FIX:" line to the block message naming the actual working escape paths (`_scratch/` for drafts, `brain set --section=` for sections, `brain import` for new records). The previous shim relied solely on `brain guard`'s misleading "→ agent-scribe" route.
- `claude/hooks/write-location-warn.sh` — extended whitelist to cover standard repo-root files (`CHANGELOG.md`, `LICENSE`, `LICENSE.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`). Evidence: the hook fired against this very `CHANGELOG.md` write during 2026-05-28 repo init — the rule was treating a standard repo file as a generated artifact.
- `claude/CLAUDE.md` — added rules synthesized from 2026-05-28 transcript mining: forensic/CSI evidence-citation discipline · sidequest spinoff via `_scratch/` + fresh `/interview` · no-guessing-hook-behavior · inventory-scripts-before-dispatch · don't-over-parallelize-from-single-go.

## 1.2.1 — 2026-05-26

Pre-repo. See `docs/WOSY.md` §13 for portability patch details.
