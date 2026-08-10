---
title: WOSY — Workflow + Knowledge System (operator manual)
version: 1.3.3
status: shipped + `brain schema` verb + agent-scribe Step 0 promotion
canonical_repo: see README.md for repo URL
---

# WOSY — the operating manual

> **Read order for a fresh session:** this file → `brain/REGISTRY.md` (registry concept) → `brain/schema/CONTRACT.md` (the schema law) → master design docs in your `.devwork/decisions/` only if you need design rationale.
>
> This is the **shipped contract**, not the design rationale. It enumerates what exists, how to use it, how to verify it's working, and what is intentionally not built.

---

## 1. What WOSY is (one paragraph)

WOSY is a per-project **knowledge + workflow substrate**: an **agnostic ontology** (7 schema types), a **single binary** (`brain`) that reads/writes/queries the store, a **PreToolUse hook** (`watchdog.sh`) that classifies and routes Claude's tool calls, an **opt-in flag file** (`.devwork/wosy.flags`) that turns enforcement on per-project, and a **set of skills** (`/wire-up`, `/interview`, `/handoff`, `/consolidate`, etc.) that drive the protocol. The brain is machine-first (one record per entity, traversable edges, write-time schema validation); humans see HTML reports on demand. Format is the tools' private business — the contract is stable regardless of md/yaml/json/sqlite.

---

## 2. Version & ground-truth pins

| key | value |
|---|---|
| WOSY version | **1.3.3** |
| `brain` binary sha | release-pinned (see root `CHANGELOG.md` for the current sha) |
| shim `watchdog.sh` sha | release-pinned |
| Sample store sha (logical) | derived per-project via `brain doctor --integrity` |
| Go tests | **125 / 125 passing** in 9 packages |
| Guard unit tests | **70 / 70** |
| CLI unit tests | **5 new** (`TestSchema_*` in `cli_schema_test.go`) at 1.3.3 |
| Live shim smoke | **14 / 14 PASS ✓** (`brain/smoke-wosy.sh`) |
| v3 harness | **167 / 167 PASS ✓** at the 1.2.0 baseline |
| Master gates | **11 / 11 done** (R6 closed via `brain audit --scratch` + `wosy-stop.sh` nudge — visibility, not block) |

---

## 3. Architecture

### 3.A Structure — the ontology (`brain/schema/`)

7 record types. Each has required fields (write-time validated). Edges are stored in a join table; one record per entity (no duplicates); every record carries `updated` + `status` (F3).

| type | role | shipped schema |
|---|---|---|
| `umbrella` | groups projects under a single capability layer | `umbrella.schema.json` |
| `project` | one product line / repo / corporate workstream | `project.schema.json` |
| `task` | one ticket / unit of work; carries `done_when`, `todo[]`, `scope` | `task.schema.json` |
| `artifact` | a preserved objective (schema, query, plan-doc) | `artifact.schema.json` |
| `decision` | an addressable choice (ADR-equivalent) | `decision.schema.json` |
| `spec` | a SRS-like spec | `spec.schema.json` |
| `edge` | typed relationship between records | `edge.schema.json` |

Shared vocabulary: `brain/schema/_base.schema.json`. Authoritative DoD: `brain/schema/CONTRACT.md`. Verifier: `brain/schema/check.py` (stdlib JSON-Schema, no deps).

### 3.A.1 Data provenance — the `SOURCE.md` convention

Every `.devwork/data/<source>/` directory carries a `SOURCE.md` declaring `origin`, `captured_at`, `captured_by`, `is_production`, `prod_truth_table`, `encoding`, and (optional) `staleness_threshold_days`. Without it, `brain detect` flags the directory as `missing SOURCE.md`; with it, `brain detect` reports days-since-capture and flags stale snapshots. The "local mirror confused for prod truth because mtime looked recent" failure mode is closed by making provenance explicit and machine-checked. Canonical spec: **`docs/conventions/SOURCE.md`**. Downstream consumer: `csv-read.sh` reads `encoding:` from SOURCE.md before falling back to path heuristics.

