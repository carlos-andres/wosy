// Package detect provides read-only, by-detection capability discovery (R8).
// Stack + DB engine come from the CANONICAL bash detectors (~/.claude/hooks/lib)
// so the brain can never drift from them — same anti-drift principle as the
// embedded schemas. The brain adds capability-presence checks on top.
// Absent capability = N/A (null), not a gap to fill.
package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Capability struct {
	Connections  *string `json:"connections"`
	Schema       *string `json:"schema"`
	QueryLibrary *string `json:"query_library"`
}

// DataProvenance is one entry per `.devwork/data/<source>/` directory.
// SOURCE.md convention: ~/Documents/.devwork/conventions/SOURCE.md (W-03).
type DataProvenance struct {
	Source                 string `json:"source"` // dir path, relative to project root when possible
	Status                 string `json:"status"` // "ok" | "stale" | "missing SOURCE.md" | "malformed SOURCE.md"
	IsProduction           *bool  `json:"is_production,omitempty"`
	CapturedAt             string `json:"captured_at,omitempty"`
	DaysSinceCaptured      *int   `json:"days_since_captured,omitempty"`
	StalenessThresholdDays int    `json:"staleness_threshold_days,omitempty"`
	Encoding               string `json:"encoding,omitempty"`
	ProdTruthTable         string `json:"prod_truth_table,omitempty"`
	Origin                 string `json:"origin,omitempty"`
}

type Result struct {
	Project        string           `json:"project"`
	Root           string           `json:"root"`
	Stack          string           `json:"stack"`
	DBEngine       string           `json:"db_engine"`
	Capability     Capability       `json:"capability"`
	Runner         *string          `json:"runner"`
	DataProvenance []DataProvenance `json:"data_provenance,omitempty"`
}

func libDir() string {
	if v := os.Getenv("BRAIN_DETECT_LIB"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "hooks", "lib")
}

// runScript runs a canonical detector if present; empty string if unavailable.
func runScript(name, dir string) string {
	script := filepath.Join(libDir(), name)
	if _, err := os.Stat(script); err != nil {
		return ""
	}
	out, err := exec.Command("bash", script, dir).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FindRoot walks up from dir to the nearest directory containing .devwork.
func FindRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	for {
		if fi, err := os.Stat(filepath.Join(abs, ".devwork")); err == nil && fi.IsDir() {
			return abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return ""
		}
		abs = parent
	}
}

// dirHasFiles reports whether dir contains at least one non-hidden matching
// file. Hidden entries (.DS_Store, .gitkeep) are ignored so an effectively-empty
// dir is honestly reported as absent (N/A), not a false-positive capability.
func dirHasFiles(dir string, exts ...string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if len(exts) == 0 {
			return dir, true
		}
		for _, ext := range exts {
			if strings.HasSuffix(e.Name(), ext) {
				return dir, true
			}
		}
	}
	return "", false
}

func ptr(s string) *string { return &s }

// Detect performs read-only capability discovery for the project owning dir.
func Detect(dir string) Result {
	root := FindRoot(dir)
	if root == "" {
		root, _ = filepath.Abs(dir)
	}
	dw := filepath.Join(root, ".devwork")
	r := Result{
		Project:  filepath.Base(root),
		Root:     root,
		Stack:    runScript("detect-stack.sh", root),
		DBEngine: runScript("detect-db-engine.sh", root),
	}

	// connections: a single file
	if _, err := os.Stat(filepath.Join(dw, "connections.md")); err == nil {
		r.Capability.Connections = ptr(".devwork/connections.md")
	}
	// schema: a dir with at least one file
	if _, ok := dirHasFiles(filepath.Join(dw, "schema")); ok {
		r.Capability.Schema = ptr(".devwork/schema/")
	}
	// query library: a dir with at least one .sql
	if _, ok := dirHasFiles(filepath.Join(dw, "queries", "library"), ".sql"); ok {
		r.Capability.QueryLibrary = ptr(".devwork/queries/library/")
	}
	// runner: q.sh (the adoption tool the WATCHDOG will route to)
	for _, cand := range []string{"bin/q.sh", "q.sh", "queries/q.sh"} {
		if _, err := os.Stat(filepath.Join(dw, cand)); err == nil {
			r.Runner = ptr(".devwork/" + cand)
			break
		}
	}
	// data provenance: walk .devwork/data/ recursively for leaf dirs that
	// contain actual data files. Each gets an entry — present-with-SOURCE.md,
	// stale-by-threshold, or missing-SOURCE.md.
	r.DataProvenance = scanDataProvenance(filepath.Join(dw, "data"), root)
	return r
}

