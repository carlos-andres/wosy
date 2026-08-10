// Package connections parses `.devwork/connections.md` to surface authoritative
// source-root paths for the REHYDRATOR (master §B3, WOSY feedback N-01).
//
// The real corporate connections.md (Globex/Teams/Platform) does NOT
// use YAML key:value; it's prose + ```shell fenced blocks with `#` comment
// labels above absolute paths. Example:
//
//	### Source repo paths (real, behind the umbrella symlinks)
//
//	```shell
//	# API (Laravel 10, PHP 8.3)
//	/Volumes/work/Globex/Source/api-core
//	/opt/homebrew/opt/php@8.3/bin/php
//
//	# Worker (PHP 7.4 CLI)
//	/Volumes/work/Globex/Source/worker-sync
//	```
//
// We extract (label, path) pairs from ```shell blocks that sit under a heading
// whose text matches path-ish keywords (path, root, source, repo, code). We
// SKIP command-like lines aggressively — false positives are worse than false
// negatives.
package connections

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// PathEntry is a (label, path) pair surfaced for the rehydration bundle.
type PathEntry struct {
	Label string // best-effort human label (from preceding `# ...` comment, or section heading)
	Path  string // absolute path as written in connections.md
}

// pathSectionHints are case-folded substrings that mark a heading as
// "this section probably documents source paths". Conservative — we want
// no false matches against unrelated sections like "## staging" or "### MySQL".
var pathSectionHints = []string{
	"path", "root", "source", "repo", "code",
}

// looksLikeCommand returns true if a line is clearly a shell command, not a
// bare path. Commands include spaces, glob/redirect/pipe punctuation, or
// well-known executable prefixes. A real source-root path on a line by itself
// has none of these.
func looksLikeCommand(s string) bool {
	if strings.ContainsAny(s, " \t|<>;&$`*?") {
		return true
	}
	return false
}

// Parse reads a connections.md file and returns path entries pulled from
// path-ish ```shell blocks. Returns (nil, nil) if the file is missing or
// no path-like fields can be extracted — caller treats that as "no block,
// emit unchanged bundle". Any I/O error is returned as-is.
func Parse(path string) ([]PathEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var (
		out             []PathEntry
		curHeading      string // most-recent ## or ### heading text (lowercased)
		inFence         bool
		inPathFence     bool   // current fence sits under a path-ish heading
		pendingLabel    string // last `# ...` comment inside fence
		sectionFallback string // raw heading text (mixed case) for label fallback
	)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)

		// Headings reset path-section state.
		if !inFence && strings.HasPrefix(line, "#") {
			// Find the heading text (after the leading #'s + space).
			t := strings.TrimLeft(line, "#")
			t = strings.TrimSpace(t)
			curHeading = strings.ToLower(t)
			sectionFallback = t
			continue
		}

		// Fence open/close.
		if strings.HasPrefix(line, "```") {
			if inFence {
				inFence = false
				inPathFence = false
				pendingLabel = ""
			} else {
				inFence = true
				inPathFence = headingLooksPathy(curHeading)
				pendingLabel = ""
			}
			continue
		}

		if !inFence || !inPathFence {
			continue
		}

		// Inside a path-ish fence.
		if line == "" {
			pendingLabel = "" // blank line breaks label association
			continue
		}
		if strings.HasPrefix(line, "#") {
			// Comment → label for the NEXT path line.
			pendingLabel = strings.TrimSpace(strings.TrimLeft(line, "#"))
			continue
		}
		// Candidate path: absolute, not a command.
		if !strings.HasPrefix(line, "/") {
			continue
		}
		if looksLikeCommand(line) {
			continue
		}
		label := pendingLabel
		if label == "" {
			label = sectionFallback
		}
		out = append(out, PathEntry{Label: label, Path: line})
		// Don't clear pendingLabel — a single `# Worker (...)` comment can
		// label multiple consecutive path lines (the PHP binary + the repo).
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func headingLooksPathy(h string) bool {
	if h == "" {
		return false
	}
	for _, hint := range pathSectionHints {
		if strings.Contains(h, hint) {
			return true
		}
	}
	return false
}

// FindAndParse walks up from start to find the nearest .devwork/connections.md
// and parses it. Returns the resolved file path (for display) and entries.
// Empty path + nil entries when nothing is found — caller treats as no-op.
func FindAndParse(start string) (string, []PathEntry, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		abs = start
	}
	for {
		candidate := filepath.Join(abs, ".devwork", "connections.md")
		if _, err := os.Stat(candidate); err == nil {
			entries, perr := Parse(candidate)
			return candidate, entries, perr
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", nil, nil
		}
		abs = parent
	}
}
