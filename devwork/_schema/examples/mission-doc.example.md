---
kind: task-mission
workstream: WS1
task_id: TASK-15993
access: read-only
question: How was PHP 7.4 put on staging-worker (/opt/php74), step by step, and which steps still apply on Ubuntu 24.04 noble?
output: /Users/you/work/source/.devwork/tasks/TASK-15993/ws1.md
established:
  - staging-worker is Ubuntu 18.04.6 bionic; /opt/php74/bin/php is 7.4.33 [V] raw/staging-worker.txt
  - /opt/php74 has apcu igbinary pdo_dblib redis yaml, which the new box lacks [V] raw/staging-worker.txt
done_when: ws1.md holds Answer, Staging baseline table, Build recipe, Noble portability and Parked, each step cited
ledger: /Users/you/work/source/.devwork/tasks/TASK-15993/logbook.jsonl
created: 2026-10-12
---

Recast of /Users/you/work/source/.devwork/tasks/TASK-15993/prompt-ws1.md into the mission schema. The body is the original prompt.
