// Package cli implements the brain command surface (master §B6):
// get / set / list, plus init / import / version.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	brain "wosy.local/brain"
	"wosy.local/brain/internal/store"
)

const Version = "0.1.0-p1"

// knownTypes are the record-type flag tokens the parser recognizes for the
// `--<type>=<id>` shorthand. Fork 4 lock 2026-06-08 (Phase III-A4b): `decision`
// + `spec` killed — `decision` absorbed by plan.yml.open_questions[],
// `spec` by .devwork/interview/<id>.md per C-01.
var knownTypes = map[string]bool{
	"umbrella": true, "project": true, "task": true,
	"artifact": true, "edge": true,
}

// boolFlags are presence-only flags: bare `--json` is the whole signal, so the
// parser must never swallow the next arg as its value (`query --json "SELECT 1"`).
var boolFlags = map[string]bool{
	"json": true, "tables": true,
}

type args struct {
	store    string
	entity   string // record type, e.g. "task"
	id       string // record id
	section  string
	input    string
	hasInput bool
	where    string            // "key=value" filter for list
	tool     string            // guard: proposed tool name
	command  string            // guard: proposed Bash command
	path     string            // guard: proposed file path
	cwd      string            // guard: project cwd (flag-walk root)
	flag     map[string]string // every --key=val (generic, avoids per-flag struct churn)
	pos      []string          // positional args
}

func parse(a []string) args {
	var p args
	for i := 0; i < len(a); i++ {
		t := a[i]
		if strings.HasPrefix(t, "--") {
			key, val, hasEq := strings.Cut(t[2:], "=")
			if !hasEq && !boolFlags[key] && i+1 < len(a) && !strings.HasPrefix(a[i+1], "--") {
				val = a[i+1]
				i++
				hasEq = true
			}
			if p.flag == nil {
				p.flag = map[string]string{}
			}
			p.flag[key] = val
			switch {
			case key == "store":
				p.store = val
			case key == "section":
				p.section = val
			case key == "input":
				p.input, p.hasInput = val, true
			case key == "where":
				p.where = val
			case key == "tool":
				p.tool = val
			case key == "command":
				p.command = val
			case key == "path":
				p.path = val
			case key == "cwd":
				p.cwd = val
			case knownTypes[key]:
				p.entity, p.id = key, val
			}
		} else {
			p.pos = append(p.pos, t)
		}
	}
	return p
}

// storePath returns the resolved store path and whether it came from an
// explicit source (--store flag or BRAIN_STORE env). R57: read-side commands
// (list/get/report/load) must NOT silently auto-create a store at the default
// fallback — that hides "BRAIN_STORE not configured" as an empty-store success.
// init/import/set explicitly create-or-open.
func storePath(p args) (string, bool) {
	if p.store != "" {
		return p.store, true
	}
	if e := os.Getenv("BRAIN_STORE"); e != "" {
		return e, true
	}
	return ".brain/store.db", false
}

// open is used by read-side and write-side commands. It refuses to lazy-create
// a store when the path is the implicit fallback (no --store, no BRAIN_STORE).
// Explicit paths still create on demand — preserves init/import/set behavior.
func open(p args) (*store.Store, error) {
	path, explicit := storePath(p)
	if !explicit {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil, fmt.Errorf("BRAIN_STORE not set and no store at default %q — pass --store=<path> or set BRAIN_STORE env", path)
		}
	}
	return store.Open(path)
}

// parseInput tries JSON first (so arrays/objects/numbers/bools work), falling
// back to a bare string.
func parseInput(s string) any {
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		return v
	}
	return s
}

func fail(msg string, a ...any) int {
	fmt.Fprintf(os.Stderr, msg+"\n", a...)
	return 1
}