### 3.B Tools — the brain's hands (`brain/internal/`)

| tool | role | command | enforced by flag |
|------|------|---------|------------------|
| **WATCHDOG** | classify + route raw ops (DB/ssh/KB-write) | `brain guard` (test mode) + `watchdog.sh` hook | `wosy_enforce` master + per-class flag |
| **SCRIBE** | schema-aware writer; no main-terminal diff (R11) | `brain set / append / get / import` | `enforce_kb_scribe` |
| **REHYDRATOR** | one-call warm working set (R5) | `brain load <id>` | — |
| **DONE-GATE** | block wrap on open todos / unmet scope (R7) | `brain gate --task=<id>` | — |
| **REPORTER** | render any record to HTML/md on demand | `brain report --<type>=<id> [--format=md\|html]` | — |
| **LINKER** | extract edges from records on `Put` | (passive; runs in `store.Put`) | — |
| **DETECT** | capability discovery (R8) | `brain detect <path>` | — |
| **PROMOTER** | capture inline query → parameterised .sql in library (R9) | `brain promote --command=<sql>` | — |
| **doctor** | dangling-edge + self-loop audit | `brain doctor` | — |
| **doctor --integrity** | logical-sha pin check (H1, anti-DROP-TABLE) | `brain doctor --integrity` | `expected_logical_sha` |

### 3.C Protocol — the gated loop

| stage | gate (what blocks "done") | vehicle |
|-------|---------------------------|---------|
| **KICKOFF** | record exists with `context · status · to-do · done_when · scope` | `/interview` skill + direct task-artifact creation, `brain import` |
| **PLAN** | scope-complete (design+wire+docs, not just code) | `interview` writes `done_when`; DONE-GATE checks at close |
| **AUTO** | each checkpoint green; 0 new ceremony files; ≤8 reads / 0 re-reads | WATCHDOG enforces SCRIBE; REHYDRATOR loads working set |
| **FEEDBACK** | no correction lost in transcript → decision records | `decisions/` folder; `interview` seeds |
| **WRAP + CLOSE** | DONE-GATE green; CONSOLIDATOR merges near-dups | `/consolidate <id>` (manual); `brain gate` |

Tiered by work-type: full loop for projects, lightweight for standalone tasks (still SCRIBE-guarded).

---

## 4. CLI surface — `brain`

```
brain <version|selftest|init|import|get|set|append|find|list|load|show|report|doctor|audit|detect|guard|gate|promote|schema>
```

