package cli

import (
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

	if p.flag["write"] != "1" && p.flag["write"] != "true" {
		fmt.Printf("PROMOTER (dry-run) — proposed library entry: %s.sql\n\n%s\n", name, content)
		fmt.Println("stage it with --write=1")
		return 0
	}

	cwd := p.cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	dir := filepath.Join(cwd, ".devwork", "queries", "library", "_staged")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail("promote: mkdir: %v", err)
	}
	dst := filepath.Join(dir, name+".sql")
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		return fail("promote: write: %v", err)
	}
	fmt.Printf("PROMOTER staged: %s\n(review, then mv to queries/library/)\n", dst)
	return 0
}
