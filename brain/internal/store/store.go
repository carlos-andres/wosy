// Package store is the brain's SQLite-backed memory. The `doc` column holds the
// full canonical JSON of each record (single source of truth); `edge` and `todo`
// are derived index tables rebuilt on every write, so graph queries (R2) and
// operational lists (R9) are indexed SQL, never a scan. Substrate is private:
// callers see only the get/set/append/find/list interface.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

const SchemaVersion = 3

// ddlV3: per-team brain.db gets the 10 project-rooted tables on TOP of the
// legacy record/edge/todo/meta. Applied on every Open (CREATE TABLE IF NOT
// EXISTS = idempotent); v2 stores are rebuilt first by migrateToV3 because
// IF NOT EXISTS cannot widen the kind CHECKs or add columns.
// Canonical text mirror at brain/sql/schema.sql — keep in sync.
const ddlV3 = `
CREATE TABLE IF NOT EXISTS workspace (
  id            TEXT PRIMARY KEY,
  team          TEXT NOT NULL,
  root_path     TEXT NOT NULL,
  created       TEXT NOT NULL,
  updated       TEXT NOT NULL,
  brain_version TEXT NOT NULL
);

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

CREATE TABLE IF NOT EXISTS pipeline_stage (
  project_id   TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  ord          INTEGER NOT NULL,
  name         TEXT NOT NULL,
  description  TEXT,
  status       TEXT NOT NULL CHECK(status IN ('green', 'yellow', 'red')),
  source_quote TEXT,
  PRIMARY KEY (project_id, ord)
);

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

CREATE TABLE IF NOT EXISTS connection (
  project_id     TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  alias          TEXT NOT NULL,
  kind           TEXT NOT NULL CHECK(kind IN ('mysql', 'postgres', 'sqlite', 'redis', 'ssh', 'http', 'path', 'other')),
  target         TEXT NOT NULL,
  credential_ref TEXT,
  notes          TEXT,
  PRIMARY KEY (project_id, alias)
);

CREATE TABLE IF NOT EXISTS query_pointer (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  path        TEXT NOT NULL,
  description TEXT,
  PRIMARY KEY (project_id, name)
);

CREATE TABLE IF NOT EXISTS runbook_pointer (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  path        TEXT NOT NULL,
  kind        TEXT NOT NULL CHECK(kind IN ('incident', 'procedure', 'diagnostic', 'recovery')),
  purpose     TEXT,
  PRIMARY KEY (project_id, name)
);

CREATE TABLE IF NOT EXISTS encyclopedia_section (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  heading     TEXT NOT NULL,
  body_md     TEXT NOT NULL,
  updated     TEXT NOT NULL,
  PRIMARY KEY (project_id, heading)
);

CREATE TABLE IF NOT EXISTS recent_task (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  plan_id     TEXT NOT NULL,
  task_id     TEXT NOT NULL,
  status      TEXT NOT NULL CHECK(status IN ('pending', 'in-progress', 'blocked', 'done')),
  updated     TEXT NOT NULL,
  PRIMARY KEY (project_id, plan_id, task_id)
);
CREATE INDEX IF NOT EXISTS idx_recent_task_updated ON recent_task(project_id, updated DESC);

CREATE TABLE IF NOT EXISTS recent_commit (
  project_id      TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  sha             TEXT NOT NULL,
  message         TEXT,
  files_changed_n INTEGER NOT NULL,
  ts              TEXT NOT NULL,
  PRIMARY KEY (project_id, sha)
);
CREATE INDEX IF NOT EXISTS idx_recent_commit_ts ON recent_commit(project_id, ts DESC);

CREATE TABLE IF NOT EXISTS gotcha (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  severity    TEXT NOT NULL CHECK(severity IN ('info', 'warn', 'error')),
  body_md     TEXT NOT NULL,
  source_ref  TEXT,
  created     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gotcha_severity ON gotcha(project_id, severity);
`

