package detect

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSourceMD writes a SOURCE.md with the given capturedAt date.
func writeSourceMD(t *testing.T, dir, capturedAt string, threshold int) {
	t.Helper()
	thresholdLine := ""
	if threshold > 0 {
		thresholdLine = "staleness_threshold_days: " + itoa(threshold) + "\n"
	}
	body := "---\n" +
		"origin: example.com\n" +
		"captured_at: " + capturedAt + "\n" +
		"captured_by: test\n" +
		"is_production: false\n" +
		"prod_truth_table: foo.bar.baz\n" +
		"encoding: utf-8\n" +
		thresholdLine +
		"---\n\n# test source\n"
	if err := os.WriteFile(filepath.Join(dir, "SOURCE.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SOURCE.md: %v", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// projectWithDataDirs creates a project root with .devwork/data/<sub>/ entries.
// dirs maps subdir name → list of files to drop in (besides any SOURCE.md the
// caller writes separately).
func projectWithDataDirs(t *testing.T, dirs map[string][]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".devwork", "data"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for sub, files := range dirs {
		p := filepath.Join(root, ".devwork", "data", sub)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		for _, f := range files {
			if err := os.WriteFile(filepath.Join(p, f), []byte("x\n"), 0o644); err != nil {
				t.Fatalf("write %s: %v", f, err)
			}
		}
	}
	return root
}

func findEntry(entries []DataProvenance, suffix string) *DataProvenance {
	for i := range entries {
		if filepath.Base(entries[i].Source) == suffix || endsWith(entries[i].Source, suffix) {
			return &entries[i]
		}
	}
	return nil
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func TestDataProvenance_FreshSourceMD(t *testing.T) {
	root := projectWithDataDirs(t, map[string][]string{"fresh": {"data.csv"}})
	writeSourceMD(t, filepath.Join(root, ".devwork", "data", "fresh"), time.Now().Format("2006-01-02"), 7)

	r := Detect(root)
	e := findEntry(r.DataProvenance, "fresh")
	if e == nil {
		t.Fatalf("fresh entry not found; got %+v", r.DataProvenance)
	}
	if e.Status != "ok" {
		t.Errorf("status=%q, want ok", e.Status)
	}
	if e.IsProduction == nil || *e.IsProduction != false {
		t.Errorf("is_production=%v, want false", e.IsProduction)
	}
	if e.Encoding != "utf-8" {
		t.Errorf("encoding=%q, want utf-8", e.Encoding)
	}
	if e.ProdTruthTable != "foo.bar.baz" {
		t.Errorf("prod_truth_table=%q", e.ProdTruthTable)
	}
	if e.DaysSinceCaptured == nil || *e.DaysSinceCaptured > 1 {
		t.Errorf("days_since_captured=%v, want ~0", e.DaysSinceCaptured)
	}
}

func TestDataProvenance_StaleSourceMD(t *testing.T) {
	root := projectWithDataDirs(t, map[string][]string{"stale": {"data.csv"}})
	old := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	writeSourceMD(t, filepath.Join(root, ".devwork", "data", "stale"), old, 7)

	r := Detect(root)
	e := findEntry(r.DataProvenance, "stale")
	if e == nil {
		t.Fatalf("stale entry not found; got %+v", r.DataProvenance)
	}
	if e.Status != "stale" {
		t.Errorf("status=%q, want stale (days=%v threshold=%d)", e.Status, e.DaysSinceCaptured, e.StalenessThresholdDays)
	}
	if e.DaysSinceCaptured == nil || *e.DaysSinceCaptured < 28 {
		t.Errorf("days_since_captured=%v, want ~30", e.DaysSinceCaptured)
	}
}

func TestDataProvenance_MissingSourceMD(t *testing.T) {
	root := projectWithDataDirs(t, map[string][]string{"orphan": {"data.csv"}})

	r := Detect(root)
	e := findEntry(r.DataProvenance, "orphan")
	if e == nil {
		t.Fatalf("orphan entry not found; got %+v", r.DataProvenance)
	}
	if e.Status != "missing SOURCE.md" {
		t.Errorf("status=%q, want missing SOURCE.md", e.Status)
	}
}

func TestDataProvenance_DefaultThreshold(t *testing.T) {
	root := projectWithDataDirs(t, map[string][]string{"default-thresh": {"data.csv"}})
	// SOURCE.md without staleness_threshold_days — must default to 7.
	body := "---\n" +
		"origin: example.com\n" +
		"captured_at: " + time.Now().AddDate(0, 0, -5).Format("2006-01-02") + "\n" +
		"captured_by: test\n" +
		"is_production: false\n" +
		"prod_truth_table: foo.bar\n" +
		"encoding: utf-8\n" +
		"---\n"
	if err := os.WriteFile(filepath.Join(root, ".devwork", "data", "default-thresh", "SOURCE.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := Detect(root)
	e := findEntry(r.DataProvenance, "default-thresh")
	if e == nil {
		t.Fatalf("entry not found")
	}
	if e.StalenessThresholdDays != 7 {
		t.Errorf("threshold=%d, want 7 (default)", e.StalenessThresholdDays)
	}
	if e.Status != "ok" {
		t.Errorf("status=%q, want ok (5 days < default 7)", e.Status)
	}
}

func TestDataProvenance_NoDataDir(t *testing.T) {
	// Project without .devwork/data — must not break detect.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".devwork"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	r := Detect(root)
	if len(r.DataProvenance) != 0 {
		t.Errorf("data_provenance=%+v, want empty for project without data/", r.DataProvenance)
	}
}

func TestDataProvenance_MalformedSourceMD(t *testing.T) {
	root := projectWithDataDirs(t, map[string][]string{"malformed": {"data.csv"}})
	// No front matter delimiters.
	if err := os.WriteFile(filepath.Join(root, ".devwork", "data", "malformed", "SOURCE.md"), []byte("origin: x\ncaptured_at: 2026-01-01\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := Detect(root)
	e := findEntry(r.DataProvenance, "malformed")
	if e == nil {
		t.Fatalf("malformed entry not found")
	}
	if e.Status != "malformed SOURCE.md" {
		t.Errorf("status=%q, want malformed SOURCE.md", e.Status)
	}
}

func TestDataProvenance_NestedLeaves(t *testing.T) {
	// .devwork/data/a/sub1/x.csv and .devwork/data/a/sub2/y.csv → two leaves.
	root := projectWithDataDirs(t, map[string][]string{
		"a/sub1": {"x.csv"},
		"a/sub2": {"y.csv"},
	})
	r := Detect(root)
	if len(r.DataProvenance) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(r.DataProvenance), r.DataProvenance)
	}
	for _, e := range r.DataProvenance {
		if e.Status != "missing SOURCE.md" {
			t.Errorf("entry %s status=%q, want missing SOURCE.md", e.Source, e.Status)
		}
	}
}
