package cli

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// report is the REPORTER (master §B2): renders a record to a human view on demand —
// markdown (default) or HTML. This is the SCRIBE's face that replaces hand-written
// dashboards/status docs: the store stays the single source of truth, the human view
// is generated, never typed (so it can't drift, and it renders no main-terminal diff).
//
//	brain report --<type>=<id> [--format=md|html] [--store=...]
func cmdReport(p args) int {
	if p.id == "" {
		return fail("report: need --<type>=<id>")
	}
	format := p.flag["format"]
	if format == "" {
		format = "md"
	}
	if format != "md" && format != "html" {
		return fail("report: --format must be md or html")
	}
	s, err := open(p)
	if err != nil {
		return fail("report: store: %v", err)
	}
	defer s.Close()
	rec, err := s.Get(p.id)
	if err != nil {
		return fail("report: %v", err)
	}
	fmt.Print(renderRecord(rec, format))
	return 0
}

// rw accumulates output in one of two formats via the same calls, so the field logic
// below is written once.
type rw struct {
	html bool
	b    strings.Builder
}

func (w *rw) open(title string) {
	if w.html {
		w.b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>")
		w.b.WriteString(html.EscapeString(title))
		w.b.WriteString("</title></head><body>\n")
	}
}
func (w *rw) close() {
	if w.html {
		w.b.WriteString("</body></html>\n")
	}
}
func (w *rw) esc(s string) string {
	if w.html {
		return html.EscapeString(s)
	}
	return s
}
func (w *rw) h1(s string) {
	if w.html {
		fmt.Fprintf(&w.b, "<h1>%s</h1>\n", w.esc(s))
	} else {
		fmt.Fprintf(&w.b, "# %s\n", s)
	}
}
func (w *rw) h2(s string) {
	if w.html {
		fmt.Fprintf(&w.b, "<h2>%s</h2>\n", w.esc(s))
	} else {
		fmt.Fprintf(&w.b, "\n## %s\n", s)
	}
}
func (w *rw) meta(s string) {
	if w.html {
		fmt.Fprintf(&w.b, "<p><em>%s</em></p>\n", w.esc(s))
	} else {
		fmt.Fprintf(&w.b, "%s\n", s)
	}
}
func (w *rw) para(s string) {
	if w.html {
		fmt.Fprintf(&w.b, "<p>%s</p>\n", w.esc(s))
	} else {
		fmt.Fprintf(&w.b, "%s\n", s)
	}
}
func (w *rw) ul(items []string) {
	if len(items) == 0 {
		return
	}
	if w.html {
		w.b.WriteString("<ul>\n")
		for _, it := range items {
			fmt.Fprintf(&w.b, "<li>%s</li>\n", w.esc(it))
		}
		w.b.WriteString("</ul>\n")
	} else {
		for _, it := range items {
			fmt.Fprintf(&w.b, "- %s\n", it)
		}
	}
}

// checklist renders {done,text} rows as checkboxes.
func (w *rw) checklist(rows []checkRow) {
	if len(rows) == 0 {
		return
	}
	if w.html {
		w.b.WriteString("<ul>\n")
		for _, r := range rows {
			box := "&#9744;"
			if r.done {
				box = "&#9745;"
			}
			fmt.Fprintf(&w.b, "<li>%s %s</li>\n", box, w.esc(r.text))
		}
		w.b.WriteString("</ul>\n")
	} else {
		for _, r := range rows {
			box := "[ ]"
			if r.done {
				box = "[x]"
			}
			fmt.Fprintf(&w.b, "- %s %s\n", box, r.text)
		}
	}
}

type checkRow struct {
	done bool
	text string
}

