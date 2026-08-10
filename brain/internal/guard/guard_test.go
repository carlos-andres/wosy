package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// proj builds a temp project dir with optional flags + provisioned tools.
func proj(t *testing.T, flags string, withLib, withConn, withStore bool) string {
	t.Helper()
	root := t.TempDir()
	dw := filepath.Join(root, ".devwork")
	must(t, os.MkdirAll(dw, 0o755))
	if flags != "" {
		must(t, os.WriteFile(filepath.Join(dw, "wosy.flags"), []byte(flags), 0o644))
	}
	if withLib {
		lib := filepath.Join(dw, "queries", "library")
		must(t, os.MkdirAll(lib, 0o755))
		must(t, os.WriteFile(filepath.Join(lib, "x.sql"), []byte("SELECT 1;"), 0o644))
	}
	if withConn {
		must(t, os.WriteFile(filepath.Join(dw, "connections.md"), []byte("engine: pgsql\n"), 0o644))
	}
	if withStore {
		must(t, os.MkdirAll(filepath.Join(dw, "brain"), 0o755))
		must(t, os.WriteFile(filepath.Join(dw, "brain", "store.db"), []byte("x"), 0o644))
	}
	return root
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestClassify(t *testing.T) {
	const onAll = "enforce_inline_sql=1\nenforce_inline_ssh=1\nenforce_kb_scribe=1\nenforce_destructive_kb=1\n"
	const sql = `mysql -e "SELECT * FROM dealers WHERE id=9"`

	cases := []struct {
		name       string
		flags      string
		lib, conn  bool
		store      bool
		in         Input
		wantAction Action
		wantClass  string
	}{
		{"fail-open: no flags, inline SQL", "", false, false, false,
			Input{Tool: "Bash", Command: sql}, Allow, ""},
		{"enforce+library: inline SQL blocked", onAll, true, false, false,
			Input{Tool: "Bash", Command: sql}, Block, "inline_sql"},
		{"enforce but NO library: allow (provision-then-enable)", onAll, false, false, false,
			Input{Tool: "Bash", Command: sql}, Allow, ""},
		{"sanctioned q.sh: allow even with enforcement", onAll, true, false, false,
			Input{Tool: "Bash", Command: ".devwork/bin/q.sh queries/library/x.sql"}, Allow, ""},
		{"heredoc psql blocked", onAll, true, false, false,
			Input{Tool: "Bash", Command: "psql -d pilot-a <<SQL\nSELECT 1;\nSQL"}, Block, "inline_sql"},
		{"pipe into mysql blocked", onAll, true, false, false,
			Input{Tool: "Bash", Command: `echo "SELECT 1" | mysql appdb`}, Block, "inline_sql"},
		{"source command flows free", onAll, true, false, false,
			Input{Tool: "Bash", Command: "go build ./... && php artisan migrate"}, Allow, ""},
		{"inline ssh blocked", onAll, false, true, false,
			Input{Tool: "Bash", Command: `ssh tower "systemctl restart worker"`}, Block, "inline_ssh"},
		{"ssh enforce but no connections doc: allow", onAll, false, false, false,
			Input{Tool: "Bash", Command: `ssh tower "uptime"`}, Allow, ""},
		{"write source file flows free", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/app/Services/DealerService.php"}, Allow, ""},

		// --- 2026-05-29 B1 policy: task-folder files are files-canonical, flow free ---
		// Pre-B1 these were blocked by reKBTaskFile + kbBase basename rules. The 3-repo
		// empirical audit (pilot-a 1/11 brain adoption, Globex 3/14) showed users
		// always fall back to plain files. The documented task-folder contract is now
		// the classifier's accepted shape.
		{"B1: status.md in task folder allowed (files-canonical)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/tasks/t1/status.md"}, Allow, ""},
		{"B1: context.md in task folder allowed", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/tasks/PROJ-1414/context.md"}, Allow, ""},
		{"B1: scope.md in task folder allowed", onAll, false, false, true,
			Input{Tool: "Edit", Path: "/proj/.devwork/tasks/cust-100200300/scope.md"}, Allow, ""},
		{"B1: custom name findings.md in task folder allowed", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/tasks/PROJ-1413/findings.md"}, Allow, ""},
		{"B1: custom name verification.md in task folder allowed", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/tasks/topbar-missing-routes/verification.md"}, Allow, ""},
		{"B1: custom name inventory.md in task folder allowed", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/tasks/t1/inventory.md"}, Allow, ""},
		{"B1: nested task folder file allowed", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/tasks/PROJ-1414/subdir/note.md"}, Allow, ""},
		{"B1: rm -rf task folder allowed (user owns it)", onAll, false, false, true,
			Input{Tool: "Bash", Command: "rm -rf /proj/.devwork/tasks/old-ticket"}, Allow, ""},

		// pre-B1 the next case asserted block; now it asserts allow (the rule that
		// caught it was removed). Kept for explicit no-regression visibility.
		{"B1 ex-block: same path with enforcement off — allow", "", false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/tasks/t1/status.md"}, Allow, ""},

		{"G9: source UNDER .devwork/brain flows free", onAll, false, false, true,
			Input{Tool: "Edit", Path: "/proj/.devwork/brain/internal/store/record.go"}, Allow, ""},
		{"G9: KB json under .devwork/brain still blocked", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/brain/records/x.json"}, Block, "kb_write"},

		// 2026-05-29 B1: consolidated.md is no longer in kbBase (shipped-marker
		// convention was 2% adoption — broken signal). Root-level consolidated.md
		// now flows free; if the convention is revived later, add a stricter rule.
		{"B1: consolidated.md at .devwork root allowed (deprecated marker)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/consolidated.md"}, Allow, ""},

		{"kill switch wosy_enforce=0: allow inline SQL", "wosy_enforce=0\n" + onAll, true, false, false,
			Input{Tool: "Bash", Command: sql}, Allow, ""},

		// --- G2: destructive ops against KB paths ---
		{"G2: rm -rf .devwork/brain blocked", onAll, false, false, true,
			Input{Tool: "Bash", Command: "rm -rf .devwork/brain"}, Block, "destructive_kb"},
		{"G2: same command, flag off → allow (fail-open)",
			"wosy_enforce=1\nenforce_kb_scribe=1\n", false, false, true,
			Input{Tool: "Bash", Command: "rm -rf .devwork/brain"}, Allow, ""},
		{"G2: rm -rf .devwork/brain in /tmp (no flags up-tree) → allow",
			"", false, false, false,
			Input{Tool: "Bash", Command: "rm -rf /tmp/nowhere/.devwork/brain"}, Allow, ""},
		{"G2: rm app/Controllers/X.php (source) → allow",
			onAll, false, false, true,
			Input{Tool: "Bash", Command: "rm app/Controllers/X.php"}, Allow, ""},
		{"G2: rm -rf vendor/ (source) → allow",
			onAll, false, false, true,
			Input{Tool: "Bash", Command: "rm -rf vendor/"}, Allow, ""},
		{"G2: rm -rf node_modules/ → allow",
			onAll, false, false, true,
			Input{Tool: "Bash", Command: "rm -rf node_modules/"}, Allow, ""},
		{"G2: rm tests/foo.test.js → allow",
			onAll, false, false, true,
			Input{Tool: "Bash", Command: "rm tests/foo.test.js"}, Allow, ""},
		{"G2: mv .devwork/brain /tmp/ blocked",
			onAll, false, false, true,
			Input{Tool: "Bash", Command: "mv .devwork/brain /tmp/"}, Block, "destructive_kb"},
		{"G2: rm absolute .devwork/state.yml blocked",
			onAll, false, false, true,
			Input{Tool: "Bash", Command: "rm /proj/.devwork/state.yml"}, Block, "destructive_kb"},
		{"G2: rm absolute .devwork/ledger.yml blocked",
			onAll, false, false, true,
			Input{Tool: "Bash", Command: "rm /proj/.devwork/ledger.yml"}, Block, "destructive_kb"},
		{"G2: find .devwork -delete blocked",
			onAll, false, false, true,
			Input{Tool: "Bash", Command: "find .devwork -delete"}, Block, "destructive_kb"},
		{"G2: find . -name '*.tmp' (no -delete) → allow",
			onAll, false, false, true,
			Input{Tool: "Bash", Command: "find . -name '*.tmp'"}, Allow, ""},
		{"G2: rm -rf .devwork/brain, store missing → allow (provision-then-enable)",
			onAll, false, false, false,
			Input{Tool: "Bash", Command: "rm -rf .devwork/brain"}, Allow, ""},
		{"G2: cd /tmp && rm -rf .devwork/brain — per-clause classification",
			onAll, false, false, true,
			Input{Tool: "Bash", Command: "cd /tmp && rm -rf .devwork/brain"}, Block, "destructive_kb"},

		// --- scribe-poc-b: widened KB scope (any .md/.yml/.yaml/.jsonl under .devwork/, with exclusions) ---
		{"scribe-poc-b: write specs/.md blocked", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/specs/feature-x.md"}, Block, "kb_write"},
		{"scribe-poc-b: edit decisions/.md blocked", onAll, false, false, true,
			Input{Tool: "Edit", Path: "/proj/.devwork/decisions/adr-007.md"}, Block, "kb_write"},
		{"scribe-poc-b: write plans/.md blocked", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/plans/migration.md"}, Block, "kb_write"},
		{"scribe-poc-b: write intake/.md blocked", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/intake/incoming.md"}, Block, "kb_write"},
		{"scribe-poc-b: project-nested specs/.md blocked", onAll, false, false, true,
			Input{Tool: "Edit", Path: "/proj/.devwork/acme-crm/specs/atw.md"}, Block, "kb_write"},
		{"scribe-poc-b: .yml under .devwork blocked", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/decisions/log.yml"}, Block, "kb_write"},
		{"scribe-poc-b: .jsonl under .devwork blocked", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/intake/inbox.jsonl"}, Block, "kb_write"},
		{"scribe-poc-b: _scratch/ excluded — allow", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/_scratch/draft.md"}, Allow, ""},
		{"scribe-poc-b: _archive/ excluded — allow", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/_archive/old.md"}, Allow, ""},
		{"scribe-poc-b: research/ excluded — allow", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/research/findings.md"}, Allow, ""},
		{"scribe-poc-b: data/ excluded — allow", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/data/sample.jsonl"}, Allow, ""},
		{"scribe-poc-b: source .go inside specs/ — allow (srcExt wins)", onAll, false, false, true,
			Input{Tool: "Edit", Path: "/proj/.devwork/specs/sample.go"}, Allow, ""},
		{"scribe-poc-b: write .md in specs but no enforcement — allow", "", false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/specs/feature-x.md"}, Allow, ""},
		{"scribe-poc-b: write .md in specs, enforcement on but no store — allow",
			"wosy_enforce=1\nenforce_kb_scribe=1\n", false, false, false,
			Input{Tool: "Write", Path: "/proj/.devwork/specs/feature-x.md"}, Allow, ""},

		// --- 2026-05-29 fixes: kbBase exclude-segment + root-level widening ---
		// (a) kbBase basename match (status.md / context.md / etc.) must honor
		// kbExcludeSegments so drafts under _scratch/ flow free even when named
		// after a canonical task file. Pre-fix bug: .devwork/_scratch/status.md
		// returned block(kb_write) under enforced flags; user fell back to
		// session-tracker.md as a workaround. After fix: _scratch/<anything>.md flows.
		//
		// NOTE 2026-05-29 B1: status.md / context.md were REMOVED from kbBase as part
		// of the files-canonical policy, so the exclude-honor check on those names
		// is moot. STATUS.md (uppercase, brain-home file) and state.yml (kbBase)
		// remain the live regression guards for this fix.
		{"2026-05-29: STATUS.md under _scratch/ allowed (kbBase + exclude)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/_scratch/STATUS.md"}, Allow, ""},
		{"2026-05-29: state.yml under _scratch/ allowed (kbBase + exclude)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/_scratch/state.yml"}, Allow, ""},
		{"2026-05-29: state.yml under _scratch/<id>/ allowed", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/_scratch/PROJ-1414/state.yml"}, Allow, ""},
		{"2026-05-29: STATUS.md outside _scratch still blocked (regression guard)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/decisions/STATUS.md"}, Block, "kb_write"},

		// (b) Root-level .devwork/foo.md flows free unless it's an explicit kbBase
		// entry. Pre-fix bug: .devwork/connections.md was caught by the widened
		// kbExt rule and produced a deadlock — file is not a brain record, but
		// Write was blocked and brain set errors on it. After fix: root-level
		// non-kbBase files flow.
		{"2026-05-29: connections.md at root allowed (not a brain record)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/connections.md"}, Allow, ""},
		{"2026-05-29: INDEX.md at root allowed (human-edited doc)", onAll, false, false, true,
			Input{Tool: "Edit", Path: "/proj/.devwork/INDEX.md"}, Allow, ""},
		{"2026-05-29: BACKLOG.md at root allowed (human-edited doc)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/BACKLOG.md"}, Allow, ""},
		// Regression guard — explicit kbBase entries still trigger at root:
		{"2026-05-29: STATUS.md at root still blocked (explicit kbBase)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/STATUS.md"}, Block, "kb_write"},
		{"2026-05-29: state.yml at root still blocked (explicit kbBase)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/state.yml"}, Block, "kb_write"},
		{"2026-05-29: ledger.yml at root still blocked (explicit kbBase)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/ledger.yml"}, Block, "kb_write"},
		// Regression guard — nested subfolder rule still fires:
		{"2026-05-29: decisions/<X>.md still blocked (nested kbExt rule)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/decisions/adr-008.md"}, Block, "kb_write"},

		// --- 2026-06-08 Phase IB I-6: new C-02/C-03 path families ---
		// Hard-block: plan.yml + task.yml (canonical 1:1 paths, no draft variant).
		{"PIB-I6: plan.yml deep blocks (kbBaseHard, regardless of subdir)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/platform/projects/acme-crm/plans/cdw-2409/plan.yml"}, Block, "kb_write"},
		{"PIB-I6: task.yml deep blocks (kbBaseHard, /tasks/ exclude does NOT apply)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/platform/projects/acme-crm/plans/cdw-2409/tasks/fix-cred/task.yml"}, Block, "kb_write"},
		{"PIB-I6: plan.yml at .devwork root still blocks (kbBaseHard)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/plan.yml"}, Block, "kb_write"},
		// Hard-allow: handoff.md (plan-level vigilante-rendered markdown).
		{"PIB-I6: handoff.md deep allows (kbBaseAllow short-circuit)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/platform/projects/acme-crm/plans/cdw-2409/handoff.md"}, Allow, ""},
		{"PIB-I6: handoff.md at .devwork root allows (kbBaseAllow)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/handoff.md"}, Allow, ""},
		// Per-team brain.db (suffix block alongside legacy store.db).
		{"PIB-I6: <team>/brain.db blocks (per-team store substrate)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/platform/brain.db"}, Block, "kb_write"},
		{"PIB-I6: workspace brain.db blocks (fallback per C-03)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/brain.db"}, Block, "kb_write"},
		// Task NOTES deep — /tasks/ exclude must match at any depth (Contains, not HasPrefix).
		{"PIB-I6: task notes deep allows (/tasks/ Contains-match at depth)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/platform/projects/acme-crm/plans/cdw-2409/tasks/fix-cred/findings.md"}, Allow, ""},
		{"PIB-I6: task custom note deep allows (verification.md)", onAll, false, false, true,
			Input{Tool: "Edit", Path: "/proj/.devwork/platform/projects/acme-crm/plans/cdw-2409/tasks/fix-cred/verification.md"}, Allow, ""},
		// Interview/ documented-gap closure (Phase IB I-6 contract).
		{"PIB-I6: .devwork/interview/<id>.md allows (gap closed, C-01 storage)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/interview/wosy-optimization-audit.md"}, Allow, ""},
		{"PIB-I6: project-nested interview/<id>.md allows", onAll, false, false, true,
			Input{Tool: "Edit", Path: "/proj/.devwork/acme-crm/interview/triage.md"}, Allow, ""},
		// Interaction guard (reviewer MEDIUM 1): a kbBase-named file inside a /tasks/
		// subtree must flow free — the soft-block gate must yield to the exclude.
		{"PIB-I6: STATUS.md inside deep /tasks/ subtree allows (kbBase + exclude interaction)", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/platform/projects/acme-crm/plans/cdw-2409/tasks/fix-cred/STATUS.md"}, Allow, ""},
		// Regression guard (reviewer MEDIUM 2): the inKBExcludedSegment delegation
		// in the widened-rule branch must NOT accidentally open the killed legacy
		// families. specs/decisions/intake stay blocked under the new Contains match.
		{"PIB-I6 regression: specs/<id>.md still blocks under new Contains-match widened rule", onAll, false, false, true,
			Input{Tool: "Write", Path: "/proj/.devwork/specs/legacy.md"}, Block, "kb_write"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := proj(t, c.flags, c.lib, c.conn, c.store)
			in := c.in
			in.Cwd = root
			// rewrite KB/source paths to live under the temp root so isKBPath sees a real .devwork
			if in.Path != "" {
				in.Path = root + in.Path[len("/proj"):]
			}
			// G2: also rewrite /proj/ inside Bash command strings to the temp root
			if in.Command != "" {
				in.Command = strings.ReplaceAll(in.Command, "/proj/", root+"/")
			}
			d := Classify(in)
			if d.Action != c.wantAction {
				t.Fatalf("action = %s, want %s (reason: %s)", d.Action, c.wantAction, d.Reason)
			}
			if d.Class != c.wantClass {
				t.Fatalf("class = %q, want %q", d.Class, c.wantClass)
			}
		})
	}
}

