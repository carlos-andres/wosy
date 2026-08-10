package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// ddlV2Fixture is the v2 project-layer DDL verbatim as shipped before v3 —
// frozen here so the migration test exercises a real pre-migration store
// even after the live DDL const moves on.
const ddlV2Fixture = `
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE workspace (
  id            TEXT PRIMARY KEY,
  team          TEXT NOT NULL,
  root_path     TEXT NOT NULL,
  created       TEXT NOT NULL,
  updated       TEXT NOT NULL,
  brain_version TEXT NOT NULL
);

CREATE TABLE project (
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
CREATE INDEX idx_project_team   ON project(team);
CREATE INDEX idx_project_status ON project(status);

CREATE TABLE pipeline_stage (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  ord         INTEGER NOT NULL,
  name        TEXT NOT NULL,
  description TEXT,
  status      TEXT NOT NULL CHECK(status IN ('green', 'yellow', 'red')),
  PRIMARY KEY (project_id, ord)
);

CREATE TABLE scope_entry (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL CHECK(kind IN ('file', 'import', 'external', 'feed')),
  value       TEXT NOT NULL,
  health      TEXT NOT NULL CHECK(health IN ('green', 'yellow', 'red')),
  last_seen   TEXT NOT NULL,
  notes       TEXT,
  PRIMARY KEY (project_id, kind, value)
);
CREATE INDEX idx_scope_health ON scope_entry(project_id, health);

CREATE TABLE connection (
  project_id     TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  alias          TEXT NOT NULL,
  kind           TEXT NOT NULL CHECK(kind IN ('mysql', 'postgres', 'sqlite', 'redis', 'ssh', 'http', 'other')),
  target         TEXT NOT NULL,
  credential_ref TEXT,
  notes          TEXT,
  PRIMARY KEY (project_id, alias)
);

CREATE TABLE query_pointer (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  path        TEXT NOT NULL,
  description TEXT,
  PRIMARY KEY (project_id, name)
);

CREATE TABLE runbook_pointer (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  path        TEXT NOT NULL,
  kind        TEXT NOT NULL CHECK(kind IN ('incident', 'procedure', 'diagnostic', 'recovery')),
  PRIMARY KEY (project_id, name)
);

CREATE TABLE encyclopedia_section (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  heading     TEXT NOT NULL,
  body_md     TEXT NOT NULL,
  updated     TEXT NOT NULL,
  PRIMARY KEY (project_id, heading)
);

CREATE TABLE recent_task (
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  plan_id     TEXT NOT NULL,
  task_id     TEXT NOT NULL,
  status      TEXT NOT NULL CHECK(status IN ('pending', 'in-progress', 'blocked', 'done')),
  updated     TEXT NOT NULL,
  PRIMARY KEY (project_id, plan_id, task_id)
);
CREATE INDEX idx_recent_task_updated ON recent_task(project_id, updated DESC);

CREATE TABLE recent_commit (
  project_id      TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  sha             TEXT NOT NULL,
  message         TEXT,
  files_changed_n INTEGER NOT NULL,
  ts              TEXT NOT NULL,
  PRIMARY KEY (project_id, sha)
);
CREATE INDEX idx_recent_commit_ts ON recent_commit(project_id, ts DESC);

CREATE TABLE gotcha (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  severity    TEXT NOT NULL CHECK(severity IN ('info', 'warn', 'error')),
  body_md     TEXT NOT NULL,
  source_ref  TEXT,
  created     TEXT NOT NULL
);
CREATE INDEX idx_gotcha_severity ON gotcha(project_id, severity);

INSERT INTO meta(key, value) VALUES ('schema_version', '2');
`

