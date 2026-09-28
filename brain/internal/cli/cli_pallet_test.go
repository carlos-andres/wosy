package cli

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedPalletEstate builds the smallest estate the pallet composes over: one
// world under <tmp>/.devwork/integrations/projects/<id> with a master.md, the
// synonym table and locate view (which store.Open does not create), one task
// about a dealer node, one gotcha, one query pointer, one read-only alias, and
// a logbook at the hub. Returns the store path and the logbook path.
func seedPalletEstate(t *testing.T) (string, string) {
	t.Helper()
	tmp := t.TempDir()
	hub := filepath.Join(tmp, ".devwork")
	root := filepath.Join(hub, "integrations", "projects", "acme-crm")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(hub, "flows"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "master.md"), []byte("# acme-crm\n\nDealer goodrich sends CSVs.\n"), 0o644)
	os.WriteFile(filepath.Join(hub, "flows", "inbound.md"), []byte("# Flow\n\ngoodrich lands in feed_queue.\n"), 0o644)
	logbook := filepath.Join(hub, "logbook.jsonl")
	os.WriteFile(logbook, []byte(`{"id":"2026-09-28T00:00:00Z-1","ts":"2026-09-28T00:00:00Z","project":"acme-crm","type":"decision","text":"seed","pointer":{"kind":"path","value":"/x"}}`+"\n"), 0o644)

	dbPath := filepath.Join(tmp, "brain.db")
	s := seedProject(t, dbPath, "acme-crm", "platform")
	defer s.Close()
	exec := func(q string, args ...any) {
		if _, err := s.DB.Exec(q, args...); err != nil {
			t.Fatalf("seed %s: %v", q, err)
		}
	}
	exec(`UPDATE project SET root_path=? WHERE id='acme-crm'`, root)
	exec(`CREATE TABLE synonym (term TEXT NOT NULL, canonical TEXT NOT NULL, source TEXT, PRIMARY KEY (term, canonical))`)
	exec(`CREATE VIEW locate AS SELECT term, canonical FROM synonym UNION SELECT id, id FROM project`)
	exec(`INSERT INTO synonym VALUES ('goodrich','dealer:600031705','test')`)
	exec(`INSERT INTO record(id,type,status,updated,doc) VALUES ('goodrich-units','task','SIN DATOS','2026-09-23',?)`, `{"root":"`+filepath.Join(hub, "tasks", "goodrich-units")+`"}`)
	exec(`INSERT INTO edge VALUES ('task:goodrich-units','about','dealer:600031705',NULL)`)
	exec(`INSERT INTO edge VALUES ('task:goodrich-units','about','acme-crm',NULL)`)
	exec(`INSERT INTO gotcha(project_id,severity,body_md,source_ref,created) VALUES ('acme-crm','warn','goodrich hides units','f:1','2026-09-23')`)
	exec(`INSERT INTO query_pointer(project_id,name,path,description) VALUES ('acme-crm','inventory_by_dealer',?,'units by dealer')`, filepath.Join(hub, "queries", "library", "inventory_by_dealer.sql"))
	exec(`INSERT INTO connection(project_id,alias,kind,target,credential_ref,notes) VALUES ('acme-crm','tc_prod_ro','mysql','prod via login-path','1password://x','prod read-only')`)
	exec(`INSERT INTO pipeline_stage(project_id,ord,name,description,status) VALUES ('acme-crm',1,'ingest','pull CSV','green')`)
	return dbPath, logbook
}

func runPallet(t *testing.T, argv ...string) (int, string) {
	t.Helper()
	var code int
	out := captureStdout(t, func() { code = Run(append([]string{"pallet"}, argv...)) })
	return code, out
}