// Regression: Claude Code invokes the PreToolUse hook with cwd = session cwd
// (often $HOME), not dirname(file_path). cwd-anchored walk-up missed flags that
// live alongside the target file, so live R10 fell through fail-open even with
// the hook installed and flags on. Fix: lookupRoot anchors write-class tools on
// dirname(Path). This test guards against regression.
func TestClassify_WriteWithUnrelatedCwd(t *testing.T) {
	const flags = "wosy_enforce=1\nenforce_kb_scribe=1\n"
	root := proj(t, flags, false, false, true)
	unrelated := t.TempDir() // a directory with no wosy.flags up-tree

	in := Input{
		Tool: "Write",
		Path: filepath.Join(root, ".devwork", "brain", "probe.json"),
		Cwd:  unrelated,
	}
	d := Classify(in)
	if d.Action != Block || d.Class != "kb_write" {
		t.Fatalf("want BLOCK kb_write (path-anchored walk-up), got %s class=%q reason=%q",
			d.Action, d.Class, d.Reason)
	}

	// Sanity: with the OLD buggy behavior (Cwd unrelated, no Path anchor), this
	// would fall through to fail-open. Confirm the kill switch still works via the
	// path anchor too:
	killFlags := "wosy_enforce=0\nenforce_kb_scribe=1\n"
	killRoot := proj(t, killFlags, false, false, true)
	in2 := Input{
		Tool: "Write",
		Path: filepath.Join(killRoot, ".devwork", "brain", "probe.json"),
		Cwd:  unrelated,
	}
	d2 := Classify(in2)
	if d2.Action != Allow {
		t.Fatalf("kill switch via path anchor: want ALLOW, got %s reason=%q", d2.Action, d2.Reason)
	}
}

