package cli

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// seedConfirm plants two projects, two gotchas on acme-crm, and a lesson
// table (the appliers create it; store.Open does not) keyed project_id+id:
// L1 on acme-crm, L-ws at workspace level (project_id NULL), and L-dup on
// both projects.
func seedConfirm(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	s := seedProject(t, dbPath, "acme-crm", "platform")
	defer s.Close()
	for _, q := range []string{
		`INSERT INTO project(id,slug,team,root_path,created,updated,status) VALUES('other','other','t','/tmp/other','2026-09-30','2026-09-30','active')`,
		`INSERT INTO gotcha(project_id,severity,body_md,source_ref,created) VALUES('acme-crm','warn','FTP host drops idle conns','f:1','2026-09-30T00:00:00Z')`,
		`INSERT INTO gotcha(project_id,severity,body_md,source_ref,created) VALUES('acme-crm','info','FTP host keeps conns','f:2','2026-09-30T00:00:00Z')`,
		`CREATE TABLE lesson (id TEXT NOT NULL, project_id TEXT REFERENCES project(id), node_type TEXT NOT NULL,
		   last_verified_at TEXT, superseded_by TEXT, created TEXT NOT NULL, source_ref TEXT NOT NULL, PRIMARY KEY (project_id, id))`,
		`INSERT INTO lesson(id,project_id,node_type,created,source_ref) VALUES('L1','acme-crm','rule','2026-09-30','f:3')`,
		`INSERT INTO lesson(id,project_id,node_type,created,source_ref) VALUES('L-ws',NULL,'rule','2026-09-30','f:4')`,
		`INSERT INTO lesson(id,project_id,node_type,created,source_ref) VALUES('L-dup','acme-crm','rule','2026-09-30','f:5')`,
		`INSERT INTO lesson(id,project_id,node_type,created,source_ref) VALUES('L-dup','other','rule','2026-09-30','f:6')`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatalf("seed %s: %v", q, err)
		}
	}
	return dbPath
}

// verification reads last_verified_at and superseded_by of every gotcha and
// lesson row, keyed table:id:project, so a refusal can prove nothing moved.
func verification(t *testing.T, dbPath string) map[string][2]sql.NullString {
	t.Helper()
	db := openForTest(t, dbPath)
	out := map[string][2]sql.NullString{}
	for _, table := range []string{"gotcha", "lesson"} {
		rows, err := db.Query(`SELECT id, coalesce(project_id,''), last_verified_at, superseded_by FROM ` + table)
		if err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		for rows.Next() {
			var id, pid string
			var v [2]sql.NullString
			if err := rows.Scan(&id, &pid, &v[0], &v[1]); err != nil {
				t.Fatal(err)
			}
			out[table+":"+id+":"+pid] = v
		}
		rows.Close()
	}
	return out
}

func runConfirm(t *testing.T, argv ...string) (code int, stdout, stderr string) {
	t.Helper()
	stderr = captureStderr(t, func() {
		stdout = captureStdout(t, func() { code = Run(append([]string{"confirm"}, argv...)) })
	})
	return
}

func TestConfirm_StampsGotchaAndLesson(t *testing.T) {
	dbPath := seedConfirm(t)
	for _, tc := range []struct{ arg, key string }{
		{"gotcha:1", "gotcha:1:acme-crm"},
		{"lesson:L1", "lesson:L1:acme-crm"},
		{"lesson:L-ws", "lesson:L-ws:"},
	} {
		code, out, errOut := runConfirm(t, tc.arg, "--store="+dbPath)
		if code != 0 {
			t.Fatalf("confirm %s exit=%d, want 0; stderr=%q", tc.arg, code, errOut)
		}
		if !strings.Contains(out, tc.arg) || !strings.Contains(out, "last_verified_at=") || strings.Count(out, "\n") != 1 {
			t.Errorf("confirm %s: want one line naming the row and the value, got %q", tc.arg, out)
		}
		v := verification(t, dbPath)[tc.key]
		if _, err := time.Parse(time.RFC3339, v[0].String); !v[0].Valid || err != nil || !strings.HasSuffix(v[0].String, "Z") {
			t.Errorf("%s last_verified_at=%v, want RFC3339 UTC", tc.key, v[0])
		}
		if v[1].Valid {
			t.Errorf("%s superseded_by=%q, want NULL without --contradicted", tc.key, v[1].String)
		}
	}
	if v := verification(t, dbPath)["gotcha:2:acme-crm"]; v[0].Valid {
		t.Errorf("gotcha 2 stamped by a confirm of gotcha 1")
	}
}

func TestConfirm_ContradictedSetsSupersededBy(t *testing.T) {
	dbPath := seedConfirm(t)
	for _, tc := range []struct{ arg, successor, key string }{
		{"gotcha:1", "gotcha:2", "gotcha:1:acme-crm"},
		{"lesson:L1", "L2", "lesson:L1:acme-crm"},
	} {
		code, out, errOut := runConfirm(t, tc.arg, "--contradicted="+tc.successor, "--store="+dbPath)
		if code != 0 {
			t.Fatalf("confirm %s exit=%d, want 0; stderr=%q", tc.arg, code, errOut)
		}
		if !strings.Contains(out, "superseded_by="+tc.successor) {
			t.Errorf("confirm %s output does not name the successor: %q", tc.arg, out)
		}
		v := verification(t, dbPath)[tc.key]
		if !v[0].Valid || v[1].String != tc.successor {
			t.Errorf("%s = %v, want last_verified_at set and superseded_by=%s", tc.key, v, tc.successor)
		}
	}
	// load skips the superseded gotcha and still serves the live one.
	var code int
	out := captureStdout(t, func() { code = Run([]string{"load", "acme-crm", "--store=" + dbPath}) })
	if code != 0 {
		t.Fatalf("load exit=%d", code)
	}
	if strings.Contains(out, "FTP host drops idle conns") || !strings.Contains(out, "FTP host keeps conns") {
		t.Errorf("load bundle gotchas wrong after supersede; out=%s", out)
	}
}

func TestConfirm_RefusesWithoutWriting(t *testing.T) {
	dbPath := seedConfirm(t)
	before := verification(t, dbPath)
	for _, argv := range [][]string{
		{"gotcha:999"},
		{"lesson:nope"},
		{"lesson:L-dup"},
		{"rule:1"},
		{"gotcha:abc"},
		{"gotcha"},
		{"gotcha:"},
		{},
		{"gotcha:1", "--contradicted="},
	} {
		code, out, errOut := runConfirm(t, append(argv, "--store="+dbPath)...)
		if code != 1 {
			t.Errorf("confirm %v exit=%d, want 1", argv, code)
		}
		if out != "" || strings.Count(errOut, "\n") != 1 {
			t.Errorf("confirm %v: want empty stdout and a one-line error, got stdout=%q stderr=%q", argv, out, errOut)
		}
	}
	if _, _, errOut := runConfirm(t, "lesson:L-dup", "--store="+dbPath); !strings.Contains(errOut, "2 projects") {
		t.Errorf("ambiguous lesson refusal does not say so: %q", errOut)
	}
	after := verification(t, dbPath)
	if len(after) != len(before) {
		t.Fatalf("row count moved: %d -> %d", len(before), len(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s changed by a refused confirm: %v -> %v", k, v, after[k])
		}
	}
}