// renderRecord turns one record into a human view. Known sections are rendered in a
// stable order; unknown scalar fields are ignored (the store JSON is the full truth).
func renderRecord(rec map[string]any, format string) string {
	w := &rw{html: format == "html"}
	id := str(rec, "id")
	title := str(rec, "title")
	if title == "" {
		title = id
	}
	w.open(title)
	w.h1(title)

	meta := fmt.Sprintf("`%s` · status: %s", str(rec, "type"), str(rec, "status"))
	if sd := str(rec, "status_detail"); sd != "" {
		meta += " — " + sd
	}
	if u := str(rec, "updated"); u != "" {
		meta += " · updated: " + u
	}
	if id != title {
		meta += " · id: " + id
	}
	w.meta(meta)

	if c := str(rec, "context"); c != "" {
		w.h2("Context")
		w.para(c)
	}
	if d := str(rec, "decision"); d != "" {
		w.h2("Decision")
		w.para(d)
	}

	// to-do as a checklist with an open/total count.
	if todos := arr(rec, "todo"); len(todos) > 0 {
		var rows []checkRow
		open := 0
		for _, t := range todos {
			tm, _ := t.(map[string]any)
			st := str(tm, "status")
			done := st == "done" || st == "archived"
			if !done {
				open++
			}
			rows = append(rows, checkRow{done, str(tm, "desc")})
		}
		w.h2(fmt.Sprintf("To-do (%d open / %d total)", open, len(todos)))
		w.checklist(rows)
	}

	if dw := arr(rec, "done_when"); len(dw) > 0 {
		w.h2("Done when")
		var rows []checkRow
		for _, c := range dw {
			rows = append(rows, checkRow{false, fmt.Sprint(c)})
		}
		w.checklist(rows)
	}

	// requirements (spec): {id,text,status}
	if reqs := arr(rec, "requirements"); len(reqs) > 0 {
		w.h2("Requirements")
		var items []string
		for _, r := range reqs {
			rm, _ := r.(map[string]any)
			items = append(items, fmt.Sprintf("[%s] %s — %s", str(rm, "status"), str(rm, "id"), str(rm, "text")))
		}
		w.ul(items)
	}

	// scope (object with in_scope/out_of_scope, or a bare string).
	if sc, ok := rec["scope"]; ok {
		w.h2("Scope")
		if sm, ok := sc.(map[string]any); ok {
			if in := arr(sm, "in_scope"); len(in) > 0 {
				w.para("in:")
				w.ul(toStrings(in))
			}
			if out := arr(sm, "out_of_scope"); len(out) > 0 {
				w.para("out:")
				w.ul(toStrings(out))
			}
		} else {
			w.para(fmt.Sprint(sc))
		}
	}

	// capability (umbrella) + runner.
	if cap, ok := rec["capability"].(map[string]any); ok {
		w.h2("Capability")
		var items []string
		keys := make([]string, 0, len(cap))
		for k := range cap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			items = append(items, fmt.Sprintf("%s: %v", k, cap[k]))
		}
		if r := str(rec, "runner"); r != "" {
			items = append(items, "runner: "+r)
		}
		if e := str(rec, "db_engine"); e != "" {
			items = append(items, "db_engine: "+e)
		}
		w.ul(items)
	}

	if projs := arr(rec, "projects"); len(projs) > 0 {
		w.h2("Projects")
		w.ul(toStrings(projs))
	}
	if tasks := arr(rec, "tasks"); len(tasks) > 0 {
		w.h2("Tasks")
		w.ul(toStrings(tasks))
	}

	// edges as a graph listing.
	if edges := arr(rec, "edges"); len(edges) > 0 {
		w.h2("Edges")
		var items []string
		for _, e := range edges {
			em, _ := e.(map[string]any)
			items = append(items, fmt.Sprintf("--%s--> %s", str(em, "rel"), str(em, "to")))
		}
		w.ul(items)
	}

	w.close()
	return w.b.String()
}

// str returns a string field or "".
func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

// arr returns a []any section or nil.
func arr(m map[string]any, k string) []any {
	a, _ := m[k].([]any)
	return a
}

func toStrings(a []any) []string {
	out := make([]string, 0, len(a))
	for _, v := range a {
		out = append(out, fmt.Sprint(v))
	}
	return out
}
