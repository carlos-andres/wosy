package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wosy.local/brain/internal/promoter"
)

// cmdPromoteTemplate generalizes PROMOTER beyond SQL. Moves a draft from
// .devwork/_scratch/ (or anywhere under the project) into a sanctioned
// templates/ location under the named umbrella, then registers an INDEX.md row.
//
// Required flags:
//
//	--source=<path>          path to the draft (anywhere; commonly .devwork/_scratch/foo.html)
//	--dest-umbrella=<id>     umbrella id under <project-root>/.devwork/<id>/templates/
//
// Optional flags:
//
//	--cwd=DIR                project root anchor; default $PWD. Walks UP to find .devwork/.
//	--name=<basename>        override destination basename (default = source basename)
//	--force                  overwrite an existing destination
//	--write=1                commit the move + INDEX update (default: dry-run)
func cmdPromoteTemplate(p args) int {
	src := p.flag["source"]
	if src == "" {
		return fail("promote --kind=template: need --source=<path-to-draft>")
	}
	umbrella := p.flag["dest-umbrella"]
	if umbrella == "" {
		return fail("promote --kind=template: need --dest-umbrella=<umbrella-id>")
	}

	// Resolve project root: --cwd or $PWD, then walk UP to nearest .devwork/.
	anchor := p.cwd
	if anchor == "" {
		anchor, _ = os.Getwd()
	}
	devwork, ok := promoter.WalkUpDevwork(anchor)
	if !ok {
		return fail("promote --kind=template: no .devwork/ found walking up from %s", anchor)
	}

	// Resolve absolute source path. Source is taken as-is if absolute, else
	// relative to the anchor (so callers can pass a path relative to where they ran).
	absSrc := src
	if !filepath.IsAbs(absSrc) {
		absSrc = filepath.Join(anchor, src)
	}
	if _, err := os.Stat(absSrc); err != nil {
		return fail("promote --kind=template: source: %v", err)
	}

	// Destination: <project>/.devwork/<umbrella>/templates/<basename>.
	name := p.flag["name"]
	if name == "" {
		name = filepath.Base(absSrc)
	}
	tmplDir := filepath.Join(devwork, umbrella, "templates")
	dst := filepath.Join(tmplDir, name)
	indexPath := filepath.Join(tmplDir, "INDEX.md")

	// Existence check (force gate). Dry-run still surfaces the conflict but
	// exits 0 so callers can preview the would-be operation. Write-mode fails.
	dstExists := false
	if _, err := os.Stat(dst); err == nil {
		dstExists = true
	}
	force := p.flag["force"] == "1" || p.flag["force"] == "true"

	indexRow := buildIndexRow(name, time.Now().UTC().Format("2006-01-02"))
	indexAction := "create"
	if _, err := os.Stat(indexPath); err == nil {
		indexAction = "append"
	}

	write := p.flag["write"] == "1" || p.flag["write"] == "true"
	if !write {
		fmt.Println("PROMOTER --kind=template (dry-run)")
		fmt.Printf("  source:   %s\n", absSrc)
		fmt.Printf("  dest:     %s\n", dst)
		fmt.Printf("  index:    %s (%s)\n", indexPath, indexAction)
		if dstExists {
			fmt.Println("  note:     destination exists; --write=1 will FAIL without --force")
		}
		fmt.Printf("  row:      %s\n", strings.TrimSpace(indexRow))
		fmt.Println("\ncommit with --write=1")
		return 0
	}

	if dstExists && !force {
		return fail("promote --kind=template: destination exists: %s (use --force to overwrite)", dst)
	}

	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		return fail("promote --kind=template: mkdir: %v", err)
	}
	if err := moveFile(absSrc, dst); err != nil {
		return fail("promote --kind=template: move: %v", err)
	}
	if err := appendIndex(indexPath, umbrella, indexRow); err != nil {
		return fail("promote --kind=template: index: %v", err)
	}
	fmt.Printf("PROMOTER promoted: %s -> %s\nindex: %s\n", absSrc, dst, indexPath)
	return 0
}

// moveFile renames if same filesystem; falls back to copy+remove otherwise.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, in, 0o644); err != nil {
		return err
	}
	return os.Remove(src)
}

// buildIndexRow renders a single INDEX.md row for the promoted template.
func buildIndexRow(name, date string) string {
	return fmt.Sprintf("| `%s` | %s | promoted |\n", name, date)
}

// appendIndex appends one row to INDEX.md, creating the file with a header on
// first write. Header carries the umbrella id so the file is self-describing.
func appendIndex(indexPath, umbrella, row string) error {
	header := fmt.Sprintf("# Templates — %s\n\nSanctioned templates for umbrella `%s`. Promoted by `brain promote --kind=template`.\n\n| file | promoted-on | status |\n|---|---|---|\n",
		umbrella, umbrella)
	f, err := os.OpenFile(indexPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() == 0 {
		if _, err := f.WriteString(header); err != nil {
			return err
		}
	} else {
		if _, err := f.Seek(0, 2); err != nil { // append
			return err
		}
	}
	_, err = f.WriteString(row)
	return err
}
