---
# yaml-language-server: $schema=../kickoff.schema.json
kind: kickoff
description:             # one line that lets an agent decide whether to open this file
form:                    # paragraph | bundle
project_id:
task_id:
cwd:                     # absolute path the session starts from
reads:                   # ordered absolute paths — pointers, never inlined content
  # - /abs/path/01-first.md
  # - /abs/path/02-second.md
verify:                  # run before building anything
  # cmd:
  # expect:              # exact expected output
first_action:            # the literal next command or edit
done_when:               # falsifiable close condition for the stage this loader opens
locked:                  # decisions the fresh session must not re-derive
  # - "…"
prohibitions:            # targeted imperatives only
  # - "Do not start phase 2 without an explicit lock."
expires_when:            # condition after which this loader is stale and must not be pasted
frozen_copies:           # form=bundle only
  # - copy: /abs/path/03-ESTADO.md
  #   live_source: /abs/path/PROGRESO.md
disposable_when:         # form=bundle only — when the whole directory can be deleted
replaces:
isReplacedBy:
created:
updated:
source_ref:
---

<!-- The loader. It names what a fresh session reads and in what order, and nothing else. Detail lives in the files it points at. Keep it under 200 lines: Claude Code loads only the first 200 lines or 25KB of an index file and drops the rest silently. -->

<!-- For form=bundle, put the files in a numbered directory and let the numeric prefix carry the read order: 00-README, 01-KICKSTART, 02-CLAUDE, 03-ESTADO, 04-CONTEXTO, 05-BACKLOG. A prose list can drift from the directory. A numbered directory cannot. -->

<!-- Front-load. Measured position bias means the first region of a document is the reliably-read one, so `first_action` and blockers go above narrative, never in the middle. -->

## What this is

<!-- One paragraph. What the session is being opened to do. -->

## Read in this order

<!-- Mirror of `reads`. One line per file: the path, then why it is needed. -->

## Verify this loader is legitimate

<!-- The command and its exact expected output. If it does not respond, the loader is stale and the session stops. -->

## Do not redo

<!-- Locked decisions and verified results. -->

## Start here

<!-- The literal first command or edit. -->
