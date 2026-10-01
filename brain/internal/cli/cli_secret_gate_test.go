package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// plantedCred is built at runtime so no credential-shaped literal reaches git (factory logbook 14).
func plantedCred() string {
	return "mysql://" + "fixture" + ":" + "notasecret9" + "@" + "db.example/tc"
}

// srs F1 on every free-text write path: import, set, project register and
// reconcile refuse a planted credential, show "refused", and leave the store
// unchanged. Before the gate only note, confirm and promote refused.
func TestSecretGate_AllWriteVerbs(t *testing.T) {
	refused := func(name string, argv []string, before, after func() string) {
		t.Helper()
		b := before()
		var code int
		errOut := captureStderr(t, func() { code = Run(argv) })
		if code == 0 || !strings.Contains(errOut, "refused") {
			t.Errorf("%s: code=%d, refusal shown=%v", name, code, strings.Contains(errOut, "refused"))
		}
		if a := after(); a != b {
			t.Errorf("%s: store changed (%s → %s)", name, b, a)
		}
	}

	// set: the planted value goes in --input.
	setDB := seedTask(t, "k1")
	ctx := func() string {
		s := openStoreForTest(t, setDB)
		defer s.Close()
		r, _ := s.Get("k1")
		return r["context"].(string)
	}
	refused("set", []string{"set", "--store=" + setDB, "--task=k1", "--section=context", "--input=see " + plantedCred()}, ctx, ctx)

	// import: a valid record whose context carries the planted value.
	impDB := filepath.Join(t.TempDir(), "i.db")
	openStoreForTest(t, impDB).Close()
	rec := map[string]any{"id": "k2", "type": "task", "status": "open", "updated": "2026-10-01", "title": "t",
		"context": "see " + plantedCred(), "todo": []any{}, "scope": map[string]any{"in_scope": []any{"x"}}, "done_when": []any{"x"}}
	j, _ := json.Marshal(rec)
	recFile := filepath.Join(t.TempDir(), "rec.json")
	os.WriteFile(recFile, j, 0o644)
	count := func(db, table string) func() string {
		return func() string {
			s := openStoreForTest(t, db)
			defer s.Close()
			var n string
			s.DB.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n)
			return n
		}
	}
	refused("import", []string{"import", recFile, "--store=" + impDB}, count(impDB, "record"), count(impDB, "record"))

	// project register: the planted value in --root.
	prjDB := filepath.Join(t.TempDir(), "p.db")
	openStoreForTest(t, prjDB).Close()
	refused("project register", []string{"project", "register", "--id=p9", "--team=t", "--root=" + plantedCred(), "--store=" + prjDB}, count(prjDB, "project"), count(prjDB, "project"))

	// reconcile: a done task.yml whose gotcha body carries the planted value.
	tmp := t.TempDir()
	recDB := filepath.Join(tmp, "brain.db")
	root := filepath.Join(tmp, "p3")
	s := seedProject(t, recDB, "p3", "team-1")
	s.DB.Exec(`UPDATE project SET root_path=? WHERE id='p3'`, root)
	s.Close()
	taskDir := filepath.Join(root, "plans", "plan-X", "tasks", "t1")
	os.MkdirAll(taskDir, 0o755)
	os.WriteFile(filepath.Join(taskDir, "task.yml"), []byte("id: t1\nstatus: done\nmaintain:\n  status: done\n  gotchas:\n    - severity: warn\n      body: |\n        reach it at "+plantedCred()+"\n"), 0o644)
	refused("reconcile", []string{"reconcile", "p3", "--store=" + recDB}, count(recDB, "gotcha"), count(recDB, "gotcha"))

	// Control that must pass: the same set with the secret removed.
	if code := Run([]string{"set", "--store=" + setDB, "--task=k1", "--section=context", "--input=see connections.md"}); code != 0 {
		t.Errorf("control: clean set refused, code=%d", code)
	}
}