const ddl = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- One canonical record per entity (F1/R1). Projected columns stay in sync with
-- doc on write so type/status/updated queries are indexed (R3/R9).
CREATE TABLE IF NOT EXISTS record (
  id            TEXT PRIMARY KEY,
  type          TEXT NOT NULL,
  status        TEXT NOT NULL,
  updated       TEXT NOT NULL,
  created       TEXT,
  title         TEXT,
  status_detail TEXT,
  doc           TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_record_type   ON record(type);
CREATE INDEX IF NOT EXISTS idx_record_status ON record(status);

-- Traversable graph (F2/R2): "what connects to X" = a query, not a scan.
CREATE TABLE IF NOT EXISTS edge (
  from_id TEXT NOT NULL,
  rel     TEXT NOT NULL,
  to_id   TEXT NOT NULL,
  note    TEXT,
  PRIMARY KEY (from_id, rel, to_id)
);
CREATE INDEX IF NOT EXISTS idx_edge_to ON edge(to_id);

-- Operational to-do index (R9): list open work across records without parsing docs.
CREATE TABLE IF NOT EXISTS todo (
  record_id TEXT NOT NULL,
  item_id   TEXT NOT NULL,
  ord       INTEGER NOT NULL,
  descr     TEXT NOT NULL,
  status    TEXT NOT NULL,
  PRIMARY KEY (record_id, item_id)
);
CREATE INDEX IF NOT EXISTS idx_todo_status ON todo(status);
`

// migrationV2ToV3: widening a CHECK requires a table rebuild, so connection and
// scope_entry are recreated and recopied (the rebuild drops idx_scope_health,
// which is recreated after the rename); the rest are plain ADD COLUMN.
const migrationV2ToV3 = `
CREATE TABLE connection_v3 (
  project_id     TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  alias          TEXT NOT NULL,
  kind           TEXT NOT NULL CHECK(kind IN ('mysql', 'postgres', 'sqlite', 'redis', 'ssh', 'http', 'path', 'other')),
  target         TEXT NOT NULL,
  credential_ref TEXT,
  notes          TEXT,
  PRIMARY KEY (project_id, alias)
);
INSERT INTO connection_v3 (project_id, alias, kind, target, credential_ref, notes)
  SELECT project_id, alias, kind, target, credential_ref, notes FROM connection;
DROP TABLE connection;
ALTER TABLE connection_v3 RENAME TO connection;

CREATE TABLE scope_entry_v3 (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL CHECK(kind IN ('file', 'import', 'external', 'feed', 'table')),
  value       TEXT NOT NULL,
  health      TEXT NOT NULL CHECK(health IN ('green', 'yellow', 'red')),
  last_seen   TEXT NOT NULL,
  notes       TEXT,
  promoted    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (project_id, kind, value)
);
INSERT INTO scope_entry_v3 (project_id, kind, value, health, last_seen, notes)
  SELECT project_id, kind, value, health, last_seen, notes FROM scope_entry;
DROP TABLE scope_entry;
ALTER TABLE scope_entry_v3 RENAME TO scope_entry;
CREATE INDEX IF NOT EXISTS idx_scope_health ON scope_entry(project_id, health);

ALTER TABLE pipeline_stage ADD COLUMN source_quote TEXT;
ALTER TABLE runbook_pointer ADD COLUMN purpose TEXT;
`

type Store struct{ DB *sql.DB }

// Open opens (or creates) the store file and ensures the schema exists.
// Stores stamped below v3 that already carry the project layer are migrated
// in place (with a file backup) before the idempotent DDL applies.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{DB: db}
	if err := s.migrateToV3(path); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate v2->v3: %w", err)
	}
	if err := s.Init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Init applies the DDL (idempotent) and stamps the schema version. Applies
// legacy `ddl` first (record/edge/todo/meta) then `ddlV3` (10 project-rooted
// tables). Both are CREATE TABLE IF NOT EXISTS, so fresh and v1 stores get
// the v3 shape directly without migration; the projection columns on
// `record` are unchanged.
func (s *Store) Init() error {
	if _, err := s.DB.Exec(ddl); err != nil {
		return fmt.Errorf("apply schema (v1): %w", err)
	}
	if _, err := s.DB.Exec(ddlV3); err != nil {
		return fmt.Errorf("apply schema (v3): %w", err)
	}
	_, err := s.DB.Exec(
		`INSERT INTO meta(key,value) VALUES('schema_version',?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		fmt.Sprint(SchemaVersion))
	return err
}

// schemaState reads the stamped schema_version plus whether the project layer
// exists. A db without a meta table (fresh or foreign file) reports version 0;
// a non-integer stamp is corruption and errors rather than reading as v0.
func (s *Store) schemaState() (version int, hasProjectLayer bool, err error) {
	var n int
	if err := s.DB.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&n); err != nil {
		return 0, false, err
	}
	if n == 0 {
		return 0, false, nil
	}
	var v string
	switch err := s.DB.QueryRow(
		`SELECT value FROM meta WHERE key='schema_version'`).Scan(&v); err {
	case nil:
		version, err = strconv.Atoi(v)
		if err != nil {
			return 0, false, fmt.Errorf("corrupt schema_version stamp %q: %w", v, err)
		}
	case sql.ErrNoRows:
	default:
		return 0, false, err
	}
	if err := s.DB.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='project'`).Scan(&n); err != nil {
		return 0, false, err
	}
	return version, n > 0, nil
}

// migrateToV3 runs migrationV2ToV3 in one transaction against stores stamped
// below v3 that already have the project layer; everything else is a no-op
// (Init creates the v3 shape directly). A timestamped copy of the db file is
// taken before any write so a bad migration is recoverable by hand.
func (s *Store) migrateToV3(path string) error {
	version, hasProjectLayer, err := s.schemaState()
	if err != nil {
		return err
	}
	if version >= SchemaVersion || !hasProjectLayer {
		return nil
	}
	// Fold pending WAL frames into the main file so the copy is complete; a
	// busy checkpoint would leave the backup stale, so abort and let the user retry.
	var busy, walFrames, checkpointed int
	if err := s.DB.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &walFrames, &checkpointed); err != nil {
		return fmt.Errorf("checkpoint before backup: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("checkpoint before backup: db busy (%d/%d WAL frames checkpointed); retry when no other reader holds the store", checkpointed, walFrames)
	}
	backup := fmt.Sprintf("%s.bak-v2-%s", path, time.Now().UTC().Format("20060102T150405Z"))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read for backup: %w", err)
	}
	if err := os.WriteFile(backup, data, 0o644); err != nil {
		return fmt.Errorf("write backup %s: %w", backup, err)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(migrationV2ToV3); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO meta(key,value) VALUES('schema_version',?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		fmt.Sprint(SchemaVersion)); err != nil {
		return fmt.Errorf("stamp schema_version: %w", err)
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.DB.Close() }
