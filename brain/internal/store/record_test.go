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
	if nb, _ := s.Neighbors("x"); len(nb) != 1 {
		t.Fatalf("edge index = %d, want 1", len(nb))
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
