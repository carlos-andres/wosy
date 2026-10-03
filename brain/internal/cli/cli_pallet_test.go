package cli

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wosy.local/brain/internal/store"
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

// The pallet carries a reached world's runbooks and the schema cards of its scope tables, even when neither
// names the term (factory logbook 98); a runbook of another world, or a top-level one that does not name the
// term, stays out.
func TestPallet_RunbooksAndWorldSchemas(t *testing.T) {
	dbPath, logbook := seedPalletEstate(t)
	hub := filepath.Dir(logbook)
	for p, body := range map[string]string{
		"runbooks/acme-crm/locate.md": "# Locate a dealer\n",
		"runbooks/other-world/x.md":   "# goodrich elsewhere\n",
		"runbooks/top.md":             "# top\n\ngoodrich steps.\n",
		"runbooks/unrelated.md":       "# nothing here\n",
		"schema/inventory.md":         "# inventory\n",
	} {
		os.MkdirAll(filepath.Join(hub, filepath.Dir(p)), 0o755)
		os.WriteFile(filepath.Join(hub, p), []byte(body), 0o644)
	}
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO scope_entry(project_id,kind,value,health,last_seen,notes) VALUES ('acme-crm','table','inventory','green','2026-10-02','units')`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	code, out := runPallet(t, "goodrich", "--store="+dbPath, "--logbook="+logbook)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"runbooks/acme-crm/locate.md", "runbooks/top.md", "schema/inventory.md", "1 schema files", "2 runbooks"} {
		if !strings.Contains(out, want) {
			t.Errorf("pallet lacks %q", want)
		}
	}
	for _, not := range []string{"runbooks/other-world/x.md", "runbooks/unrelated.md"} {
		if strings.Contains(out, not) {
			t.Errorf("pallet carries %q, which belongs to no reached world and does not name the term", not)
		}
	}
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
	// the gaps name what the seed left out: no lesson or ruled_out row (the tables
	// now come with every store, V1 2026-10-01), no todo, no logbook under the task root
	gaps := strings.Join(toStrings(d["gaps"].([]any)), "\n")
	for _, want := range []string{"0 lesson rows name goodrich", "0 ruled_out rows name goodrich", "0 open todo", "carry no logbook.jsonl", "SIN DATOS"} {
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

// A superseded gotcha is not served, as a superseded lesson is not; the live
// one that names the same term still is.
func TestPallet_SkipsSupersededGotcha(t *testing.T) {
	dbPath, logbook := seedPalletEstate(t)
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO gotcha(project_id,severity,body_md,source_ref,created,superseded_by) VALUES ('acme-crm','error','goodrich sends no units','f:2','2026-09-23','gotcha:1')`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	code, out := runPallet(t, "goodrich", "--store="+dbPath, "--logbook="+logbook)
	if code != 0 {
		t.Fatalf("pallet exit=%d, want 0; out=%s", code, out)
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	porque := d["brief"].(map[string]any)["porque"].(string)
	if !strings.Contains(porque, "gotcha acme-crm#1 warn") {
		t.Errorf("live gotcha not served: %s", porque)
	}
	if strings.Contains(porque, "#2") {
		t.Errorf("superseded gotcha served: %s", porque)
	}
	for _, b := range d["built_from"].([]any) {
		if src := b.(map[string]any)["source"]; src == "gotcha:2" {
			t.Errorf("superseded gotcha in built_from")
		}
	}
}

// A world's own rows whose text does not say the world's name are pointed at, not
// gapped and not rendered: one line with the counts and the query that lists them.
func TestPallet_PointsAtWorldRowsTheTermDoesNotName(t *testing.T) {
	dbPath, logbook := seedPalletEstate(t)
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO lesson(id,project_id,node_type,symptom_effect,created,source_ref) VALUES ('2026-10-03-verify-reads-fixtures','acme-crm','rule','verify reads frozen fixtures','2026-10-03','f:3')`,
		`INSERT INTO ruled_out(project_id,claim,negative_evidence,scope,origin_task,date) VALUES ('acme-crm','the alert is a leak','planted fixtures','project','t','2026-10-03')`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	code, out := runPallet(t, "acme-crm", "--store="+dbPath, "--logbook="+logbook)
	if code != 0 {
		t.Fatalf("pallet exit=%d, want 0; out=%s", code, out)
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	porque := d["brief"].(map[string]any)["porque"].(string)
	if !strings.Contains(porque, "world acme-crm files 1 gotcha, 1 lesson, 1 ruled_out that do not name acme-crm") || !strings.Contains(porque, "project_id='acme-crm'") {
		t.Errorf("no pointer to the world's rows: %s", porque)
	}
	if strings.Contains(porque, "verify reads frozen fixtures") {
		t.Errorf("the pointer rendered a row: %s", porque)
	}
	gaps := strings.Join(toStrings(d["gaps"].([]any)), "\n")
	for _, n := range []string{"gotcha", "lesson", "ruled_out"} {
		if strings.Contains(gaps, "0 "+n+" rows name acme-crm") {
			t.Errorf("false gap for %s; gaps=\n%s", n, gaps)
		}
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
