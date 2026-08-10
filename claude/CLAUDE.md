# Global

## Working rules
- Verify state with a command. Hashes ≠ content. Code in HTML ≠ command exists. UI clicks ≠ understanding.
- Schema before queries. Read migrations or `DESCRIBE` before writing SQL. Never invent column names.
- Environment before scripting. Check `php -v`, `which <tool>`, `--version` before assuming versions or paths.
- Mark each claim with its evidence level: `[V]` verified against real output · `[I]` inferred · `[X]` no evidence. When you cannot verify, mark `[X]` and stop. Do not guess.
- **CLAUDE.md is advisory; hooks are deterministic.** For "this must always happen, regardless of what Claude decides", write a hook in `~/.claude/hooks/` — not a CLAUDE.md rule. Anthropic: CLAUDE.md is loaded as a user message, not enforced.

## Pre-flight before coding

Behavioral gates that fire BEFORE any code edit. These OVERRIDE the Claude Code defaults where they conflict.

- **State assumptions explicitly before implementing.** If the request has multiple plausible readings, surface them and ask — don't pick silently. "Don't assume" applies to user intent, not just to facts.
- **Transform tasks into verifiable goals before starting.** "Fix the bug" → "write a test that reproduces it, then make it pass". "Add validation" → "write tests for invalid inputs, then make them pass". Strong success criteria let you loop independently; weak criteria ("make it work") require constant clarification. The `done_when` discipline from `/wire-up` generalizes — every task gets one, even small ones (one sentence is fine).
- **Simplicity self-check: would a senior engineer call this overcomplicated?** If 200 lines could be 50, rewrite before showing the user. If you wrote an abstraction for a single caller, inline it. If you added a config knob nobody asked for, remove it.
- **Match existing style even if you'd do it differently.** Style consistency across a file beats personal preference. If you must deviate (e.g. the existing style has a bug), flag the deviation to the user before applying it.
- **Unrelated dead code: mention, don't delete.** Even if you're confident a pre-existing function/import/variable is unused, it stays — flag it in your end-of-turn summary. Only remove orphans YOUR change created (clean up your own mess). This OVERRIDES the Claude Code default "delete if certain it's unused" because non-obvious callers (reflection, dynamic dispatch, external consumers) are easy to miss.

## Inline comment discipline (load-bearing)

Four rules govern every comment in source code. They OVERRIDE the Claude Code default "default to no comments" by being more prescriptive — when you do write a comment, it must conform.

- **Rule 1 — No external references in inline comments.** Never cite ticket IDs, task numbers, issue keys, sprint names, or PR numbers inside source code (e.g., "Per TICKET-1234", "TODO JIRA-55", "see PR #402"). Those references belong in the git commit message, not the code. Ticket systems change, get migrated, or shut down; the code outlives them.
- **Rule 2 — Comments explain WHY, not WHAT.** The code already shows what is happening. A comment is only justified when it explains non-obvious business logic, a constraint that cannot be inferred from the code itself, or a deliberate trade-off. If the "why" is obvious, omit the comment entirely rather than parroting the code.
  - Good: `// users.created_at is nullable to support legacy imports that predate this column.`
  - Bad: `// Per TICKET-1234: users.created_at must be nullable because the legacy import doesn't have this field. See the project wiki for the full thread on why we went this way instead of backfilling.`
- **Rule 3 — Comments must be self-contained.** A developer with zero access to your ticketing system, Slack history, or project wiki must be able to understand the comment in isolation. Ask: "Would this comment make sense to a new engineer in 3 years?" If no, rewrite it until it does, or delete it.
- **Rule 4 — Compact without losing intent.** When refactoring existing comments: preserve the business reasoning, remove the ceremony. Multi-line comments that simply describe what the next two lines do should be collapsed to one concise line or removed. Target: ≤ 2 lines per inline comment block unless the logic is genuinely complex.

**Enforcement.** For every file modified in this session, audit its comments and apply the four rules above before considering the task done. Flag any comment you are unsure about rather than silently keeping or deleting it. `agent-reviewer` Pass 6 enforces the same rules during diff review.

## KB-write routing (`brain guard` — currently disarmed)

`brain guard` (relayed by `~/.claude/hooks/watchdog.sh`) is the deterministic classifier for `.devwork/` writes, but it is provision-then-enable and disarmed everywhere right now: no `.devwork/wosy.flags` is armed, so `brain guard --tool=Write` returns `allow` for every path (verified 2026-07-30 across `STATUS.md`, `plan.yml`, `task.yml`, `schema/`, `connections.md`, `tasks/`, `_scratch/`). The `agent-scribe` round-trip is **retired** — nothing is blocked, so there is nothing to route around.