// ddlV1Fixture is a v1-legacy store verbatim: meta + record/edge/todo only,
// no project layer — frozen so the skip-migration test stays a real v1 file.
const ddlV1Fixture = `
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE record (
  id            TEXT PRIMARY KEY,
  type          TEXT NOT NULL,
  status        TEXT NOT NULL,
  updated       TEXT NOT NULL,
  created       TEXT,
  title         TEXT,
  status_detail TEXT,
  doc           TEXT NOT NULL
);
CREATE INDEX idx_record_type   ON record(type);
CREATE INDEX idx_record_status ON record(status);

CREATE TABLE edge (
  from_id TEXT NOT NULL,
  rel     TEXT NOT NULL,
  to_id   TEXT NOT NULL,
  note    TEXT,
  PRIMARY KEY (from_id, rel, to_id)
);
CREATE INDEX idx_edge_to ON edge(to_id);

CREATE TABLE todo (
  record_id TEXT NOT NULL,
  item_id   TEXT NOT NULL,
  ord       INTEGER NOT NULL,
  descr     TEXT NOT NULL,
  status    TEXT NOT NULL,
  PRIMARY KEY (record_id, item_id)
);
CREATE INDEX idx_todo_status ON todo(status);

INSERT INTO meta(key, value) VALUES ('schema_version', '1');
INSERT INTO record(id, type, status, updated, doc)
  VALUES ('rec-1', 'task', 'open', '2026-06-01T00:00:00Z', '{"id":"rec-1"}');
`

