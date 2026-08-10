package connections

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realConnectionsLike mirrors the corporate Platform connections.md shape
// (prose + ```shell fences, `#` comment labels above absolute paths). Designed
// around the actual file at Globex/Teams/Platform/.devwork/connections.md
// (verified 2026-05-26).
const realConnectionsLike = "# Connections\n" +
	"\n" +
	"## local (E2E)\n" +
	"\n" +
	"### MySQL\n" +
	"\n" +
	"```shell\n" +
	"mysql --login-path=local --database=tower_mainfeed -B \"$@\"\n" +
	"```\n" +
	"\n" +
	"Local DB synced from production.\n" +
	"\n" +
	"### Source repo paths (real, behind the umbrella symlinks)\n" +
	"\n" +
	"```shell\n" +
	"# API (Laravel 10, PHP 8.3)\n" +
	"/Volumes/work/Globex/Source/api-core\n" +
	"/opt/homebrew/opt/php@8.3/bin/php\n" +
	"\n" +
	"# Worker (PHP 7.4 CLI)\n" +
	"/Volumes/work/Globex/Source/worker-sync\n" +
	"/opt/homebrew/opt/php@7.4/bin/php\n" +
	"```\n" +
	"\n" +
	"## staging\n" +
	"\n" +
	"### SSH\n" +
	"\n" +
	"```shell\n" +
	"ssh staging-worker \"/usr/bin/php /srv/worker-sync/legacy/run.php\"\n" +
	"```\n"

func writeTemp(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestParse_RealShape_ExtractsLabelledPaths(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "connections.md", realConnectionsLike)

	entries, err := Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("want 4 entries from the path-ish fence, got %d: %+v", len(entries), entries)
	}

	want := []struct {
		labelSubstr string
		path        string
	}{
		{"API", "/Volumes/work/Globex/Source/api-core"},
		{"API", "/opt/homebrew/opt/php@8.3/bin/php"},
		{"Worker", "/Volumes/work/Globex/Source/worker-sync"},
		{"Worker", "/opt/homebrew/opt/php@7.4/bin/php"},
	}
	for i, w := range want {
		if entries[i].Path != w.path {
			t.Errorf("entry %d: path = %q, want %q", i, entries[i].Path, w.path)
		}
		if !strings.Contains(entries[i].Label, w.labelSubstr) {
			t.Errorf("entry %d: label %q missing %q", i, entries[i].Label, w.labelSubstr)
		}
	}
}

func TestParse_SkipsCommandsAndNonPathSections(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "connections.md", realConnectionsLike)

	entries, err := Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Must NOT pick up the mysql command (has spaces/flags) or the ssh command
	// under "### SSH" (heading isn't path-ish, plus the line has spaces).
	for _, e := range entries {
		if strings.Contains(e.Path, "mysql") || strings.Contains(e.Path, "ssh ") {
			t.Errorf("unexpected command-line entry: %+v", e)
		}
		if strings.Contains(e.Path, " ") {
			t.Errorf("entry has whitespace, must be a bare path: %q", e.Path)
		}
	}
}

func TestParse_MissingFile_NoError(t *testing.T) {
	entries, err := Parse(filepath.Join(t.TempDir(), "nope.md"))
	if err != nil {
		t.Fatalf("missing file should not error, got: %v", err)
	}
	if entries != nil {
		t.Fatalf("missing file should yield nil entries, got: %+v", entries)
	}
}

func TestParse_NoPathFields_EmptyResult(t *testing.T) {
	dir := t.TempDir()
	body := "# Connections\n\n## staging\n\n### MySQL\n\n```shell\nmysql --login-path=foo\n```\n"
	path := writeTemp(t, dir, "connections.md", body)
	entries, err := Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries (no path-ish heading), got %d: %+v", len(entries), entries)
	}
}

func TestFindAndParse_WalksUp(t *testing.T) {
	root := t.TempDir()
	dw := filepath.Join(root, ".devwork")
	if err := os.MkdirAll(dw, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeTemp(t, dw, "connections.md", realConnectionsLike)

	// Start from a nested subdir so walk-up must climb to find it.
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	src, entries, err := FindAndParse(nested)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if src == "" {
		t.Fatalf("expected to find connections.md, got empty path")
	}
	if len(entries) == 0 {
		t.Fatalf("expected entries from walk-up parse")
	}
}

func TestFindAndParse_NotFound(t *testing.T) {
	root := t.TempDir()
	src, entries, err := FindAndParse(root)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if src != "" || entries != nil {
		t.Fatalf("expected empty result, got src=%q entries=%+v", src, entries)
	}
}
