package cli

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// cmdBuild handles `brain build <project>` (Phase I I-4): prepares the
// brain.db for re-extraction by `agent-brain-builder` and prints a dispatch
// contract. The agent itself is invoked by the main session, which parses
// the contract from stdout.
//
// Idempotency contract (per agent-brain-builder.md): re-running on the same
// project root with unchanged .md produces the same brain.db. This verb
// upholds that by TRUNCATING the agent-managed tables (pipeline_stage,
// scope_entry, encyclopedia_section, gotcha, runbook_pointer) for this
// project before the agent runs, inside ONE transaction with the
// cache_version stamp. The agent then writes a fresh row-set; if dispatch
// fails the rollback restores prior state — no half-built bundle.
func cmdBuild(p args) int {
	projectID := ""
	if len(p.pos) > 0 {
		projectID = p.pos[0]
	}
	if projectID == "" {
		return fail("build: need <project> (positional)")
	}

	s, err := open(p)
	if err != nil {
		return fail("build: store: %v", err)
	}
	defer s.Close()

	var rootPath, team string
	err = s.DB.QueryRow(
		`SELECT root_path, team FROM project WHERE id=? OR slug=?`,
		projectID, projectID,
	).Scan(&rootPath, &team)
	if err == sql.ErrNoRows {
		return fail("build: project %q not registered in brain.db — run `brain init --team=<team>` and insert a project row first", projectID)
	}
	if err != nil {
		return fail("build: project %q lookup failed (driver error, not 'not found'): %v", projectID, err)
	}

	masterPath := filepath.Join(rootPath, "master.md")
	if _, err := os.Stat(masterPath); os.IsNotExist(err) {
		return fail("build: missing master.md at %s — fill via `/wire-up` Tier 4 mini-workshop first", masterPath)
	}

	encyclopediaPath := filepath.Join(rootPath, "encyclopedia.md")
	runbooksDir := filepath.Join(rootPath, "runbooks")

	// Truncate-and-stamp in one tx. The agent's brain.set / brain.import
	// sub-calls are NOT inside this tx (different process) — but the
	// truncate is, so a failed UPDATE rolls back the truncate too. This is
	// the strongest atomicity available without an inline agent invocation.
	cacheVersion := newULID()
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.DB.Begin()
	if err != nil {
		return fail("build: tx: %v", err)
	}
	defer tx.Rollback()
	for _, tbl := range []string{"pipeline_stage", "scope_entry", "encyclopedia_section", "gotcha", "runbook_pointer"} {
		if _, err := tx.Exec(`DELETE FROM `+tbl+` WHERE project_id=?`, projectID); err != nil {
			return fail("build: truncate %s: %v", tbl, err)
		}
	}
	if _, err := tx.Exec(
		`UPDATE project SET cache_version=?, updated=? WHERE id=? OR slug=?`,
		cacheVersion, now, projectID, projectID,
	); err != nil {
		return fail("build: stamp cache_version: %v", err)
	}
	if err := tx.Commit(); err != nil {
		return fail("build: commit: %v", err)
	}

	fmt.Printf(`build: dispatch contract ready
  project:       %s
  team:          %s
  project_root:  %s
  master.md:     %s (present)
  encyclopedia:  %s (%s)
  runbooks dir:  %s (%s)
  cache_version: %s
  truncated:     pipeline_stage, scope_entry, encyclopedia_section, gotcha, runbook_pointer (project_id=%s)

Next: dispatch agent-brain-builder against project_root above.
  Agent definition: claude/agents/agent-brain-builder.md
  Agent return:     one-line "extracted N pipeline_stages, M scope_entries, ..."
  Agent writes via: brain set / brain import
  Stamped at:       %s
`,
		projectID, team, rootPath, masterPath,
		encyclopediaPath, fileExistsLabel(encyclopediaPath),
		runbooksDir, dirExistsLabel(runbooksDir),
		cacheVersion, projectID, now,
	)
	return 0
}

func fileExistsLabel(path string) string {
	if _, err := os.Stat(path); err == nil {
		return "present"
	}
	return "absent"
}

func dirExistsLabel(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "absent"
	}
	if !info.IsDir() {
		return "not a dir"
	}
	entries, _ := os.ReadDir(path)
	return fmt.Sprintf("%d files", len(entries))
}

// newULID returns a ULID-shaped string for cache_version stamping. Uniqueness
// within a single brain.db is sufficient for the brain-load-trigger.sh
// idempotency check (A-03 §4.2). A proper Crockford-Base32 ULID can wait
// until v2 — this is enough today.
func newULID() string {
	return fmt.Sprintf("01%s", time.Now().UTC().Format("20060102T150405.000000Z"))
}
