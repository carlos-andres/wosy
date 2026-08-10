package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wosy.local/brain/internal/store"
)

// seedProject inserts a minimal project row into the per-team brain.db plus
// optional rows in the satellite tables. Returns the opened store handle so
// the test can add more rows if needed.
func seedProject(t *testing.T, dbPath, projectID, team string) *store.Store {
	t.Helper()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	if _, err := s.DB.Exec(
		`INSERT INTO project(id,slug,team,display_name,root_path,created,updated,status,cache_version)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		projectID, projectID, team, "Test Project", "/tmp/test-"+projectID,
		"2026-06-08T00:00:00Z", "2026-06-08T00:00:00Z", "active", "01HZ-TEST",
	); err != nil {
		s.Close()
		t.Fatalf("seed project: %v", err)
	}
	return s
}

func TestLoadProjectBundle_HappyPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	s := seedProject(t, dbPath, "acme-crm", "platform")
	defer s.Close()

	// One row per satellite table — covers every loader in cli_load_bundle.go.
	exec := func(q string, args ...any) {
		if _, err := s.DB.Exec(q, args...); err != nil {
			t.Fatalf("seed %s: %v", q, err)
		}
	}
	exec(`INSERT INTO pipeline_stage(project_id,ord,name,description,status) VALUES(?,?,?,?,?)`,
		"acme-crm", 1, "feed-fetch", "pull CSV", "green")
	exec(`INSERT INTO scope_entry(project_id,kind,value,health,last_seen,notes) VALUES(?,?,?,?,?,?)`,
		"acme-crm", "file", "workers/upload.php", "yellow", "2026-06-08T00:00:00Z", "active fix")
	exec(`INSERT INTO connection(project_id,alias,kind,target,credential_ref,notes) VALUES(?,?,?,?,?,?)`,
		"acme-crm", "prod-db", "mysql", "db.host:3306/x", "1password://x", nil)
	exec(`INSERT INTO query_pointer(project_id,name,path,description) VALUES(?,?,?,?)`,
		"acme-crm", "qname", "queries/q.sql", "test query")
	exec(`INSERT INTO runbook_pointer(project_id,name,path,kind) VALUES(?,?,?,?)`,
		"acme-crm", "rb", "runbooks/rb.md", "recovery")
	exec(`INSERT INTO encyclopedia_section(project_id,heading,body_md,updated) VALUES(?,?,?,?)`,
		"acme-crm", "Section", "body", "2026-06-08T00:00:00Z")
	exec(`INSERT INTO recent_task(project_id,plan_id,task_id,status,updated) VALUES(?,?,?,?,?)`,
		"acme-crm", "plan-1", "task-A", "in-progress", "2026-06-08T00:00:00Z")
	exec(`INSERT INTO recent_commit(project_id,sha,message,files_changed_n,ts) VALUES(?,?,?,?,?)`,
		"acme-crm", "abc1234", "fix(x)", 2, "2026-06-08T00:00:00Z")
	exec(`INSERT INTO gotcha(project_id,severity,body_md,source_ref,created) VALUES(?,?,?,?,?)`,
		"acme-crm", "warn", "watch out", "file:1", "2026-06-08T00:00:00Z")

	bundle, isProject, err := loadProjectBundle(s, "acme-crm")
	if err != nil {
		t.Fatalf("loadProjectBundle: %v", err)
	}
	if !isProject {
		t.Fatalf("expected isProject=true for acme-crm")
	}

	// Validate every required section is present + non-nil.
	required := []string{
		"cache_version", "identity", "pipeline_stages", "scope",
		"connections", "queries", "runbooks", "encyclopedia_sections",
		"recent_tasks", "recent_commits", "gotchas",
	}
	for _, key := range required {
		if _, ok := bundle[key]; !ok {
			t.Errorf("bundle missing required key %q", key)
		}
	}

	// Identity fidelity.
	identity, _ := bundle["identity"].(map[string]any)
	if identity["project_id"] != "acme-crm" || identity["team"] != "platform" || identity["status"] != "active" {
		t.Errorf("identity wrong: %+v", identity)
	}

	// Spot-check one satellite section round-trips.
	stages, _ := bundle["pipeline_stages"].([]map[string]any)
	if len(stages) != 1 || stages[0]["name"] != "feed-fetch" {
		t.Errorf("pipeline_stages wrong: %+v", stages)
	}

	// JSON marshal should not error (the dispatcher prints this).
	if _, err := json.Marshal(bundle); err != nil {
		t.Errorf("bundle json.Marshal: %v", err)
	}
}

func TestLoadProjectBundle_SlugMatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	s := seedProject(t, dbPath, "proj-xyz", "team-1")
	defer s.Close()

	_, isProject, err := loadProjectBundle(s, "proj-xyz")
	if err != nil || !isProject {
		t.Fatalf("lookup by id failed: isProject=%v err=%v", isProject, err)
	}
}

func TestLoadProjectBundle_NotFoundFallsThrough(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	s, err := store.Open(dbPath) // empty store, no project rows
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	defer s.Close()

	bundle, isProject, err := loadProjectBundle(s, "nonexistent")
	if err != nil {
		t.Fatalf("expected nil error on miss, got %v", err)
	}
	if isProject {
		t.Errorf("expected isProject=false for unknown id")
	}
	if bundle != nil {
		t.Errorf("expected nil bundle on miss, got %+v", bundle)
	}
}

func TestLoad_FuzzySlug_SingleHit_ResolvesWithStderrNote(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "acme-crm", "platform").Close()

	var code int
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			code = Run([]string{"load", "acme", "--store=" + dbPath})
		})
	})
	if code != 0 {
		t.Fatalf("load exit=%d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "resolved 'acme' -> 'acme-crm'") {
		t.Errorf("missing resolution note on stderr: %q", stderr)
	}
	// stdout must stay pure JSON — the resolution note may not leak into it.
	var bundle map[string]any
	if err := json.Unmarshal([]byte(stdout), &bundle); err != nil {
		t.Fatalf("stdout not pure JSON: %v\nstdout=%s", err, stdout)
	}
	identity, _ := bundle["identity"].(map[string]any)
	if identity["project_id"] != "acme-crm" {
		t.Errorf("resolved wrong project: %+v", identity)
	}
}

func TestLoad_FuzzySlug_Ambiguous_ListsCandidates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "acme-crm", "platform").Close()
	seedProject(t, dbPath, "acme-xyz", "platform").Close()

	var code int
	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			code = Run([]string{"load", "acme", "--store=" + dbPath})
		})
	})
	if code == 0 {
		t.Fatalf("ambiguous prefix must exit non-zero")
	}
	if !strings.Contains(stderr, "acme-crm") || !strings.Contains(stderr, "acme-xyz") {
		t.Errorf("stderr should list both candidates, got: %q", stderr)
	}
}

// End-to-end hub walk-up: no --store flag, BRAIN_STORE blanked — load must
// find the store through the satellite .devwork/wosy.yml hub pointer alone.
func TestLoad_HubWalkUp_NoFlagNoEnv_LoadsBundle(t *testing.T) {
	t.Setenv("BRAIN_STORE", "")
	tmp := t.TempDir()
	hub := filepath.Join(tmp, "hubws", ".devwork")
	dbPath := filepath.Join(hub, "platform", "brain.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	seedProject(t, dbPath, "acme-crm", "platform").Close()

	root := filepath.Join(tmp, "proj")
	cwd := filepath.Join(root, "sub", "dir")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir satellite cwd: %v", err)
	}
	writeHubPointer(t, root, hub)
	t.Chdir(cwd)

	var code int
	stdout := captureStdout(t, func() {
		code = Run([]string{"load", "acme-crm"})
	})
	if code != 0 {
		t.Fatalf("load exit=%d, want 0; stdout=%s", code, stdout)
	}
	var bundle map[string]any
	if err := json.Unmarshal([]byte(stdout), &bundle); err != nil {
		t.Fatalf("stdout not a JSON bundle: %v\nstdout=%s", err, stdout)
	}
	identity, _ := bundle["identity"].(map[string]any)
	if identity["project_id"] != "acme-crm" {
		t.Errorf("wrong project loaded: %+v", identity)
	}
}

// A project id that also exists as a legacy record must resolve to the
// project bundle on the exact fast path — no fuzzy resolution note.
func TestLoad_ProjectShadowsLegacyRecord_ExactFastPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	s := seedProject(t, dbPath, "px-alpha", "team-1")
	for _, rec := range []map[string]any{
		{"id": "px-alpha", "type": "task", "status": "open", "updated": "2026-06-11", "title": "shadow record"},
		{"id": "rec-1", "type": "task", "status": "open", "updated": "2026-06-11", "title": "legacy only"},
	} {
		if err := s.Put(rec); err != nil {
			t.Fatalf("seed record %v: %v", rec["id"], err)
		}
	}
	s.Close()

	var code int
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			code = Run([]string{"load", "px-alpha", "--store=" + dbPath})
		})
	})
	if code != 0 {
		t.Fatalf("load px-alpha exit=%d, want 0; stderr=%q", code, stderr)
	}
	if strings.Contains(stderr, "resolved") {
		t.Errorf("exact project hit must skip the fuzzy path, got stderr: %q", stderr)
	}
	var bundle map[string]any
	if err := json.Unmarshal([]byte(stdout), &bundle); err != nil {
		t.Fatalf("expected project bundle JSON, got: %v\nstdout=%s", err, stdout)
	}
	identity, _ := bundle["identity"].(map[string]any)
	if identity["project_id"] != "px-alpha" {
		t.Errorf("project bundle must win over the legacy record: %+v", identity)
	}

	// Exact record id with no matching project still loads via the legacy path.
	stdout = captureStdout(t, func() {
		code = Run([]string{"load", "rec-1", "--store=" + dbPath})
	})
	if code != 0 {
		t.Fatalf("load rec-1 exit=%d, want 0", code)
	}
	if !strings.Contains(stdout, "# working set") || !strings.Contains(stdout, "legacy only") {
		t.Errorf("legacy rehydrate output missing, got: %s", stdout)
	}
}

func TestLoadProjectBundle_EmptySatellites(t *testing.T) {
	// A project with zero satellite rows must still return all sections as
	// empty arrays (not nil/missing) — schema requires presence of every key.
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	s := seedProject(t, dbPath, "empty-p", "team-1")
	defer s.Close()

	bundle, isProject, err := loadProjectBundle(s, "empty-p")
	if err != nil || !isProject {
		t.Fatalf("bundle lookup: isProject=%v err=%v", isProject, err)
	}
	for _, section := range []string{
		"pipeline_stages", "scope", "connections", "queries", "runbooks",
		"encyclopedia_sections", "recent_tasks", "recent_commits", "gotchas",
	} {
		v, ok := bundle[section].([]map[string]any)
		if !ok {
			t.Errorf("section %q: expected []map[string]any, got %T", section, bundle[section])
			continue
		}
		if len(v) != 0 {
			t.Errorf("section %q: expected empty, got %d rows", section, len(v))
		}
	}
}
