package promoter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtract(t *testing.T) {
	cases := map[string]string{
		`mysql -e "SELECT 1 FROM dealers"`:         "SELECT 1 FROM dealers",
		`psql -c 'SELECT id FROM users'`:           "SELECT id FROM users",
		"psql -d pilot-a <<SQL\nSELECT 2 FROM t\nSQL": "SELECT 2 FROM t",
		`echo "SELECT 3 FROM x" | mysql appdb`:     "SELECT 3 FROM x",
	}
	for cmd, want := range cases {
		got, ok := Extract(cmd)
		if !ok || got != want {
			t.Errorf("Extract(%q) = %q,%v; want %q", cmd, got, ok, want)
		}
	}
}

func TestParameterize(t *testing.T) {
	sql := "SELECT * FROM dealers WHERE dealer_id = 9 AND status = 'active'"
	got, params := Parameterize(sql)
	want := "SELECT * FROM dealers WHERE dealer_id = :dealer_id AND status = :status"
	if got != want {
		t.Errorf("Parameterize = %q; want %q", got, want)
	}
	if len(params) != 2 || params[0] != "dealer_id" || params[1] != "status" {
		t.Errorf("params = %v; want [dealer_id status]", params)
	}
}

func TestSuggestAndRender(t *testing.T) {
	sql, params := Parameterize("SELECT * FROM dealers WHERE dealer_id = 9")
	name := SuggestName(sql, params)
	if name != "dealers_by_dealer_id" {
		t.Errorf("name = %q; want dealers_by_dealer_id", name)
	}
	out := Render(name, sql, params)
	if !strings.Contains(out, "-v dealer_id=<value>") || !strings.HasSuffix(strings.TrimSpace(out), ";") {
		t.Errorf("Render missing run-hint or terminator:\n%s", out)
	}
}

func TestWalkUpDevwork(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".devwork"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	got, ok := WalkUpDevwork(nested)
	if !ok {
		t.Fatalf("WalkUpDevwork(%q) returned !ok", nested)
	}
	want := filepath.Join(root, ".devwork")
	// Resolve symlinks (macOS /var → /private/var, /tmp → /private/tmp) for stable compare.
	gotR, _ := filepath.EvalSymlinks(got)
	wantR, _ := filepath.EvalSymlinks(want)
	if gotR != wantR {
		t.Fatalf("WalkUpDevwork(%q) = %q; want %q", nested, gotR, wantR)
	}
}

func TestWalkUpDevwork_NotFound(t *testing.T) {
	// t.TempDir() returns a dir under /tmp which definitely has no .devwork/
	// above it in test environments.
	dir := t.TempDir()
	if _, ok := WalkUpDevwork(dir); ok {
		t.Fatalf("expected WalkUpDevwork to return !ok for dir without .devwork/ ancestor")
	}
}