func TestPallet_ComposesGatedShape(t *testing.T) {
	dbPath, logbook := seedPalletEstate(t)
	code, out := runPallet(t, "goodrich", "--store="+dbPath, "--logbook="+logbook)
	if code != 0 {
		t.Fatalf("pallet exit=%d, want 0; out=%s", code, out)
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	for _, k := range []string{"kind", "consumer", "project", "token_budget", "delivered", "gaps", "built_from", "inputs", "brief", "evidence", "verify", "receiver_verdict", "created", "source_ref"} {
		if _, ok := d[k]; !ok {
			t.Errorf("missing required key %q", k)
		}
	}
	if d["kind"] != "pallet-brief" || d["project"] != "acme-crm" {
		t.Errorf("kind/project wrong: %v %v", d["kind"], d["project"])
	}
	brief := d["brief"].(map[string]any)
	for _, k := range []string{"de_que_trata", "porque", "para_que", "en_que_nos_quedamos", "que_sigue"} {
		if s, _ := brief[k].(string); strings.TrimSpace(s) == "" {
			t.Errorf("brief.%s empty", k)
		}
	}
	if !strings.Contains(brief["de_que_trata"].(string), "dealer:600031705 reaches acme-crm through its tasks") {
		t.Errorf("world not reached through the about join: %s", brief["de_que_trata"])
	}
	if !strings.Contains(brief["porque"].(string), "gotcha acme-crm#1 warn") {
		t.Errorf("gotcha not in porque: %s", brief["porque"])
	}
	del := d["delivered"].(map[string]any)
	if del["tokens"].(float64) <= 0 || del["tokens"].(float64) > float64(d["token_budget"].(float64)) {
		t.Errorf("delivered.tokens out of range: %v of %v", del["tokens"], d["token_budget"])
	}
	// every built_from statement re-reads exactly one row, every path exists (srs D2)
	for _, b := range d["built_from"].([]any) {
		bm := b.(map[string]any)
		if st, ok := bm["statement"].(string); ok {
			var n int
			rows, err := openForTest(t, dbPath).Query(st)
			if err != nil {
				t.Errorf("statement fails: %s: %v", st, err)
				continue
			}
			for rows.Next() {
				n++
			}
			rows.Close()
			if n != 1 {
				t.Errorf("statement returns %d rows, want 1: %s", n, st)
			}
		} else if _, err := os.Stat(bm["source"].(string)); err != nil {
			t.Errorf("built_from path missing: %s", bm["source"])
		}
	}
	// the gaps name what the seed left out: lesson and ruled_out tables absent, no todo, no logbook under the task root
	gaps := strings.Join(toStrings(d["gaps"].([]any)), "\n")
	for _, want := range []string{"lesson absent", "ruled_out absent", "0 open todo", "carry no logbook.jsonl", "SIN DATOS"} {
		if !strings.Contains(gaps, want) {
			t.Errorf("gap %q not named; gaps=\n%s", want, gaps)
		}
	}
	ev := d["evidence"].([]any)[0].(map[string]any)
	if ev["logbook_id"] != "2026-09-28T00:00:00Z-1" {
		t.Errorf("evidence id wrong: %v", ev["logbook_id"])
	}
}

func TestPallet_ControlsThatMustFail(t *testing.T) {
	dbPath, logbook := seedPalletEstate(t)
	if code, out := runPallet(t, "zzz-no-existe", "--store="+dbPath, "--logbook="+logbook); code != 1 || out != "" {
		t.Errorf("unknown term: exit=%d stdout=%q, want 1 and empty", code, out)
	}
	if code, out := runPallet(t, "goodrich", "--store="+dbPath, "--logbook="+logbook, "--budget=10"); code != 3 || out != "" {
		t.Errorf("over budget: exit=%d stdout=%q, want 3 and empty", code, out)
	}
	if code, out := runPallet(t, "goodrich", "--store="+dbPath, "--logbook="+filepath.Join(t.TempDir(), "none.jsonl")); code != 2 || out != "" {
		t.Errorf("no logbook: exit=%d stdout=%q, want 2 and empty (srs C1)", code, out)
	}
	if code, _ := runPallet(t, "--store="+dbPath); code == 0 {
		t.Errorf("no term must fail usage")
	}
}

func openForTest(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
