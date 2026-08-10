package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"wosy.local/brain/internal/store"
)

// cmdInitTeam handles `brain init --team=<team>` (Phase I I-2 + Fork 3 lock
// 2026-06-08): creates `<cwd>/.devwork/<team>/brain.db` with both the legacy
// `ddl` (record/edge/todo/meta) AND the new `ddlV2` (project layer, pipeline,
// scope, etc.) applied. Also creates the per-team `projects/` directory.
//
// The default (no --team) form continues to fall through cli.go's existing
// init case — preserves `brain init` and `brain init <path>` behavior.
func cmdInitTeam(p args, team string) int {
	cwd, err := os.Getwd()
	if err != nil {
		return fail("init --team: cwd: %v", err)
	}
	devwork := filepath.Join(cwd, ".devwork")
	if _, statErr := os.Stat(devwork); os.IsNotExist(statErr) {
		return fail("init --team: no .devwork/ under %s — run from a wosy workspace root or `mkdir .devwork` first", cwd)
	}

	teamDir := filepath.Join(devwork, team)
	if err := os.MkdirAll(filepath.Join(teamDir, "projects"), 0o755); err != nil {
		return fail("init --team: mkdir %s: %v", teamDir, err)
	}

	dbPath := filepath.Join(teamDir, "brain.db")
	s, err := store.Open(dbPath)
	if err != nil {
		return fail("init --team: open: %v", err)
	}
	defer s.Close()

	// Stamp identity row in `workspace` table (idempotent upsert).
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.DB.Exec(
		`INSERT INTO workspace(id, team, root_path, created, updated, brain_version)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET updated=excluded.updated, brain_version=excluded.brain_version`,
		team, team, teamDir, now, now, Version,
	); err != nil {
		return fail("init --team: stamp workspace: %v", err)
	}

	fmt.Printf("init --team=%s OK\n  brain.db: %s (schema v%d)\n  projects: %s\n",
		team, dbPath, store.SchemaVersion, filepath.Join(teamDir, "projects"))
	return 0
}
