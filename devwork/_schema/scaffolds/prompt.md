---
# yaml-language-server: $schema=/Users/you/.devwork/_schema/prompt-doc.schema.json
kind: task-prompt
role: ""
task_id: ""
project_id: ""
ticket_ref: null
context: ""
para: ""
access: read-only
verify: ""
pass_when: ""
rollback: ""
output: ""
created: ""
updated: ""
---

<!-- Reference template (factory-redesign logbook 75). Keep the 4 REQUIRED sections; take an
     OPTIONAL one only when the task needs it. Long material first, the ask last. Rules in the
     positive. Tags [V] ran it · [I] inferred · [X] no evidence: say so inside <role>. -->

<prompt>

<role>REQUIRED. Who executes, with which expertise, in one short paragraph. Legend of [V]/[I]/[X].</role>

<objective>REQUIRED. The outcome, not the activity, in one to three lines.</objective>

<context>REQUIRED. At most two paragraphs: the badge of the task in prose (entity, world and
pipeline, mission, where to look first and with which query or runbook). Point at the context.md
of the frontmatter and at the world master.md; never copy them.
  <facts>OPTIONAL. Each one tagged [V] with its evidence.</facts>
  <relationships>OPTIONAL. Who feeds whom; which source wins.</relationships>
  <traps>OPTIONAL. What was tried and failed; what looks accidental and is deliberate.</traps>
</context>

<scope>OPTIONAL. out: what is explicitly not touched.</scope>

<research_process>OPTIONAL. The ordered phases of the work, one line each.</research_process>

<instructions>OPTIONAL. Ordered, imperative steps, each with its full working command.</instructions>

<example>OPTIONAL. One worked case, input to output, when the task transforms data.</example>

<analysis_requirements>OPTIONAL. What every recommendation must carry: rationale, trade-offs, risks.</analysis_requirements>

<output_format>REQUIRED. The artifact, its path and its shape.</output_format>

<rules>OPTIONAL. Task-specific limits only; access and rollback are in the frontmatter.</rules>

<final_checks>OPTIONAL. The questions the executor asks itself before it hands back.</final_checks>

<references>OPTIONAL. file:line · task folder · query file · schema card · brain term.</references>

</prompt>
