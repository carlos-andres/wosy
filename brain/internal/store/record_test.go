package store

import (
	"path/filepath"
	"testing"
)

func tmpStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutGetRoundTrip(t *testing.T) {
	s := tmpStore(t)
	rec := map[string]any{
		"id": "x", "type": "task", "status": "open", "updated": "2026-05-25",
		"todo":  []any{map[string]any{"id": "a", "desc": "do", "status": "open"}},
		"edges": []any{map[string]any{"rel": "part_of", "to": "p"}},
	}
	if err := s.Put(rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("x")
	if err != nil {
		t.Fatal(err)
	}
	if got["type"] != "task" || got["status"] != "open" {
		t.Fatalf("round-trip lost fields: %v", got)
	}
	// derived indexes populated
	if n, _ := s.ListTodos("open"); len(n) != 1 {
		t.Fatalf("todo index = %d, want 1", len(n))
	}
	// edge dialect (logbook 160): a task's edges hang from "task:<id>"
	if nb, _ := s.Neighbors("task:x"); len(nb) != 1 {
		t.Fatalf("edge index = %d, want 1", len(nb))
	}
}

// Decision 2026-09-25 (logbook 160): edges speak the estate's dialect, from_id = "task:<id>"
// for a task and bare for a project, and Put never deletes a row it did not write.
func TestPutEdgeDialectAndNoDelete(t *testing.T) {
	s := tmpStore(t)
	put := func(to string) {
		t.Helper()
		if err := s.Put(map[string]any{
			"id": "x", "type": "task", "status": "open", "updated": "2026-09-25",
			"edges": []any{map[string]any{"rel": "part_of", "to": to}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	put("p1")
	// a row an applier wrote, which Put never saw
	if _, err := s.DB.Exec(`INSERT INTO edge(from_id,rel,to_id,note) VALUES('task:x','about','dealer:1','applier')`); err != nil {
		t.Fatal(err)
	}
	put("p2") // re-import with a different doc edge
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM edge WHERE from_id='task:x'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("edges under task:x = %d, want 3 (p1, applier row, p2: nothing deleted)", n)
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM edge WHERE from_id='x'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("bare-id edges = %d, want 0", n)
	}
	// control: a project stays bare
	if err := s.Put(map[string]any{"id": "w", "type": "project", "status": "active", "updated": "2026-09-25",
		"edges": []any{map[string]any{"rel": "part_of", "to": "u"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM edge WHERE from_id='w'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("project edges under bare id = %d, want 1", n)
	}
}

func TestTypeCollisionGuard(t *testing.T) {
	s := tmpStore(t)
	if err := s.Put(map[string]any{"id": "n", "type": "umbrella", "status": "active", "updated": "2026-05-25"}); err != nil {
		t.Fatal(err)
	}
	// same id, different type -> must refuse (no silent clobber)
	err := s.Put(map[string]any{"id": "n", "type": "project", "status": "active", "updated": "2026-05-25"})
	if err == nil {
		t.Fatal("expected id-collision error, got nil (silent type clobber)")
	}
	// original survives
	got, _ := s.Get("n")
	if got["type"] != "umbrella" {
		t.Fatalf("original clobbered: type=%v", got["type"])
	}
}
