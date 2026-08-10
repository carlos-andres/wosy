-- brain.db schema v3 — per-team / per-project tables (additive to legacy)
-- Canonical reference. Mirror lives in brain/internal/store/store.go as const ddlV3.
-- If you edit one, edit both. Hook to enforce sync is a Phase III backlog item.
--
-- v2 -> v3 (migrated in store.go migrateToV3, table rebuild for the CHECK changes):
--   connection.kind        + 'path'
--   scope_entry.kind       + 'table'
--   scope_entry.promoted   INTEGER NOT NULL DEFAULT 0 (boolean)
--   pipeline_stage.source_quote TEXT
--   runbook_pointer.purpose     TEXT
--
-- Path conventions:
--   per-team:      .devwork/<team>/brain.db
--   workspace:     .devwork/brain.db   (single-project workspaces, fallback)
--
-- Coexists with legacy ddl in store.go (CREATE TABLE IF NOT EXISTS is idempotent).
-- Legacy `record / edge / todo / meta` tables stay verbatim; existing CLI verbs
-- (get/set/import/promote/list/report) operate on them unchanged.
--
-- All timestamps: ISO 8601 UTC strings.

PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

-- ===========================================================================
-- IDENTITY LAYER (workspace-level)
-- ===========================================================================

CREATE TABLE IF NOT EXISTS workspace (
  id            TEXT PRIMARY KEY,
  team          TEXT NOT NULL,
  root_path     TEXT NOT NULL,
  created       TEXT NOT NULL,
  updated       TEXT NOT NULL,
  brain_version TEXT NOT NULL
);

-- ===========================================================================
-- PROJECT LAYER (one row per project under <team>/projects/)
-- ===========================================================================

CREATE TABLE IF NOT EXISTS project (
  id            TEXT PRIMARY KEY,
  slug          TEXT NOT NULL UNIQUE,
  team          TEXT NOT NULL,
  display_name  TEXT,
  root_path     TEXT NOT NULL,
  created       TEXT NOT NULL,
  updated       TEXT NOT NULL,
  status        TEXT NOT NULL CHECK(status IN ('active', 'paused', 'archived')),
  cache_version TEXT
);

CREATE INDEX IF NOT EXISTS idx_project_team   ON project(team);
CREATE INDEX IF NOT EXISTS idx_project_status ON project(status);

-- ===========================================================================
-- PIPELINE STAGES
-- ===========================================================================

-- source_quote: verbatim prose sentence the stage was extracted from, so a
-- reader can trace structure back to master.md without re-running the builder.
CREATE TABLE IF NOT EXISTS pipeline_stage (
  project_id   TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  ord          INTEGER NOT NULL,
  name         TEXT NOT NULL,
  description  TEXT,
  status       TEXT NOT NULL CHECK(status IN ('green', 'yellow', 'red')),
  source_quote TEXT,
  PRIMARY KEY (project_id, ord)
);

-- ===========================================================================
-- SCOPE — what files/imports/feeds/tables the project touches + their health
-- ===========================================================================

-- promoted: 0/1 boolean — entry was verified/promoted by a human session, not
-- just extracted from prose.
CREATE TABLE IF NOT EXISTS scope_entry (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL CHECK(kind IN ('file', 'import', 'external', 'feed', 'table')),
  value       TEXT NOT NULL,
  health      TEXT NOT NULL CHECK(health IN ('green', 'yellow', 'red')),
  last_seen   TEXT NOT NULL,
  notes       TEXT,
  promoted    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (project_id, kind, value)
);

CREATE INDEX IF NOT EXISTS idx_scope_health ON scope_entry(project_id, health);

-- ===========================================================================
-- CONNECTIONS — DB / SSH / API endpoints / filesystem paths used by the project
-- ===========================================================================

CREATE TABLE IF NOT EXISTS connection (
  project_id     TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  alias          TEXT NOT NULL,
  kind           TEXT NOT NULL CHECK(kind IN ('mysql', 'postgres', 'sqlite', 'redis', 'ssh', 'http', 'path', 'other')),
  target         TEXT NOT NULL,
  credential_ref TEXT,
  notes          TEXT,
  PRIMARY KEY (project_id, alias)
);

-- ===========================================================================
-- QUERIES — pointers to canonical queries in .devwork/queries/library/
-- ===========================================================================

CREATE TABLE IF NOT EXISTS query_pointer (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  path        TEXT NOT NULL,
  description TEXT,
  PRIMARY KEY (project_id, name)
);

-- ===========================================================================
-- RUNBOOKS — pointers to runbook .md files
-- ===========================================================================

-- purpose: one-line "when to reach for this" so the bundle is scannable
-- without opening the runbook file.
CREATE TABLE IF NOT EXISTS runbook_pointer (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  path        TEXT NOT NULL,
  kind        TEXT NOT NULL CHECK(kind IN ('incident', 'procedure', 'diagnostic', 'recovery')),
  purpose     TEXT,
  PRIMARY KEY (project_id, name)
);

-- ===========================================================================
-- ENCYCLOPEDIA — extracted sections of <project>/encyclopedia.md
-- ===========================================================================

CREATE TABLE IF NOT EXISTS encyclopedia_section (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  heading     TEXT NOT NULL,
  body_md     TEXT NOT NULL,
  updated     TEXT NOT NULL,
  PRIMARY KEY (project_id, heading)
);

-- ===========================================================================
-- RECENT TASKS — joined to plan.yml/task.yml on disk
-- ===========================================================================

CREATE TABLE IF NOT EXISTS recent_task (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  plan_id     TEXT NOT NULL,
  task_id     TEXT NOT NULL,
  status      TEXT NOT NULL CHECK(status IN ('pending', 'in-progress', 'blocked', 'done')),
  updated     TEXT NOT NULL,
  PRIMARY KEY (project_id, plan_id, task_id)
);

CREATE INDEX IF NOT EXISTS idx_recent_task_updated ON recent_task(project_id, updated DESC);

-- ===========================================================================
-- RECENT COMMITS — git commits touching project scope
-- ===========================================================================

CREATE TABLE IF NOT EXISTS recent_commit (
  project_id      TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  sha             TEXT NOT NULL,
  message         TEXT,
  files_changed_n INTEGER NOT NULL,
  ts              TEXT NOT NULL,
  PRIMARY KEY (project_id, sha)
);

CREATE INDEX IF NOT EXISTS idx_recent_commit_ts ON recent_commit(project_id, ts DESC);

-- ===========================================================================
-- GOTCHAS — durable warnings surfaced into context
-- ===========================================================================

CREATE TABLE IF NOT EXISTS gotcha (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  severity    TEXT NOT NULL CHECK(severity IN ('info', 'warn', 'error')),
  body_md     TEXT NOT NULL,
  source_ref  TEXT,
  created     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gotcha_severity ON gotcha(project_id, severity);
