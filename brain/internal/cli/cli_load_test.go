package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wosy.local/brain/internal/store"
)

const fixtureConnectionsMD = "# Connections\n" +
	"\n" +
	"### Source repo paths\n" +
	"\n" +
	"```shell\n" +
	"# API (Laravel 10, PHP 8.3)\n" +
	"/Volumes/work/Globex/Source/api-core\n" +
	"\n" +
	"# Worker (PHP 7.4 CLI)\n" +
	"/Volumes/work/Globex/Source/worker-sync\n" +
	"```\n"

// seedLoadTask plants a minimal-valid task in a fresh sqlite store at the
// given path and returns the absolute store path.
func seedLoadTask(t *testing.T, dbPath, id string) {
	t.Helper()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	defer s.Close()
	rec := map[string]any{
		"id": id, "type": "task", "status": "open", "updated": "2026-05-26",
		"title":     "rehydrator test",
		"context":   "task context body",
		"todo":      []any{map[string]any{"id": "a", "desc": "do a", "status": "open"}},
		"scope":     map[string]any{"in_scope": []any{"x"}},
		"done_when": []any{"x done"},
	}
	if err := s.Put(rec); err != nil {
		t.Fatalf("seed put: %v", err)
	}
}

// withCwd chdirs to dir for the duration of fn (restored on cleanup).
func withCwd(t *testing.T, dir string, fn func()) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() {
		_ = os.Chdir(prev)
	}()
	fn()
}

// TestLoad_PrependsSourcePaths_WhenConnectionsMDPresent verifies the N-01
// behavior: when a project root has .devwork/connections.md with path-ish
// fenced blocks, `brain load <id>` surfaces them at the top of the bundle.
func TestLoad_PrependsSourcePaths_WhenConnectionsMDPresent(t *testing.T) {
	projRoot := t.TempDir()
	dw := filepath.Join(projRoot, ".devwork")
	if err := os.MkdirAll(dw, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dw, "connections.md"), []byte(fixtureConnectionsMD), 0o644); err != nil {
		t.Fatalf("write connections: %v", err)
	}
	dbPath := filepath.Join(dw, "store.db")
	seedLoadTask(t, dbPath, "tsk-001")

	var out string
	withCwd(t, projRoot, func() {
		out = captureStdout(t, func() {
			Run([]string{"load", "tsk-001", "--store=" + dbPath})
		})
	})

	if !strings.Contains(out, "## Source paths (from") {
		t.Fatalf("expected 'Source paths' block in output, got:\n%s", out)
	}
	if !strings.Contains(out, "/Volumes/work/Globex/Source/api-core") {
		t.Fatalf("expected API path in output, got:\n%s", out)
	}
	if !strings.Contains(out, "/Volumes/work/Globex/Source/worker-sync") {
		t.Fatalf("expected Worker path in output, got:\n%s", out)
	}
	// Ordering: source paths block must come BEFORE the task context body.
	idxPaths := strings.Index(out, "## Source paths")
	idxContext := strings.Index(out, "task context body")
	if idxPaths < 0 || idxContext < 0 || idxPaths >= idxContext {
		t.Fatalf("expected Source paths to precede context; idxPaths=%d idxContext=%d\nout:\n%s",
			idxPaths, idxContext, out)
	}
}

// TestLoad_NoConnectionsMD_BundleUnchanged verifies the absence path: no
// .devwork/connections.md, no prepended block. The rest of the bundle is
// identical to baseline output.
func TestLoad_NoConnectionsMD_BundleUnchanged(t *testing.T) {
	projRoot := t.TempDir()
	dw := filepath.Join(projRoot, ".devwork")
	if err := os.MkdirAll(dw, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Intentionally NO connections.md.
	dbPath := filepath.Join(dw, "store.db")
	seedLoadTask(t, dbPath, "tsk-002")

	var out string
	withCwd(t, projRoot, func() {
		out = captureStdout(t, func() {
			Run([]string{"load", "tsk-002", "--store=" + dbPath})
		})
	})

	if strings.Contains(out, "## Source paths") {
		t.Fatalf("unexpected Source paths block when no connections.md present:\n%s", out)
	}
	if !strings.Contains(out, "task context body") {
		t.Fatalf("baseline context body missing from bundle:\n%s", out)
	}
}

// TestLoad_ConnectionsMD_NoPathFields_NoBlock verifies that a connections.md
// that contains only non-path-ish sections (e.g. just MySQL command lines)
// does NOT produce a spurious empty Source paths block.
func TestLoad_ConnectionsMD_NoPathFields_NoBlock(t *testing.T) {
	projRoot := t.TempDir()
	dw := filepath.Join(projRoot, ".devwork")
	if err := os.MkdirAll(dw, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "# Connections\n\n## staging\n\n### MySQL\n\n```shell\nmysql --login-path=foo\n```\n"
	if err := os.WriteFile(filepath.Join(dw, "connections.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write connections: %v", err)
	}
	dbPath := filepath.Join(dw, "store.db")
	seedLoadTask(t, dbPath, "tsk-003")

	var out string
	withCwd(t, projRoot, func() {
		out = captureStdout(t, func() {
			Run([]string{"load", "tsk-003", "--store=" + dbPath})
		})
	})

	if strings.Contains(out, "## Source paths") {
		t.Fatalf("expected NO Source paths block (no path-ish heading); got:\n%s", out)
	}
}
