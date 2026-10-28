---
kind: task-prompt
role: Platform engineer reading the feed-alpha pipeline read-only
task_id: goodrich-600031705-missing-units
project_id: feed-alpha
ticket_ref: TICKET-1436
para: Explicar por qué los stocks que reporta el PM no aparecen activos para Goodrich 600031705.
access: read-only
verify: .devwork/bin/q.sh inventory_by_stock 600031705 22199 feedco <STOCK>
pass_when: every PM stock has one inventory row with its is_archived value and the AER row that links it
output: findings.md in the task folder, one row per stock
created: 2026-10-02
updated: 2026-10-02
---

<prompt>
<role>You read the feed-alpha pipeline and the TC schema read-only. [V] ran it and saw the output, [I] inferred, [X] no evidence.</role>
<objective>Say why each stock the PM listed is not active, with the row that proves it.</objective>
<context>Goodrich, merchant 600031705, feeds TC from 22199.csv through the feed-alpha pipeline (see projects/feed-alpha/master.md), the inbound path that standardizes feedco listing data into inventory. The mission is to find why the PM's stocks do not show as active; for the PM, "missing" means "not active". Look first at inventory with inventory_by_stock.sql, then the AER link (aer_inventory_by_stock.sql), then the feed tables and the original CSV.</context>
<output_format>findings.md: stock · inventory_id · is_archived · AER row · CSV line · cause.</output_format>
</prompt>
