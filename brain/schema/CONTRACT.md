# The Brain — Contract (P0)

> Authoritative structural definition-of-done. The Go tools (P1+) implement against this.
> Master design: `../../decisions/devwork-kb-redesign.md` (wins on any overlap). Gates: master §7 (R1–R11).
> Verify: `python3 schema/check.py` (stdlib-only; lints schemas + validates `examples/`).

## 1. What this locks
The **STRUCTURE** (master §A): one agnostic ontology, the same shape for every project. Capability
differences are *instantiation*, never *shape*. Completeness is a property of **this schema, not the
format** — md/yaml/jsonl/sqlite is the tools' private business; the caller sees only the §6 interface.

## 2. The completeness law (universal — every record)
Required on **every** record (`_base.schema.json#/$defs/record`):

| field | rule | satisfies |
|---|---|---|
| `id` | unique stable key (`^[A-Za-z0-9][A-Za-z0-9._-]*$`) | **F1/R1** one record per entity |
| `type` | enum `umbrella·project·task·artifact·decision·spec·edge` | **F1** |
| `status` | enum `open·active·blocked·review·done·archived` (queryable) | **F3/R3**, R2/R9 |
| `updated` | `YYYY-MM-DD` (absolute, never relative) | **F3/R3** currency |

Optional-but-universal: `created`, `title`, `status_detail` (free-text nuance behind the controlled
`status`), `tags`, `edges` (adjacency; present even if empty → every record is edge-aware, **F2/R2**).

**Invariants** (master §A): one record per entity (F1) · traversable edges (F2) · every record carries
`updated`+`status` (F3) · records, not narrative (F4 — `additionalProperties` is open for domain fields,
but completeness is enforced by `required`, never by forbidding extras).

## 3. Record types — required fields = the gate
The transversal triad `context · status · to-do` is universal to every *assignment* (project/task).

| type | required (beyond the universal 4) | role | gate |
|---|---|---|---|
| **umbrella** | `title`, `projects[]` | registry/TOC + capability owner (owned once, referenced) | single entry point |
| **project** | `title`, `context`, `todo[]`, `tasks[]` | ONE canonical record (kills F1) | R1 |
| **task** | `title`, `context`, `todo[]`, `scope`, `done_when[]` | bounded work unit; pilot specimen | KICKOFF gate (C.1), R7 |
| **artifact** | `title`, `kind` | preserved objective thing (schema/query/plan/deploy/…) | — |
| **decision** | `title`, `context`, `decision` | addressable ADR; FEEDBACK lands here | C.4 (no correction lost) |
| **spec** | `title`, `requirements[]` | SRS-anchored tracking; completeness is the schema's | R8 (PilotB) |
| **edge** | `from`, `rel`, `to` (+`type`) | join-table graph row (not a completeness record) | R2 |

`task.entry_points[]` is recommended (not required): it powers the warm load (**R5** ≤8 reads). The pilot
specimen `research/platform-evidence/bench/canonical.json` maps onto `task` (see
`examples/task.acme-subscription-hotfix.json` — the P1 ingest target).

## 4. Edge model (R2)
`rel` vocabulary: `depends_on · blocks · relates_to · decides · implements · produces · supersedes ·
part_of`. Two storage shapes, **both fixed here so either store satisfies the same contract**:
- **adjacency-in-record** — `edges[]` of `{rel, to, note?}` on each record (pilot-a's `depends_on` pattern).
- **join-table** — standalone `edge` rows `{from, rel, to, note?}` (queryable graph).

The LINKER emits edges *on write* so links are never hand-typed. **Edge STORAGE choice (inline vs
join-table) is the P0/P1 open item** (master §8) — decided at P1 from the SQLite store design; the *shape*
above does not change.

## 5. The interface (the SCRIBE/router contract — master §B6, stable across substrate)
The agent fires **one line**; the tool does the locating/parsing/writing. Format-agnostic by construction:

```
brain get    --<entity>=<id> --section=<field>            # read one section
brain set    --<entity>=<id> --section=<field> --input=…  # replace one section (stdin/file for large)
brain append --<entity>=<id> --section=<field> --input=…  # append, never rewrite
brain find   --<query>                                    # graph/field query → ids
brain list   --<collection> [--where status=open]         # operational lists
```
This is **why "format doesn't matter" is literally true**: substrate = SQLite (locked P0→P1), but the
caller never sees it; REPORTER renders md/yaml/jsonl/HTML on demand.

## 6. Status & verification
- `python3 schema/check.py` → lints all `*.schema.json` (valid JSON, `$schema`/`$id`, refs resolve) and
  validates `examples/*.json` against their type schema. **P0 exit gate = ALL GREEN.**
- Negative-tested: bad enum / bad date pattern / missing required / empty `done_when` / missing item
  field are all caught with non-zero exit.
- This harness is Python stdlib only (no node, no `jsonschema` lib). The Go SCRIBE (P1) re-implements the
  same required-field checks as write-time validation.
