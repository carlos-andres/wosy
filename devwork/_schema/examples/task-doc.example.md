---
# yaml-language-server: $schema=../task-doc.schema.json
kind: task-context
role: Verify that worker-service runs against MySQL 8.4 in production with TLS enabled, read-only, before the engine upgrade lands.
task_id: mysql84-prod-verification-20260922
project_id: feed-alpha
ticket_ref: null
task_type: audit
root: /Users/you/work/umbrella/.devwork/tasks/mysql84-prod-verification-20260922
para: Confirm that the worker survives engine 8.4 with TLS on before operations upgrades, without touching production.
state: in-progress
where_we_are: TLS is enabled on the production host; the engine is still below 8.4, so the verification waits on the upgrade window.
next_step: re-run the readiness probe the day the engine reports 8.4
done_when: the probe returns 8.4 and the worker's smoke run exits 0 over TLS
waiting_on: null
ledger: /Users/you/work/umbrella/.devwork/tasks/mysql84-prod-verification-20260922/logbook.jsonl
created: 2026-09-22
updated: 2026-09-29
source_ref: /Users/you/work/umbrella/.devwork/tasks/mysql84-prod-verification-20260922/status.md
---

# mysql84-prod-verification-20260922 — context

## What it is
## Why
## What for
## Where we are
## What is next
