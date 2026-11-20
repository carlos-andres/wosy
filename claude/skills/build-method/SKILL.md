---
name: build-method
description: The orchestrator protocol of a main session that builds plan nodes through subagents. Use when you orchestrate a plan node, ask the owner's go, write a dispatch brief, verify an agent's report, wire a hook, run a live acceptance call, write a ledger event, report to the owner, or close a phase and continue in the same session.
---

# Build method

The owner's build method. This skill is the one home of the protocol text.

`<task>` is the bound task folder, `<node>` a plan node id, `S<n>` the phase. `[hook <id>: deny]` or `[hook <id>: warn]` marks a rule a hook also checks.

## 1. The role

- The main session is the advisor and the orchestrator. It decides, briefs, verifies and reports. It does not build.
- It owns the design calls, the brief of each dispatch, the re-run of each proof, the wiring of a hook, the live acceptance call, the ledger, the handoff, and every report to the owner.
- A subagent owns reading many files, writing code, auditing a finished change and reading external docs; the main session may write a proof under `<task>/proofs/`. [hook P12: warn]
- The owner owns the go for each node and each wiring, and every decision the handoff lists as open.
- The owner owns every command that deletes: put the path in the dry-run cleanup script and never run the deletion yourself. [hook P13: deny]

## 2. What enters the main context, and what does not

- In: the handoff, the plan nodes of the phase, the requirement lines a node names, each agent's report, the last line of each proof, and the few files the main session edits itself (the trial settings, a wiring file, a cleanup script).
- Load the minimum that fully defines the work. Minimal does not mean short.
- Out: the source of a hook, of protocolo and of the guard; a full table of hundreds of rows; a transcript; the output of a long proof. Ask the agent for counts and the verdict line, and read a sample. [hook P18: warn]
- Exception: a fix under 10 lines whose cause is already diagnosed. Read only those lines, fix it, and record it in `<task>/research/dispatch-log.tsv` as a `main` line with result `redone`.
- Never poll a running agent and never read its output file. Its report arrives as a hand-back.
- An agent that waits on its own background work has not reported yet.

## 3. One node, step by step

1. Ask the owner the go for the node in one question. Record his words as a `decision` event before you act, and do nothing of the node before them.
2. Work one plan node at a time, and ask the same go before each wiring.
3. Pick the classifier row for each piece of work from the home's `hooks/classifier.tsv`: scout haiku low, extractor sonnet low, archaeologist and web-researcher sonnet medium, general-purpose opus high, builder opus high, qa-gate opus high.
4. Pass `model` and `effort` on a `general-purpose`, a `builder` and a `qa-gate`; pass `effort` on the others. [hook P25: deny, secret_guard.py]
5. Measure before you design when a fact is cheap to get: a scout for a path, a web-researcher for a hook payload, a one-line command for a baseline. Do not let a builder invent an interface.
6. Scan `.devwork/bin/` and `.devwork/bin/protocol/` (under `/Users/you/`) before you provision new tooling.
7. Write the brief (section 4) and dispatch. Tell the owner in a short report what was asked and what happens next.
8. When the report arrives, re-run the proof and its control yourself, check the isolation find, and read a sample of the output. Never trust the report. [hook P28: warn]
9. Mark each claim `[V]` only if you ran it; an agent's claim is `[I]`.
10. For a live call, check the result on disk and in the transcript as well, not only a sample of output (recipe at the end).
11. A hook that can block gets one `qa-gate`, then one fix round, then your re-run, before wiring.
12. A security hook can need two `qa-gate` rounds and two fix rounds; plan for them.
13. An additive or message-only change with a "decides the same" control needs no `qa-gate`.
14. Wire last: copy the settings to `<task>/backup/<node>/`, edit with the Edit tool, then one live acceptance call with a control arm. [hook P30: deny]
15. Record one ledger event for the node, one line in `<task>/research/dispatch-log.tsv` per dispatch, and every sandbox path in the dry-run cleanup script.
16. Report to the owner and stop. One question at the end, or one AskUserQuestion round of up to 4 when several decisions are open, with the recommended option first.

- Run dependent phases FIFO: one at a time, and dependent nodes the same way: re-verify each return with your own reads, report, and wait for the owner's go before the next.
- A FAIL stops the chain. Independent work may run in parallel.

## 4. The brief

A subagent knows only its brief. Each one holds, in this order:

- The classifier row and the plan node, on the first line. [hook P38: deny]
- The goal in two sentences, and the order: proof first, see it red, build, see it green.
- "Read first": absolute paths with line ranges. Point at files; do not paste them.
- The decided design, including every ruling of the owner that binds it, with its ledger number.
- The proof: what it must show, each control that must come out red, the regression to run and how long it takes.
- Traps the agent will hit: the guards that judge its own Bash commands, the live hooks, the BSD tools, the empty `rg`.
  - A command that prints a key-material file (`.netrc`, `id_rsa`, `.ssh/`) or carries a password attached to a database client's `-p` is denied: read the file with the Read tool, which arrives redacted.
  - The push verb, in any form, is denied: the owner pushes.
  - An assignment that looks like a secret (a `MYSQL_PWD` or `PGPASSWORD` value in the command) or an echo of a secret-named variable is denied: keep the value in `.envrc` and print `set: yes`.
  - An environment dump (`env`, `printenv`, `export -p`, a bare `set`) is denied: test one name with `[ -n "$NAME" ] && echo set: yes`.
  - Inline interpreter code (`python3 -c`, `node -e`) that can print a file is noted and wrapped: put the code in a script file, or use the Read tool.
  - A content search that can print a credential line is denied or wrapped: `rg -l`, `rg -c` or `rg -o --replace`.
  - A deleting command (`rm`, `rmdir`, `unlink`) in a bound session is denied: put the path in the dry-run cleanup script.
  - In a brief for a bound session, put the gate's phase write list at the top (R02: what the current phase may write).
