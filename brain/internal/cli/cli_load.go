package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"wosy.local/brain/internal/connections"
)

// load is the REHYDRATOR. Two paths since C-03 / Fork 2 lock 2026-06-08:
//
//  1. NEW (project bundle): if the id matches a row in the `project` table
//     (id or slug), emit the 10-section JSON bundle defined in
//     brain/schema/bundle.schema.json and return. Warm-caches a session in
//     ONE call, replacing the prior ~39-call cold start at the project layer.
//
//  2. LEGACY (single record rehydrate, master §B3): if the id does not match
//     a project, fall through to the existing per-record warm path —
//     canonical record + umbrella capability + work items + entry points.
//     Preserves all existing callers that pass umbrella/task/spec/etc. ids.
//
// One verb, two paths. Argument shape (presence/absence of a `project` row)
// disambiguates. No --bundle flag needed.
//
//	brain load <id> [--store=...]
func cmdLoad(p args) int {
	id := p.id
	if id == "" && len(p.pos) > 0 {
		id = p.pos[0]
	}
	if id == "" {
		return fail("load: need an id (positional or --<type>=<id>)")
	}
	// Lenient locator: load is the session warm-up verb, so it may follow a
	// satellite .devwork/wosy.yml hub pointer instead of requiring --store.
	s, err := openResolved(p)
	if err != nil {
		return fail("load: store: %v", err)
	}
	defer s.Close()

	// Fork 2 dispatch: is `id` a project? If yes, emit bundle and return.
	if bundle, isProject, bErr := loadProjectBundle(s, id); bErr != nil {
		return fail("load (project bundle): %v", bErr)
	} else if isProject {
		out, _ := json.MarshalIndent(bundle, "", "  ")
		fmt.Println(string(out))
		return 0
	}

	// Fuzzy fallback so `load acme` finds project `acme-crm`. The resolution
	// note goes to stderr — bundle stdout must stay pure JSON for pipelines.
	if matches, mErr := resolveProjectFuzzy(s.DB, id); mErr != nil {
		return fail("load: project lookup: %v", mErr)
	} else if len(matches) == 1 {
		fmt.Fprintf(os.Stderr, "resolved '%s' -> '%s'\n", id, matches[0].slug)
		bundle, isProject, bErr := loadProjectBundle(s, matches[0].id)
		if bErr != nil {
			return fail("load (project bundle): %v", bErr)
		}
		if !isProject {
			return fail("load: internal inconsistency — fuzzy match resolved %q to project %q but the bundle lookup found no project row", id, matches[0].id)
		}
		out, _ := json.MarshalIndent(bundle, "", "  ")
		fmt.Println(string(out))
		return 0
	} else if len(matches) > 1 {
		fmt.Fprintf(os.Stderr, "load: ambiguous project %q — candidates:\n", id)
		for _, m := range matches {
			fmt.Fprintf(os.Stderr, "  %s\n", m.slug)
		}
		return 1
	}

	rec, err := s.Get(id)
	if err != nil {
		return fail("load: %v", err)
	}

	var b strings.Builder
	typ := str(rec, "type")
	title := str(rec, "title")
	if title == "" {
		title = id
	}
	fmt.Fprintf(&b, "# working set: %s\n`%s` · status: %s · updated: %s\n", title, typ, str(rec, "status"), str(rec, "updated"))

	// N-01: Surface authoritative source-root paths from connections.md at the
	// TOP of the bundle, before any free-form context. Without this, the
	// interview protocol's hand-written path hints can drift (Source/worker-sync
	// was real; Teams/Platform/worker-sync didn't
	// exist — cost 3 redundant Bash calls in a live session).
	//
	// Walk-up root: prefer the store's directory, then cwd. Either may live
	// under a project's .devwork/.
	walkRoots := connectionWalkRoots(p)
	for _, root := range walkRoots {
		if root == "" {
			continue
		}
		src, entries, err := connections.FindAndParse(root)
		if err != nil || len(entries) == 0 {
			continue
		}
		// Display source path relative to root when possible, for brevity.
		display := src
		if rel, rerr := filepath.Rel(root, src); rerr == nil && !strings.HasPrefix(rel, "..") {
			display = rel
		}
		fmt.Fprintf(&b, "\n## Source paths (from %s)\n", display)
		for _, e := range entries {
			if e.Label != "" {
				fmt.Fprintf(&b, "- %s: `%s`\n", e.Label, e.Path)
			} else {
				fmt.Fprintf(&b, "- `%s`\n", e.Path)
			}
		}
		break // first hit wins; don't merge multiple files
	}

	if c := str(rec, "context"); c != "" {
		fmt.Fprintf(&b, "\n%s\n", c)
	}

	// Capability is owned once at the umbrella; a project references it.
	umb := str(rec, "umbrella")
	if typ == "umbrella" {
		umb = id
	}
	if umb != "" {
		if urec, err := s.Get(umb); err == nil {
			fmt.Fprintf(&b, "\n## capability (umbrella: %s)\n", umb)
			if capm, ok := urec["capability"].(map[string]any); ok {
				for _, k := range sortedKeys(capm) {
					fmt.Fprintf(&b, "- %s: %v\n", k, capm[k])
				}
			}
			if r := str(urec, "runner"); r != "" {
				fmt.Fprintf(&b, "- runner: %s\n", r)
			}
			if e := str(urec, "db_engine"); e != "" {
				fmt.Fprintf(&b, "- db_engine: %s\n", e)
			}
		}
	}

	// Open to-dos on the record itself.
	if open := openTodoLines(rec); len(open) > 0 {
		fmt.Fprintf(&b, "\n## open to-do (%d)\n", len(open))
		for _, l := range open {
			fmt.Fprintf(&b, "- %s\n", l)
		}
	}

	switch typ {
	case "project":
		ids := toStrings(arr(rec, "tasks"))
		if len(ids) == 0 {
			ns, _ := s.Neighbors(id)
			for _, n := range ns {
				if n.Rel == "part_of" && n.Dir == "in" {
					ids = append(ids, n.Other)
				}
			}
		}
		writeWorkItems(&b, s, "work items", ids)
	case "umbrella":
		writeWorkItems(&b, s, "projects", toStrings(arr(rec, "projects")))
	case "task":
		if eps := toStrings(arr(rec, "entry_points")); len(eps) > 0 {
			fmt.Fprintf(&b, "\n## entry points\n")
			for _, ep := range eps {
				fmt.Fprintf(&b, "- %s\n", ep)
			}
		}
		if dw := arr(rec, "done_when"); len(dw) > 0 {
			fmt.Fprintf(&b, "\n## done when\n")
			for _, c := range dw {
				fmt.Fprintf(&b, "- %v\n", c)
			}
		}
	}

	fmt.Print(b.String())
	return 0
}

