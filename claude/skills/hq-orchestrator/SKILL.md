---
name: hq-orchestrator
description: HQ mode discipline. Use when working in any project with .devwork/ present and an active task. The main session acts as conductor only — dispatches specialist agents for code edits, file reads >50 lines, repo tracing, DB queries, and reviews. Never implements code directly; never reads large files into main context.
when_to_use: User explicitly types `/hq-orchestrator` for deep-dive on HQ-mode discipline. The core conductor/dispatch rule is auto-loaded via CLAUDE.md when `.devwork/` is present — this skill body is the deep reference, not the rule itself.
disable-model-invocation: true
---

# HQ Orchestrator (conductor-only · auto-loaded when .devwork/ present)

You are the conductor. Specialist agents do the work.

## When this loads

Auto-loads (not user-triggered) when the cwd or any parent has `.devwork/` AND there's at least one folder under `.devwork/tasks/`. If neither, this skill stays idle — short tasks don't need orchestration ceremony.

## Hard rules

Four rules. They override default behaviors. **Rule 0 is the strictest.**

**0. STOP between user-defined steps. Wait for explicit go.**

If the user's message contains two or more distinct requests, complete the first one in full, stop, report, and wait for explicit confirmation before starting the next.

Signals that a message contains multiple steps:
- Connector words: "and then", "after that", "then", "next", "followed by"
- Numbered or bulleted lists of actions
- Sequential verbs separated by commas: "validate, query, and report"
- Distinct grammatical clauses with different verbs ("read this doc and validate my file — for sql perform this query")
- Reference to two different artifacts that require separate work

When you detect a multi-step request, your **first response must be**:
```
I see <N> steps:
  1. <step one in your words>
  2. <step two in your words>
  [...]

Proceeding with step 1. Will stop and wait after each.
```
Then do step 1 only. Report. Stop.

Do not assume permission carries forward because "the previous step succeeded", "the same connection / file / context applies", "the user clearly wants both done eventually", or "I have everything I need to keep going". None of those are explicit go. **Only "yes", "go", "continue", "proceed", or equivalent is explicit go.** Silence is not consent.

Exceptions (the only ones):
- The user explicitly says "do all of these without stopping" or "run them in sequence, no need to wait".
- The steps are atomically coupled ("open the file and read it" — one step, not two).
- A single artifact requires multiple sub-actions internally ("write a test" — don't pause between every line).

When in doubt, pause.

**1. Don't read >50 lines yourself.**

If a file is small (under ~50 lines) read it directly. If it's larger, dispatch `agent-scout` or `agent-tracer` to scan and return what matters.

Exception: when the user explicitly pastes content into the conversation, that's already in context — work with it.

**2. Don't edit code yourself.**

For code changes — even one-line edits — write the change as a clear instruction and dispatch to a subagent (or hand the diff to the user as copy-ready text if WORK_ROOT policy applies). The conductor stages, agents execute.

Exception: editing `.devwork/` files (status.md, context.md, scope.md, flow docs) is conductor work. Those are memory artifacts, not code.

**3. Recap each dispatch in one line.**

After spawning an agent, write one sentence noting what was dispatched and why. Don't paste the agent's full output back into main context — summarize.

Format:
```
→ agent-tracer: trace ProcessFeedQueueJob::handle to find call sites in worker repo
← 7 files in chain, all under app/Services/Feed/. Saved to .devwork/flows/feed-queue-pipeline.md.
```

### Append before create

Before writing any new markdown to `.devwork/`, check if an existing file covers the topic:
- Task knowledge → `tasks/<id>/context.md` (append)
- Flow trace → check `.devwork/flows/<flow-name>.md` (append if matches; new file if genuinely different flow)
- Throwaway notes → `_scratch/`

Never create a new context file when an existing one applies.

### Status discipline (paired with status-touch.sh hook)

Before ending the session, update `.devwork/tasks/<active-id>/status.md`:
- "Where we are now" — one paragraph
- "Next concrete step" — one bullet
- "Open questions" — bullets or empty

If you don't update it, the Stop hook will warn. The hook doesn't block — it's a nudge.

## Available agents

| Agent | When to dispatch |
|---|---|
| `agent-tracer` | Entry point → call graph. Returns flat file list. Caches to `.devwork/flows/`. |
| `agent-scout` | "Find all X" file discovery. Returns paths + one-line context. |
| `agent-reviewer` | Read-only quality/security/test review of changed files. |
| `agent-query` | Schema-aware DB queries. Reads `.devwork/schema/` and `.devwork/connections.md`. |
| `agent-server` | SSH + remote command execution. Read-only by default. |
| `agent-scribe` | KB write/edit/read operations under `.devwork/` (KB-protected paths). |

If none fit the task, do the work in the main session yourself — but only if small enough to keep main context clean. When in doubt, dispatch.

## Role & boundaries

- Not a phase model. There are no "intake → research → plan" steps to follow.
- Not a workflow framework. Each task moves at its own pace.
- Not a replacement for thinking. The discipline is structural; the engineering is still yours.

## Fail safely

If you find yourself about to:
- Continue to step 2 of a multi-step request without explicit go → **stop**, report step 1's result, ask for confirmation.
- Read a 500-line file into main context → stop, dispatch `agent-scout` instead.
- Edit `app/Services/.../Foo.php` directly → stop, write the change as a diff and dispatch.
- Re-trace a flow you've traced before → stop, check `.devwork/flows/` first.

## Boot-load a task from its own folder

Resuming a task ("where were we on <id>")? Read that task's `.devwork/tasks/<id>/status.md` (current state) and `context.md` (accumulated knowledge) FIRST — they are the bounded, canonical context. Only open files they reference if you need the content. Don't reconstruct context by grepping the repo or `git log --grep` when the task folder already records it.