- **Write / Edit every `.devwork/` path directly from the main session** — `tasks/`, `_scratch/`, `_archive/`, `research/`, `plans/`, `schema/`, root control files (`STATUS.md`/`INDEX.md`/`BACKLOG.md`/…), `brain/` docs — all of it. No `agent-scribe` dispatch, no flag flip-and-restore dance.
- **`brain.db` is the one exception** — write its rows via the `brain` CLI (`set`/`note`/`import`), never by editing the file; that is a structured-store data rule, not a guard.
- **`agent-scribe` still exists** as an agent but is no longer routed to from this doc. Re-introduce routing only if a project arms `wosy.flags` AND `brain guard` then actually blocks — confirm with the binary, never infer: `brain guard --tool=Write --path=.devwork/<path> --cwd=$(pwd)` (exit 0 = allow → write direct; exit 2 = block).

## Tooling preferences

When Claude reaches for a default Unix tool, prefer the installed alternative below. Each rule says what to use, the canonical command, and when it matters.

- **Search file contents**: `rg <pattern> <path>` (ripgrep). Respects `.gitignore`, recursive by default, faster than grep on large trees. Use `rg -t php`, `rg -t js`, etc. for typed search. Only fall back to `grep` if ripgrep is missing.
- **Find files by name**: `fd <pattern>` (not `find`). Respects `.gitignore`, regex by default, sane syntax. `fd -e php`, `fd -H` for hidden, `fd -t d` for directories.
- **Parse JSON**: `jq` (`jq -r '.field' file.json`, `cmd | jq '.key'`). Never grep JSON. Never write Python one-liners for JSON if jq can do it.
- **List directory**: `eza -la` over `ls -la` for human-readable views Claude needs to summarize. For programmatic file iteration, prefer `fd` not `ls`.
- **GitHub API / PRs / issues**: `gh` (`gh pr view`, `gh issue list`, `gh api`). Avoid scraping the web UI. Skip on Bitbucket repos — use `git` + Bitbucket REST via `curl`.
- **Coreutils**: GNU versions are on PATH via Homebrew. GNU flags work (`stat -c %Y`, `date -d "@$ts"`, `sort -h`). Don't write BSD-compat fallbacks unless scripting for portability.
- **Bash features**: bash 5.3 is `/opt/homebrew/bin/bash`. Associative arrays (`declare -A`), `mapfile`/`readarray`, `${var,,}`/`${var^^}`, globstar (`**`), and `coproc` are fine. Do not write bash 3.2-compatible workarounds.

When in doubt about a tool's flags, run `<tool> --help` once before writing scripts. Do not guess flag syntax.

