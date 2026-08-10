package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wosy.local/brain/internal/store"
)

func openStoreForTest(t *testing.T, dbPath string) *store.Store {
	t.Helper()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	return s
}

func TestExtractTopLevelYAMLStatus(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "status: pending\n", "pending"},
		{"quoted", `status: "in-progress"` + "\n", "in-progress"},
		{"single-quoted", "status: 'blocked'\n", "blocked"},
		{"with-inline-comment", "status: done # finalized 2026-06-08\n", "done"},
		{"comment-no-space-preserved", "status: foo#bar\n", "foo#bar"},
		{"block-scalar-pipe", "status: |\n  multi\n  line\n", ""},
		{"block-scalar-fold", "status: >\n  multi line\n", ""},
		{"block-scalar-pipe-strip", "status: |-\n  x\n", ""},
		{"empty", "status:\n", ""},
		{"empty-quoted", `status: ""` + "\n", ""},
		{"missing", "id: x\ntype: task\n", ""},
		{"indented-skipped", "  status: nope\nstatus: yes\n", "yes"},
		{"frontmatter-delim-skipped", "---\nstatus: in-progress\n---\n", "in-progress"},
		{"only-comments", "# status: nope\n# status: also-nope\n", ""},
		{"hyphen-line-skipped", "- status: nope\nstatus: real\n", "real"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractTopLevelYAMLStatus(c.in)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestReconcile_RefreshesRecentTask plants a project + a synthetic
// plans/<id>/tasks/<id>/task.yml tree on disk, runs cmdReconcile, and
// verifies the recent_task row materialized via the new YAML peeker.
func TestReconcile_RefreshesRecentTask(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "brain.db")
	projectRoot := filepath.Join(tmp, "projects", "test-proj")
	s := seedProject(t, dbPath, "test-proj", "team-1")
	if _, err := s.DB.Exec(`UPDATE project SET root_path=? WHERE id='test-proj'`, projectRoot); err != nil {
		s.Close()
		t.Fatalf("set root_path: %v", err)
	}
	s.Close()

	taskDir := filepath.Join(projectRoot, "plans", "plan-A", "tasks", "task-1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	taskYML := "id: task-1\nplan_id: plan-A\nstatus: in-progress # active\n"
	if err := os.WriteFile(filepath.Join(taskDir, "task.yml"), []byte(taskYML), 0o644); err != nil {
		t.Fatalf("write task.yml: %v", err)
	}

	// Construct args inline (cli_test pattern).
	p := args{flag: map[string]string{"store": dbPath}, store: dbPath, pos: []string{"test-proj"}}
	code := cmdReconcile(p)
	if code != 0 {
		t.Fatalf("cmdReconcile exit %d", code)
	}

	// Re-open to verify.
	s2 := openStoreForTest(t, dbPath)
	defer s2.Close()
	var status string
	err := s2.DB.QueryRow(
		`SELECT status FROM recent_task WHERE project_id='test-proj' AND plan_id='plan-A' AND task_id='task-1'`,
	).Scan(&status)
	if err != nil {
		t.Fatalf("recent_task lookup: %v", err)
	}
	// Inline-comment strip must have rescued this from CHECK constraint.
	if status != "in-progress" {
		t.Errorf("recent_task.status = %q, want in-progress", status)
	}
}

// TestReconcile_DefaultsWhenStatusBlockScalar ensures a task.yml with a
// block-scalar status (which extractTopLevelYAMLStatus returns "" for)
// defaults to "pending" and does not trip the CHECK constraint.
func TestReconcile_DefaultsWhenStatusBlockScalar(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "brain.db")
	projectRoot := filepath.Join(tmp, "projects", "p2")
	s := seedProject(t, dbPath, "p2", "team-1")
	if _, err := s.DB.Exec(`UPDATE project SET root_path=? WHERE id='p2'`, projectRoot); err != nil {
		s.Close()
		t.Fatalf("set root_path: %v", err)
	}
	s.Close()

	taskDir := filepath.Join(projectRoot, "plans", "plan-B", "tasks", "task-X")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	taskYML := "id: task-X\nplan_id: plan-B\nstatus: |\n  active\n  more\n"
	if err := os.WriteFile(filepath.Join(taskDir, "task.yml"), []byte(taskYML), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := args{flag: map[string]string{"store": dbPath}, store: dbPath, pos: []string{"p2"}}
	if code := cmdReconcile(p); code != 0 {
		t.Fatalf("cmdReconcile exit %d (block-scalar status should default to pending, not error)", code)
	}
	s2 := openStoreForTest(t, dbPath)
	defer s2.Close()
	var status string
	if err := s2.DB.QueryRow(
		`SELECT status FROM recent_task WHERE project_id='p2'`,
	).Scan(&status); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if status != "pending" {
		t.Errorf("default = %q, want pending", status)
	}
}

// TestReconcile_AppliesMaintainDeltas: full Phase III-A4 end-to-end.
// Plants a task.yml with maintain.status=done + encyclopedia_delta +
// runbook_delta + 2 gotchas, runs cmdReconcile, then verifies:
//   - encyclopedia.md was created with the delta under a per-source marker
//   - runbooks/<name>.md was written with the body
//   - both gotcha rows landed in brain.db with the right source_ref
//   - re-running is idempotent (no duplicate appends, no duplicate gotchas)
func TestReconcile_AppliesMaintainDeltas(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "brain.db")
	projectRoot := filepath.Join(tmp, "projects", "p3")
	s := seedProject(t, dbPath, "p3", "team-1")
	if _, err := s.DB.Exec(`UPDATE project SET root_path=? WHERE id='p3'`, projectRoot); err != nil {
		s.Close()
		t.Fatalf("set root_path: %v", err)
	}
	s.Close()

	taskDir := filepath.Join(projectRoot, "plans", "plan-X", "tasks", "fix-cred")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	taskYML := `id: fix-cred
plan_id: plan-X
status: done
implement:
  status: done
  done_when: shipped
verify:
  status: done
  done_when: smoked
test:
  status: skipped
  done_when: n/a
maintain:
  status: done
  done_when: deltas merged
  encyclopedia_delta: |
    ### Image upload silent failures (2026-05-30 → 2026-06-08)
    Cause: AWS credential rotation did not propagate to worker-2 .env.
  runbook_delta:
    name: s3-credential-stale-fix
    body: |
      # S3 credential stale fix
      1. ssh worker; grep AWS_SECRET_ACCESS_KEY /etc/env.d/*
      2. Replace + SIGHUP.
  gotchas:
    - severity: warn
      body: |
        upload.php swallows S3 403s as DEBUG, not WARN.
    - severity: info
      body: |
        Credential rotation must SIGHUP each worker individually.
`
	if err := os.WriteFile(filepath.Join(taskDir, "task.yml"), []byte(taskYML), 0o644); err != nil {
		t.Fatalf("write task.yml: %v", err)
	}

	p := args{flag: map[string]string{"store": dbPath}, store: dbPath, pos: []string{"p3"}}
	if code := cmdReconcile(p); code != 0 {
		t.Fatalf("cmdReconcile pass 1 exit %d", code)
	}

	// 1) encyclopedia.md was written with the marker + body.
	enc, err := os.ReadFile(filepath.Join(projectRoot, "encyclopedia.md"))
	if err != nil {
		t.Fatalf("encyclopedia.md: %v", err)
	}
	encStr := string(enc)
	wantMarker := "<!-- reconciled-from: plan-X/fix-cred -->"
	if !strings.Contains(encStr, wantMarker) {
		t.Errorf("encyclopedia.md missing marker %q\n%s", wantMarker, encStr)
	}
	if !strings.Contains(encStr, "Image upload silent failures") {
		t.Errorf("encyclopedia.md missing delta body:\n%s", encStr)
	}

	// 2) runbooks/s3-credential-stale-fix.md was written.
	rb, err := os.ReadFile(filepath.Join(projectRoot, "runbooks", "s3-credential-stale-fix.md"))
	if err != nil {
		t.Fatalf("runbook: %v", err)
	}
	if !strings.Contains(string(rb), "# S3 credential stale fix") {
		t.Errorf("runbook body wrong:\n%s", string(rb))
	}

	// 3) 2 gotcha rows landed with the right source_ref.
	s2 := openStoreForTest(t, dbPath)
	defer s2.Close()
	var nGotcha int
	if err := s2.DB.QueryRow(
		`SELECT COUNT(1) FROM gotcha WHERE project_id='p3' AND source_ref='plan-X/fix-cred'`,
	).Scan(&nGotcha); err != nil {
		t.Fatalf("gotcha count: %v", err)
	}
	if nGotcha != 2 {
		t.Errorf("gotcha count = %d, want 2", nGotcha)
	}

	// 4) Idempotency: re-run does not duplicate.
	if code := cmdReconcile(p); code != 0 {
		t.Fatalf("cmdReconcile pass 2 exit %d", code)
	}
	enc2, _ := os.ReadFile(filepath.Join(projectRoot, "encyclopedia.md"))
	if strings.Count(string(enc2), wantMarker) != 1 {
		t.Errorf("encyclopedia.md marker not idempotent (count=%d):\n%s",
			strings.Count(string(enc2), wantMarker), string(enc2))
	}
	if err := s2.DB.QueryRow(
		`SELECT COUNT(1) FROM gotcha WHERE project_id='p3' AND source_ref='plan-X/fix-cred'`,
	).Scan(&nGotcha); err != nil {
		t.Fatalf("gotcha count pass2: %v", err)
	}
	if nGotcha != 2 {
		t.Errorf("gotcha count after rerun = %d, want 2 (idempotent insert)", nGotcha)
	}
}

