package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"wosy.local/brain/internal/store"
)

func readGotcha(t *testing.T, dbPath, projectID string) (severity, body, sourceRef, created string) {
	t.Helper()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	defer s.Close()
	if err := s.DB.QueryRow(
		`SELECT severity, body_md, source_ref, created FROM gotcha WHERE project_id=?`, projectID,
	).Scan(&severity, &body, &sourceRef, &created); err != nil {
		t.Fatalf("read gotcha for %s: %v", projectID, err)
	}
	return
}

func TestNote_InsertsGotcha_VisibleInLoadBundle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "acme-crm", "platform").Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"note", "acme-crm", "FTP host drops idle conns", "--store=" + dbPath})
	})
	if code != 0 {
		t.Fatalf("note exit=%d, want 0; out=%s", code, out)
	}
	severity, body, sourceRef, created := readGotcha(t, dbPath, "acme-crm")
	if severity != "info" {
		t.Errorf("default severity=%q, want info", severity)
	}
	if body != "FTP host drops idle conns" {
		t.Errorf("body_md=%q", body)
	}
	if !strings.HasPrefix(sourceRef, "note:") {
		t.Errorf("source_ref=%q, want note:<ts> prefix", sourceRef)
	}
	if !strings.Contains(created, "T") || !strings.HasSuffix(created, "Z") {
		t.Errorf("created not ISO 8601 UTC: %q", created)
	}

	loadOut := captureStdout(t, func() {
		code = Run([]string{"load", "acme-crm", "--store=" + dbPath})
	})
	if code != 0 {
		t.Fatalf("load exit=%d, want 0", code)
	}
	if !strings.Contains(loadOut, "FTP host drops idle conns") {
		t.Errorf("note missing from load bundle gotchas; out=%s", loadOut)
	}
}

func TestNote_SeverityFlag(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "px", "t").Close()
	code := Run([]string{"note", "px", "host flaps", "--severity=warn", "--store=" + dbPath})
	if code != 0 {
		t.Fatalf("note exit=%d, want 0", code)
	}
	severity, _, _, _ := readGotcha(t, dbPath, "px")
	if severity != "warn" {
		t.Errorf("severity=%q, want warn", severity)
	}
}

func TestNote_BadSeverity_Rejected(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "px", "t").Close()
	var code int
	msg := captureStderr(t, func() {
		code = Run([]string{"note", "px", "text", "--severity=fatal", "--store=" + dbPath})
	})
	if code == 0 {
		t.Fatalf("bad severity accepted; stderr=%q", msg)
	}
	if !strings.Contains(msg, "fatal") {
		t.Errorf("stderr should name the rejected severity, got: %q", msg)
	}
}

func TestNote_FuzzyProjectResolves(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "acme-crm", "platform").Close()
	var code int
	msg := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			code = Run([]string{"note", "acme", "fuzzy landed", "--store=" + dbPath})
		})
	})
	if code != 0 {
		t.Fatalf("note exit=%d, want 0; stderr=%q", code, msg)
	}
	if !strings.Contains(msg, "resolved 'acme' -> 'acme-crm'") {
		t.Errorf("missing resolution note on stderr: %q", msg)
	}
	if _, body, _, _ := readGotcha(t, dbPath, "acme-crm"); body != "fuzzy landed" {
		t.Errorf("note did not land on resolved project: body=%q", body)
	}
}

func TestNote_UnknownProject_Fails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "px", "t").Close()
	var code int
	captureStderr(t, func() {
		code = Run([]string{"note", "zzz-nope", "orphan", "--store=" + dbPath})
	})
	if code == 0 {
		t.Fatalf("note on unknown project must fail")
	}
}