// Run dispatches one command; returns the process exit code.
func Run(argv []string) int {
	if len(argv) == 0 {
		return fail("usage: brain <version|init|import|get|set|list|load|note|query|build|reconcile|promote|project|report|schema|guard|help> ... (run `brain help` for full surface)")
	}
	cmd, rest := argv[0], argv[1:]
	p := parse(rest)

	switch cmd {
	case "version":
		fmt.Printf("brain %s\n", Version)
		return 0

	case "init":
		// Phase I I-2 (Fork 3 lock 2026-06-08): --team=<team> shifts the store
		// path to .devwork/<team>/brain.db AND creates the per-team projects/
		// dir. Without --team, falls back to legacy `brain init [path]` shape.
		if team := p.flag["team"]; team != "" {
			return cmdInitTeam(p, team)
		}
		path, _ := storePath(p)
		if len(p.pos) > 0 {
			path = p.pos[0]
		}
		s, err := store.Open(path)
		if err != nil {
			return fail("init FAIL: %v", err)
		}
		s.Close()
		fmt.Printf("init OK: %s (schema v%d)\n", path, store.SchemaVersion)
		return 0

	case "import":
		return cmdImport(p)
	case "get":
		return cmdGet(p)
	case "set":
		return cmdSet(p)
	case "list":
		return cmdList(p)
	case "guard":
		return cmdGuard(p)
	case "promote":
		return cmdPromote(p)
	case "load":
		return cmdLoad(p)
	case "note":
		return cmdNote(p)
	case "query":
		return cmdQuery(p)
	case "build":
		return cmdBuild(p)
	case "project":
		return cmdProject(p)
	case "reconcile":
		return cmdReconcile(p)
	case "report":
		return cmdReport(p)
	case "schema":
		return cmdSchema(p)
	case "help", "--help", "-h":
		fmt.Print(helpText)
		return 0
	default:
		return fail("unknown command: %s", cmd)
	}
}

// helpText is printed by `brain help|--help|-h`. Mirrors WOSY.md §4 (single
// source of truth for the operator manual lives there). Keep one line per
// command, terse — discovery, not reference. Update when CLI surface changes.
const helpText = `brain — Workflow + Knowledge System CLI

usage: brain <command> [args] [--store=<path>]

Read / discover:
  version                                         Binary version
  list [records|todos] [--where status=open]      Operational lists
  load <id>                                       REHYDRATOR — project bundle (10 sections) OR legacy single-record warm
                                                  (fuzzy project slug ok; resolution note goes to stderr)
  get --<type>=<id> [--section=<f>]               Read one section or whole record
  query "<SELECT ...>" [--json]                   Read-only SQL against the resolved store (single SELECT/PRAGMA only)
  report --<type>=<id> [--format=md|html]         Render record to markdown/HTML
  schema [--type=<name>]                          List record types or dump JSON schema for one
  schema --tables                                 Print the live SQL DDL of the resolved store

Write (existing records use set, new records use import):
  import [-|<file>]                               Create record from JSON (stdin or file)
  set --<type>=<id> --section=<f> --input=<v>     Replace one section (record must exist)
  note <project> "<text>" [--severity=info|warn|error]   Append a gotcha to a project (fuzzy slug ok)

  NOTE: array-section append is Fork-4-locked at the RMW pattern (Phase III-A4b
  killed ` + "`brain append`" + `). Read with ` + "`brain get --<type>=<id> --section=<f>`" + `,
  splice client-side, write back with
  ` + "`brain set --<type>=<id> --section=<f> --input=<full-array-json>`" + `.

Per-project (Phase I I-4/I-5, 2026-06-08):
  project register --id=<s> --team=<t> --root=<p> Upsert project identity row ([--status=active] [--display-name=<n>])
  build <project>                                 Dispatch contract for agent-brain-builder (prose-to-structure)
  reconcile <project>                             C-02 Rule 3: walk plans/*/tasks/*/task.yml, apply maintain.* deltas

Provisioning order (per-team store, from zero to warm session):
  1. brain init --team=<team>                     Create .devwork/<team>/brain.db + projects/ dir
  2. brain project register --id=<slug> ...       Upsert the project identity row
  3. brain build <project>                        Fill the satellite tables (prose-to-structure)
  4. brain load <project>                         One-call session warm-up bundle

Bootstrap / inspect:
  init [path]                                     Create empty store (default: .brain/store.db)
  init --team=<team>                              Create per-team brain.db at .devwork/<team>/brain.db + projects/ dir
  promote --command=<sql>                         Capture inline SQL → query library (R9)

Gates (CI / pre-commit / hook integration):
  guard --tool=<t> --command=<c> --path=<p> --cwd=<d>   WATCHDOG test mode (exit 0=allow, 2=block)

Record types (for --<type>=<id> flags):
  umbrella · project · task · artifact · edge

Environment:
  BRAIN_STORE     Default store path. Flag --store=<path> wins. Fallback: ./.brain/store.db
                  Read-side commands refuse implicit fallback when missing (R57).
                  load/note/query/schema --tables additionally follow a satellite
                  .devwork/wosy.yml ` + "`hub:`" + ` pointer (walking up from cwd) before
                  the legacy fallback.

For the full operator manual see ~/Documents/.devwork/WOSY.md (or the canonical
repo at ~/Documents/GitHub/wosy/docs/WOSY.md).
`

