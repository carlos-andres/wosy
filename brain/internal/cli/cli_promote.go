package cli

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"wosy.local/brain/internal/promoter"
)

// promote dispatches the PROMOTER by kind. Default kind=sql preserves the
// original behavior (capture inline SQL → staged library .sql). kind=template
// generalizes the ceremony to arbitrary artifacts (HTML/MD/JSON/etc.) staged
// under an umbrella's templates/ folder (R6: scratch → sanctioned, with an
// INDEX.md anchor so the artifact is discoverable).
//
//	brain promote                            --command='<sql>'   [--name=…] [--write=1] [--cwd=DIR]
//	                                         [--project=<slug> [--desc=…]]  writes to queries/library/ AND
//	                                         inserts the query_pointer row; without --project it only stages
//	brain promote --kind=template            --source=<scratch>  --dest-umbrella=<umbrella-id>
//	                                         [--cwd=DIR] [--force] [--write=1]
func cmdPromote(p args) int {
	kind := p.flag["kind"]
	if kind == "" {
		kind = "sql"
	}
	switch kind {
	case "sql":
		return cmdPromoteSQL(p)
	case "template":
		return cmdPromoteTemplate(p)
	default:
		return fail("promote: unknown --kind=%q (want sql|template)", kind)
	}
}

// cmdPromoteSQL is the original PROMOTER: capture inline SQL → staged .sql.
// Kept verbatim under kind=sql so existing callers see no behavior change.
func cmdPromoteSQL(p args) int {
	raw := p.command
	if raw == "" {
		raw = p.input // accept --input as the SQL/command too
	}
	if raw == "" {
		return fail("promote: need --command='<inline query>' (or --input)")
	}
	sql, ok := promoter.Extract(raw)
	if !ok {
		return fail("promote: no SQL found in command")
	}
	paramSQL, params := promoter.Parameterize(sql)
	name := p.flag["name"]
	if name == "" {
		name = promoter.SuggestName(paramSQL, params)
	}
	content := promoter.Render(name, paramSQL, params)
	if c := secretClass(raw); c != "" {
		return refuseSecret("promote", c)
	}

	if p.flag["write"] != "1" && p.flag["write"] != "true" {
		fmt.Printf("PROMOTER (dry-run) — proposed library entry: %s.sql\n\n%s\n", name, content)
		fmt.Println("stage it with --write=1")
		return 0
	}

	cwd := p.cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	// 2026-09-25: with --project the file lands in the library itself and the
	// query_pointer row is written in the same call. The binary audit found that
	// promote wrote a file and inserted no row: 270 .sql on disk, 0 pointers. A
	// pointer to a _staged file would go stale on the mv, so the pointer path is
	// the library path and the file is written there.
	project := p.flag["project"]
	dir := filepath.Join(cwd, ".devwork", "queries", "library")
	if project == "" {
		dir = filepath.Join(dir, "_staged")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail("promote: mkdir: %v", err)
	}
	dst := filepath.Join(dir, name+".sql")
	if project == "" {
		if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
			return fail("promote: write: %v", err)
		}
		fmt.Printf("PROMOTER staged: %s\n(review, then mv to queries/library/ — no query_pointer row: pass --project=<slug> to write one)\n", dst)
		return 0
	}
	s, err := openResolved(p)
	if err != nil {
		return fail("promote: store: %v", err)
	}
	defer s.Close()
	id, slug, code := resolveProjectOrFail("promote", s.DB, project)
	if code != 0 {
		return code
	}
	if _, err := os.Stat(dst); err == nil && p.flag["force"] != "1" {
		return fail("promote: %s exists — pass --force=1 to overwrite", dst)
	}
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		return fail("promote: write: %v", err)
	}
	abs, _ := filepath.Abs(dst)
	if _, err := s.DB.Exec(
		`INSERT INTO query_pointer(project_id, name, path, description) VALUES(?, ?, ?, ?)
		 ON CONFLICT(project_id, name) DO UPDATE SET path=excluded.path, description=excluded.description`,
		id, name, abs, p.flag["desc"]); err != nil {
		return fail("promote: query_pointer insert: %v", err)
	}
	fmt.Printf("PROMOTER: %s\nquery_pointer OK: %s/%s -> %s\n", dst, slug, name, abs)
	return 0
}

// resolveProjectOrFail is the note/promote project lookup: exact id or slug,
// then the lenient fuzzy tier; zero matches is an error, never a create.
func resolveProjectOrFail(verb string, db *sql.DB, arg string) (id, slug string, code int) {
	err := db.QueryRow(`SELECT id, slug FROM project WHERE id=? OR slug=?`, arg, arg).Scan(&id, &slug)
	if err == nil {
		return id, slug, 0
	}
	if err != sql.ErrNoRows {
		return "", "", fail("%s: project lookup: %v", verb, err)
	}
	matches, mErr := resolveProjectFuzzy(db, arg)
	if mErr != nil {
		return "", "", fail("%s: project lookup: %v", verb, mErr)
	}
	switch len(matches) {
	case 1:
		fmt.Fprintf(os.Stderr, "resolved '%s' -> '%s'\n", arg, matches[0].slug)
		return matches[0].id, matches[0].slug, 0
	case 0:
		return "", "", fail("%s: no project matching %q — register it first (brain project register)", verb, arg)
	default:
		fmt.Fprintf(os.Stderr, "%s: ambiguous project %q — candidates:\n", verb, arg)
		for _, m := range matches {
			fmt.Fprintf(os.Stderr, "  %s\n", m.slug)
		}
		return "", "", 1
	}
}