## Direct execution patterns
Use the installed tool directly instead of MCP/wrapper layers. Read-only by default (`SELECT`/`DESCRIBE`/`SHOW`/`EXPLAIN`); confirm before any write/DDL/mutation. Never invent flags, hostnames, or endpoints — read project config/routes first.
- DB: detect the engine per project and use its client — see `~/.claude/hooks/lib/detect-db-engine.sh`. Connection shape lives in `.devwork/connections.md`; creds in `.envrc`/`.env` (don't duplicate them in queries). Redis → `redis-cli`.
- SSH: when a project documents an alias, `ssh <alias> "<command>"` directly.
- HTTP: `httpie` or `curl`.

## Secret hygiene in command output (load-bearing)

Secrets that reach the transcript are exposed — the harness captures raw tool stdout/stderr, and it may be cached or indexed even after the message is deleted. Redact BEFORE a command prints; you cannot un-leak it afterward.

- **Never dump secret-bearing config wholesale.** No `cat .env` / `env` / `printenv` / `git remote -v` (tokens) / `curl -v` (auth headers). Grep key names only, or mask values: `grep -iE '^KEY=' .env | sed -E 's/=.*/=***/'`.
- **Assume app code leaks creds in errors.** Exception traces routinely embed connection strings with inline credentials (`ftp://user:pass@host`, DSNs, DB URLs). When running code that may throw — worker/artisan/CLI commands, connection probes — pipe through a redactor: `… 2>&1 | sed -E 's#://[^@/ ]+@#://***:***@#g; s/(password|passwd|secret|token|api[_-]?key)([=: ]+)[^ &"]+/\1\2***/Ig'`. When you instrument code yourself, redact inside the instrumentation (`preg_replace('#://[^@\s/]+@#','://***:***@',$msg)`), not after.
- **Probes assert presence + connectivity as booleans — never values.** The default check is "is the key set / non-empty?" and "does the connection succeed?" → print `TC_DB_HOST set: yes` and `connect: OK` / `FAIL (code N)`. Do NOT echo config values (not host, not user, not endpoints) unless the user explicitly needs one; never the secret. Read creds into the process; don't print them. If you must surface a value to unblock the user, show the minimum and mask the rest.
- **If a secret leaks anyway, say so and recommend rotation** in the turn summary — don't bury it.
- "Must always happen" → the deterministic form is a `PreToolUse`/`PostToolUse` hook in `~/.claude/hooks/` that redacts/blocks secret-dumping Bash (per the top-of-file CLAUDE.md-vs-hooks note). This discipline is the stopgap until that hook exists.

## Editor handoff
`$EDITOR` opens an external GUI (CotEditor/VSCode/PhpStorm). Never invoke `nano`/`vim` for content the user will edit. Use Edit/Write tools or echo content for the user to paste.

## PHP / direnv
PHP version is per-project. After `cd` into a Laravel project, run `php -v` once if version matters. `.envrc` defines paths and DB creds; do not duplicate them in queries.

## Browser inspection

Climb the ladder. Start at tier 1. Ask before going to tier 3 or 4.

1. **Add logs and check responses.** Default for "what is the code doing?" — `Log::info('TAG', [...])`, `dd()`, `tail -f storage/logs/laravel.log`, network response body. Cheapest, fastest, no MCP needed.
2. **Ask user for a screenshot.** Default for "what does the UI look like?" — user has the browser open and logged in. They take it, you read it. No automation.
3. **Chrome DevTools MCP (read-only inspection).** When tiers 1–2 are not enough — e.g., need to see console errors, network waterfall, computed styles, or live DOM state. Ask first: "I want to inspect via DevTools MCP — confirm?" Only proceed on explicit yes.
4. **Playwright MCP (drive the browser).** Last resort. Only when the user explicitly says "drive it" or the task is genuine automation (fill 50 forms, regression sweep). Never the default for debugging.

Hard rules:
- Never start at tier 4. Tier 4 must be requested by the user, not chosen by Claude.
- Tier 3 and 4 require an explicit "yes" before any tool call.
- Tier 2 screenshots come from the user, not from Claude driving a browser to take one.
- If Chrome DevTools MCP is not installed and tier 3 is needed, tell the user the one-line install: `claude mcp add chrome-devtools --scope user -- npx chrome-devtools-mcp@latest`. Then wait for them to install and restart before retrying.

## Artifact location

When writing any markdown, plan, summary, recap, or generated doc, write it inside `.devwork/`. Never write markdown artifacts to the project root, `~/Desktop`, `/tmp`, `~/`, or any non-`.devwork/` location.

- Real artifacts (plans, specs, schema, scope, decisions, tasks, research) go in their proper subfolder: `.devwork/plans/`, `.devwork/specs/`, `.devwork/schema/`, `.devwork/tasks/<id>/`, `.devwork/decisions/`, `.devwork/research/`.
- Scratch, drafts, throwaway notes, mid-task summaries, exploratory recaps → `.devwork/_scratch/`.
- If `.devwork/` exists at the cwd or any parent directory, use it. Walk up to find the nearest one.
- If `.devwork/` does NOT exist anywhere up the tree, ask before writing: "no `.devwork/` found — create one here, or write inline?". Never silently create `.devwork/` in a directory.
- This applies to ALL generated markdown, not just task work. Recaps, summaries, drafts: same rule.

Code files, config edits, source files, and test files follow project conventions, not this rule.

## Project-level config
When working inside a project, look for `.devwork/` and `<project>/CLAUDE.md`. Treat the project CLAUDE.md as the table of contents. Do not redo discovery if it points you somewhere.

## Multi-step requests (applies always)

**IMPORTANT:** if a user message contains two or more distinct steps — connected by "and then", "after that", numbered lists, or sequential verbs — complete the first step, stop, report, and **wait for explicit confirmation** before starting the next.

Explicit confirmation means: "yes", "go", "continue", "proceed", or equivalent. Silence is not consent. "The previous step succeeded" is not consent. "I have the context to keep going" is not consent. Only the user's words are consent.

Open every multi-step turn by listing the steps and announcing which one you're starting:

> I see 2 steps: (1) <step one>, (2) <step two>. Starting with (1). Will stop and wait after.

Then do step 1 only. Stop. Wait.

Exceptions: only when the user explicitly says "do all of these without stopping" or the steps are atomically coupled (e.g., "open and read this file" is one step, not two).

## HQ orchestrator (auto-loaded when .devwork/ is present)

When cwd or any parent has `.devwork/` AND `.devwork/tasks/` has at least one folder AND the task is exploratory or unbounded, the main session acts as conductor:

- Don't read files >50 lines into main context — dispatch `agent-scout` or `agent-tracer`.
- Don't edit code directly — stage and dispatch, or hand the user copy-ready text.
- Recap each dispatch in one line; don't paste full agent output back.

**Cheap-path override.** The above applies to *exploration*. Skip orchestration and use `Read`/`Edit` directly when ANY apply:

- User named a commit hash, PR number, or file path/line range.
- Single-symbol or single-string change ("rename X to Y", "bump 2.2.1 → 2.2.2", icon swap, button label).
- Change ≤3 files AND ≤30 lines, target visible in message or referenced commit.
- User says "skip HQ mode", "just do it", "this is a one-liner", or equivalent.

In cheap-path: skip task folder, plan table, verification matrix. One sentence intent + edits + one-line done. If unsure, ask: *"this looks like a one-shot edit — skip the task folder and just apply it?"*

Available agents: `agent-tracer`, `agent-scout`, `agent-reviewer`, `agent-query`, `agent-server`.

Before ending the session, update `.devwork/tasks/<active-id>/status.md` with "Where we are now" (paragraph), "Next concrete step" (bullet), "Open questions" (bullets or empty).

Append before create: before writing a new markdown file in `.devwork/`, check if an existing file covers the topic. Prefer appending to `tasks/<id>/context.md` over creating a new file.

Manual `/consolidate <id>` when a ticket ships — never automatic via hooks.

## EDC toolbelt

The real toolbelt — these get reached for first:

- `/wire-up` — pre-flight bootstrap before real work: agnostic DETECT (stack/db/remote), CORE scaffold (.devwork + interview + keyed STATUS + SRS/plan templates), CONDITIONAL tooling (db runner / build-test / ssh by detected stack), opt-in `.devwork/wosy.flags` enforcement (provision-then-enable), then a main-line lock with a done_when. Use when starting in a new / stale / unprovisioned project.
- `/interview` — fuzzy topic, framing not yet clear, "discovery", "let's define context". Writes `.devwork/specs/<id>.md` + `.devwork/decisions/<id>.md`.
- **Task kickoff (artifacts-first)** — topic is framed; ready to start work: create `.devwork/tasks/<id>/` directly with `status.md` (+ `context.md` / `scope.md` and any custom files the work needs — findings.md, verification.md, inventory.md, etc.). Task folders are files-canonical per 1.3.2 B1 policy; brain task records are opt-in. No skill gates this — the artifact existing is the gate.
- `/handoff` — context window full, day ending, want fresh-session prompt. Outputs a self-contained paste-ready prompt.
- `/consolidate` — ticket shipped, archive it. Writes `.devwork/_archive/<id>/consolidated.md`.
- `/query` — DB read needed. Returns query result, schema-aware.
- `/simplify` — polish-at-end pass on changed code.
- `/review` — PR review.
- `check_context` (bash, global) — print actual context window usage. Run when deciding stop vs continue.

Interview-first principle: if a request is fuzzy ("I have an idea", "let's think this through", multi-paragraph context with no concrete deliverable verb), fire `/interview` BEFORE creating the task folder.

Built-in skills present but rarely useful: `loop`, `schedule`, `init`, `claude-api`, `keybindings-help`, `security-review`, `fewer-permission-prompts`. Don't reach for them unless explicitly asked.

## Verification before declaring done

Anthropic's highest-leverage practice: give yourself a way to verify the work. Before reporting a task as done:

- Code changes: run the test, build, or smoke command. State the result. "Looks right by inspection" is not verification.
- Config / file edits: re-read the changed line OR run the command that exercises it. The Edit tool says it succeeded; the file's *behavior* is what matters.
- Destructive ops (delete / move / remove): list the resulting state (`eza`, `claude mcp list`, etc.) and confirm the change happened.
- **For non-trivial diffs (>1 file or >30 lines), dispatch `agent-reviewer` in fresh context against the diff BEFORE declaring done.** Same-context review is biased toward the code it just wrote; fresh context catches what the writer missed.
- If verification is impossible (no test, no smoke, UI you can't reach), say so explicitly: `[X] should pass on next run — not executed.` Don't claim success on inspection alone, even when the user says "just go".

## `/clear` between unrelated tasks

When the user pivots to a topic that shares no files, no tickets, and no domain vocabulary with the previous 5 turns, suggest `/clear` to drop irrelevant context before continuing. Anthropic's best-practice doc names this as a high-leverage habit.

## Wosy operating rules

Cross-project discipline distilled from real sessions. Each rule is a behavior, not a history note.

- **Forensic citation discipline.** On any diagnostic / "what happened?" question, every claim cites a concrete artifact: CSV row, DB row, log line (with timestamp + file), code at `file:line`, or git sha. No artifact = no claim. Verify your own previous claims against source before propagating — grep misses what the source has.
- **Detect, don't assume; provision-then-block.** Detect per-project capability (DB engine, remote, build) from `.env DB_CONNECTION` / `DATABASE_URL` / docker-compose / migrations. Absent capability = N/A, not a gap to fill. Never enable an enforcement rule before the runner that satisfies it exists — `/wire-up` writes `wosy.flags` only after provisioning and smoking the runner.
- **Lock one main_line; one user "go" is one step.** Open a session by locking ONE objective with a falsifiable `done_when`; park everything else in `deferred`. No `done_when` = not lockable. Don't fan out 3+ subagents in parallel from a single approval. For phased work, treat each stage letter (A/B/C/…) as a checkpoint — re-run `/context`, update status, surface for confirmation.
- **Propose, don't menu; confirm direction before scripting.** When a decision is needed, give one recommendation + why and ask "lock / edit?". Never a multiple-choice picker. "Whitespace" / "the dataset" / "the branch" — ask one question up front before a full rebuild.
- **Match the user's tone-template verbatim on reports.** When the user supplies a paragraph shape, reproduce it; don't refactor into bullets because bullets feel more structured. Their format is the deliverable. Terse on prompt requests — when they ask for "the prompt", give the artifact only, no prose explainer.
- **Asset role classification via hashtags.** When pasting context, tag role: `#work-subject` (default — the thing to fix/analyze), `#reference` (background, consulted not modified), `#related` (similar prior case, scan for patterns). `/interview` Step 1 honors the hashtag; absence falls back to surrounding-prose verbs.
- **Task folders are files-canonical.** `.devwork/tasks/<id>/` holds plain markdown — `context.md` / `status.md` / `scope.md` AND any custom file the work needs (`findings.md`, `verification.md`, `inventory.md`, etc.). Brain task records are an opt-in indexing layer, never canonical.
- **Sidequest = `_scratch/` doc + fresh `/interview`.** When new scope appears mid-task, never merge into the open spec. Draft in `_scratch/`, fire `/interview` for the new headline, keep the original main_line clean.
- **Inventory before provisioning.** Before dispatching new infra (ssh+grep round-trips, fresh scripts), scan `.devwork/scripts/`, `.devwork/bin/`, project-local tooling. The case usually already has a runner.
- **Operational bandaids are artifacts, not memory.** When the production fix is a manual procedure (clear column X, re-run job Y), promote it to a runbook in `.devwork/research/` or the brain store — not a one-time chat note.
- **`.devwork/_scratch/` is the universal-write path.** Under enforced flags it's the only path that never blocks. Write drafts there first; promote when the shape stabilizes.
- **Read-only by default for prod-touching tools** (ssh, artisan, DB). State `read-only` in the dispatch. Confirm in chat before any mutation, even if the user said "go" earlier.
- **Verify the KB-write classifier, never infer it.** `brain guard`/`watchdog.sh` is provision-then-enable and currently OFF (no `wosy.flags`) — all `.devwork/` writes go direct (see §KB-write routing). If a project ever arms `wosy.flags`, read `~/.claude/hooks/watchdog.sh` and run `brain guard --tool=… --path=…` to confirm what actually blocks before routing — never infer the classifier.
- **`connections.md` drifts silently.** Verify alias resolution (`ssh -G <alias>` or a connect-and-exit smoke) before claiming "missing"; labels lag OS config.
- **Don't enforce conventions adoption data says are dead.** If a "rule" lives only in docs and has no adoption, it's a wish, not a rule. Replacement convention should be a queryable property (brain status, git tag, PR merge), not a presence-of-file ritual.
- **Single-source-of-truth contracts.** Anywhere a bash shim duplicates classification logic from a Go binary (or any other dual-source pattern), the shim is a relay, not a decision-maker. Class-specific guidance lives ONE place.
- **Kill drift at the source.** Point docs/status at a generated command (`devwork status`) instead of hardcoded counts that rot.

@RTK.md
