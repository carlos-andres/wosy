---
# yaml-language-server: $schema=../status.schema.json
id:                      # folder slug, e.g. apc-600031231-inventory-gap
project_id:
task_type:               # bug | feature | audit | config
ticket_ref:              # optional
symptom_origin:          # user's opening phrase VERBATIM, or 'SIN DATOS' — never synthesized
state:                   # open | in-progress | blocked | in-verification | closed
waiting_on:              # optional
done_when:               # falsifiable close condition a command can test
where_we_are:            # status trio (1)
next_step:               # status trio (2), optional
open_questions:          # status trio (3), optional list of strings
  # - "…"
env_residue:             # optional list of {scope, what, reversible?}
  # - scope:
  #   what:
  #   reversible:
created:                 # ISO date
updated:                 # ISO date — bump on every write
source_ref:              # tasks/<id>/status.md
---

<!-- status.md is live state and is overwritten in place. History lives in handoff-<ISO>-<seq>.md. Body holds the prose the frontmatter does not index. Frontmatter is byte-identical to brain task.schema.json, so import to brain is a copy rather than a mapping. The `updated` key appears in 1.3% of existing devwork files, and that absence is why 75% of them go stale unnoticed — bump it on every write. -->

## Where we are now

<!-- One paragraph, facts only, absolute dates and absolute paths. Corpus median is 92 words and the worst case is 1478. -->

## Next concrete step

<!-- The literal command or edit, and the result it must produce. Not a description of it. -->

## Open questions

<!-- One bullet each. An unknown that nobody names becomes an assumption. -->

## Ruled out

<!-- Claim, then the negative evidence that killed it. A dead end that nobody records gets walked again. -->
