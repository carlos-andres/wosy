---
kind: task-mission
workstream: WS1
task_id: TASK-15993
access: read-only
question: How was PHP 7.4 put on staging-worker, and which steps still apply on noble?
output:
  - /Users/you/work/source/.devwork/tasks/TASK-15993/ws1.md
  - /Users/you/work/source/.devwork/tasks/TASK-15993/ws1-extra.md
established:
  - staging-worker is Ubuntu 18.04.6 bionic [V] raw/staging-worker.txt
done_when: ws1.md written
ledger: /Users/you/work/source/.devwork/tasks/TASK-15993/logbook.jsonl
created: 2026-10-12
---

Broken on purpose: two outputs. A mission writes ONE file, so the schema must reject it.