// seedV2Store builds a populated v2 store at dbPath using raw database/sql,
// bypassing Open so no migration runs during setup.
func seedV2Store(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(ddlV2Fixture); err != nil {
		t.Fatalf("apply v2 fixture DDL: %v", err)
	}
	seed := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO workspace(id,team,root_path,created,updated,brain_version) VALUES(?,?,?,?,?,?)`,
			[]any{"ws-1", "platform", "/tmp/ws", "2026-06-08T00:00:00Z", "2026-06-08T00:00:00Z", "2"}},
		{`INSERT INTO project(id,slug,team,display_name,root_path,created,updated,status,cache_version)
		  VALUES(?,?,?,?,?,?,?,?,?)`,
			[]any{"acme-crm", "acme-crm", "platform", "CRM", "/tmp/acme",
				"2026-06-08T00:00:00Z", "2026-06-08T00:00:00Z", "active", "01HZ"}},
		{`INSERT INTO pipeline_stage(project_id,ord,name,description,status) VALUES(?,?,?,?,?)`,
			[]any{"acme-crm", 1, "feed-fetch", "pull CSV", "green"}},
		{`INSERT INTO scope_entry(project_id,kind,value,health,last_seen,notes) VALUES(?,?,?,?,?,?)`,
			[]any{"acme-crm", "file", "workers/upload.php", "yellow", "2026-06-08T00:00:00Z", "active fix"}},
		{`INSERT INTO scope_entry(project_id,kind,value,health,last_seen,notes) VALUES(?,?,?,?,?,?)`,
			[]any{"acme-crm", "feed", "ftp://feed.example/x.csv", "green", "2026-06-08T00:00:00Z", nil}},
		{`INSERT INTO connection(project_id,alias,kind,target,credential_ref,notes) VALUES(?,?,?,?,?,?)`,
			[]any{"acme-crm", "prod-db", "mysql", "db.host:3306/x", "1password://x", "read-only"}},
		{`INSERT INTO query_pointer(project_id,name,path,description) VALUES(?,?,?,?)`,
			[]any{"acme-crm", "q1", "queries/library/q1.sql", "feed delta"}},
		{`INSERT INTO runbook_pointer(project_id,name,path,kind) VALUES(?,?,?,?)`,
			[]any{"acme-crm", "rb", "runbooks/rb.md", "recovery"}},
		{`INSERT INTO encyclopedia_section(project_id,heading,body_md,updated) VALUES(?,?,?,?)`,
			[]any{"acme-crm", "Feeds", "feed notes", "2026-06-08T00:00:00Z"}},
		{`INSERT INTO recent_task(project_id,plan_id,task_id,status,updated) VALUES(?,?,?,?,?)`,
			[]any{"acme-crm", "plan-1", "task-1", "in-progress", "2026-06-08T00:00:00Z"}},
		{`INSERT INTO recent_commit(project_id,sha,message,files_changed_n,ts) VALUES(?,?,?,?,?)`,
			[]any{"acme-crm", "abc1234", "fix feed", 2, "2026-06-08T00:00:00Z"}},
		{`INSERT INTO gotcha(project_id,severity,body_md,source_ref,created) VALUES(?,?,?,?,?)`,
			[]any{"acme-crm", "warn", "FTP host drops idle conns", "runbooks/rb.md", "2026-06-08T00:00:00Z"}},
	}
	for _, s := range seed {
		if _, err := db.Exec(s.q, s.args...); err != nil {
			t.Fatalf("seed %s: %v", s.q, err)
		}
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func hasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, table, column).Scan(&n); err != nil {
		t.Fatalf("table_info %s: %v", table, err)
	}
	return n > 0
}

func schemaVersionOf(t *testing.T, db *sql.DB) string {
	t.Helper()
	var v string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&v); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	return v
}

func bakFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "brain.db.bak-v2-*"))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	return matches
}

func TestOpen_MigratesV2StoreToV3(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "brain.db")
	seedV2Store(t, dbPath)

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open on v2 store: %v", err)
	}
	defer s.Close()

	if v := schemaVersionOf(t, s.DB); v != "3" {
		t.Errorf("schema_version = %q, want \"3\"", v)
	}

	// Data preserved through the table rebuilds, and the tables the migration
	// does not target keep their rows untouched.
	for table, want := range map[string]int{
		"project": 1, "pipeline_stage": 1, "scope_entry": 2,
		"connection": 1, "runbook_pointer": 1,
		"workspace": 1, "query_pointer": 1, "encyclopedia_section": 1,
		"recent_task": 1, "recent_commit": 1, "gotcha": 1,
	} {
		if got := countRows(t, s.DB, table); got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}
	var alias, kind, notes string
	if err := s.DB.QueryRow(
		`SELECT alias, kind, notes FROM connection WHERE project_id='acme-crm'`).Scan(&alias, &kind, &notes); err != nil {
		t.Fatalf("connection row after migration: %v", err)
	}
	if alias != "prod-db" || kind != "mysql" || notes != "read-only" {
		t.Errorf("connection row mangled: alias=%q kind=%q notes=%q", alias, kind, notes)
	}
	var gotchaBody string
	if err := s.DB.QueryRow(
		`SELECT body_md FROM gotcha WHERE project_id='acme-crm'`).Scan(&gotchaBody); err != nil {
		t.Fatalf("gotcha row after migration: %v", err)
	}
	if gotchaBody != "FTP host drops idle conns" {
		t.Errorf("gotcha row mangled: body_md=%q", gotchaBody)
	}
	var taskStatus string
	if err := s.DB.QueryRow(
		`SELECT status FROM recent_task WHERE project_id='acme-crm' AND task_id='task-1'`).Scan(&taskStatus); err != nil {
		t.Fatalf("recent_task row after migration: %v", err)
	}
	if taskStatus != "in-progress" {
		t.Errorf("recent_task row mangled: status=%q", taskStatus)
	}

	// New columns exist.
	for table, col := range map[string]string{
		"pipeline_stage":  "source_quote",
		"scope_entry":     "promoted",
		"runbook_pointer": "purpose",
	} {
		if !hasColumn(t, s.DB, table, col) {
			t.Errorf("%s missing column %s after migration", table, col)
		}
	}

	// Migrated rows get the promoted default.
	var promoted int
	if err := s.DB.QueryRow(
		`SELECT promoted FROM scope_entry WHERE kind='file'`).Scan(&promoted); err != nil {
		t.Fatalf("promoted default: %v", err)
	}
	if promoted != 0 {
		t.Errorf("promoted = %d, want 0", promoted)
	}

	// Widened CHECKs accept the new kinds.
	if _, err := s.DB.Exec(
		`INSERT INTO connection(project_id,alias,kind,target) VALUES('acme-crm','src','path','/Volumes/x/src')`); err != nil {
		t.Errorf("INSERT connection kind='path': %v", err)
	}
	if _, err := s.DB.Exec(
		`INSERT INTO scope_entry(project_id,kind,value,health,last_seen) VALUES('acme-crm','table','dealer_location','green','2026-06-11T00:00:00Z')`); err != nil {
		t.Errorf("INSERT scope_entry kind='table': %v", err)
	}

	// Rebuild kept the scope health index.
	var idx int
	if err := s.DB.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_scope_health'`).Scan(&idx); err != nil {
		t.Fatalf("index lookup: %v", err)
	}
	if idx != 1 {
		t.Errorf("idx_scope_health missing after rebuild")
	}

	if baks := bakFiles(t, dir); len(baks) != 1 {
		t.Errorf("backup files = %v, want exactly one .bak-v2-*", baks)
	}
}

