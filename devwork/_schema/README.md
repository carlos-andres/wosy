---
kind: reference
id: schema-readme
created: 2026-08-07
updated: 2026-09-29
---

# Context artifact schemas

Four documents carry state across the loss of a context window. Each has a JSON Schema here and a scaffold in `scaffolds/`. Derivation and evidence are at `/Users/you/Documents/.devwork/research/document-fix-proposal.md`.

| artifact | file | schema | brain target |
|---|---|---|---|
| status | `tasks/<id>/status.md` | `status.schema.json` | `task` record — byte-identical, import is a copy |
| handoff | `tasks/<id>/handoff-<ISO>-<seq>.md` | `handoff.schema.json` | `wrap` record, partial |
| kickoff | `00-README.md` | `kickoff.schema.json` | none yet |
| backlog | `backlog.md` | `backlog.schema.json` | none yet |

Three more were added on 2026-09-24 by decision D-7 of `tasks/brain-continuity-20260923/context.md`. They are not loader documents and carry no scaffold, so nothing in `scaffolds/` changes.

| artifact | file | schema | brain target |
|---|---|---|---|
| logbook event | `<task>/logbook.jsonl` — one event per line, append-only | `logbook-event.schema.json` | `stats` — loads, 0-hits, latencies |
| pallet/brief | composed per request, never stored whole | `pallet-brief.schema.json` | `brief`, `wrap` |
| freshness rendering | the freshness block of any rendered answer | `freshness-rendering.schema.json` | `last_verified_at`, `superseded_by` |
| world doc | `projects/<world>/master.md`, `architecture.md`, `connections.md` — the frame documents, frontmatter only | `world-doc.schema.json` | `project.root_path` — the body is reached, never copied |
| task doc | `tasks/<id>/context.md` — the single task document, frame plus living state; since 2026-10-02 it may carry the badge fields `entity`, `route`, `out_of_scope`, `neighbours`, `world_doc` (factory-redesign logbook 77); its ledger `logbook.jsonl` reuses `logbook-event.schema.json` | `task-doc.schema.json` | `record` of type task — the living-state fields keep the names of `status.schema.json` |
| task prompt | `tasks/<id>/prompt.md` — the flight plan, frontmatter plus XML sections; template `scaffolds/prompt.md`, 4 required sections (role, objective, context, output_format), the rest optional; points at the task `context.md`, never copies it (factory-redesign logbook 75) | `prompt-doc.schema.json` | none |
| task mission | `tasks/<id>/mission-<ws>.md` in the Earth task that owns it — one mission from HQ to the Obra: one workstream, one question, one output, an established block, a return event to the task ledger; template `scaffolds/mission.md`, shape of TASK-15993 `prompt-ws1.md` (advisor-lab-20261013 FR-001) | `mission-doc.schema.json` | none |
| protocol | a protocol document, e.g. `tasks/factory-redesign-20260911/protocol-encuadre-20261002.md`; its graph is the ```yaml graph block checked by `plan-graph` (factory-redesign logbook 77) | `protocol-doc.schema.json` | none |
| runbook | `runbooks/<name>.md` — a work runbook under the standard (logbook 196.1); the 17 runbooks before 2026-09-29 are reference, not validated | `runbook.schema.json` | none |
| context card | the root `context.md` of an umbrella, loaded at every session start; the JSON port of card/1 plus `para` (logbook 196.2, 200.1) | `context-card.schema.json` | `workspace` row |
| entry | the `CLAUDE.md` of an umbrella, a world or a repo (logbook 213.4); Anthropic defines no frontmatter, so it stays minimal | `entry.schema.json` | none |
| rule | one file in `.claude/rules/`, one topic (logbook 213.1); Claude Code strips the frontmatter before loading, so it costs no context | `rule.schema.json` | none |

Each of the three carries a real example and a deliberately broken one under `examples/`. The broken one is the control, because a gate that only ever goes green is a baseline and not a test. The three controls fail on the conditional each schema exists for: a `delivery` event with no `delivery.hit`, a `consumer: subagent` brief missing D1's four slots, and a claim with `corrected_at` but no `superseded_text`.

`logbook-event.schema.json` governs one line, not the file, because `check-jsonschema` has no JSONL reader. The gate splits the file. Run all six:

```
S=/Users/you/.devwork/_schema; T=$(mktemp)
for f in example broken; do
  while IFS= read -r l; do printf '%s' "$l" > "$T"
    check-jsonschema --schemafile $S/logbook-event.schema.json "$T"; echo "  ^ $f, exit $?"
  done < $S/examples/logbook-event.$f.jsonl
  for s in pallet-brief freshness-rendering; do
    check-jsonschema --schemafile $S/$s.schema.json $S/examples/$s.$f.json; echo "  ^ $s $f, exit $?"
  done