// scanDataProvenance walks dataRoot for "data leaf" directories (directories
// that hold at least one non-hidden file). For each leaf it emits a
// DataProvenance entry. Honors the W-03 SOURCE.md convention.
func scanDataProvenance(dataRoot, projectRoot string) []DataProvenance {
	info, err := os.Stat(dataRoot)
	if err != nil || !info.IsDir() {
		return nil
	}
	var entries []DataProvenance
	_ = filepath.WalkDir(dataRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate read errors per-dir, don't break the scan
		}
		if !d.IsDir() {
			return nil
		}
		if !dirIsDataLeaf(path) {
			return nil
		}
		entries = append(entries, classifyDataDir(path, projectRoot))
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Source < entries[j].Source })
	return entries
}

// dirIsDataLeaf reports whether dir contains at least one non-hidden file
// (csv, json, sql, txt, md, parquet — anything). SOURCE.md alone does not
// count as data (it's the manifest); but if SOURCE.md is the only file we
// still want to surface that for the operator, so a SOURCE.md by itself
// also qualifies.
func dirIsDataLeaf(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			continue
		}
		// Any non-hidden file qualifies.
		return true
	}
	return false
}

// classifyDataDir reads SOURCE.md (if present) and emits a status entry.
func classifyDataDir(dir, projectRoot string) DataProvenance {
	rel := dir
	if r, err := filepath.Rel(projectRoot, dir); err == nil {
		rel = r
	}
	entry := DataProvenance{Source: rel}
	srcPath := filepath.Join(dir, "SOURCE.md")
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		entry.Status = "missing SOURCE.md"
		return entry
	}
	fm, ok := parseFrontMatter(srcBytes)
	if !ok {
		entry.Status = "malformed SOURCE.md"
		return entry
	}
	entry.Origin = fm["origin"]
	entry.Encoding = fm["encoding"]
	entry.ProdTruthTable = fm["prod_truth_table"]
	entry.CapturedAt = fm["captured_at"]
	if b, has := parseBool(fm["is_production"]); has {
		entry.IsProduction = &b
	}
	threshold := 7
	if v := fm["staleness_threshold_days"]; v != "" {
		if n, ok := parseInt(v); ok {
			threshold = n
		}
	}
	entry.StalenessThresholdDays = threshold

	// Compute days since captured_at when parseable.
	if entry.CapturedAt != "" {
		if t, ok := parseISODate(entry.CapturedAt); ok {
			days := int(time.Since(t).Hours() / 24)
			entry.DaysSinceCaptured = &days
			if days > threshold {
				entry.Status = "stale"
			} else {
				entry.Status = "ok"
			}
		} else {
			entry.Status = "malformed SOURCE.md"
		}
	} else {
		entry.Status = "malformed SOURCE.md"
	}
	return entry
}

// parseFrontMatter reads a YAML-ish front-matter block delimited by "---"
// lines. It's intentionally simple: key: value, one per line. No nesting,
// no arrays. The SOURCE.md convention is flat by design.
func parseFrontMatter(b []byte) (map[string]string, bool) {
	s := string(b)
	if !strings.HasPrefix(s, "---") {
		return nil, false
	}
	rest := s[3:]
	// skip leading newline after opening ---
	rest = strings.TrimLeft(rest, "\r\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, false
	}
	body := rest[:end]
	out := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		// only top-level key: value (no indent)
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		val = strings.Trim(val, `"'`)
		out[key] = val
	}
	return out, true
}

func parseBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1":
		return true, true
	case "false", "no", "0":
		return false, true
	}
	return false, false
}

func parseInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func parseISODate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	// Try a few common ISO-8601 shapes.
	layouts := []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
