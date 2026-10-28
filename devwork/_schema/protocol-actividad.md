---
kind: protocol
task_id: advisor-lab-20261013
project_id: devwork-infra
role: "The protocolo de actividad: the seven steps of any activity, which home runs each step, its artifact and its gate, and how HQ hands Earth work to the Obra."
para: "Que cualquier sesión fresca sepa cómo se trabaja una actividad: quién hace cada paso, qué archivo deja y qué lo valida."
status: "ruled 2026-10-13 (advisor-lab-20261013, ledger events 18 to 47); tested end to end on TASK-15993 WS5"
created: "2026-10-13"
updated: "2026-10-13"
---

# Protocolo de actividad

HQ is /Users/you/.claude, the central. The Obra is /Users/you/.claude-work, the ship, and it works only on Earth: /Users/you/work/umbrella, /Users/you/work/source, its own home, and reads of /tmp.

```
HQ   1 SALÓN        conversación, research            → eventos en el ledger
HQ   2 ORDEN        prompt.md                           gate: prompt-doc
HQ   3 HORIZONTE    context.md                          gate: task-doc, encuadre traps
HQ   4 SPEC+PLAN    spec.md, plan.md                    gate: spec-doc, plan-graph
        │
        ├─ ¿dónde vive el trabajo?
        │    sistema o Combat → HQ hace 5 y 6 con sus subagentes (qa-gate revisa)
        │    Tierra (umbrella/Source) → ¿encargo o misión?
        │
        │    encargo-check <plan> = exit 0  → ENCARGO: la Obra corre el plan entero
        │    exit ≠ 0 o sin plan            → MISIÓN: una pregunta, un archivo
        │         mission-<ws>.md en la carpeta de la tarea   gate: mission-doc
        │         la Obra la corre con claude -p dentro de su límite
        │
OBRA 5 EJECUCIÓN    escribe UN archivo de salida
OBRA   RETORNO      protocolo --event <ledger> verification "<WS>: ..." --pointer <salida>
HQ   6 REVISIÓN     re-verifica la salida (no se cree el reporte)
HQ   7 CIERRE       entregable, sello del ledger, next-session-prompt.md
```

## Where each piece lives

- Schemas and scaffolds: /Users/you/.devwork/_schema (`mission-doc.schema.json`, `scaffolds/mission.md`, `prompt-doc`, `spec-doc`, `plan-doc`, `task-doc`, `logbook-event`).
- Tools: /Users/you/.devwork/bin/protocolo, /Users/you/.devwork/bin/encargo-check, /Users/you/.devwork/bin/encuadre. The graph checker is /Users/you/work/umbrella/.devwork/bin/plan-graph.
- A new ledger needs `protocolo --event <ledger> <type> "<text>" --task <id> --project <id>` once. The file must exist first.

## Rules

- Earth check before the Obra branch: the task folder must be under /Users/you/work/umbrella or /Users/you/work/source. Outside them (a system task, Combat), HQ runs steps 5 and 6 itself and the Obra route is not considered (Calo, 2026-10-13).
- A small Earth task can skip steps 3 and 4: the mission carries the one question and its done_when (TASK-15993 WS5).
- When a request has two readings (a full revision or a patch), ask before step 2.
- The Obra's limit is a blocklist (deny rules), not an allowlist. A runtime-built path in an interpreter is held only by the auto classifier.
