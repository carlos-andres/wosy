---
# yaml-language-server: $schema=../backlog.schema.json
kind: backlog
project_id:
rule:                    # the standing rule that governs how a phase closes
phases:                  # ordered; sequence position is the dependency order
  # - id: F1
  #   name:
  #   state:             # pending | in-progress | blocked | in-verification | closed
  #   acceptance:        # the command that proves this phase closed, and its expected output
  #   gate:              # what must be true before this phase may start
  #   closed_on:         # ISO date, set only when state=closed
deferred:                # carried to a later stage, one line each
  # - "…"
replaces:
isReplacedBy:
created:
updated:
source_ref:
---

<!-- The backlog IS the plan and is not re-derived. A session that re-plans from the objective has lost the backlog. -->

<!-- This is the only artifact where naming excluded work is correct. Here it is scope control and the user decides. In a loader it is hallucination surface, because naming what you will not do supplies the vocabulary to do it. -->

<!-- Every phase closes with its acceptance command executed, not with an argument that it should work. Verifying the reasoning is not verifying the code. -->

## Phases

<!-- Mirror of `phases`. One section per phase with its acceptance command and expected output. -->

## Deferred

<!-- One line each. The user decides whether it is promoted. -->
