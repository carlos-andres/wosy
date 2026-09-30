package cli

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const palletUsage = `usage: brain pallet <term> [--consumer=fresh-session|mid-session-gap] [--budget=N] [--logbook=<path>]`

// cmdPallet is the read verb as the protocol (brain-continuity-20260923,
// logbook 153): a term goes in, one JSON pallet comes out, composed over the
// five asset layers (materiales, wosy, master, flow, pipeline), carrying the
// five fields and every gap named. It validates against
// devwork/_schema/pallet-brief.schema.json. Read-only: mode=ro on the store,
// plain reads on disk. Exit 1 when locate does not know the term (nothing on
// stdout), 2 when no logbook resolves the evidence (srs C1), 3 over budget.
// delivered.tokens is bytes/4, the rule bitacora 15.2 measured with.
// Port of tasks/brain-continuity-20260923/s7-pallet.py, 2026-09-28.
func cmdPallet(p args) int {
	if len(p.pos) == 0 || strings.TrimSpace(p.pos[0]) == "" {
		return usageFail("pallet: need one term\n%s", palletUsage)
	}
	term := strings.TrimSpace(p.pos[0])
	consumer := p.flag["consumer"]
	if consumer == "" {
		consumer = "fresh-session"
	}
	if consumer != "fresh-session" && consumer != "mid-session-gap" {
		return usageFail("pallet: --consumer must be fresh-session or mid-session-gap")
	}
	budget := 6000 // receiver's number 2026-09-28: goodrich 3.6k, manufacturers 4.6k, brain load a123-irv 14.3k
	if v := p.flag["budget"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return usageFail("pallet: --budget must be a positive integer")
		}
		budget = n
	}
	db, err := openResolvedReadOnly(p)
	if err != nil {
		return fail("pallet: %v", err)
	}
	defer db.Close()
	cwd, _ := os.Getwd()
	storePath, _, _ := resolveStore(p, cwd)

	c := &palletComposer{db: db, store: storePath, term: term, like: "%" + strings.ToLower(strings.TrimRight(term, "s")) + "%"}
	doc, delivered, code := c.compose(consumer, budget, p.flag["logbook"])
	if code != 0 {
		return code
	}
	body, _ := json.MarshalIndent(doc, "", " ")
	delivered[tokKey] = (len(body) + 3) / 4
	body, _ = json.MarshalIndent(doc, "", " ")
	if tok := (len(body) + 3) / 4; tok > budget {
		fmt.Fprintf(os.Stderr, "[X] over budget: %d > %d\n", tok, budget)
		return 3
	}
	fmt.Println(string(body))
	return 0
}

// tokKey is the schema's delivered.tokens field, the measured weight beside token_budget (srs A4).
const tokKey = "tokens"

type palletComposer struct {
	db    *sql.DB
	store string
	term  string
	like  string
	built []map[string]any
	gaps  []string
	rows  int
}

// q runs one SELECT whose first n columns are the primary key of table, and
// records every row in built_from with the statement that re-reads it (srs
// A4, D2). A missing table is a named gap, not a failure: lesson, ruled_out
// and synonym come from v2 and the appliers, and store.Open does not create
// them.
func (c *palletComposer) q(table, pk string, sqlText string, params ...any) [][]any {
	rows, err := c.db.Query(sqlText, params...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such view") {
			c.gap("wosy: table or view " + table + " absent from this store")
			return nil
		}
		fmt.Fprintf(os.Stderr, "pallet: %s: %v\n", table, err)
		return nil
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	n := strings.Count(pk, "{")
	var out [][]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return out
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		where := pk
		var keyParts []string
		for i := 0; i < n && i < len(vals); i++ {
			s := fmt.Sprint(vals[i])
			keyParts = append(keyParts, s)
			where = strings.Replace(where, "{"+strconv.Itoa(i)+"}", strings.ReplaceAll(s, "'", "''"), 1)
		}
		c.built = append(c.built, map[string]any{"source": table + ":" + strings.Join(keyParts, "|"), "statement": "SELECT * FROM " + table + " WHERE " + where})
		c.rows++
		out = append(out, vals)
	}
	return out
}

