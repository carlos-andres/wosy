package store

import (
	"path/filepath"
	"testing"
)

// TestOpen_CreatesMemoryObjects: a fresh store carries lesson, ruled_out,
// synonym and the 10 read views, so `brain init` recreates the live shape.
// A second Open must be a no-op (IF NOT EXISTS).
func TestOpen_CreatesMemoryObjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	want := []string{"lesson", "ruled_out", "synonym",
		"brief_project", "brief_task", "continent", "gotcha_located", "lesson_index",
		"locate", "map", "ruled_out_index", "stage_anchors", "task_index"}
	for round := 1; round <= 2; round++ {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open round %d: %v", round, err)
		}
		for _, name := range want {
			var n int
			s.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name=?`, name).Scan(&n)
			if n != 1 {
				t.Errorf("round %d: %s present %d times, want 1", round, name, n)
			}
			// every view must compile against the fresh tables
			if _, err := s.DB.Exec(`SELECT * FROM ` + name + ` LIMIT 0`); err != nil {
				t.Errorf("round %d: select from %s: %v", round, name, err)
			}
		}
		s.Close()
	}
}
