---
name: wire-up
description: Pre-flight bootstrap before real work in any project. Tier-aware (4 tiers: STATIC / APP / APP+DATA / UMBRELLA, classified by detected stack + db + project count). Agnostic DETECT (stack/db/remote), CORE scaffold (.devwork + /interview kickoff + keyed STATUS + SRS/plan templates), CONDITIONAL tooling (db runner / build-test / ssh per Tier), per-project OPT-IN enforcement flags (.devwork/wosy.flags, provision-then-enable, fail-open), Tier 4 mini-workshop (per-team `brain init --team` + lazy per-project folders + master.md scaffold), Solomonic reset on existing .devwork/. Then a main-line lock with a falsifiable done_when. Trigger phrases — "wire up", "wire-up", "pre-flight", "bootstrap workspace", "clean house and set up" — or before starting work in a new / stale / unprovisioned project.
when_to_use: User types `/wire-up` or says "wire up" / "pre-flight" / "bootstrap workspace" / "clean house and set up". Suggest invoking when starting work in a new / stale / unprovisioned project. Manual only — high-cost pre-flight requires explicit user intent.
disable-model-invocation: true
---

# Wire-up (agnostic · 4-tier · flag-aware · brain-aware · pre-flight)

Four tiers, one scaffold engine:

| tier | classifier | scaffold focus |
|---|---|---|
| **STATIC**    | only `index.html` / no app stack / no db | CORE only (.devwork shell, STATUS, SRS) |
| **APP**       | one project with a build stack (node / php-plain / go / rust / swift / python) and NO db | CORE + build/test pack + (remote if detected) |
| **APP+DATA**  | one project, build stack + db (mysql / postgres / sqlite) | CORE + build pack + db runner pack + (remote if detected) |
| **UMBRELLA**  | multi-project workspace (≥2 projects under a team root, or a `<team>/` dir at the top) | CORE + per-team `brain init --team` + per-project lazy mini-workshop folders + Tier 4 master.md scaffold |

Three layers (apply per tier):
- **CORE (ALWAYS, stack-agnostic):** `.devwork` scaffold + `/interview` kickoff + keyed(yaml) STATUS + SRS/plan templates.
- **TOOLING (CONDITIONAL, detected):** db runner / build-test / ssh — only what DETECT proves the repo uses.
- **ENFORCEMENT (OPT-IN):** per-project `.devwork/wosy.flags`; absent = off (fail-open). Provision → smoke → enable — write the flag only AFTER the runner exists AND a trivial query through it succeeds; it can never self-lockout.

Operating rules apply to every step — see `## Hard rules` at the end. Run phases below in order. Stop at the Phase B lock.

## When this fires

User types `/wire-up` or says "wire up" / "pre-flight" / "bootstrap workspace" / "clean house and set up". Suggest invoking when starting work in a new / stale / unprovisioned project. Manual only — high-cost pre-flight requires explicit user intent.

## Workflow

### Phase A — Diagnose (read-only; DETECT + TIER classification mandatory + FIRST; emit ONE block)

- **A1 orient:** nearest `.devwork` (walk up from cwd); project; CLAUDE.md/constitution TOC only. No `.devwork` → CREATE-PROPERLY.
- **A2 DETECT:** for the PRIMARY stack run `bash ~/.claude/hooks/lib/detect-stack.sh <project-dir>` (deterministic; recurses with bounded depth so nested markers like `app/composer.json` are found). PRIORITY ORDER (first match = PRIMARY):
  1 `*.xcodeproj/*.xcworkspace` → swift-xcode (else `Package.swift` only → swift-spm) · 2 `artisan`+`composer.json` → php-laravel · 3 `composer.json` (no artisan) → php-plain · 4 `*.php` (no composer) → php-legacy · 5 `Package.swift` → swift-spm · 6 `Cargo.toml` → rust (WORKSPACE ROOT = ONE project) · 7 `go.mod` → go · 8 `build.gradle(.kts)` → android · 9 `manifest.json`+content_scripts → browser-ext · 10 `package.json` → node · 11 `pyproject/requirements` → python · 12 only `index.html` → static · 13 none/empty/media → none (CORE only). Dockerfile/compose = sub-signal, never primary.
  - **db (ORTHOGONAL):** run `bash ~/.claude/hooks/lib/detect-db-engine.sh <project-dir>` → echoes `mysql|pgsql|sqlite|` (empty=unknown). It reads `.env DB_CONNECTION` / `DATABASE_URL` scheme / compose db image / `.devwork/connections.md` (re-entry truth) / `*.sqlite` — config-is-truth, and safe on gitignored `.env`. `*.xcdatamodeld`/SwiftData → coredata (app-managed, NOT a q.sh target). Empty after the helper → unknown (ASK before db pack).
  - **remote:** URLSession/networking · fastlane/ · deploy script · documented ssh alias → yes|no.
  - **build:** the REAL run/test cmd (xcodebuild -scheme / swift test / npm test / php artisan test / cargo test / go test).
  - **TIER:** classify per the table above. >1 project under cwd (subdirs each with their own stack marker) OR `<team>/projects/<slug>/` shape → UMBRELLA. Else stack+db → APP+DATA / APP / STATIC.
  → `capability {tier, stack(+extras), db, remote, build}`. Everything below keys off it.
