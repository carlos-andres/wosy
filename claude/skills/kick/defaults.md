# kick defaults

Standing defaults baked from the user's own Claude Code history (493 sessions). `kick` reads this to pre-fill option defaults and the `# HARD RULES` block, so the interview asks less and the generated prompt already reflects how this user works. These are defaults, not locks — any can be overridden per prompt.

## Language & output
- **Artifacts in English; conversation may be Spanish.** Every generated prompt, doc, and code comment is English.
- Output format is never assumed — always an explicit choice (prose-English / MD / YAML / HTML). The user has been burned by format whiplash.

## Assumptions & scope (the #1 recurring pain)
- **Do not treat the user's proposed solution or examples as ground truth.** They give a *hypothesis*; contrast it, then confirm. ("NO ASUMAS que mi propuesta es definitiva" / "no asumas en mis ejemplos".)
- **Confirm scope before doing anything non-trivial.** ("BEFORE DO ANYTHING ASK AND CONFIRM THE SCOPE.")
- Default assumption-handling: `stop-and-confirm-scope` for exploratory / hypothesis-shaped tasks; `assume-and-continue` only for tightly-scoped, self-contained tasks (e.g. a DSA/algorithm problem with a clear spec).

## Verification & evidence
- **Verify before claiming success** — run, trace, or test; never assert unverified. (Mirrors the user's own template rule.)
- On diagnostics: no claim without evidence (`file:line` / row / log line / commit sha).

## Simplicity
- **KISS.** Senior-engineer simplicity check: if 200 lines could be 50, rewrite. No abstraction for a single caller. No config knob nobody asked for.

## Safety
- **Read-only by default** for prod-touching work (ssh / DB / artisan). Any mutation (DB write, git push, delete) is confirmed in chat first — even under "just go".

## Tooling preferences
- `fd` over `find` · `rg` over `grep` · `jq` for JSON · `uv` over `pip` · `pnpm` over `npm`/`yarn` · `gh` for GitHub · `eza` over `ls`.
- PHP is per-project (`php -v` before assuming a version). `.envrc`/`.env` hold creds — reference, don't duplicate.

## Workflow awareness
- If cwd or a parent has `.devwork/`, this is a **wosy** project: HQ-orchestrator default (dispatch subagents for >50-line reads / multi-file work), files-canonical task folders, `/context` checks before stop/continue.
- Comments in code: explain **WHY**, not WHAT. No ticket IDs / issue keys / PR numbers in the prompt body or code — those belong in commit messages.
