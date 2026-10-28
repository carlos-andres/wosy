---
# yaml-language-server: $schema=/Users/you/.devwork/_schema/mission-doc.schema.json
kind: task-mission
workstream: ""
task_id: ""
access: read-only
question: ""
output: ""
established: []
done_when: ""
ledger: ""
created: ""
---

<!-- One mission from HQ to the Obra (advisor-lab-20261013 FR-002). Shape of
     TASK-15993/prompt-ws1.md. One question, one file. HQ writes it; the Obra runs it. -->

You are running workstream <WS> of task <TASK>. <access>. Answer ONE question and write ONE file.

QUESTION: <the question of the frontmatter, with its context>

READ (only these):
- <absolute path>: <why>

ESTABLISHED FACTS (verify against the source, do not contradict without citing a line):
- <fact> [V] <source>

OUT OF SCOPE (one line under "Parked" if seen, do not investigate): <list>

WRITE exactly one file: <output>, in English, with these sections:
- `## Answer`
- `## Parked`
Never print or copy secret values. Do not modify any file other than the output.

RETURN: as the last action, run
/Users/you/.devwork/bin/protocolo --event <ledger> verification "<WS>: <one-line answer>" --pointer <output>
