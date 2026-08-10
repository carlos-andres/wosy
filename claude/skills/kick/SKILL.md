---
name: kick
description: Audited prompt builder — the first insumo of the wosy flow. Runs a two-round adaptive interview (Round 1: ROLE + SCOPE as open free-text the user writes; Round 2: only the discrete follow-ups the analyzed scope needs, via AskUserQuestion), assembles a professional Claude prompt using current 2026 format best practices (`#` headers delimit the sections; XML tags reserved for specific in-section payloads like a code snippet or example — bare `<code>` for code; large payloads hoisted high; positive phrasing; WHY on constraints; explicit output format; no prefill), tailors hard-rules/output/verification per task-family, self-audits against a format+content checklist, then returns it ready to copy or run in-session. No storage — it is the ephemeral session opener. Loads defaults.md (baked from the user's own history) to pre-fill sensible defaults.
when_to_use: User types `/kick`, or says "help me write a prompt", "build me a prompt", "I need a prompt for X", "write a prompt to …", "audit this prompt". Manual only — the session opener that produces the prompt the user will then use or paste.
disable-model-invocation: true
---

# Kick — audited prompt builder (ROLE → SCOPE → smart follow-ups → audit → deliver)

Turn a rough idea into a professional, Claude-native, self-audited prompt. Two short interview rounds, one template, one audit, one deliverable. This is the **first insumo** — the prompt the user then runs in-session or pastes into a fresh session / feeds to `/interview`, `/intake`, an SRS or PRD. **Never stored** by default; it is ephemeral by design.

## When this fires

`/kick`, or "help me write a prompt", "build/write a prompt for X", "audit this prompt". Skip the interview only if the user pastes a finished prompt and asks *only* to audit it — then jump straight to **Self-audit**.

## Step 0 — Load defaults (silent)

Read the companion `defaults.md` before asking anything. It carries the user's standing defaults (English artifacts, confirm-before-mutation, tool prefs, wosy/`.devwork` awareness) baked from their history. Use it to pre-fill option defaults and the `# HARD RULES` block. Optionally glance at the current project's recent kickstarts (`~/.claude/history.jsonl`, last few rows for this `project`) to bias defaults — light touch, never a full re-scan.

## Step 1 — Round 1 (fixed · TWO open-text questions · NOT a picker)

Ask both as free text — the user writes them. Do **not** offer fixed options: real tasks are unpredictable ("build a skill", "migrate PHP 7.3→7.4", "analyze my sessions") and won't fit a menu.

- **Q1 · ROLE** — always `Senior`; the user writes the specialization. Seed the field `Senior expert <…> developer`. If they say only "senior", infer the specialization from the task (e.g. `Senior expert PHP developer`) and show it for confirmation.
- **Q2 · SCOPE** — the task in the user's own words, verbatim. If the kickstart already contains the task, extract it, echo it back for confirmation, and don't re-ask.

Then **analyze** the free-text SCOPE to classify it into a task-family (Step 2 table) — internal only, to pick the Round-2 follow-ups and the Step 3 tailoring. The families are a non-exhaustive guide; if the task fits none, infer from first principles.

## Step 2 — Round 2 (derived · smart · KISS)

Now `AskUserQuestion` earns its place: from the **analyzed free-text SCOPE**, ask only the discrete follow-ups this task-family needs, and **skip any the kickstart already answered**. Max one call, ≤4 questions. Bank (families are a guide, not a fixed list):

| task-family (inferred from SCOPE) | derived follow-ups (pick the relevant subset) |
|---|---|
| Implement | language · perf / input-size constraint · verification (run? tests?) · pattern to follow (`@file`) · output shape |
| Fix a bug | symptom · suspected location · what "fixed" means · failing-test-first? · repro data |
| Plan / spec | depth · output format (YAML / MD / HTML) · two-step gate? · what's out of scope |
| Analyze / audit | data source · evidence-citation required? · output format · read-only? |
| Data / query | which table / columns · read-only? · output = fields or rows · schema source |
| Ops / prod | target env · mutation vs read-only · who runs the push / deploy · rollback needed? |
| Quick one-shot | references only · output shape |
| Refactor / simplify | target files · what to preserve · behavior-change allowed? · verification |
| meta / tooling / other | infer from first principles: inputs · constraints · output artifact · verification · what's out of scope |

**Always resolve (from defaults or by asking):**
- **Assumption-handling** — `assume-and-continue` (well-scoped tasks; the DSA/algorithm default) · `stop-and-confirm-scope` (exploratory / the user's stated hypothesis is not final) · `report-first` (explore + report findings before proposing or acting). Default per `defaults.md`: *contrast the user's proposal, don't treat examples as ground truth.*
- **References** — `@files` / links / pasted code or data.
- **Language** — default: English artifacts (conversation may be Spanish).

## Step 3 — Assemble (the template)

Fill the skeleton below. **Rules that make it professional:**
- **`#` headers delimit the sections — the header IS the delimiter.** Do not re-wrap a whole section in an XML tag (no `<task>` under `# TASK`, no `<output_format>` under `# OUTPUT`). That is redundant double-fencing.
- **Reserve XML tags for a specific payload *inside* a section** that needs precise delimiting: a code snippet, a data block, one exact line, or an example. Everything else is plain text under its header.
- **Code goes in bare `<code>…</code>` tags** (user's chosen convention).
- **Hoist large reference payloads high** (just under `# CONTEXT`) — Anthropic measures up to ~30% quality lift on long context with data-first / query-last ordering.
- **Positive phrasing** — say "do X", not "don't do Y". Add the **WHY** after a constraint when non-obvious (Claude generalizes from the rationale).
- **Drop any optional block that would be empty** (KISS). Coding-only rules appear only for coding scope.
- Add an `<example>` block **only** when the desired format is hard to state in words. **No prefill** (deprecated since Claude 4.6).

```
# ROLE
Senior expert <…> developer.

# CONTEXT            (drop this block entirely if there is no background)
Stack / background / situational constraints.

# TASK
The exact problem, verbatim — never summarized or paraphrased.
Wrap only a specific payload in a tag when needed: <code>…</code> for a snippet,
<example>…</example> for a sample.

# REFERENCES         (drop if none; large code/data payloads are hoisted just under CONTEXT)
<code>
existing code / API contract / data to ground on
</code>

# HARD RULES
- Verify before claiming success (family form below); do not assert it works unverified.
- Assumptions: <state assumptions and continue> | <STOP and confirm scope first> | <report findings first>.
- KISS — idiomatic, no over-engineering (WHY: keeps the diff reviewable).
- <family-specific rule from the tailoring table below.>
- <baked from defaults.md as relevant: English artifacts · confirm before mutations · fd/rg·uv·pnpm.>

# OUTPUT
<the family's output shape — see tailoring table.>
```

### Per-family tailoring (beyond the always-on rules)

Inject the family's characteristic rule + output + verification form. This is what makes a non-coding prompt as sharp as a coding one:

| family | add to `# HARD RULES` | `# OUTPUT` shape | verification form |
|---|---|---|---|
| Implement | optimal time/space; idiomatic | code + edge cases + trace | run / tests |
| Fix a bug | fix root cause not symptom; failing test first | failing test → fix → green | reproduce, then show green |
| Plan / spec | out-of-scope explicit; gate if large | named artifact (MD/YAML/HTML) + coverage | cross-check each claim to a source |
| Analyze / audit | evidence per claim (file:line / row / sha); read-only | named report format (often HTML/MD, English) | cite an artifact for every finding |
| Data / query | read the schema first; never invent columns; read-only | the fields/rows asked for, nothing more | run read-only; show query + result |
| Ops / prod | read-only default; confirm each mutation; user runs the push/deploy | exact commands + rollback path | dry-run / echo commands before running |
| Refactor / simplify | preserve behavior; no drive-by changes | diff + what was preserved | tests green before & after |

## Step 4 — Self-audit (before returning)

Score the assembled prompt. Fix every WARN in place, then show the table + a one-line note on each fix.

| # | check | passes when |
|---|---|---|
| 1 | Verification hook | the prompt requires the family's verification form — run/tests (code), evidence-citation (analyze), read-only query (data), dry-run + confirm (ops) — and forbids claiming success unverified |
| 2 | Specificity | subject verbatim; files named with `@`; no "make it work" vagueness |
| 3 | Scope + done_when | single objective + a falsifiable done condition; out-of-scope stated if relevant |
| 4 | Assumption clause | explicit — assume-and-continue OR stop-and-confirm OR report-first |
| 5 | Output format | concrete deliverable shape under `# OUTPUT`, matching the family |
| 6 | References grounded | sources in-prompt, or a pointer telling Claude how to fetch them |
| 7 | KISS / clean | no over-engineering; generated **code/comments** carry no ticket IDs (those go in commit messages) — referencing a ticket as task *context* is fine; comments explain WHY not WHAT |
| 8 | Format pro | `#` headers delimit sections (no redundant section-wrapping tags); XML tags only for a specific payload (code / data / line / example); bare `<code>`; large payloads high; positive phrasing; WHY on constraints; example only if needed; no prefill |

## Step 5 — Deliver

1. Print the finished prompt in **one fenced block** the user can copy verbatim.
2. Print the audit table (all PASS after fixes) + fixes applied.
3. Offer: **"Run this now in this session, or is it yours to copy?"** If run → execute the prompt as the next task. No file is written.
