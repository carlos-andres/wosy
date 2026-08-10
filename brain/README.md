# brain/ — WOSY knowledge + workflow system (build home)

The `brain` binary is the structural backbone of wosy. Sqlite-backed, schema-validated, one record per entity, traversable edges. Plugged into Claude Code via the PreToolUse `watchdog.sh` shim.

## Locked toolchain (P0→P1 boundary)
- **Go** native arm64, `CGO_ENABLED=0` static binary — the hot-path hands (WATCHDOG/SCRIBE/REHYDRATOR/DONE-GATE + `get/set/append/find/list`). Binding constraint = per-hook process startup.
- **SQLite** store, one `.db` per project, pure-Go `modernc.org/sqlite` (no CGo). Format stays agnostic (REPORTER renders md/yaml/jsonl/HTML on demand).
- **Python 3 stdlib** — migration/measurement + the P0 verification harness.
- **Bash** — one-line hook shims that `exec` the binary. (Node/npm rejected.)

## Layout
```
brain/
  README.md            this
  schema/              P0 — the contract
    _base.schema.json    universal completeness law + shared vocab ($defs)
    {umbrella,project,task,artifact,decision,spec,edge}.schema.json
    CONTRACT.md          authoritative structural definition-of-done
    check.py             stdlib JSON-Schema verifier (no node/deps)
    examples/            one valid instance per type
  cmd/ internal/ go.mod  P1+ — the Go tool
  hooks/PROTOCOL.md      brain-side hook protocol docs
  smoke-wosy.sh          live shim + classifier smoke
```

## Phase tracker (high-level)
- [x] **P0 — Lock the contract.** Ontology + per-type required-field JSON schemas + interface. Exit: `check.py` ALL GREEN (8 schemas lint, 7 examples validate, negative test catches violations).
- [x] **P1 — SCRIBE + store.** Go `get/set/append/find/list` + `import/init` over SQLite (pure-Go, static binary). Round-trips clean; ratios reproduced from the real binary. Write-time validation via embedded schemas.
- [x] **P2 — Capability layer, detected (R8).** `brain detect` (read-only): stack + engine shell out to canonical `~/.claude/hooks/lib/detect-*.sh` (no drift); brain adds capability-presence (connections/schema/query-library/runner), hidden-file-aware. Detection composes a valid `umbrella.capability`.
- [x] **P3 — WATCHDOG + DONE-GATE + protocol (R7, R10).** `brain guard` classifier (fail-open · provision-then-enable · kill switch · source flows free) · PROMOTER (`brain promote`) · DONE-GATE (`brain gate`) · PreToolUse shim `hooks/watchdog.sh` + `hooks/PROTOCOL.md` opt-in wiring.
- [x] **P4 — Sample migration + measurement (R1, R5; additive-only).** Working set: 1 read (≤8), contradictions → 0 via decision records, edges by query (not scan). Snapshot + `verify` = source untouched.
- [x] **P5 — Generalize + registry (R3).** R3 = 100% carry `updated`+`status` across stores. Registry (`REGISTRY.md`) resolves umbrellas.

**Status:** GROUNDED. The schemas, binary, and protocol are stable.

## Registry

See `REGISTRY.md` for the registry model. The registry is per-installation — it grows as you onboard projects via `/wire-up` and direct plan/task artifact creation.
