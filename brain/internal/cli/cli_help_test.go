package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHelpFlagNeverWrites: `-h`/`--help` anywhere after the verb prints usage
// and touches no store. Before the guard, `brain note <p> -h` appended a gotcha
// whose text was "-h" and `brain init -h` created a store file named "-h".
func TestHelpFlagNeverWrites(t *testing.T) {
	cases := [][]string{
		{"note", "acme", "-h", "--source=probe"},
		{"note", "--help"},
		{"init", "-h"},
		{"import", "-h"},
		{"confirm", "-h"},
		{"set", "-h"},
		{"reconcile", "acme", "-h"},
		{"promote", "-h"},
		{"project", "register", "-h"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		t.Chdir(dir)
		db := filepath.Join(dir, "probe.db")
		var code int
		out := captureStdout(t, func() { code = Run(append(c, "--store="+db)) })
		if code != 0 || !strings.Contains(out, "usage: brain") {
			t.Errorf("%v: code=%d, usage printed=%v", c, code, strings.Contains(out, "usage: brain"))
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Errorf("%v: wrote %d file(s) in cwd, first %q", c, len(entries), entries[0].Name())
		}
	}
}
