package promoter

import (
	"os"
	"path/filepath"
)

// WalkUpDevwork walks UP from anchor looking for a .devwork/ directory and
// returns its absolute path. Mirrors ~/.claude/hooks/lib/walk-up.sh semantics:
// nearest-ancestor-first, stops at /. Returns false if none found.
func WalkUpDevwork(anchor string) (string, bool) {
	dir, err := filepath.Abs(anchor)
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(dir, ".devwork")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
