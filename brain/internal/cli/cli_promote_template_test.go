package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTemplateProject builds a fake project root with .devwork/<umbrella>/ and
// a draft file under .devwork/_scratch/. Returns (projectRoot, srcPath, umbrella).
func setupTemplateProject(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	umbrella := "acme-crm"
	// Just .devwork/ needs to exist for walk-up; the umbrella + templates are
	// what the promoter will create (or assert against).
	if err := os.MkdirAll(filepath.Join(root, ".devwork", umbrella), 0o755); err != nil {
		t.Fatalf("mkdir umbrella: %v", err)
	}
	scratch := filepath.Join(root, ".devwork", "_scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	src := filepath.Join(scratch, "report.template.html")
	if err := os.WriteFile(src, []byte("<html>{{ body }}</html>"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	return root, src, umbrella
}

func TestPromoteTemplate_DryRun(t *testing.T) {
	root, src, umb := setupTemplateProject(t)
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{
			"promote",
			"--kind=template",
			"--source=" + src,
			"--dest-umbrella=" + umb,
			"--cwd=" + root,
		})
	})
	if code != 0 {
		t.Fatalf("exit=%d, want 0; out=%q", code, out)
	}
	if !strings.Contains(out, "dry-run") {
		t.Fatalf("expected dry-run banner, got: %q", out)
	}
	wantDst := filepath.Join(root, ".devwork", umb, "templates", "report.template.html")
	if !strings.Contains(out, wantDst) {
		t.Fatalf("expected dest %q in output, got: %q", wantDst, out)
	}
	// Dry-run must NOT move the file.
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("dry-run moved source: %v", err)
	}
	if _, err := os.Stat(wantDst); err == nil {
		t.Fatalf("dry-run created destination: %s", wantDst)
	}
}

func TestPromoteTemplate_Write_MovesAndIndexes(t *testing.T) {
	root, src, umb := setupTemplateProject(t)
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{
			"promote",
			"--kind=template",
			"--source=" + src,
			"--dest-umbrella=" + umb,
			"--cwd=" + root,
			"--write=1",
		})
	})
	if code != 0 {
		t.Fatalf("exit=%d, want 0; out=%q", code, out)
	}
	wantDst := filepath.Join(root, ".devwork", umb, "templates", "report.template.html")
	if _, err := os.Stat(wantDst); err != nil {
		t.Fatalf("expected dest to exist: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected source removed, stat err=%v", err)
	}
	idx := filepath.Join(root, ".devwork", umb, "templates", "INDEX.md")
	data, err := os.ReadFile(idx)
	if err != nil {
		t.Fatalf("read INDEX.md: %v", err)
	}
	if !strings.Contains(string(data), "report.template.html") {
		t.Fatalf("INDEX.md missing row, got: %q", string(data))
	}
	if !strings.Contains(string(data), "# Templates — "+umb) {
		t.Fatalf("INDEX.md missing header, got: %q", string(data))
	}
}

func TestPromoteTemplate_RefusesClobberWithoutForce(t *testing.T) {
	root, src, umb := setupTemplateProject(t)
	tmplDir := filepath.Join(root, ".devwork", umb, "templates")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	dst := filepath.Join(tmplDir, "report.template.html")
	if err := os.WriteFile(dst, []byte("EXISTING"), 0o644); err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	code := Run([]string{
		"promote",
		"--kind=template",
		"--source=" + src,
		"--dest-umbrella=" + umb,
		"--cwd=" + root,
		"--write=1",
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit when destination exists without --force")
	}
	// Existing file must be untouched.
	data, _ := os.ReadFile(dst)
	if string(data) != "EXISTING" {
		t.Fatalf("destination clobbered without --force: got %q", string(data))
	}
}

func TestPromoteTemplate_DryRun_NotesExistingDest(t *testing.T) {
	root, src, umb := setupTemplateProject(t)
	tmplDir := filepath.Join(root, ".devwork", umb, "templates")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	dst := filepath.Join(tmplDir, "report.template.html")
	if err := os.WriteFile(dst, []byte("EXISTING"), 0o644); err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{
			"promote",
			"--kind=template",
			"--source=" + src,
			"--dest-umbrella=" + umb,
			"--cwd=" + root,
		})
	})
	if code != 0 {
		t.Fatalf("dry-run should exit 0 even with existing dest, got %d: %q", code, out)
	}
	if !strings.Contains(out, "destination exists") {
		t.Fatalf("expected conflict note in dry-run, got: %q", out)
	}
	// Existing file must still be untouched.
	data, _ := os.ReadFile(dst)
	if string(data) != "EXISTING" {
		t.Fatalf("dry-run touched destination: %q", string(data))
	}
}

func TestPromoteTemplate_ForceOverwrites(t *testing.T) {
	root, src, umb := setupTemplateProject(t)
	tmplDir := filepath.Join(root, ".devwork", umb, "templates")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	dst := filepath.Join(tmplDir, "report.template.html")
	if err := os.WriteFile(dst, []byte("EXISTING"), 0o644); err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	code := Run([]string{
		"promote",
		"--kind=template",
		"--source=" + src,
		"--dest-umbrella=" + umb,
		"--cwd=" + root,
		"--write=1",
		"--force=1",
	})
	if code != 0 {
		t.Fatalf("exit=%d, want 0 with --force", code)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "<html>{{ body }}</html>" {
		t.Fatalf("expected overwrite with source content, got: %q", string(data))
	}
}

func TestPromoteTemplate_MissingSource(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".devwork", "u1"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	code := Run([]string{
		"promote",
		"--kind=template",
		"--source=/no/such/path.html",
		"--dest-umbrella=u1",
		"--cwd=" + root,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit for missing source")
	}
}

func TestPromoteTemplate_MissingDevwork(t *testing.T) {
	// anchor with NO .devwork/ anywhere above it (use tempdir; macOS /tmp may
	// resolve to /private/tmp but neither has .devwork/).
	root := t.TempDir()
	src := filepath.Join(root, "draft.html")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	code := Run([]string{
		"promote",
		"--kind=template",
		"--source=" + src,
		"--dest-umbrella=u1",
		"--cwd=" + root,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit when no .devwork/ found")
	}
}

func TestPromote_DefaultKindIsSQL(t *testing.T) {
	// Sanity: default kind preserves the SQL path. No --kind flag -> needs --command.
	code := Run([]string{"promote"})
	if code == 0 {
		t.Fatalf("expected non-zero from kind=sql with no --command (preserved behavior)")
	}
}

func TestPromote_UnknownKind(t *testing.T) {
	code := Run([]string{"promote", "--kind=bogus"})
	if code == 0 {
		t.Fatalf("expected non-zero from unknown --kind")
	}
}