func (c *palletComposer) gap(s string) { c.gaps = append(c.gaps, s) }

func (c *palletComposer) path(p string) {
	c.built = append(c.built, map[string]any{"source": p, "statement": nil})
}

func cell(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func cut(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (c *palletComposer) mentions(p string) int {
	f, err := os.Open(p)
	if err != nil {
		return -1
	}
	defer f.Close()
	stem := strings.ToLower(strings.TrimRight(c.term, "s"))
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if strings.Contains(strings.ToLower(sc.Text()), stem) {
			n++
		}
	}
	return n
}

func logbookIDs(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var ids []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.ID != "" {
			ids = append(ids, e.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func (c *palletComposer) compose(consumer string, budget int, logbook string) (map[string]any, map[string]any, int) {
	term := c.term
	X := "[X] "

	// ---- layer 2 · wosy: identification, relation, conclusion ----------------------------
	var canon []string
	for _, r := range c.q("locate", "term='{0}' AND canonical='{1}'", "SELECT ?, canonical FROM locate WHERE term=? ORDER BY 2", term, term) {
		canon = append(canon, cell(r[1]))
	}
	if len(canon) == 0 {
		fmt.Fprintf(os.Stderr, "[X] `%s` is not in locate (synonym + project). Nothing to compose.\n", term)
		return nil, nil, 1
	}
	worlds := map[string]string{}
	for _, r := range c.q("project", "id='{0}'", "SELECT id, root_path FROM project") {
		worlds[cell(r[0])] = cell(r[1])
	}
	var nodes, wset, roles []string
	seen := map[string]bool{}
	addW := func(w string) {
		if !seen[w] {
			seen[w] = true
			wset = append(wset, w)
		}
	}
	for _, cn := range canon {
		if _, ok := worlds[cn]; ok {
			addW(cn)
		} else {
			nodes = append(nodes, cn)
		}
	}
	for _, n := range nodes {
		po := c.q("edge", "from_id='{0}' AND rel='{1}' AND to_id='{2}'", "SELECT from_id, rel, to_id, note FROM edge WHERE from_id=? AND rel='part_of' ORDER BY 3", n)
		for _, r := range po {
			roles = append(roles, n+" part_of "+cell(r[2])+": "+cut(cell(r[3]), 90))
			addW(cell(r[2]))
		}
		if len(po) == 0 && strings.HasPrefix(n, "dealer:") {
			tw := c.q("edge", "from_id='{0}' AND rel='{1}' AND to_id='{2}'", "SELECT DISTINCT a2.from_id, a2.rel, a2.to_id FROM edge a1 JOIN edge a2 ON a2.from_id=a1.from_id AND a2.rel='about' WHERE a1.rel='about' AND a1.to_id=? AND a2.to_id IN (SELECT id FROM project)", n)
			ws := map[string]bool{}
			for _, r := range tw {
				ws[cell(r[2])] = true
			}
			var wl []string
			for w := range ws {
				wl = append(wl, w)
			}
			sort.Strings(wl)
			for _, w := range wl {
				roles = append(roles, n+" reaches "+w+" through its tasks")
				addW(w)
			}
		}
	}
	if len(wset) == 0 {
		c.gap("wosy: no world reachable from the term by part_of or about")
	}
	home := ""
	if len(wset) > 0 {
		home = wset[0]
	}
	targets := nodes
	if len(targets) == 0 {
		targets = canon
	}
	targetArgs := make([]any, len(targets))
	for i, t := range targets {
		targetArgs[i] = t
	}
	tasks := c.q("record", "id='{0}'", "SELECT r.id, r.status, r.updated, json_extract(r.doc,'$.root') FROM edge e JOIN record r ON 'task:'||r.id=e.from_id WHERE e.rel='about' AND e.to_id IN ("+placeholders(len(targets))+") ORDER BY 1", targetArgs...)
	if len(tasks) == 0 {
		c.gap("wosy: 0 tasks about " + term + " in edge")
	}
	var conts [][]any
	if home != "" {
		conts = c.q("continent", "continent='{0}' AND world='{1}'", "SELECT continent, world, substr(note,1,70) FROM continent WHERE world=? ORDER BY 1", home)
	}
	gotcha := c.q("gotcha", "id={0}", "SELECT id, project_id, severity FROM gotcha WHERE superseded_by IS NULL AND lower(body_md) LIKE ? ORDER BY 1", c.like)
	lesson := c.q("lesson", "id='{0}' AND project_id='{1}'", "SELECT id, project_id, node_type, operating_rule, forbidden FROM lesson WHERE superseded_by IS NULL AND lower(id||coalesce(symptom_effect,'')||coalesce(mechanism,'')||coalesce(operating_rule,'')||coalesce(forbidden,'')) LIKE ? ORDER BY 1", c.like)
	ruled := c.q("ruled_out", "id={0}", "SELECT id, project_id, claim, negative_evidence, origin_task FROM ruled_out WHERE lower(claim||negative_evidence) LIKE ? ORDER BY 1", c.like)
	for _, nr := range []struct {
		n string
		r [][]any
	}{{"gotcha", gotcha}, {"lesson", lesson}, {"ruled_out", ruled}} {
		if len(nr.r) == 0 {
			c.gap("wosy: 0 " + nr.n + " rows name " + term)
		}
	}
	tables := c.q("scope_entry", "project_id='{0}' AND kind='table' AND value='{1}'", "SELECT project_id, value, notes FROM scope_entry WHERE kind='table' AND lower(value) LIKE ? ORDER BY 2", c.like)
	if len(tables) == 0 {
		c.gap("wosy: 0 scope_entry kind=table name " + term)
	}
	var todos [][]any
	if len(tasks) > 0 {
		tids := make([]any, len(tasks))
		for i, t := range tasks {
			tids[i] = t[0]
		}
		todos = c.q("todo", "record_id='{0}' AND item_id='{1}'", "SELECT record_id, item_id, descr FROM todo WHERE status<>'done' AND record_id IN ("+placeholders(len(tids))+") ORDER BY record_id, ord", tids...)
	}
	if len(todos) == 0 {
		c.gap(fmt.Sprintf("wosy: 0 open todo rows on the %d tasks about %s", len(tasks), term))
	}

	// ---- layer 3 · master, layer 4 · flow, layer 5 · pipeline, layer 1 · materiales --------
	type masterInfo struct {
		path  string
		mtime string
		n     int
	}
	masters := map[string]masterInfo{}
	for _, w := range wset {
		m := filepath.Join(worlds[w], "master.md")
		mi := masterInfo{path: m, n: c.mentions(m)}
		if st, err := os.Stat(m); err == nil {
			mi.mtime = st.ModTime().UTC().Format("2006-01-02")
		}
		masters[w] = mi
		c.path(m)
		if mi.mtime == "" {
			c.gap("master: " + m + " does not exist")
		} else if mi.n == 0 {
			c.gap("master: " + m + " names " + term + " 0 times")
		}
	}
	// The hub is the .devwork that wosy.yml points at (C.13, 2026-09-29): the worlds sit at
	// <hub>/projects/<w>, so the old three-levels-up walk from root_path lands on the umbrella.
	// The walk stays as the fallback for estates without a pointer (the tests seed one).
	hub := ""
	if wd, err := os.Getwd(); err == nil {
		hub = findHubPointer(wd)
	}
	if hub == "" && home != "" {
		hub = filepath.Dir(filepath.Dir(filepath.Dir(worlds[home]))) // pre-C.13 layout
	}
	var flows []string
	if hub != "" {
		fl, _ := filepath.Glob(filepath.Join(hub, "flows", "*.md"))
		for _, f := range fl {
			if c.mentions(f) > 0 {
				flows = append(flows, f)
			}
		}
	}
	sort.Strings(flows)
	for _, f := range flows {
		c.path(f)
	}
	if len(flows) == 0 {
		c.gap("flow: 0 files in " + filepath.Join(hub, "flows") + " name " + term)
	}
	var stages [][]any
	if home != "" {
		stages = c.q("pipeline_stage", "project_id='{0}' AND ord={1}", "SELECT project_id, ord, name, status FROM pipeline_stage WHERE project_id=? ORDER BY ord", home)
	}
	if len(stages) == 0 {
		c.gap("pipeline: 0 pipeline_stage rows for " + home)
	}
	var pointers [][]any
	if len(wset) > 0 {
		wa := make([]any, len(wset))
		for i, w := range wset {
			wa[i] = w
		}
		pointers = c.q("query_pointer", "project_id='{0}' AND name='{1}'", "SELECT project_id, name, path, substr(description,1,60) FROM query_pointer WHERE project_id IN ("+placeholders(len(wset))+") ORDER BY 1,2", wa...)
	}
	if len(pointers) == 0 {
		c.gap("materiales: 0 query_pointer rows in the worlds reached")
	}
	var conns [][]any
	if home != "" {
		conns = c.q("connection", "project_id='{0}' AND alias='{1}'", "SELECT project_id, alias, kind, notes FROM connection WHERE project_id=? ORDER BY 2", home)
	}
	var ro, aliases []string
	for _, cn := range conns {
		a := cell(cn[1])
		aliases = append(aliases, a)
		if strings.HasSuffix(a, "_ro") || strings.Contains(strings.ToLower(cell(cn[3])), "read") {
			ro = append(ro, a)
		}
	}
	if len(ro) == 0 {
		c.gap("materiales: no read-only connection alias for " + home)
	}
	var schemas []string
	if hub != "" {
		for _, t := range tables {
			s := filepath.Join(hub, "schema", cell(t[1])+".md")
			if _, err := os.Stat(s); err == nil {
				schemas = append(schemas, s)
				c.path(s)
			}
		}
	}
	rules := filepath.Join(hub, "rules.md") // generated at the hub root since C.13
	if _, err := os.Stat(rules); err != nil {
		rules = filepath.Join(hub, "integrations", "rules.md") // pre-C.13 layout
	}
	rulesOK := false
	if hub != "" {
		if _, err := os.Stat(rules); err == nil {
			rulesOK = true
			c.path(rules)
		}
	}

	// ---- evidence (C1): every state claim resolves to a logbook line, or the build fails ---
	if logbook == "" {
		logbook = filepath.Join(hub, "logbook.jsonl")
	}
	ids := logbookIDs(logbook)
	if len(ids) == 0 {
		fmt.Fprintf(os.Stderr, "[X] evidence: no logbook at %s (pass --logbook=<path>, srs C1)\n", logbook)
		return nil, nil, 2
	}
	last := ids[len(ids)-1]
	var nt, nv, ne, ns int
	_ = c.db.QueryRow("SELECT (SELECT count(*) FROM pragma_table_list WHERE type='table' AND name NOT LIKE 'sqlite_%'), (SELECT count(*) FROM pragma_table_list WHERE type='view'), (SELECT count(*) FROM edge), (SELECT count(*) FROM synonym)").Scan(&nt, &nv, &ne, &ns)
	evidence := []map[string]any{{"claim": fmt.Sprintf("Composed read-only from %s: %d tables, %d views, edge %d, synonym %d. The task-state lines come from record rows, not from a logbook.", c.store, nt, nv, ne, ns), "logbook_id": last}}
	taskLogs, sinDatos := 0, 0
	for _, t := range tasks {
		if root := cell(t[3]); root != "" {
			if tl := logbookIDs(filepath.Join(root, "logbook.jsonl")); len(tl) > 0 {
				taskLogs++
				evidence = append(evidence, map[string]any{"claim": fmt.Sprintf("task %s: status %s, updated %s", cell(t[0]), cell(t[1]), cell(t[2])), "logbook_id": tl[len(tl)-1]})
			}
		}
		if cell(t[1]) == "SIN DATOS" {
			sinDatos++
		}
	}
	if taskLogs < len(tasks) {
		c.gap(fmt.Sprintf("evidence: %d of %d task roots carry no logbook.jsonl; their state is record.status only (srs C1)", len(tasks)-taskLogs, len(tasks)))
	}
	if sinDatos > 0 {
		c.gap(fmt.Sprintf("en_que_nos_quedamos: %d of %d record rows carry status SIN DATOS (status.md without frontmatter)", sinDatos, len(tasks)))
	}

	// ---- the five fields -----------------------------------------------------------------
	var sb strings.Builder
	sb.WriteString("`" + term + "` locates to " + strings.Join(canon, ", ") + ". ")
	if len(roles) > 0 {
		sb.WriteString(strings.Join(roles, "; ") + ". ")
	}
	if len(tasks) > 0 {
		sb.WriteString(fmt.Sprintf("%d tasks about it. ", len(tasks)))
	}
	if len(conts) > 0 {
		var cl []string
		for _, r := range conts {
			cl = append(cl, cell(r[0]))
		}
		sb.WriteString("Continents of " + home + ": " + strings.Join(cl, ", ") + ". ")
	}
	if len(tables) > 0 {
		var tl []string
		for _, t := range tables {
			tl = append(tl, cell(t[1])+" ("+cell(t[0])+")")
		}
		sb.WriteString("Tables named: " + strings.Join(tl, ", ") + ".")
	} else {
		sb.WriteString(X + "0 registered tables name " + term + ".")
	}
	deQue := sb.String()

	var why []string
	for _, g := range gotcha {
		why = append(why, "gotcha "+cell(g[1])+"#"+cell(g[0])+" "+cell(g[2]))
	}
	for _, l := range lesson {
		why = append(why, "lesson "+cell(l[0])+" ("+cell(l[2])+")")
	}
	for _, r := range ruled {
		why = append(why, "ruled_out #"+cell(r[0])+": "+cut(cell(r[2]), 80))
	}
	porque := X + "0 gotcha, 0 lesson, 0 ruled_out name " + term + "; the conclusions of its tasks live in their status.md and never reached the store (gate 2)."
	if len(why) > 0 {
		porque = strings.Join(why, "; ") + "."
	}

	sb.Reset()
	if len(pointers) > 0 {
		var pn []string
		for _, p := range pointers {
			pn = append(pn, cell(p[1]))
		}
		sb.WriteString(fmt.Sprintf("Consult through %d query_pointer rows in %s: %s. ", len(pointers), strings.Join(wset, ", "), strings.Join(pn, ", ")))
	} else {
		sb.WriteString(X + "0 query pointers. ")
	}
	if len(ro) > 0 {
		sb.WriteString("Production read-only through alias " + strings.Join(ro, ", ") + ". ")
	}
	if len(stages) > 0 {
		var sn []string
		for _, s := range stages {
			sn = append(sn, cell(s[2]))
		}
		sb.WriteString(fmt.Sprintf("Pipeline of %s: %d stages, %s.", home, len(stages), strings.Join(sn, ", ")))
	}
	paraQue := sb.String()

	enQue := X + "no task about the term."
	if len(tasks) > 0 {
		var tl []string
		for _, t := range tasks {
			s := cell(t[0]) + " status " + cell(t[1]) + " updated " + cell(t[2])
			if cell(t[3]) == "" {
				s += " (no root)"
			}
			tl = append(tl, s)
		}
		enQue = strings.Join(tl, "; ") + "."
	}

	queSigue := X + "0 open todo rows. "
	if len(todos) > 0 {
		var tl []string
		for _, t := range todos {
			tl = append(tl, cell(t[0])+": "+cut(cell(t[2]), 80))
		}
		queSigue = "Open todo: " + strings.Join(tl, "; ") + ". "
	}
	queSigue += "What no query answers stays for the interview: the ask itself, the identifiers of the day, the production values (logbook 153)."

	var ref []string
	for _, w := range wset {
		ref = append(ref, masters[w].path)
	}
	if rulesOK {
		ref = append(ref, rules)
	}
	for _, t := range tasks {
		if r := cell(t[3]); r != "" {
			ref = append(ref, r)
		}
	}
	dirs := map[string]bool{}
	for _, p := range pointers {
		dirs[filepath.Dir(cell(p[2]))] = true
	}
	var dl []string
	for d := range dirs {
		dl = append(dl, d)
	}
	sort.Strings(dl)
	ref = append(ref, dl...)
	ref = append(ref, flows...)
	ref = append(ref, schemas...)
	if len(ref) == 0 {
		ref = []string{c.store}
	}

	var forbidden []map[string]any
	for _, l := range lesson {
		zone := cell(l[4])
		if zone == "" {
			zone = cell(l[3])
		}
		if zone != "" {
			forbidden = append(forbidden, map[string]any{"zone": zone, "negative_evidence": "lesson " + cell(l[0]) + " node_type " + cell(l[2]), "source_ref": "lesson:" + cell(l[0])})
		}
	}
	for _, r := range ruled {
		forbidden = append(forbidden, map[string]any{"zone": cell(r[2]), "negative_evidence": cell(r[3]), "source_ref": "ruled_out:" + cell(r[0]) + " origin " + cell(r[4])})
	}
	var forbiddenAny any
	if len(forbidden) > 0 {
		forbiddenAny = forbidden
	}
	if c.gaps == nil {
		c.gaps = []string{}
	}
	roText := strings.Join(ro, ", ")
	if roText == "" {
		roText = "no read-only alias"
	}
	rulesText := "absent"
	if rulesOK {
		rulesText = "present"
	}
	project := home
	if project == "" {
		project = canon[0]
	}
	hm := masters[home]
	delivered := map[string]any{"rows": c.rows}
	doc := map[string]any{
		"kind": "pallet-brief", "consumer": consumer, "project": project, "task_id": nil,
		"token_budget": budget, // redact:allow  srs A4, the stated bound
		"delivered":    delivered,
		"gaps":         c.gaps,
		"built_from":   c.built,
		"inputs": map[string]any{
			"contexto":    fmt.Sprintf("Term `%s` in world %s, root %s, master mtime %s naming it %d times. Store shape %d tables, %d views.", term, home, worlds[home], hm.mtime, hm.n, nt, nv),
			"materiales":  fmt.Sprintf("master.md of %s; %d flows naming the term; %d schema files; %d query pointers under %s; connection aliases %s; rules.md %s.", strings.Join(wset, ", "), len(flows), len(schemas), len(pointers), filepath.Join(hub, "queries", "library"), strings.Join(aliases, ", "), rulesText),
			"rol":         fmt.Sprintf("Session on world %s. Store read-only. Production only through %s; the owner runs every mutation.", home, roText),
			"alcance":     fmt.Sprintf("Composed for `%s` only, from the five layers (materiales, wosy, master, flow, pipeline), SELECT only, no detail inlined: the pallet is the pointer set and the .md holds the description.", term),
			"referencias": ref,
		},
		"brief": map[string]any{
			"de_que_trata": deQue, "porque": porque, "para_que": paraQue, "en_que_nos_quedamos": enQue, "que_sigue": queSigue,
		},
		"evidence": evidence,
		"verify": map[string]any{
			"cmd":    fmt.Sprintf("cd %s && brain query \"SELECT canonical FROM locate WHERE term='%s' ORDER BY 1\"", filepath.Dir(hub), term),
			"expect": strings.Join(canon, "\n"),
		},
		"receiver_verdict": nil, "destination": nil, "route": nil,
		"forbidden":       forbiddenAny,
		"return_contract": nil,
		"created":         time.Now().UTC().Format("2006-01-02"),
		"updated":         nil,
		"source_ref":      "brain pallet " + term,
	}
	return doc, delivered, 0
}