// writeWorkItems resolves a list of ids and prints each with status + open-todo count
// + entry points — the one-call orientation that kills the cold-start re-read tax.
func writeWorkItems(b *strings.Builder, s opener, heading string, ids []string) {
	if len(ids) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n", heading)
	for _, id := range ids {
		r, err := s.Get(id)
		if err != nil {
			fmt.Fprintf(b, "- %s (missing record)\n", id)
			continue
		}
		fmt.Fprintf(b, "- [%s] %s — %d open todo(s)\n", str(r, "status"), id, len(openTodoLines(r)))
		for _, ep := range toStrings(arr(r, "entry_points")) {
			fmt.Fprintf(b, "    · entry: %s\n", ep)
		}
	}
}

// opener is the slice of *store.Store that load needs (just Get) — keeps the helper testable.
type opener interface {
	Get(id string) (map[string]any, error)
}

// openTodoLines returns "desc (id)" for each not-done/-archived to-do item.
func openTodoLines(rec map[string]any) []string {
	var out []string
	for _, t := range arr(rec, "todo") {
		tm, _ := t.(map[string]any)
		st := str(tm, "status")
		if st == "done" || st == "archived" {
			continue
		}
		out = append(out, fmt.Sprintf("%s (%s)", str(tm, "desc"), str(tm, "id")))
	}
	return out
}

// connectionWalkRoots returns directories to walk up from when looking for the
// nearest .devwork/connections.md. Order matters: prefer the store's directory
// (typically project-rooted like ".devwork/brain/store.db"), then cwd, then
// $BRAIN_PROJECT_ROOT if set (escape hatch for tests / unusual layouts).
func connectionWalkRoots(p args) []string {
	var roots []string
	wd, wdErr := os.Getwd()
	startDir := wd
	if wdErr != nil {
		startDir = "."
	}
	// Lenient locator (mirrors the load verb) so a hub-pointed store's directory
	// is walked too; a resolution error just drops this root — cwd and BRAIN_PROJECT_ROOT remain.
	if path, _, err := resolveStore(p, startDir); err == nil {
		if abs, aErr := filepath.Abs(path); aErr == nil {
			roots = append(roots, filepath.Dir(abs))
		}
	}
	if wdErr == nil {
		roots = append(roots, wd)
	}
	if env := os.Getenv("BRAIN_PROJECT_ROOT"); env != "" {
		roots = append(roots, env)
	}
	return roots
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
