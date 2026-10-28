---
# yaml-language-server: $schema=../handoff.schema.json
kind: handoff
date:                    # ISO; filename-ordered == chronological
seq: 1                   # Nth handoff of that date
boundary:                # clean | dirty
dirty_state:             # required when boundary=dirty
  # - what:              # uncommitted diff, paused job, mutated env, corrected prior claim
  #   where:             # absolute path, host, or branch
  #   reversible:
task_id:
project_id:
cwd:                     # absolute path the next session starts from
git_ref:                 # branch@sha; empty = this session did not ship
done_when:               # carried forward, falsifiable
reads:                   # ordered absolute paths
  # - /abs/path/one.md
  # - /abs/path/two.md
verify:                  # legitimacy smoke
  # cmd:
  # expect:              # exact expected output, not a description of it
prohibitions:            # targeted imperatives only
  # - "Do not relaunch the loop."
replaces:                # path of the handoff this one supersedes (DCMI dcterms:replaces)
isReplacedBy:            # set when a later handoff supersedes this one
created:
updated:
source_ref:              # tasks/<id>/handoff-<ISO>-<seq>.md
---

<!-- One session boundary, written for the model that loads it next rather than for a person. One schema, not two: 74% of real handoffs carry both a mandate and a state recap, so cold-start versus continuation is the `boundary` field and not a separate document type. Read the latest only, with `ls handoff-*.md | tail -1`. Earlier ones stay for forensics and are never auto-loaded. A handoff that claims something is done gets verified before anything is built on top of it, which is what `verify` is for. -->

## Mandate

<!-- What the next session is for. One paragraph. -->

## State at the boundary

<!-- What is done, what is in flight, what is untouched. Facts with absolute paths. -->

## Ruled out

<!-- Claim, then the negative evidence. -->

## Do not redo

<!-- Locked decisions and verified results. Naming these is what stops the next session re-deriving them. -->