| command | purpose | gate / R | notes |
|---|---|---|---|
| `version` / `selftest` | binary + driver sanity | P1 | exit 0 on success |
| `init [path]` | create empty store at path | P1 | sqlite, pure-Go |
| `import [-|file]` | validate JSON record → store | P1 | empty input → exit 1 with usage (R58) |
| `get --<type>=<id> [--section=<f>]` | read one section or whole record | R4 | |
| `set --<type>=<id> --section=<f> --input=<v>` | replace one section | R4, R11 | rejects unknown section (G3); store untouched on reject |
| `append --<type>=<id> --section=<f> --input=<item>` | append to array section | R4, R11 | rejects dup id on id-keyed collections (G4) |
| `find --<type>=<id>` or `find <id>` | graph neighbors via edge join | R2 | not a scan |
| `list [records\|todos] [--where status=open]` | operational lists | R2, R9 | |
| `load <id>` | REHYDRATOR — one-call warm working set | R5 | record + capability + work-items + entry_points; prepends `## Source paths` block at top of bundle when `.devwork/connections.md` has path-ish ```shell fences |
| `show <id>` | DISCOVERABILITY — one-shot human card | N-04 | header + capability/umbrella/tasks/decisions + store freshness; exit 1 not-found / 2 usage |
| `report --<type>=<id> [--format=md\|html]` | render record (SCRIBE → REPORTER) | B2 | replaces hand-written dashboards |
| `doctor` | dangling edges + self-loops | F2 | exit 1 if any |
| `doctor --integrity` | logical-sha pin check (H1) | H1 | exit 1 on drift; advisory when unpinned |
| `audit --scratch [--path=<dir>] [--max-age-days=N]` | report on `.devwork/_scratch/*.md` accretion (R6 visibility) | R6 | always exit 0 (informational); walks up from cwd by default |
| `detect <path>` | stack + db_engine + capability layer | R8 | reads `~/.claude/hooks/lib/detect-*.sh` |
| `schema [--type=<name>]` | list record types (no flag) or dump embedded JSON schema for one type | 1.3.3 | reads embedded `schema/*.schema.json`; unknown type → exit 1 with available list. Pair with `brain set` to know allowed `--section` values upfront (G3) |
| `guard --tool=… --command=… --path=… --cwd=…` | WATCHDOG test mode | R10 | prints JSON decision; exit 0 allow / 2 block |
| `gate --task=<id>` | DONE-GATE | R7 | exit 0 may wrap / exit 2 blocked |
| `promote [--kind=sql] --command=<sql>` | PROMOTER (capture inline → library) | R9 | dry-run by default; `--kind=sql` is the default |
| `promote --kind=template --source=<path> --dest-umbrella=<id>` | PROMOTER for non-SQL artifacts (HTML/MD/JSON) | R9 | moves draft → `<project>/.devwork/<id>/templates/` + writes `INDEX.md`; dry-run unless `--write=1`; refuses clobber without `--force` |

Store resolution: `--store=…` → `$BRAIN_STORE` env → fallback `./.brain/store.db` (R57: read-side commands refuse implicit fallback when missing).

---

## 5. Hooks (the enforcement layer)

Wired in `~/.claude/settings.json` (see `claude/settings.example.json` for the reference shape). Order matters per event.

| event / matcher | hooks (in order) | what they do |
|---|---|---|
| `PreToolUse Bash` | `rtk hook claude` → `validate-bash.sh` → `canonicalize-bash.sh` | inline-SQL nudge (legacy), bash validation |
| `PreToolUse Write\|Edit` | `write-location-warn.sh` | enforce `.devwork/` artifact placement |
| `PreToolUse Bash\|Write\|Edit\|MultiEdit` | **`watchdog.sh`** | WATCHDOG (R10) — the shim |
| `PostToolUse Edit\|Write` | `edit-batch-nudge.sh` | hot-file nudge |
| `UserPromptSubmit` | `multi-step-pause.sh` → `route-graphify.sh` → `session-wrap-detect.sh` | multi-step gate + `/graphify` routing + wrap detection |
| `Stop` | `wosy-stop.sh` → `output-telemetry.sh` | stale-task nudge + `_scratch/` accretion nudge (R6) + telemetry |
| `SessionEnd` | `session-audit.sh` | tool-use tallies + per-file hotspots |
| `SessionStart` | `inject-toc.sh` | TOC injection |

Library helpers (`~/.claude/hooks/lib/`): `detect-db-engine.sh`, `detect-stack.sh`, `walk-up.sh`, `wosy-flags.sh`. These are read by both the shim and the brain binary's `detect` subcommand — single source of truth for capability discovery.

---

## 6. Skills (the protocol vehicles)

`~/.claude/skills/`. Invoke with `/<skill>`.

| skill | trigger / when | writes to |
|---|---|---|
| `wire-up` | new / stale / unprovisioned project | scaffolds `.devwork/`, opt-in flags |
| `interview` | fuzzy topic, "let's frame this" | `specs/<id>.md` + `decisions/<id>.md` |
| `handoff` | context full, day end | self-contained paste-ready prompt |
| `consolidate` | shipped ticket → archive | `_archive/<id>/consolidated.md` (manual only — never hook) |
| `query` | DB read | schema-aware result |
| `graphify` | "where is X", "trace this" | `graphify-out/GRAPH_REPORT.md` + `graph.json` |
| `hq-orchestrator` | `.devwork/` present, exploratory work | dispatches `agent-scout` / `agent-tracer` / `agent-reviewer` / `agent-query` / `agent-server` |
| `context` | "how much context left?" | stdout |
| `constitution-rebuild` | missing `constitution.md` | `.devwork/constitution.next.md` (hand-merge) |

Task kickoff is artifacts-first — when a topic is framed and ready to start, create `tasks/<id>/{status,context,scope}.md` (or the plan/task YAML hierarchy) directly. No skill gates it; the protocol verifies the artifact exists, and the Stop hook nudges when persistent `.devwork/` work lands outside a task folder.

---

## 7. Opt-in enforcement model (`.devwork/wosy.flags`)

Provision-then-enable. **Absent file = fail-open = today's behavior.** Per-class flags only fire when their sanctioned tool already exists locally (so they can't self-lockout).

| key | what it enforces | requires (provisioned-first) |
|---|---|---|
| `wosy_enforce=0` | kill switch — disables enforcement for this project | — |
| `enforce_inline_sql=1` | block raw `mysql/psql/sqlite3 -c` outside `q.sh` | `.devwork/queries/library/` exists |
| `db_engine=<mysql\|pgsql\|sqlite>` | per-engine classifier (informational) | — |
| `enforce_inline_ssh=1` | block raw `ssh host cmd` outside connections doc | `.devwork/connections.md` exists |
| `enforce_kb_scribe=1` | block `Write/Edit/MultiEdit/NotebookEdit` of brain-owned files | `.devwork/brain/store.db` exists |
| `enforce_bash_kb_writes=1` | block Bash redirect / `tee` into KB (R52) | brain store exists |
| `enforce_destructive_kb=1` | block `rm/mv/cp/rsync/find -delete` on KB paths (R54) | brain store exists |
| `expected_logical_sha=<sha1>` | `brain doctor --integrity` pin (H1) | sha derived from `brain list \| shasum` |

### Reference flag files

Two starter profiles ship in `reference-flags/` at the repo root:

- **`reference-flags/full-enforcement.flags`** — full enforcement profile (drop into a real project that has connections + schema + queries/library + brain store):
  ```
  wosy_enforce=1
  enforce_inline_sql=1
  db_engine=pgsql
  enforce_kb_scribe=1
  enforce_bash_kb_writes=1
  enforce_destructive_kb=1
  expected_logical_sha=<your-store-sha>
  ```

- **`reference-flags/brain-home-minimal.flags`** — brain-home dogfood (minimal):
  ```
  wosy_enforce=1
  enforce_kb_scribe=1
  ```

---

## 8. Gates — current status (master §7)

The falsifiable definition of "WOSY is working":

| R | gate | status | proof |
|---|---|---|---|
| R1 | sample project → 1 record, 0 contradictions | ✅ | grounding pilot: ~60 fragments → 1 project + 4 family records |
| R2 | graph edges queryable, not scanned | ✅ | `brain find` uses `Neighbors` join-table |
| R3 | 100 % records carry `updated` + `status` | ✅ | schema-required field |
| R4 | one tool line per section, constant cost | ✅ | `brain get/set/append --section=`; observed ratios 27.8× / 25.1× / 11.6× |
| R5 | working set ≤ 8 reads, 0 re-reads | ✅ | REHYDRATOR (`brain load`); smoke-phase3 §A |
| R6 | md-write share < 10 %, zero new ceremony files | ✅ | mechanism enforced (SCRIBE blocks KB writes); accretion visibility via `brain audit --scratch` wired into `wosy-stop.sh` (nudge on session end when `_scratch/*.md` >`STALE_DAYS` old) |
| R7 | wrap blocked on unmet scope | ✅ | `brain gate` (DONE-GATE) |
| R8 | provisioning by detection | ✅ | `brain detect` |
| R9 | query built once + PROMOTER grows library | ✅ | `brain promote` + `q.sh` + `queries/library/` |
| R10 | WATCHDOG enforced — inline/raw ops blocked | ✅ | v3 §2 / §3 / §16 / §19 / §20 / §21 / §22 / §23 / §24 all green |
| R11 | KB writes — 0 main-terminal diff | ✅ | demonstrated via `Edit tasks/<id>/status.md` blocked |

### Hardening cycle (post-P0–P5)

| id | bug | status |
|---|---|---|
| G1 | `state.yml`/`plan.yml` not in KB list | ✅ |
| G2 | rm/mv into KB unblocked | ✅ |
| G3 | unknown `--section` accepted | ✅ |
| G4 | dup-id `append` accepted | ✅ |
| G5 | bad-store CLI exits 0 | ✅ |
| H1 | byte-sha gate too brittle (VACUUM/schema-add) | ✅ `brain doctor --integrity` |
| R52 | Bash redirect into KB | ✅ |
| R53 | `bash -c` subshell bypass | ✅ |
| R54 | cp / mv / rsync into KB | ✅ |
| R55 | raw `sqlite3` on store | ✅ |
| R56 | symlink bypass | ✅ |
| R57 | CLI exit codes on bad store | ✅ |
| R58 | `brain import` no-args | ✅ |
| R59a | relative-path classifier | ✅ |
| R59b | `NotebookEdit` branch | ✅ |
| R6 (audit) | ceremony-file accretion visibility | ✅ `brain audit --scratch` + `wosy-stop.sh` nudge |
| harness | env-i fragility under sandboxed bash tools | ✅ subshell-unset pattern |

---

## 9. How to verify WOSY is working (smoke recipe)

Run these in order. Each is read-only / non-mutating against live stores. Replace `<YOUR_PROJECT_PATH>` with an actual project that has been onboarded via `/wire-up`.

```bash
# 1. Binary + driver
brain version        # → brain X.Y.Z
brain selftest       # → selftest OK (sqlite driver live)

# 2. Project resolution + working set (REHYDRATOR / R5)
BRAIN_STORE=<YOUR_PROJECT_PATH>/.devwork/brain/store.db brain load <project-id> | head -20

# 3. Graph + dangling edges (doctor)
BRAIN_STORE=<YOUR_PROJECT_PATH>/.devwork/brain/store.db brain doctor

# 4. Logical-sha integrity (H1)
cd <YOUR_PROJECT_PATH>
BRAIN_STORE=<YOUR_PROJECT_PATH>/.devwork/brain/store.db brain doctor --integrity
# → "doctor --integrity OK: logical sha matches pin (<sha>...)"

# 5. WATCHDOG classifier sanity (R10 — should ALL block)
brain guard --tool=Write --path=<YOUR_PROJECT_PATH>/.devwork/STATUS.md --cwd=<YOUR_PROJECT_PATH>
brain guard --tool=Bash  --command="psql -c 'SELECT 1'"             --cwd=<YOUR_PROJECT_PATH>
brain guard --tool=Bash  --command="rm -rf <YOUR_PROJECT_PATH>/.devwork/brain"  --cwd=<YOUR_PROJECT_PATH>

# 6. WATCHDOG classifier sanity (R10 — should ALL allow)
brain guard --tool=Edit --path=<YOUR_PROJECT_PATH>/app/Controllers/X.php   --cwd=<YOUR_PROJECT_PATH>
brain guard --tool=Bash --command="<YOUR_PROJECT_PATH>/.devwork/bin/q.sh queries/library/foo.sql"  --cwd=<YOUR_PROJECT_PATH>

# 7. Hook contract (live)
echo '{"tool_name":"Write","tool_input":{"file_path":"'"$HOME"'/<project>/.devwork/brain/x.md"},"cwd":"'"$HOME"'/<project>"}' \
  | bash ~/.claude/hooks/watchdog.sh; echo "exit=$?"   # → exit 2

# 8. R6 convergence visibility — list stale _scratch/*.md
cd <YOUR_PROJECT_PATH> && brain audit --scratch --max-age-days=7

# 9. Full repo smoke
bash brain/smoke-wosy.sh
```

---

## 10. Open items (what's intentionally NOT done)

| id | item | class | notes |
|---|---|---|---|
| O.4 | external-record → `brain import` (`ledger.yml`, issue trackers) | DEFERRED | unification path; design-only today |
| O.5 | byte-sha harness gate → informational | OPEN | superseded by H1's logical-sha; harness still hard-checks |
| O.7 | LINKER auto-edge extraction from prose | DEFERRED | today edges are explicit |
| O.8 | `subagent-diff` long-write route | DEFERRED | "optional, later" |
| O.9 | R6 convergence — ceremony-file accretion | ✅ DONE | `brain audit --scratch` + `wosy-stop.sh` nudge; visibility, not block |

No OPEN items block real-world rollout.

---

## 11. Project rollout — recipe for a new project

When you want WOSY on a real project, run this sequence:

```bash
# 1. Bootstrap (interactive — discovers stack + db engine + tools)
cd /path/to/project
# In Claude Code: /wire-up

# 2. Confirm detection
brain detect /path/to/project | jq .

# 3. Seed the brain store with the umbrella + project records
mkdir -p .devwork/brain
brain init .devwork/brain/store.db
brain detect /path/to/project | jq '{...umbrella...}' | BRAIN_STORE=.devwork/brain/store.db brain import -
# (project record drafted via /interview → brain import)

# 4. Provision flags — START with kill-switch on + KB-scribe only.
# DO NOT enable enforce_inline_sql until queries/library exists with a real .sql in it.
cat > .devwork/wosy.flags <<EOF
wosy_enforce=1
enforce_kb_scribe=1
EOF

# 5. Smoke: write to a KB file → should be blocked.
echo "x" > .devwork/brain/probe.md && echo "FAIL: not blocked" || echo "OK: blocked"

# 6. Pin the logical sha after the initial seed.
BRAIN_STORE=.devwork/brain/store.db brain doctor --integrity
# Copy the suggested expected_logical_sha=... line into wosy.flags.

# 7. Optionally add more enforcement (one class at a time, provision-then-enable):
# enforce_inline_sql=1    # only if queries/library/ exists with .sql files
# enforce_inline_ssh=1    # only if .devwork/connections.md exists
# enforce_bash_kb_writes=1
# enforce_destructive_kb=1

# 8. (After a day of work) — accretion visibility, surface stale drafts
brain audit --scratch --max-age-days=7
# Wired into ~/.claude/hooks/wosy-stop.sh; nudges on session end automatically.
```

**Roll-back recipe (full disable):** remove `.devwork/wosy.flags` → everything fails open → today's behavior.

---

## 12. Pointers (deep design / evidence)

The deep design rationale, evidence base, and decision records live in your local `.devwork/` after install. Suggested locations:

| pointer | role |
|---|---|
| `<DEVWORK>/decisions/<id>.md` | per-decision logs (append, never edit) |
| `<DEVWORK>/specs/<id>.md` | SRS-like specs from `/interview` |
| `<DEVWORK>/brain/REGISTRY.md` | your local registry (per-installation, populated as you onboard projects) |
| `<DEVWORK>/brain/schema/CONTRACT.md` | structural definition-of-done |
| `<DEVWORK>/research/` | reproducible miners + dashboards (evidence base) |
| `<DEVWORK>/plans/` | session plans + harness scripts |

`<DEVWORK>` defaults to `$HOME/Documents/.devwork/`. Override via `$WOSY_DEVWORK`.

---

## 13. Changelog

See the root **`CHANGELOG.md`** for the user-facing changelog and per-release notes. Per-fix commit history lives in `git log`. This section previously held an embedded mega-changelog; it was moved to the root file to avoid drift.
