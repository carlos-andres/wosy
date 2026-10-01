package cli

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

const noteUsage = `usage: brain note <project> "<text>" --source=<path|file:line|ticket|command> [--severity=info|warn|error]`

// cmdNote appends a gotcha row to a project — the zero-ceremony capture path
// for session findings ("FTP host drops idle conns") that would otherwise die
// in chat scrollback. The project resolves with the same lenient matching as
// `brain load`, but a note must land on an existing project: zero matches is
// an error, never a create.
func cmdNote(p args) int {
	if len(p.pos) < 2 || strings.TrimSpace(p.pos[1]) == "" {
		return usageFail("note: need a project and a non-empty text\n%s", noteUsage)
	}
	projectArg, text := p.pos[0], p.pos[1]
	// 2026-09-25: a source is required. 43 of 44 gotchas in the pilot store carried
	// source_ref = "note:"+created, which records WHEN and never WHERE FROM. A
	// conclusion with no source does not enter the store (gate 3 applied at write).
	source := strings.TrimSpace(p.flag["source"])
	if source == "" {
		return usageFail("note: need --source=<path|file:line|ticket|command> — where this fact came from\n%s", noteUsage)
	}
	if c := secretClass(text + " " + source); c != "" {
		return refuseSecret("note", c)
	}
	severity := p.flag["severity"]
	if severity == "" {
		severity = "info"
	}
	switch severity {
	case "info", "warn", "error":
	default:
		return usageFail("note: invalid --severity %q (info|warn|error)", severity)
	}

	s, err := openResolved(p)
	if err != nil {
		return fail("note: store: %v", err)
	}
	defer s.Close()

	var id, slug string
	err = s.DB.QueryRow(`SELECT id, slug FROM project WHERE id=? OR slug=?`,
		projectArg, projectArg).Scan(&id, &slug)
	if err == sql.ErrNoRows {
		matches, mErr := resolveProjectFuzzy(s.DB, projectArg)
		if mErr != nil {
			return fail("note: project lookup: %v", mErr)
		}
		switch len(matches) {
		case 1:
			id, slug = matches[0].id, matches[0].slug
			fmt.Fprintf(os.Stderr, "resolved '%s' -> '%s'\n", projectArg, slug)
		case 0:
			return fail("note: no project matching %q — register it first (brain project register)", projectArg)
		default:
			fmt.Fprintf(os.Stderr, "note: ambiguous project %q — candidates:\n", projectArg)
			for _, m := range matches {
				fmt.Fprintf(os.Stderr, "  %s\n", m.slug)
			}
			return 1
		}
	} else if err != nil {
		return fail("note: project lookup: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.DB.Exec(
		`INSERT INTO gotcha(project_id, severity, body_md, source_ref, created, last_verified_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		id, severity, text, source, now, now,
	); err != nil {
		return fail("note: insert: %v", err)
	}
	fmt.Printf("note OK: %s [%s]\n", slug, severity)
	return 0
}