func TestOpen_MigrationRunsOnce(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "brain.db")
	seedV2Store(t, dbPath)

	for i := 0; i < 2; i++ {
		s, err := Open(dbPath)
		if err != nil {
			t.Fatalf("Open #%d: %v", i+1, err)
		}
		s.Close()
	}
	if baks := bakFiles(t, dir); len(baks) != 1 {
		t.Errorf("backup files after re-open = %v, want exactly one (migration must not re-run)", baks)
	}
}

func TestOpen_V1LegacyStoreSkipsMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "brain.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	if _, err := db.Exec(ddlV1Fixture); err != nil {
		db.Close()
		t.Fatalf("apply v1 fixture DDL: %v", err)
	}
	db.Close()

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open on v1 store: %v", err)
	}
	defer s.Close()

	// No project layer pre-Open means the v2->v3 migration must not run.
	if baks := bakFiles(t, dir); len(baks) != 0 {
		t.Errorf("v1 store created backups: %v (migration must not run)", baks)
	}
	if v := schemaVersionOf(t, s.DB); v != "3" {
		t.Errorf("schema_version = %q, want \"3\"", v)
	}
	if got := countRows(t, s.DB, "record"); got != 1 {
		t.Errorf("record rows = %d, want 1 (legacy data must survive)", got)
	}
	// Init created the v3 tables fresh, in their post-migration shape.
	if _, err := s.DB.Exec(
		`INSERT INTO project(id,slug,team,root_path,created,updated,status)
		 VALUES('p1','p1','t','/tmp/p1','2026-06-11T00:00:00Z','2026-06-11T00:00:00Z','active')`); err != nil {
		t.Fatalf("seed project on upgraded v1 store: %v", err)
	}
	if _, err := s.DB.Exec(
		`INSERT INTO scope_entry(project_id,kind,value,health,last_seen) VALUES('p1','table','t1','green','2026-06-11T00:00:00Z')`); err != nil {
		t.Errorf("upgraded v1 store INSERT kind='table': %v", err)
	}
}

func TestOpen_FreshStoreIsV3WithoutBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "brain.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open fresh: %v", err)
	}
	defer s.Close()

	if v := schemaVersionOf(t, s.DB); v != "3" {
		t.Errorf("schema_version = %q, want \"3\"", v)
	}
	if _, err := s.DB.Exec(
		`INSERT INTO project(id,slug,team,root_path,created,updated,status)
		 VALUES('p1','p1','t','/tmp/p1','2026-06-11T00:00:00Z','2026-06-11T00:00:00Z','active')`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := s.DB.Exec(
		`INSERT INTO connection(project_id,alias,kind,target) VALUES('p1','src','path','/x')`); err != nil {
		t.Errorf("fresh store INSERT kind='path': %v", err)
	}
	if _, err := s.DB.Exec(
		`INSERT INTO scope_entry(project_id,kind,value,health,last_seen) VALUES('p1','table','t1','green','2026-06-11T00:00:00Z')`); err != nil {
		t.Errorf("fresh store INSERT kind='table': %v", err)
	}
	if baks := bakFiles(t, dir); len(baks) != 0 {
		t.Errorf("fresh store created backups: %v", baks)
	}
}