// import reads a JSON record from a file or stdin, validates against its embedded
// schema, and stores it. This is how records are born (ingest / KICKOFF).
func cmdImport(p args) int {
	var src string
	if len(p.pos) > 0 {
		src = p.pos[0]
	} else {
		src = "-"
	}
	var data []byte
	var err error
	if src == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(src)
	}
	if err != nil {
		return fail("import: read: %v", err)
	}
	// R58: no input → usage message + exit 1 (was: tried to unmarshal empty,
	// printed a bad-JSON error from the json package). Keep "usage" + "input"
	// + "error" keywords in the message so log-grep harnesses recognize it.
	if len(strings.TrimSpace(string(data))) == 0 {
		return fail("import: no input — error: nothing to import. usage: brain import [-|<file>]; expects a JSON record on stdin or from a file")
	}
	var rec map[string]any
	if err := json.Unmarshal(data, &rec); err != nil {
		return fail("import: bad JSON: %v", err)
	}
	if errs := brain.Validate(rec); len(errs) > 0 {
		return fail("import: REJECTED (schema):\n  - %s", strings.Join(errs, "\n  - "))
	}
	s, err := open(p)
	if err != nil {
		return fail("import: store: %v", err)
	}
	defer s.Close()
	if err := s.Put(rec); err != nil {
		return fail("import: put: %v", err)
	}
	fmt.Printf("import OK: %s (%s)\n", rec["id"], rec["type"])
	return 0
}

func cmdGet(p args) int {
	if p.id == "" {
		return fail("get: need --<type>=<id>")
	}
	s, err := open(p)
	if err != nil {
		return fail("get: store: %v", err)
	}
	defer s.Close()
	rec, err := s.Get(p.id)
	if err != nil {
		return fail("get: %v", err)
	}
	if p.section == "" {
		out, _ := json.MarshalIndent(rec, "", "  ")
		fmt.Println(string(out))
		return 0
	}
	v, ok := rec[p.section]
	if !ok {
		return fail("get: no section %q on %s", p.section, p.id)
	}
	if sv, isStr := v.(string); isStr {
		fmt.Println(sv)
	} else {
		out, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(out))
	}
	return 0
}

// set replaces ONE section, re-validates the WHOLE record, and only then writes.
// A rejected write leaves the store untouched.
func cmdSet(p args) int {
	if p.id == "" || p.section == "" || !p.hasInput {
		return fail("set: need --<type>=<id> --section=<field> --input=<value>")
	}
	s, err := open(p)
	if err != nil {
		return fail("set: store: %v", err)
	}
	defer s.Close()
	rec, err := s.Get(p.id)
	if err != nil {
		return fail("set: %v", err)
	}
	// G3 write-gate: refuse unknown sections BEFORE we touch the doc or the
	// store. The previous validator-only path would accept arbitrary keys
	// (additionalProperties: true on the type schema) and silently mint them.
	recType, _ := rec["type"].(string)
	if recType != "" {
		ok, allowed, sErr := brain.SectionAllowed(recType, p.section)
		if sErr == nil && !ok {
			return fail("set: REJECTED, store unchanged: unknown section %q for type %q (allowed: %s)",
				p.section, recType, strings.Join(allowed, ", "))
		}
	}
	rec[p.section] = parseInput(p.input)
	if errs := brain.Validate(rec); len(errs) > 0 {
		return fail("set: REJECTED (schema), store unchanged:\n  - %s", strings.Join(errs, "\n  - "))
	}
	if err := s.Put(rec); err != nil {
		return fail("set: put: %v", err)
	}
	fmt.Printf("set OK: %s.%s\n", p.id, p.section)
	return 0
}