// TestReconcile_SkipsMaintainPending: a task with maintain.status != "done"
// must NOT apply any deltas (we only ship when maintain stage is done).
func TestReconcile_SkipsMaintainPending(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "brain.db")
	projectRoot := filepath.Join(tmp, "projects", "p4")
	s := seedProject(t, dbPath, "p4", "team-1")
	if _, err := s.DB.Exec(`UPDATE project SET root_path=? WHERE id='p4'`, projectRoot); err != nil {
		s.Close()
		t.Fatalf("set root_path: %v", err)
	}
	s.Close()

	taskDir := filepath.Join(projectRoot, "plans", "plan-Y", "tasks", "wip-task")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	taskYML := `id: wip-task
plan_id: plan-Y
status: in-progress
maintain:
  status: pending
  done_when: later
  encyclopedia_delta: |
    Should NOT be applied — maintain pending.
`
	if err := os.WriteFile(filepath.Join(taskDir, "task.yml"), []byte(taskYML), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := args{flag: map[string]string{"store": dbPath}, store: dbPath, pos: []string{"p4"}}
	if code := cmdReconcile(p); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "encyclopedia.md")); !os.IsNotExist(err) {
		t.Errorf("encyclopedia.md should not exist (maintain pending)")
	}
}

func TestReconcile_NoPlansDir(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "brain.db")
	s := seedProject(t, dbPath, "no-plans", "team-1")
	if _, err := s.DB.Exec(`UPDATE project SET root_path=? WHERE id='no-plans'`,
		filepath.Join(tmp, "projects", "no-plans"),
	); err != nil {
		s.Close()
		t.Fatalf("set root_path: %v", err)
	}
	s.Close()
	p := args{flag: map[string]string{"store": dbPath}, store: dbPath, pos: []string{"no-plans"}}
	if code := cmdReconcile(p); code != 0 {
		t.Fatalf("cmdReconcile exit %d on missing plans/ (should no-op, not error)", code)
	}
}