- **A3 house:** tasks/* status mtime + has-consolidated.md; FLAG abandoned, stale>7d, dup handoffs, off-subfolder md, _scratch>7d. **For UMBRELLA tier:** also list per-project folders under `<team>/projects/*/` + their `master.md` presence.
- **A4 fit:** CORE present? (scaffold + STATUS + SRS). Per PRESENT capability, matching tooling wired? db!=none→runner pack in the dialect; build→build/test shortcut; remote→ssh. db=none|coredata→q.sh N/A; remote=no→ssh N/A. **Tier 4 additions:** is `<team>/brain.db` present (`ls .devwork/<team>/brain.db`)? Are per-project `master.md` files present + non-empty? Read `.devwork/wosy.flags` → report `{wosy_enforce, enforce_inline_sql, db_engine}` (absent=off).
- **A4b citation smoke (Tier 4):** every repo path cited in each project's `master.md` must resolve against the live tree — extract backticked `repo/...` citations, strip `:line` / `::method` suffixes, expand `{a,b}` braces and globs, `test -e` each. Any miss = KB drift: fix `master.md` first, then re-run `brain build <project>` (scope_entry rows built from stale paths are worse than none).
- **A5 state:** memory banners >7d? drift? improvements-of-record active vs not-wired?

Emit:
```
DIAGNOSIS:
  orient:     <project · .devwork path>
  tier:       <STATIC|APP|APP+DATA|UMBRELLA>
  capability: {stack:<x(+extras)>, db:<engine|none|coredata|unknown>, remote:<y|n>, build:<cmd>}
  core:       [scaffold|STATUS|SRS: present|missing]
  house:      [<flags>]
  fit:        [<tool: present|missing|N/A per capability>]
  brain:      [<team>/brain.db: present|missing · projects: <N> registered · master.md: <K>/<N> filled]   # Tier 4 only
  flags:      {wosy_enforce:<v|unset>, enforce_inline_sql:<v|unset>, db_engine:<v|unset>}
  state:      [<stale/drift flags>]
