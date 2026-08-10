package cli

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"wosy.local/brain/internal/store"
)

type projectRow struct {
	slug, team, displayName, rootPath, status, created, updated string
}

func readProjectRow(t *testing.T, dbPath, id string) projectRow {
	t.Helper()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	defer s.Close()
	var r projectRow
	var dn sql.NullString
	if err := s.DB.QueryRow(
		`SELECT slug, team, display_name, root_path, status, created, updated FROM project WHERE id=?`, id,
	).Scan(&r.slug, &r.team, &dn, &r.rootPath, &r.status, &r.created, &r.updated); err != nil {
		t.Fatalf("read project %s: %v", id, err)
	}
	r.displayName = dn.String
	return r
}

func countProjects(t *testing.T, dbPath string) int {
	t.Helper()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	defer s.Close()
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM project`).Scan(&n); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	return n
}

func TestProjectRegister_CreatesRow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	code := Run([]string{"project", "register",
		"--id=px", "--team=platform", "--root=/tmp/px",
		"--display-name=Project X", "--store=" + dbPath})
	if code != 0 {
		t.Fatalf("register exit=%d, want 0", code)
	}
	r := readProjectRow(t, dbPath, "px")
	if r.slug != "px" || r.team != "platform" || r.rootPath != "/tmp/px" ||
		r.status != "active" || r.displayName != "Project X" {
		t.Errorf("row wrong: %+v", r)
	}
	// ISO 8601 UTC, e.g. 2026-06-11T12:00:00Z.
	if !strings.HasSuffix(r.created, "Z") || !strings.Contains(r.created, "T") {
		t.Errorf("created not ISO 8601 UTC: %q", r.created)
	}
	if r.updated == "" {
		t.Errorf("updated empty")
	}
}

func TestProjectRegister_ReregisterUpdatesNotDuplicates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	if code := Run([]string{"project", "register",
		"--id=px", "--team=t1", "--root=/tmp/old", "--store=" + dbPath}); code != 0 {
		t.Fatalf("first register exit=%d", code)
	}
	first := readProjectRow(t, dbPath, "px")

	if code := Run([]string{"project", "register",
		"--id=px", "--team=t2", "--root=/tmp/new", "--status=paused", "--store=" + dbPath}); code != 0 {
		t.Fatalf("second register exit=%d", code)
	}
	if n := countProjects(t, dbPath); n != 1 {
		t.Fatalf("project rows = %d, want 1 (upsert, not duplicate)", n)
	}
	second := readProjectRow(t, dbPath, "px")
	if second.team != "t2" || second.rootPath != "/tmp/new" || second.status != "paused" {
		t.Errorf("re-register did not update: %+v", second)
	}
	if second.created != first.created {
		t.Errorf("created changed on re-register: %q -> %q", first.created, second.created)
	}
	if second.slug != "px" {
		t.Errorf("slug changed on re-register: %q", second.slug)
	}
}

func TestProjectRegister_LoadFindsRegisteredProject(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	if code := Run([]string{"project", "register",
		"--id=px", "--team=t", "--root=/tmp/px", "--store=" + dbPath}); code != 0 {
		t.Fatalf("register exit=%d", code)
	}
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"load", "px", "--store=" + dbPath})
	})
	if code != 0 {
		t.Fatalf("load exit=%d, want 0; out=%s", code, out)
	}
	for _, want := range []string{`"project_id": "px"`, `"team": "t"`, `"root_path": "/tmp/px"`} {
		if !strings.Contains(out, want) {
			t.Errorf("load bundle missing %s; out=%s", want, out)
		}
	}
}

func TestProject_BadInvocations_Exit2(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	cases := [][]string{
		{"project", "--store=" + dbPath},                        // no subaction
		{"project", "bogus", "--store=" + dbPath},               // unknown subaction
		{"project", "register", "--id=px", "--store=" + dbPath}, // missing --team/--root
		{"project", "register", "--id=px", "--team=t", "--root=/tmp/px", // bad status
			"--status=zombie", "--store=" + dbPath},
	}
	for _, argv := range cases {
		if code := Run(argv); code != 2 {
			t.Errorf("Run(%v) exit=%d, want 2", argv, code)
		}
	}
}
