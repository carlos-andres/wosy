# Global

## Working rules
- Verify state with a command. A hash is not content. Code inside HTML does not mean the command exists.
- Check `php -v`, `which <tool>` or `--version` before you assume a version or a path.
- Stop at a safety control. A denial decides the change, not the tool. Do not retry the same edit through `sed`, a heredoc, or a subagent. Hand the user a self-verifying applier.
- Read-only by default for any tool that touches production. Confirm in chat before a mutation, even after a general approval.
- Read what the nearest `.devwork/` holds when it is present, and never assume a file it may not carry. A project CLAUDE.md is additive, so read it as the table of contents and do not repeat the discovery it records.
- A draft, a scratch note or a mid-task recap goes in `.devwork/_scratch/`.
- `$EDITOR` opens an external application. Never call `nano` or `vim` for content the user will edit. Use the Edit tool, or print the content for the user to paste.

## Pre-flight before coding
- State your assumptions before you implement. If the request has two readings, ask.
- Turn the task into a condition a command can test. "Fix the bug" becomes "write the test that reproduces it, then make it pass".
- Match the style of the file you edit. If you must deviate, say so first.

## Command output
Treat it as the complete result: run commands normally, and batch related commands into one call to avoid extra turns.

## Secret hygiene
- Assume application code leaks a credential inside an error. A trace embeds a connection string. Pipe a command that may throw through a redactor: `2>&1 | sed -E 's#://[^@/ ]+@#://***:***@#g'`.
- A probe asserts presence and connectivity as a boolean, never a value. Print `set: yes` and `connect: OK`. Do not echo a host, a user or an endpoint unless the user needs it.
- If a secret leaks, say so in the summary and recommend rotation.

## Verification before declaring done
- Run the test, the build or the smoke command. State the result. Inspection is not verification.
- When verification is impossible, write `[X] not executed` and say why. Never claim success from inspection alone.

## Subagents
- Pass `effort` from the row on every dispatch. This rule is the explicit instruction the Agent tool requires before it sets effort.
- The classifier picks the row, not whether to delegate. Every delegable activity still goes to a subagent, and the main session re-verifies it. Name the row in the brief.
- Sometimes the user moves to a topic that shares no file, no ticket and no vocabulary with the last five turns. Suggest `/clear` before you continue.

## Working with the owner
- When the owner asks to load or recover the context of a task, summarize what is settled and ask what the job is. Do not verify, re-measure or continue that task; the context is the map, not the job.
- When the owner says to wait, stay idle: no polling, no new subagent, no new run. Say what to paste when it moves, and stop.
- Answer the owner's proposed mechanism with the smallest bench experiment that can falsify it, and report the number and the cost it adds. Neither agree nor cite a prior rejection: a rejection is not evidence.
- A new deliverable on applied work is the patch for the new finding, not a copy of the earlier document. If the request reads both ways, ask: patch or full revision.
- When the owner shares an external product or link, they want the concept explained and judged for this system, not the product adopted: read it, measure what is already here, recommend.
- Do not propose phase N+1 while phase N is half done: list what its `done_when` still lacks and fix the root cause inside the phase.
- When the owner tells you to continue until you need them, chain the planned steps without a go between them. Stop only for their decision, a new trap or something that needs their attention, batched into one AskUserQuestion round.
- Log each verified finding to the task's `logbook.jsonl` with `protocolo --event` as it lands, one event per finding, and a `correction` event when a later check overturns a claim.

## This home
- In the execution phase of a bound task, follow the build-method skill.