```

### Solomonic reset (offer when A1 finds an EXISTING `.devwork/` with stale/abandoned shape)

When A3 surfaces ≥3 abandoned tasks OR ≥7-day-stale STATUS OR shape doesn't match the new C-02/C-03 contract (legacy `tasks/<id>/` only, no `plans/<plan-id>/`):

```
SOLOMONIC RESET (proposed):
  archive:  .devwork/ → .devwork.archived-<date>/   (full tree preserved)
  rescue:   [connections.md, queries/library/, custom bin/*, <named live tasks>]
  create:   fresh .devwork/ scaffold per the tier above
ASK: lock / edit / skip?
```

Dry-run ALWAYS first. Show the full archive tree + rescue selection. User must type the project name (or `confirm`) before any `mv`. No silent overwrite.

### Create properly (only when A1 finds NO .devwork)

- **CORE (always)** — mkdir -p idempotent, NEVER overwrite an existing file: `.devwork/{tasks,plans,interview,research,flows,schema,_scratch,_archive}/` + `.devwork/STATUS.md` (yaml seed: project · date · purpose · "fresh") + `.devwork/templates/{srs.md, plan.md}`. (Note: `specs/`, `decisions/`, `intake/` are NOT created — absorbed into `plans/<plan-id>/plan.yml.open_questions[]` + `.devwork/interview/<id>.md` per C-02/C-03 kill list.)
- **TOOLING** — provision STRICTLY by the A2 profile: db=mysql→q.sh `mysql --login-path=<x> --database=<db>` · db=pgsql→q.sh `psql -d <db>` (or `"$DATABASE_URL"`) · db=sqlite→q.sh `sqlite3 <path>` (+ connections.md naming engine+cmd, queries/library/INDEX.md). db=none|coredata→NO db pack. db=unknown→ASK. swift-xcode→REUSE scripts/build.sh else bin/{build,test}.sh wrapping `xcodebuild -scheme <scheme w/ Testables>` + schemes.md. swift-spm→swift build/test. node→bin shortcuts from package.json scripts. rust→cargo. go→go. python→per runner. static|none→NO build pack. remote=yes→ssh helper; remote=no→none.
- **TIER 4 (UMBRELLA only) — mini-workshop:** run `brain init --team=<team>` to create `.devwork/<team>/brain.db` (11 project-rooted tables + legacy record/edge/todo/meta per C-03 Fork 1) + `.devwork/<team>/projects/` dir. For each detected project under `<team>/projects/*/`:
  - scaffold the LAZY per-project folder: `master.md` (from template at `claude/skills/wire-up/templates/master.md`) + `status.md` (yaml seed) + `INDEX.md`. Other folders (`runbooks/`, `traces/`, `templates/`) are LAZY — created on first use, not at scaffold time.
  - INSERT the project row into `brain.db.project` (id / slug / team / display_name / root_path / status='active').
  - PROPOSE: "hand-fill `<team>/projects/<slug>/master.md` for the factory-manual pattern (pipeline + scope + gotchas), then run `brain build <slug>` to extract structure into brain.db". Do NOT auto-fill master.md — user prose is the input.
- **FLAGS (provision → SMOKE → enable)** — after the db runner pack exists, SMOKE-TEST it before trusting/flagging: run the runner on a trivial read (`q.sh` on a `SELECT 1` .sql / `psql -c 'SELECT 1'` / `sqlite3 <db> .tables`) and confirm exit 0. ONLY then write `.devwork/wosy.flags` with `enforce_inline_sql=1` + `db_engine="$(bash ~/.claude/hooks/lib/detect-db-engine.sh .)"` (DERIVED from config, never hand-typed — config-is-truth). If the smoke test fails, fix the runner first — never flag a runner you haven't proven works. db=none/coredata/unknown→no SQL flag. Never enable before the runner.
- PROPOSE the tree, ask "lock/edit?". On lock, create. Stale `.archive_devwork/`/`_archive/` → LIST, ask to fold in (no auto-move). No CLAUDE.md/constitution → offer `constitution-rebuild` FILLED FROM DETECTED CONFIG not prose. VERIFY: list created vs skipped. → Phase B (type: setup).

### Phase B — Main-line lock (gate: never a main_line without a done_when)

Classify SETUP|WORK. PROPOSE one: `PROPOSED {type, main_line, done_when (provable by a command), deferred:[...]}`. Ask once "lock/edit?". Custom main_line w/o done_when → reject, help write one. STOP until locked. Touch nothing.

### Phase C — Execute (only after lock AND explicit "go")

Keyed PLAN from main_line. Show it. STOP for "go" before any delete/move/overwrite. Work in order; destructive step → show diff/list, confirm. Non-done_when → deferred. If you provision a db runner here, set `.devwork/wosy.flags` AFTER it exists. **Tier 4:** if a master.md was hand-filled this phase, end with `brain build <slug>` invocation. done_when looks met → VERIFY each by running its command; report pass/fail. End `STATUS {main_line, done_when:[cond:pass/fail], changed:[...], deferred:[...], next}`.

Start at Phase A. DETECT + TIER first. Stop at the Phase B lock.

## Hard rules

- Terse. KEYED blocks only (key: value). Never prose paragraphs. PROPOSE one + ask "lock/edit?", no pickers.
- TIER FIRST: don't propose any TOOLING/FLAGS until Tier is classified. Tier 4 unlocks the brain mini-workshop; Tiers 1–3 never touch `brain init --team`.
- AGNOSTIC: dig first, apply only what fits. no DB→no SQL tooling. no remote→no ssh. not mysql→the DETECTED engine. Never impose a tool/blocker the repo lacks.
- ENFORCEMENT IS OPT-IN: rules live in `.devwork/wosy.flags` (absent = off). Provision → SMOKE → enable: set `enforce_inline_sql=1` + `db_engine` ONLY after the runner pack exists AND a trivial smoke query through it returns exit 0.
- CONFIG IS TRUTH: read pbxproj/composer.json/.env/package.json as authoritative OVER prose docs. Flag doc drift.
- DON'T CLOBBER: detect existing runners (scripts/build.sh, bin/*, package.json scripts) and REUSE — never overwrite. Same for `<team>/brain.db` — if present, `brain init --team` is a no-op (CREATE TABLE IF NOT EXISTS handles the schema delta).
- SOLOMONIC RESET is destructive: dry-run + show + confirm-by-name. Never silent.
- Phase A read-only. Confirm before delete/move/overwrite. Idempotent. Verify by running.
- Scope guard: work outside main_line → deferred, don't start it.
