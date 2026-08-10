package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuery_Select_AlignedColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "acme-crm", "platform").Close()
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"query", "SELECT id, team FROM project", "--store=" + dbPath})
	})
	if code != 0 {
		t.Fatalf("query exit=%d, want 0; out=%s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "id") || !strings.Contains(lines[0], "team") {
		t.Errorf("missing header row; out=%s", out)
	}
	if !strings.Contains(out, "acme-crm") || !strings.Contains(out, "platform") {
		t.Errorf("missing data row; out=%s", out)
	}
}

func TestQuery_JSONOutput(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "acme-crm", "platform").Close()
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"query", "SELECT id, team FROM project", "--json", "--store=" + dbPath})
	})
	if code != 0 {
		t.Fatalf("query --json exit=%d, want 0; out=%s", code, out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("output not a JSON array: %v\nout=%s", err, out)
	}
	if len(rows) != 1 || rows[0]["id"] != "acme-crm" || rows[0]["team"] != "platform" {
		t.Errorf("rows wrong: %+v", rows)
	}
}

// Boolean flag placed BEFORE the positional SQL must parse the same as after.
func TestQuery_JSONFlagBeforeSQL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "acme-crm", "platform").Close()
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"query", "--json", "SELECT id, team FROM project", "--store=" + dbPath})
	})
	if code != 0 {
		t.Fatalf("query --json (flag-first) exit=%d, want 0; out=%s", code, out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("output not a JSON array: %v\nout=%s", err, out)
	}
	if len(rows) != 1 || rows[0]["id"] != "acme-crm" || rows[0]["team"] != "platform" {
		t.Errorf("rows wrong: %+v", rows)
	}
}

func TestQuery_PragmaAndTrailingSemicolonAllowed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "px", "t").Close()
	for _, q := range []string{
		"PRAGMA table_info(project)",
		"SELECT id FROM project;",
		"  select id from project  ",
	} {
		var code int
		captureStdout(t, func() {
			code = Run([]string{"query", q, "--store=" + dbPath})
		})
		if code != 0 {
			t.Errorf("query %q exit=%d, want 0", q, code)
		}
	}
}

func TestQuery_RejectsMutationsMultiStatementAttach(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "px", "t").Close()
	for _, q := range []string{
		"DELETE FROM project",
		"UPDATE project SET team='x'",
		"INSERT INTO project(id) VALUES('y')",
		"DROP TABLE project",
		"SELECT 1; SELECT 2",
		"SELECT 1; DELETE FROM project",
		"ATTACH DATABASE '/tmp/x.db' AS evil",
		"SELECT 1 FROM x; ATTACH DATABASE '/tmp/x.db' AS evil",
	} {
		var code int
		captureStderr(t, func() {
			code = Run([]string{"query", q, "--store=" + dbPath})
		})
		if code == 0 {
			t.Errorf("query %q accepted, want rejection", q)
		}
	}

	// Write-PRAGMAs pass the statement gate (it filters by statement type
	// only); the mode=ro open is what must stop them at execution.
	var code int
	msg := captureStderr(t, func() {
		code = Run([]string{"query", "PRAGMA user_version=5", "--store=" + dbPath})
	})
	if code == 0 {
		t.Errorf("PRAGMA user_version=5 accepted, want mode=ro execution failure")
	}
	if strings.Contains(msg, "only SELECT/PRAGMA") {
		t.Errorf("write-PRAGMA must fail at execution, not the gate; stderr=%q", msg)
	}
}

func TestQuery_MissingColumn_PrintsDDLHint(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "px", "t").Close()
	var code int
	msg := captureStderr(t, func() {
		code = Run([]string{"query", "SELECT nonexistent_col FROM project", "--store=" + dbPath})
	})
	if code == 0 {
		t.Fatalf("missing column should fail")
	}
	if !strings.Contains(msg, "CREATE TABLE") || !strings.Contains(msg, "project") {
		t.Errorf("stderr should carry a CREATE TABLE DDL hint, got: %q", msg)
	}
}
