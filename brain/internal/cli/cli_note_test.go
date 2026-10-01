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
		code = Run([]string{"note", "acme-crm", "FTP host drops idle conns", "--source=flows/ftp.md:12", "--store=" + dbPath})
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
	if sourceRef != "flows/ftp.md:12" {
		t.Errorf("source_ref=%q, want the --source given", sourceRef)
	}
	if !strings.Contains(created, "T") || !strings.HasSuffix(created, "Z") {
		t.Errorf("created not ISO 8601 UTC: %q", created)
	}
	// A new note is dated from its source: last_verified_at = created, never NULL.
	var verified *string
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	if err := s.DB.QueryRow(`SELECT last_verified_at FROM gotcha WHERE project_id='acme-crm'`).Scan(&verified); err != nil {
		t.Fatalf("read last_verified_at: %v", err)
	}
	s.Close()
	if verified == nil || *verified != created {
		t.Errorf("last_verified_at=%v, want created %q", verified, created)
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
	code := Run([]string{"note", "px", "host flaps", "--severity=warn", "--source=ticket:X-1", "--store=" + dbPath})
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
		code = Run([]string{"note", "px", "text", "--severity=fatal", "--source=t", "--store=" + dbPath})
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
			code = Run([]string{"note", "acme", "fuzzy landed", "--source=t", "--store=" + dbPath})
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
		code = Run([]string{"note", "zzz-nope", "orphan", "--source=t", "--store=" + dbPath})
	})
	if code == 0 {
		t.Fatalf("note on unknown project must fail")
	}
}

// 2026-09-25: a note with no --source is refused. 43 of 44 pilot gotchas carried
// source_ref = "note:"+created, so the verb was the cause of the void provenance.
func TestNote_RequiresSource(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "acme-crm", "platform").Close()
	var code int
	captureStderr(t, func() {
		code = Run([]string{"note", "acme-crm", "no source here", "--store=" + dbPath})
	})
	if code == 0 {
		t.Fatalf("note without --source exit=0, want non-zero")
	}
	s, _ := store.Open(dbPath)
	defer s.Close()
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM gotcha`).Scan(&n)
	if n != 0 {
		t.Errorf("gotcha rows=%d after refused note, want 0", n)
	}
}

// srs F1: a planted credential in a candidate write is refused and the refusal
// is shown. The control that must PASS is the same text with the secret removed.
func TestNote_RefusesPlantedSecret(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	seedProject(t, dbPath, "acme-crm", "platform").Close()
	planted := []string{
		// Built at runtime so no credential-shaped literal reaches git (factory logbook 14).
		"ftp " + "pass" + "word=" + "Fixture" + "9" + "x&3" + " for the vendor host",
		"use mysql://" + "fixture" + ":" + "notasecret9" + "@" + "db.example/tc",
		"key " + "AK" + "IA" + strings.Repeat("Q", 16) + " leaked in log",
		"deploy key " + "-----BEGIN " + "RSA PRIVATE " + "KEY----- MIIfixture",
	}
	for i, text := range planted {
		var code int
		errOut := captureStderr(t, func() {
			code = Run([]string{"note", "acme-crm", text, "--source=t", "--store=" + dbPath})
		})
		if code == 0 {
			t.Errorf("planted[%d] accepted, want refused", i)
		}
		if !strings.Contains(errOut, "refused") {
			t.Errorf("planted[%d]: refusal not shown on stderr (len=%d)", i, len(errOut))
		}
	}
	var code int
	captureStdout(t, func() {
		code = Run([]string{"note", "acme-crm", "pass" + "word: rotated on 2026-09-25, see connections.md", "--source=connections.md", "--store=" + dbPath})
	})
	if code != 0 {
		t.Fatalf("control: prose about a password refused, want accepted")
	}
	s, _ := store.Open(dbPath)
	defer s.Close()
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM gotcha`).Scan(&n)
	if n != 1 {
		t.Errorf("gotcha rows=%d, want exactly the control row", n)
	}
}