- Limits: the exact files it may write, and the list it must not touch. "Files only, never wire."
- The stop rule: when a limit, a guard or a safety control blocks a correct build, stop and report the exact lines. Do not route around it, and do not change a verdict or a threshold to pass.
- The report shape and a word cap: files with line counts, the proof line before and after verbatim, each control, every choice where the spec is silent marked as open for the owner, gaps, denials, scratch left.

Around the brief:

- Cost follows turns and accumulated context, not output. Give an extractor compact inputs that it reads in one pass.
- Two builders never write the same files.
- Readers and extractors run in parallel, one output file each.

## 5. Verification rules

- A proof exists before the thing it proves, and is seen red.
- A proof with no control that fails is a baseline, not a test.
- The control is in the same command as the check: a no-op hook, the unchanged copy, a mutant of the artifact, the old version.
- A step with several checks ends in one task script under `<task>/proofs/` that exits 0 only when every check passes. Run it before the change as a control that must fail, and after it, with 0 fails.
- A fixture cannot see timing, a missing settings key or a wrong assumption. Only a live call sees them. Never skip it for a hook.
- A live deny test never aims at a real sensitive file. Use a path with the protected name that does not exist. [hook P55: deny]
- When a check overturns an earlier claim, write a `correction` event. Do not edit the past.
- When an agent reports FAIL or stops, the failure goes to the owner as a failure, with the options and one recommendation.
- When a safety control stops an agent, do not send the same work through another agent. Change the design so the work is not needed, or hand the decision to the owner.
- A wrong belief in a handoff spreads to every brief. Verify a trap the first time it matters.

## 6. Reporting to the owner

- The owner's language in chat, English in every file.
- First sentence: the state, with the verdict in bold.
- Then "measured by me" `[V]`, "reported by the agent" `[I]`, "not executed" `[X]`, each as a short list.
- Give the cost of every node in subagent tokens against the plan's estimate.
- List what is open for the owner. Never decide an open item silently; never bury it in prose.
- One recommendation, not a menu. Then one question, and stop.

## 7. The ledger

Write each event with `/Users/you/.devwork/bin/protocolo --event <task>/logbook.jsonl <type> "<text>" --pointer <path>`, never `--close`.

- `verification`: a node or a step, with the proof line verbatim, the control, what was not executed, what stays open.
- `decision`: the owner's words or his AskUserQuestion answers, with the date. Record it when he gives it, before acting on it.
- `correction`: an earlier claim that a later check overturned.

## 8. Closing a phase and continuing

1. A phase closes with the isolation find and its control: run the find against the phase stamp `<task>/backup/S<n>.stamp`, then `touch` a scratch file and see the same find report it. [hook P79: deny]
2. Add the `main` line to `<task>/research/dispatch-log.tsv`: model, peak context, dispatch count, subagent tokens. [hook P80: deny]
3. Copy `<task>/next-session-prompt.md` to `<task>/backup/next-session-prompt.md.<date>-<phase>`, write the new one, and pass `/Users/you/.devwork/bin/doc-gate`. [hook P81: deny]
4. Load-probe the handoff — a fresh read-only `claude -p` must load the new next-session prompt with no blocking gap, and its `LOAD-PROBE GO` verification event goes in the ledger before the owner gets the line. [hook S43: deny]
5. Record the close event, a `verification` whose text starts `S<n> close`, and give the owner the cleanup line.
6. To continue in the same session: follow the first_action of the new handoff (verify, a new stamp, the reads it names), then ask the go for the first node.
7. Advise a clean cut when the next node is an acceptance or an install, or when the context passes about 45 % of the window. [hook P84: warn]
8. The evidence of a high-stakes node belongs in a clean context, and every turn re-reads what is loaded.

## How to run a live acceptance call

- Build a sandbox task under `<task>/research/fixtures/<node>-live/`, never a real task.
- Take a session id with `uuidgen`, lower case, and bind it in a trial home, a separate Claude config directory that carries the settings under test: `CLAUDE_CONFIG_DIR=<a trial home> /Users/you/.devwork/bin/protocolo --bind <sandbox> --session <id>`.
- A sandbox task that will record QA needs a ledger with one event that names a project.
- First call: `CLAUDE_CONFIG_DIR=<a trial home> claude -p "<prompt>" --session-id <id> --strict-mcp-config --model haiku --output-format json --debug=hooks --debug-file <task>/research/debug/<node>.log`.
- Next calls of the same session: `--resume <id>`.
- In print mode a script does not run. A test command needs `--allowedTools "Bash(python3 -B -m unittest *)"` on the call.
- The prompt asks for one tool call per expected decision, "each once, never retry".
- Check the result on disk and in the transcript under `<a trial home>/projects/`, not only in the model's answer.
- Print line types and markers, never turn text.
