package cli

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"wosy.local/brain/internal/store"
)

// resolveStore is the lenient store locator for the session-facing verbs
// (load / note / query / schema --tables). Order: --store flag > BRAIN_STORE
// env > nearest .devwork/wosy.yml `hub:` pointer walking up from startDir >
// legacy .brain/store.db fallback (the only tier reported as non-explicit).
func resolveStore(p args, startDir string) (path string, explicit bool, err error) {
	if p.store != "" {
		return p.store, true, nil
	}
	if e := os.Getenv("BRAIN_STORE"); e != "" {
		return e, true, nil
	}
	if hub := findHubPointer(startDir); hub != "" {
		path, err := hubStorePath(hub)
		if err != nil {
			return "", false, err
		}
		return path, true, nil
	}
	return ".brain/store.db", false, nil
}

// findHubPointer walks up from dir looking for a .devwork/wosy.yml satellite
// pointer carrying a `hub:` key. Returns the hub path or "" when none exists.
func findHubPointer(dir string) string {
	for d := dir; ; {
		if hub := readHubKey(filepath.Join(d, ".devwork", "wosy.yml")); hub != "" {
			return hub
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// readHubKey extracts the `hub:` value from a wosy.yml pointer file. The file
// is a flat key: value list, so a line scan beats pulling in a YAML parser.
func readHubKey(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.TrimSpace(key) == "hub" {
			val, _, _ = strings.Cut(val, "#") // drop trailing inline comment
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// hubStorePath picks the store inside a hub dir: a workspace-level brain.db
// wins; otherwise exactly one per-team <hub>/<team>/brain.db must exist —
// guessing among teams would warm the wrong project silently.
func hubStorePath(hub string) (string, error) {
	direct := filepath.Join(hub, "brain.db")
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	}
	matches, _ := filepath.Glob(filepath.Join(hub, "*", "brain.db"))
	sort.Strings(matches)
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("hub %s has no brain.db (checked brain.db and */brain.db) — run `brain init --team=<team>` there or pass --store=<path>", hub)
	default:
		return "", fmt.Errorf("hub %s has %d team stores — pass --store=<path> or set BRAIN_STORE:\n  %s",
			hub, len(matches), strings.Join(matches, "\n  "))
	}
}

// openResolved opens the store located by resolveStore. At the implicit
// legacy fallback it refuses to auto-create: lazily materializing an empty
// store would hide an unset/implicit store behind an empty-store success — a
// silent footgun (same contract as storePath).
func openResolved(p args) (*store.Store, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cwd: %w", err)
	}
	path, explicit, err := resolveStore(p, cwd)
	if err != nil {
		return nil, err
	}
	if !explicit {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil, fmt.Errorf("BRAIN_STORE not set and no store at default %q — pass --store=<path> or set BRAIN_STORE env", path)
		}
	}
	return store.Open(path)
}

// projectMatch pairs a project's canonical id with the slug shown in
// resolution notes and candidate lists.
type projectMatch struct{ id, slug string }

// resolveProjectFuzzy matches arg against project.id+slug case-insensitively:
// prefix hits win outright; substring hits are the fallback tier. Exact
// matches are the caller's job (the bundle query already covers them).
// Returns every hit in the winning tier; the caller decides what 0/1/many
// means for its verb.
func resolveProjectFuzzy(db *sql.DB, arg string) ([]projectMatch, error) {
	rows, err := db.Query(`SELECT id, slug FROM project ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	needle := strings.ToLower(arg)
	var prefix, substr []projectMatch
	for rows.Next() {
		var m projectMatch
		if err := rows.Scan(&m.id, &m.slug); err != nil {
			return nil, err
		}
		idL, slugL := strings.ToLower(m.id), strings.ToLower(m.slug)
		switch {
		case strings.HasPrefix(idL, needle) || strings.HasPrefix(slugL, needle):
			prefix = append(prefix, m)
		case strings.Contains(idL, needle) || strings.Contains(slugL, needle):
			substr = append(substr, m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(prefix) > 0 {
		return prefix, nil
	}
	return substr, nil
}