// 2026-05-29 (1.3.2): every block path now ships a per-class fix_hint in the
// Decision JSON. The watchdog shim relays it verbatim (no case statement).
// Tests cover the 5 block classes: kb_write, inline_sql, inline_ssh,
// destructive_kb, bash_kb_write. Each fix_hint must (a) be non-empty,
// (b) name at least one sanctioned brain CLI verb OR sanctioned tool,
// (c) end with the brain --help promotion tail for the 4 brain-CLI classes
// (inline_ssh is the exception — connection alias is the right answer, not brain).
func TestClassify_FixHint(t *testing.T) {
	const onAll = "enforce_inline_sql=1\nenforce_inline_ssh=1\nenforce_kb_scribe=1\nenforce_bash_kb_writes=1\nenforce_destructive_kb=1\n"

	cases := []struct {
		name       string
		mkProj     func(*testing.T) string
		in         Input
		wantClass  string
		wantSubstr []string // every substring must appear in fix_hint
	}{
		{
			name:   "kb_write fix_hint names brain set/import/_scratch + brain --help",
			mkProj: func(t *testing.T) string { return proj(t, onAll, false, false, true) },
			in: Input{Tool: "Write",
				Path: filepath.Join("REPLACE", ".devwork", "specs", "feature-x.md")},
			wantClass:  "kb_write",
			wantSubstr: []string{"brain set", "brain import", "_scratch/", "brain --help"},
		},
		{
			name:   "inline_sql fix_hint names q.sh + brain promote + brain --help",
			mkProj: func(t *testing.T) string { return proj(t, onAll, true, false, false) },
			in: Input{Tool: "Bash",
				Command: `psql -c "SELECT 1"`},
			wantClass:  "inline_sql",
			wantSubstr: []string{"q.sh", "brain promote", "brain --help"},
		},
		{
			name:   "inline_ssh fix_hint names connections.md (no brain --help — connections is the right answer)",
			mkProj: func(t *testing.T) string { return proj(t, onAll, false, true, false) },
			in: Input{Tool: "Bash",
				Command: `ssh tower "uptime"`},
			wantClass:  "inline_ssh",
			wantSubstr: []string{"connections.md"},
		},
		{
			name:   "destructive_kb fix_hint names brain CLI + warns about task folders",
			mkProj: func(t *testing.T) string { return proj(t, onAll, false, false, true) },
			in: Input{Tool: "Bash",
				Command: "rm -rf .devwork/brain"},
			wantClass:  "destructive_kb",
			wantSubstr: []string{"brain", "tasks/", "user-owned", "brain --help"},
		},
		{
			name:   "bash_kb_write fix_hint names brain set/append/import + _scratch + brain --help",
			mkProj: func(t *testing.T) string { return proj(t, onAll, false, false, true) },
			in: Input{Tool: "Bash",
				Command: "echo x > .devwork/specs/feature-x.md"},
			wantClass:  "bash_kb_write",
			wantSubstr: []string{"brain set/append", "brain import", "_scratch/", "brain --help"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := c.mkProj(t)
			in := c.in
			in.Cwd = root
			if in.Path != "" {
				// rewrite REPLACE prefix in path to the temp root
				in.Path = strings.Replace(in.Path, "REPLACE", root, 1)
			}
			d := Classify(in)
			if d.Action != Block {
				t.Fatalf("want BLOCK, got %s reason=%q", d.Action, d.Reason)
			}
			if d.Class != c.wantClass {
				t.Fatalf("class = %q, want %q", d.Class, c.wantClass)
			}
			if d.FixHint == "" {
				t.Fatalf("FixHint empty for class %q", d.Class)
			}
			for _, want := range c.wantSubstr {
				if !strings.Contains(d.FixHint, want) {
					t.Fatalf("FixHint missing %q\n  got: %s", want, d.FixHint)
				}
			}
		})
	}
}

// 2026-05-29: TestClassify_ScribeBypass removed. The WOSY_SCRIBE_BYPASS POC
// (scribe-poc-b) was removed from Classify in the same commit — Claude Code
// subagent tool calls do not inherit env vars set via `Bash export …` across
// to subsequent Write/Edit calls (each tool call gets a fresh process env),
// so the POC was unreachable from inside a session. The 2026-05-28 transcript
// audit showed agents repeatedly citing the env var without success. Removed
// as dead code; the contract is now: brain CLI for KB-protected paths, Write
// to .devwork/_scratch/ for drafts. See claude/agents/agent-scribe.md.
