# Global

## Working rules

- Verify state with a command. A hash is not content. Code inside HTML does not mean the command exists.
- A rule here that names no command is human judgment. No mechanism enforces it.
- Read the migrations or run `DESCRIBE` before you write SQL. Never invent a column name.
- Check `php -v`, `which <tool>` or `--version` before you assume a version or a path.
- This file is advisory. A pin in `settings.json` and a hook are both deterministic. If something must always happen, pin it or hook it in the resolved Claude home, never write a rule here.
- Stop at a safety control. A denial decides the change, not the tool. Do not retry the same edit through `sed`, a heredoc, or a subagent. Hand the user a self-verifying applier.
- Resolve the Claude home from `$CLAUDE_CONFIG_DIR`, or from the directory the transcript path sits in. **Never assume `~/.claude`.** A session can run against another home, and reading the wrong one reports a configuration that is not in force.

## Pre-flight before coding

- State your assumptions before you implement. If the request has two readings, ask.
- Turn the task into a condition a command can test. "Fix the bug" becomes "write the test that reproduces it, then make it pass".
- Ask whether a senior engineer would call the result overcomplicated. If 200 lines fit in 50, rewrite before you show it.
- Match the style of the file you edit. If you must deviate, say so first.
- Comments follow their own rule, not the file's density. Default to none. Write one line only for a why the code cannot say: a gotcha, a business rule, a workaround. Never narrate what the code does, restate a name or type, leave commented-out code, add a section banner, or add an @param/@return that repeats the signature. Leave existing comments on lines you did not change.
- Unrelated dead code stays. Report it in the summary. Remove only the orphans your own change created.

## Direct execution

Use the installed tool and not a wrapper. Read-only by default: `SELECT`, `DESCRIBE`, `SHOW`, `EXPLAIN`. Confirm before any write, any DDL and any mutation.

- Detect the database engine from `.env DB_CONNECTION`, from `DATABASE_URL`, from a compose file, or from the migrations. Credentials live in `.envrc` or `.env`, and you must not repeat them in a query.
- Run a remote command over `ssh <alias> "<command>"` when the nearest `.devwork/` documents the alias. Verify it with `ssh -G <alias>` before you call it missing. A mutation needs the user to confirm it.

## Secret hygiene

The transcript exposes every secret that reaches it. The harness captures raw stdout and stderr. Redact before a command prints, because you cannot un-leak it after.

- Never dump a config file that carries secrets. Grep the key names only, or mask the values with `rg -o '^KEY=' | sed -E 's/=.*/=***/'`.
- Assume application code leaks a credential inside an error. A trace embeds a connection string. Pipe a command that may throw through a redactor: `2>&1 | sed -E 's#://[^@/ ]+@#://***:***@#g'`.
- A probe asserts presence and connectivity as a boolean, never a value. Print `set: yes` and `connect: OK`. Do not echo a host, a user or an endpoint unless the user needs it.
- If a secret leaks, say so in the summary and recommend rotation.
- `secret_guard.py` enforces this. Every denial names the safe form. Count them in the transcripts when you need the rate, never from a number written here.
- PII reaches the transcript only as a token (`<email:1a2b3c4d>`) or a first name, and the guard enforces that too. Open a `.csv`, `.tsv` or `.out` that holds personal data with the Read tool, and run `rg`, `sed` or `awk` on the redacted copy it names. A Bash command on the original is denied, because its rows can arrive without the header the column mask needs.

## Artifact location

Write every generated markdown file inside `.devwork/`. Never write one to the project root, to `~/Desktop`, to `/tmp` or to the home directory.

- A real artifact goes in its subfolder: `.devwork/plans/`, `.devwork/specs/`, `.devwork/schema/`, `.devwork/tasks/<id>/`, `.devwork/decisions/`, `.devwork/research/`.
- A draft, a scratch note or a mid-task recap goes in `.devwork/_scratch/`.
- A repository's own `.devwork/` holds repo-level cards only (`context.md`, `constitution.md`, `flows/`). Read it for orientation and never put a task, a plan, a spec, a query or a scratch file there (decided at the last reinstall, recorded 2026-10-14).
- `.devwork/` is the convention. The toolchain knows no other name, so recognize a different one and never add a second.
- Do not assume a hook enforces this. Read `settings.json` in the resolved Claude home when the answer matters.

Source, config and test files follow the conventions of their project.

## Editor handoff

`$EDITOR` opens an external application. Never call `nano` or `vim` for content the user will edit. Use the Edit tool, or print the content for the user to paste.

## Project-level config

Read what the nearest `.devwork/` holds when it is present, and never assume a file it may not carry. A project CLAUDE.md is additive, so read it as the table of contents and do not repeat the discovery it records.

## Verification before declaring done

- Run the test, the build or the smoke command. State the result. Inspection is not verification.
- Re-read the changed line, or run the command that exercises it. The Edit tool reports success, and behavior is what matters.
- When verification is impossible, write `[X] not executed` and say why. Never claim success from inspection alone.

## Operating rules

- Detect capability per project. An absent capability is N/A and not a gap to fill.
- Lock one objective and park everything else.
- A task folder holds plain markdown, and the files are canonical.
- Scan `.devwork/bin/` and `.devwork/bin/protocol/` before you provision new tooling. `.devwork/scripts/` does not exist, and on 2026-09-19 that error made a session rebuild an instrument already on the bench.
- Read-only by default for any tool that touches production. Confirm in chat before a mutation, even after a general approval.
- Do not enforce a convention the adoption data says is dead. A rule that lives only in a document is a wish.
- Kill drift at the source. Point a document at a generated command, never at a count you typed.

## `/clear` between unrelated tasks

Sometimes the user moves to a topic that shares no file, no ticket and no vocabulary with the last five turns. Suggest `/clear` before you continue.