done; rm -f "$T"
```

Every `example` run must exit 0 and every `broken` run must exit 1. Measured 2026-09-24: 7 of 7. Six files, seven runs, because `examples/logbook-event.example.jsonl` carries two lines.

**Step 1e added three fields to `pallet-brief.schema.json` on 2026-09-24**, each read off a brief that second-brain v2 renders rather than designed — see `tasks/brain-continuity-20260923/decision-delivery-briefs-20260924.md`. `gaps` is required and may be empty, so a brief that delivered nothing no longer looks like one that delivered everything. `delivered` carries the measured `tokens` beside the stated `token_budget`, because a bound with no actual cannot be exceeded and so cannot be tested; the comparison `delivered.tokens <= token_budget` belongs to the checker, since JSON Schema cannot compare two instance values. `built_from` items are now objects carrying `source` and `statement`, because the SQL re-runs where a name does not; `statement` may be null only where the source is an absolute path, and a conditional refuses it empty for any other source. The six-file gate stays at 7 of 7 and the additions carry their own controls: **11 of 11 mutations red, 2 of 2 negative controls green, 3 of 3 schemas valid against the 2020-12 meta-schema** `[V]`.

**Required now means the field says something, corrected 2026-09-24.** In the four original schemas, `required` only asserted that the key was present, so an empty string passed. Measured that day: **13 required string fields across the four had no `minLength`, no `enum`, no `const` and no `format`** — 7 of the 9 in `status.schema.json`, plus `description` and `source_ref` in `kickoff`, `date` and `source_ref` in `handoff`, `project_id` and `source_ref` in `backlog`. The three schemas added by D-7 had none, so the new contracts were stricter than the ones they were told not to redesign. A `status.md` whose `symptom_origin` was the empty string passed the gate, confirmed by mutation.

`"minLength": 1` is now on all 13. **No field was added, removed or renamed**, so the field set that D-7 and `srs.md` C5 protect is byte-for-byte the same shape. Measured in one block `[V]`: **13 of 13 empty-field mutations exit 1**, all four schemas still validate against the 2020-12 meta-schema, and **0 existing files broke** — every real instance of the four classes returns the same verdict as before the change. The pre-change files are kept beside each schema as `<name>.schema.json.bak-minlength`.

One thing the regression sweep found on the way: **`.devwork` holds no `handoff` artefact and no `backlog` artefact at all.** Exactly one file carries `kind: handoff` and one carries `kind: backlog`, and both are the scaffolds in `scaffolds/`. Files named `handoff-*.md` are design documents, not instances. The schema and the scaffold exist and nothing has ever been written against them.


**`world-doc.schema.json` added 2026-09-29** (brain-continuity logbook 195, rulings 2 and 3): `kind` picks the document among `master`, `architecture`, `connections`, `role` is one line of prose, `para` is the only human field, `project_id` is required under `scope: world`, and `additionalProperties: false` makes a `tipo:` document fail. Its examples are `.md` files, so the gate extracts the frontmatter first: `yq --front-matter=extract -o=json '.' examples/world-doc.example.md > /tmp/x.json && check-jsonschema --schemafile world-doc.schema.json /tmp/x.json`. Measured 2026-09-29: example exit 0, broken exit 1, and five mutation controls red, one negative control green.

**`task-doc.schema.json` added 2026-09-29** (logbook 187 and 195.6): one document per task folder, `kind: task-context`, with `task_id`, `root`, `para`, the living state (`state`, `where_we_are`, `next_step`, `done_when`, `waiting_on`, same names and enum as `status.schema.json`) and `ledger`, the absolute path of the task's `logbook.jsonl`. `project_id` is optional on purpose: a task with no derivable world keeps its gap. `blocked` requires `waiting_on`. Same extraction recipe as world-doc. Measured 2026-09-29: example exit 0, broken exit 1, six mutation controls red, two negative controls green, and the manufacturer pilot's current `context.md` in the `task-card/1` dialect exits 1, which is the D.16 control.

**`done_when` is required in `kickoff.schema.json` since 2026-09-29** (logbook 195.5, plan A.6): an opener without a falsifiable close condition is not an opener. Measured in one block: 10 of 10 real kickoff instances under the hub still exit 0, the leg-9 prompt without `done_when` exits 1 and with an empty one exits 1. `scaffolds/00-README.md` fails as it failed before, because a scaffold carries empty required fields by design. The pre-change file is `kickoff.schema.json.bak-donewhen`.

## The rules that govern all four

Write for the model that loads the file next, not for a person who reads it. If someone asks for the content, render a view — bullets, plain language, an analogy — and do not rewrite the file to match the view.

Give an absolute path for every file named. Give an absolute date. Never "yesterday", "last session", or "recently".

Bump `updated` on every write. The key appears in 1.3% of existing devwork files today, and that absence is why 75% of status files are silently superseded by a newer sibling.

Store pointers, never payloads. `reads:` is a YAML sequence of paths, and YAML sequences are ordered by definition, so no rank field exists.

Express supersession with `replaces` and `isReplacedBy`. These are DCMI Metadata Terms, not coined vocabulary.

Front-load. Position bias is measured, so the first region of a document is the reliably-read one. `next_step`, blockers and `first_action` go above narrative.

## Two asymmetries, both deliberate

`status.md` carries no `kind` key. Its frontmatter is a byte-identical mirror of `brain/_schema/task.schema.json`, which sets `additionalProperties: false`, so adding a key would break the import. Identity comes from the filename, exactly as `brain` derives `project_id` and `task_id` from the path. The other three carry `kind` because `brain` does not constrain them yet.

The timestamp key is `updated`, not `modified`. Claude Code writes `modified` into agent-read frontmatter and has the better ecosystem precedent, but `brain/_schema/task.schema.json` requires `updated` and `brain` is the port target. The divergence buys a zero-step import.

## Handoff is one schema, not two

Cold-start and continuation were tested against 28 handoffs read in full: 7 continuation, 8 cold-start, 13 hybrid, 0 neither. Corpus-wide, 49 of 66 carry both a mandate and a state recap. No field is unique to cold-start. The difference is boundary state, so it is the `boundary: clean | dirty` field rather than a second document type.

Read the latest handoff only: `ls tasks/<id>/handoff-*.md | tail -1`. Filenames are ISO-stamped, so lexicographic order is chronological. Earlier handoffs stay for forensics and are never auto-loaded.

## Excluded work belongs in exactly one place

The backlog may carry a `deferred` list, because there it is scope control and the user decides. No loader, handoff or status file names what will not be done. Only 1 of 60 sampled kickstart prompts catalogued non-goals, and 29 used targeted imperatives instead. Naming excluded work supplies the vocabulary to hallucinate with.

## Validation

The scaffolds carry a `# yaml-language-server: $schema=` modeline, so an editor with the YAML language server validates as you type. The modeline is a YAML comment and is ignored by every parser.
