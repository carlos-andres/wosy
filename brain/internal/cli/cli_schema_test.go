package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"wosy.local/brain/internal/store"
)

// publicTypes mirrors brain.SchemaTypes — keep in sync if the type set
// grows. Used to drive table tests + assert the no-flag listing.
var publicTypes = []string{
	"artifact", "decision", "edge", "project", "spec", "task", "umbrella",
}

// TestSchema_NoFlag_ListsTypes: `brain schema` with no --type prints the 7
// public record types, one per line, in alphabetical order.
func TestSchema_NoFlag_ListsTypes(t *testing.T) {
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"schema"})
	})
	if code != 0 {
		t.Fatalf("schema no-flag exit=%d, want 0; stdout=%q", code, out)
	}
	for _, name := range publicTypes {
		if !strings.Contains(out, name+"\n") {
			t.Fatalf("expected line %q in listing, got:\n%s", name, out)
		}
	}
	if strings.Contains(out, "_base") {
		t.Fatalf("listing must exclude _base (internal $ref), got:\n%s", out)
	}
}

// TestSchema_TypeTask_DumpsJSON: `brain schema --type=task` emits a valid
// JSON object whose top-level keys include $schema + title + properties.
// The 4 Globex fumbles asked for exactly this verb shape (2026-05-29
// brain --help mining).
func TestSchema_TypeTask_DumpsJSON(t *testing.T) {
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"schema", "--type=task"})
	})
	if code != 0 {
		t.Fatalf("schema --type=task exit=%d, want 0; stdout=%q", code, out)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\nstdout=%s", err, out)
	}
	for _, k := range []string{"$schema", "title", "properties"} {
		if _, ok := doc[k]; !ok {
			t.Fatalf("schema dump missing top-level key %q; keys=%v", k, mapKeys(doc))
		}
	}
	if title, _ := doc["title"].(string); title != "task" {
		t.Fatalf("expected title=\"task\", got %q", title)
	}
}

// TestSchema_AllPublicTypes_OK: every public type resolves to valid JSON
// (regression-guards against any future renamed/removed schema file).
func TestSchema_AllPublicTypes_OK(t *testing.T) {
	for _, name := range publicTypes {
		var code int
		out := captureStdout(t, func() {
			code = Run([]string{"schema", "--type=" + name})
		})
		if code != 0 {
			t.Fatalf("schema --type=%s exit=%d, want 0", name, code)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("schema --type=%s not valid JSON: %v", name, err)
		}
	}
}

// TestSchema_UnknownType_Fails: bogus --type → exit 1, stderr names
// "available" + lists the valid set so the next reach is informed.
func TestSchema_UnknownType_Fails(t *testing.T) {
	var code int
	msg := captureStderr(t, func() {
		code = Run([]string{"schema", "--type=nope"})
	})
	if code != 1 {
		t.Fatalf("unknown type exit=%d, want 1; stderr=%q", code, msg)
	}
	if !strings.Contains(msg, "available") {
		t.Fatalf("stderr should name available types, got: %q", msg)
	}
	// Spot-check at least one valid type appears in the help list.
	if !strings.Contains(msg, "task") {
		t.Fatalf("stderr should list valid types incl. 'task', got: %q", msg)
	}
}

// TestSchema_BaseType_Allowed: --type=_base is not hidden. Callers who ask
// for the internal base record by name get it — advanced use, no surprise.
func TestSchema_BaseType_Allowed(t *testing.T) {
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"schema", "--type=_base"})
	})
	if code != 0 {
		t.Fatalf("--type=_base exit=%d, want 0; stdout=%q", code, out)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("_base output not valid JSON: %v", err)
	}
}

// TestSchema_Tables_PrintsLiveDDL: `brain schema --tables` dumps the CREATE
// TABLE statements of the resolved store (legacy + project layer).
func TestSchema_Tables_PrintsLiveDDL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "brain.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	s.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"schema", "--tables", "--store=" + dbPath})
	})
	if code != 0 {
		t.Fatalf("schema --tables exit=%d, want 0; out=%q", code, out)
	}
	if !strings.Contains(out, "CREATE TABLE") {
		t.Fatalf("expected CREATE TABLE lines, got: %q", out)
	}
	for _, table := range []string{"project", "gotcha", "record"} {
		if !strings.Contains(out, table) {
			t.Errorf("DDL dump missing table %q; out=%s", table, out)
		}
	}
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
