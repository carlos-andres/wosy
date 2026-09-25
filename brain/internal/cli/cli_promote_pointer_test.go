package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wosy.local/brain/internal/store"
)

// 2026-09-25: promote --project writes the library file AND its query_pointer
// row. Before this the verb wrote a file and inserted nothing (binary audit).
func TestPromote_WithProject_WritesLibraryAndPointer(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "brain.db")
	seedProject(t, dbPath, "acme-crm", "platform").Close()
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"promote", "--command=mysql -e 'SELECT id FROM dealer WHERE id = 42'",
			"--name=dealer_by_id", "--desc=one dealer row", "--project=acme-crm", "--write=1", "--cwd=" + root, "--store=" + dbPath})
	})
	if code != 0 {
		t.Fatalf("promote exit=%d out=%s", code, out)
	}
	want := filepath.Join(root, ".devwork", "queries", "library", "dealer_by_id.sql")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("library file not written at %s", want)
	}
	if _, err := os.Stat(filepath.Join(root, ".devwork", "queries", "library", "_staged", "dealer_by_id.sql")); err == nil {
		t.Errorf("with --project the file must not be staged")
	}
	s, _ := store.Open(dbPath)
	defer s.Close()
	var path, desc string
	if err := s.DB.QueryRow(`SELECT path, description FROM query_pointer WHERE project_id='acme-crm' AND name='dealer_by_id'`).Scan(&path, &desc); err != nil {
		t.Fatalf("query_pointer row missing: %v", err)
	}
	if !strings.HasSuffix(path, "dealer_by_id.sql") || desc != "one dealer row" {
		t.Errorf("pointer path=%q desc=%q", path, desc)
	}
	// control: without --project nothing is pointed at, as before
	captureStdout(t, func() {
		code = Run([]string{"promote", "--command=mysql -e 'SELECT 1'", "--name=one", "--write=1", "--cwd=" + root, "--store=" + dbPath})
	})
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM query_pointer`).Scan(&n)
	if code != 0 || n != 1 {
		t.Errorf("staged promote: exit=%d pointers=%d, want 0 and 1", code, n)
	}
}

// 2026-09-25: report/list/reconcile open the store through the same locator as
// load/query, so a satellite .devwork/wosy.yml hub pointer is enough. The audit
// found report refusing from the umbrella with "BRAIN_STORE not set".
func TestOpen_FollowsHubPointer(t *testing.T) {
	root := t.TempDir()
	hub := filepath.Join(root, "hub")
	os.MkdirAll(hub, 0o755)
	hs := seedProject(t, filepath.Join(hub, "brain.db"), "acme-crm", "platform")
	if _, err := hs.DB.Exec(`INSERT INTO record(id,type,status,updated,created,title,status_detail,doc)
		VALUES('hub-t1','task','open','2026-09-25T00:00:00Z','2026-09-25T00:00:00Z','t','','{}')`); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	hs.Close()
	sat := filepath.Join(root, "satellite", ".devwork")
	os.MkdirAll(sat, 0o755)
	os.WriteFile(filepath.Join(sat, "wosy.yml"), []byte("hub: "+hub+"\n"), 0o644)
	t.Chdir(filepath.Join(root, "satellite"))
	t.Setenv("BRAIN_STORE", "")
	var code int
	// list goes through open(), the same path report and reconcile use.
	out := captureStdout(t, func() { code = Run([]string{"list"}) })
	if code != 0 || !strings.Contains(out, "hub-t1") {
		t.Fatalf("list via hub: exit=%d out=%s", code, out)
	}
	// the write landed in the HUB store, not in a lazily created local one
	if _, err := os.Stat(filepath.Join(root, "satellite", ".brain", "store.db")); err == nil {
		t.Fatalf("a local .brain/store.db was created instead of following the hub")
	}
	// control: from a directory with no pointer the same call still refuses
	t.Chdir(root)
	errOut := captureStderr(t, func() { code = Run([]string{"list"}) })
	if code == 0 {
		t.Fatalf("list with no hub and no store must refuse; err=%s", errOut)
	}
}
