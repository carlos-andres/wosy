package cli

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cmdReconcile handles `brain reconcile <project>` (Phase III-A4 FULL impl
// 2026-06-08, supersedes the Phase I I-5 narrow stub that only refreshed
// recent_task). Walks the project's plan/task tree and applies the C-02
// Rule 3 wrap contract end-to-end:
//
//	(1) refresh `recent_task` rows from task.yml status (Phase I behavior)
//	(2) for every task.yml whose maintain.status == "done", apply the
//	    maintain-stage deltas:
//	      - maintain.encyclopedia_delta  →  append to <project_root>/encyclopedia.md
//	      - maintain.runbook_delta       →  write  <project_root>/runbooks/<name>.md
//	      - maintain.gotchas[]           →  INSERT into brain.db gotcha table
//	(3) print a summary line per task and a roll-up at the end.
//
// Idempotency:
//
//	encyclopedia.md   marker `<!-- reconciled-from: <plan-id>/<task-id> -->`
//	                  is grepped before append; skip if already present.
//	runbooks/<name>.md whole-file overwrite (the delta IS the canonical body).
//	gotcha rows       skip-on-exists by (project_id, source_ref) where
//	                  source_ref = "<plan-id>/<task-id>".
//
// Ambiguity queue (DRIFT-2 lock proposal, deferred to v2.1): the audit
// envisioned auto-queueing "ambiguous" deltas to plan.yml.open_questions[].
// v1 has no merge-conflict detection on encyclopedia.md (we always append
// under a marker), so no delta is classified ambiguous. If a v2 conflict
// detector lands, this is where it hooks in.
//
// Called by: /consolidate skill Phase C (Wave D); brain-reconcile.sh hook
// (Wave B, not yet shipped).
func cmdReconcile(p args) int {
	projectID := ""
	if len(p.pos) > 0 {
		projectID = p.pos[0]
	}
	if projectID == "" {
		return fail("reconcile: need <project> (positional)")
	}

	s, err := open(p)
	if err != nil {
		return fail("reconcile: store: %v", err)
	}
	defer s.Close()

	var rootPath string
	err = s.DB.QueryRow(
		`SELECT root_path FROM project WHERE id=? OR slug=?`,
		projectID, projectID,
	).Scan(&rootPath)
	if err == sql.ErrNoRows {
		return fail("reconcile: project %q not found in brain.db: %v", projectID, err)
	}
	if err != nil {
		return fail("reconcile: project %q lookup failed: %v", projectID, err)
	}

	plansDir := filepath.Join(rootPath, "plans")
	if _, err := os.Stat(plansDir); os.IsNotExist(err) {
		fmt.Printf("reconcile: no plans/ under %s — nothing to refresh\n", rootPath)
		return 0
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return fail("reconcile: tx: %v", err)
	}
	defer tx.Rollback()

	// recent_task is files-canonical: truncate + rebuild from disk.
	// gotcha rows are NOT truncated — they're additive (each one carries its
	// source_ref); duplicate detection happens at insert time.
	if _, err := tx.Exec(`DELETE FROM recent_task WHERE project_id=?`, projectID); err != nil {
		return fail("reconcile: truncate recent_task: %v", err)
	}

	planEntries, err := os.ReadDir(plansDir)
	if err != nil {
		return fail("reconcile: read plans dir: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var (
		recentTaskUpserts   int
		encyclopediaApplied int
		runbookApplied      int
		gotchasInserted     int
	)

	for _, planEntry := range planEntries {
		if !planEntry.IsDir() {
			continue
		}
		planID := planEntry.Name()
		tasksDir := filepath.Join(plansDir, planID, "tasks")
		taskEntries, err := os.ReadDir(tasksDir)
		if err != nil {
			continue // plan with no tasks dir — skip
		}
		for _, taskEntry := range taskEntries {
			if !taskEntry.IsDir() {
				continue
			}
			taskID := taskEntry.Name()
			taskYML := filepath.Join(tasksDir, taskID, "task.yml")
			data, err := os.ReadFile(taskYML)
			if err != nil {
				continue
			}
			content := string(data)

			// (1) recent_task refresh
			status := extractTopLevelYAMLStatus(content)
			if status == "" {
				status = "pending"
			}
			if _, err := tx.Exec(
				`INSERT INTO recent_task(project_id, plan_id, task_id, status, updated)
				 VALUES(?, ?, ?, ?, ?)
				 ON CONFLICT(project_id, plan_id, task_id) DO UPDATE SET
				   status=excluded.status, updated=excluded.updated`,
				projectID, planID, taskID, status, now,
			); err != nil {
				return fail("reconcile: upsert recent_task: %v", err)
			}
			recentTaskUpserts++

			// (2) maintain-stage delta application — only when stage is done.
			ms := extractMaintainStage(content)
			if ms.status != "done" {
				continue
			}
			sourceRef := planID + "/" + taskID

			if ms.encyclopediaDelta != "" {
				applied, err := applyEncyclopediaDelta(rootPath, sourceRef, ms.encyclopediaDelta)
				if err != nil {
					return fail("reconcile: encyclopedia merge for %s: %v", sourceRef, err)
				}
				if applied {
					encyclopediaApplied++
				}
			}
			if ms.runbookName != "" && ms.runbookBody != "" {
				if err := applyRunbookDelta(rootPath, ms.runbookName, ms.runbookBody); err != nil {
					return fail("reconcile: runbook write for %s: %v", ms.runbookName, err)
				}
				runbookApplied++
			}
			if len(ms.gotchas) > 0 {
				n, err := applyGotchas(tx, projectID, sourceRef, ms.gotchas, now)
				if err != nil {
					return fail("reconcile: gotcha insert for %s: %v", sourceRef, err)
				}
				gotchasInserted += n
			}
		}
	}

	if _, err := tx.Exec(`UPDATE project SET updated=? WHERE id=? OR slug=?`, now, projectID, projectID); err != nil {
		return fail("reconcile: stamp project: %v", err)
	}
	if err := tx.Commit(); err != nil {
		return fail("reconcile: commit: %v", err)
	}

	fmt.Printf("reconcile %s: %d recent_task rows · %d encyclopedia deltas · %d runbooks · %d gotchas inserted\n",
		projectID, recentTaskUpserts, encyclopediaApplied, runbookApplied, gotchasInserted)
	return 0
}

// applyEncyclopediaDelta appends `delta` to <project_root>/encyclopedia.md
// under a per-source marker comment. If the marker already exists in the
// file the call is a no-op (returns false). Creates encyclopedia.md if it
// doesn't exist (writes a single-line header + the first delta).
func applyEncyclopediaDelta(rootPath, sourceRef, delta string) (bool, error) {
	path := filepath.Join(rootPath, "encyclopedia.md")
	marker := fmt.Sprintf("<!-- reconciled-from: %s -->", sourceRef)

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if strings.Contains(string(existing), marker) {
		return false, nil
	}

	var sb strings.Builder
	if len(existing) == 0 {
		sb.WriteString("# Encyclopedia\n\n")
	} else {
		sb.Write(existing)
		if !strings.HasSuffix(string(existing), "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(marker)
	sb.WriteString("\n")
	sb.WriteString(strings.TrimRight(delta, "\n"))
	sb.WriteString("\n")
	return true, os.WriteFile(path, []byte(sb.String()), 0o644)
}

// applyRunbookDelta writes <project_root>/runbooks/<name>.md with `body`
// (whole-file overwrite — the delta is canonical). Creates the runbooks dir
// if missing.
func applyRunbookDelta(rootPath, name, body string) error {
	dir := filepath.Join(rootPath, "runbooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, name+".md")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

// applyGotchas inserts gotcha rows for a single task. Skips entries whose
// (project_id, source_ref, body_md) tuple already exists — body_md is part
// of the key because a single task may produce N gotchas with the same
// source_ref but different bodies. Returns the count of NEW rows inserted.
func applyGotchas(tx *sql.Tx, projectID, sourceRef string, gotchas []gotchaEntry, now string) (int, error) {
	n := 0
	for _, g := range gotchas {
		if g.severity == "" || g.body == "" {
			continue
		}
		var exists int
		err := tx.QueryRow(
			`SELECT COUNT(1) FROM gotcha WHERE project_id=? AND source_ref=? AND body_md=?`,
			projectID, sourceRef, g.body,
		).Scan(&exists)
		if err != nil {
			return n, err
		}
		if exists > 0 {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO gotcha(project_id, severity, body_md, source_ref, created)
			 VALUES(?, ?, ?, ?, ?)`,
			projectID, g.severity, g.body, sourceRef, now,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// extractTopLevelYAMLStatus is a deliberately tiny YAML peeker — it scans
// for a top-level `status:` line and returns the value. Avoids pulling in a
// full YAML parser for one field. Skips frontmatter delimiters and indented
// `status:` lines (which belong to nested stage objects).
//
// Returns "" for: missing key, inline-comment-only value (`status: # foo`),
// block-scalar indicators (`status: |`, `status: >`), and quoted-empty
// values. Caller treats "" as `pending` (the schema default).
func extractTopLevelYAMLStatus(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if len(line) == 0 || line[0] == ' ' || line[0] == '\t' || line[0] == '-' || line[0] == '#' {
			continue
		}
		if !strings.HasPrefix(line, "status:") {
			continue
		}
		value := strings.TrimPrefix(line, "status:")
		if hash := strings.Index(value, " #"); hash >= 0 {
			value = value[:hash]
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		value = strings.TrimSpace(value)
		if value == "" || value == "|" || value == ">" || value == "|-" || value == ">-" {
			return ""
		}
		return value
	}
	return ""
}

// maintainStage is the subset of task.yml's maintain block that reconcile
// consumes per C-02 §2.1. The YAML peeker below knows ONLY this shape —
// it is not a general parser. If the schema gains new maintain fields,
// extend extractMaintainStage to match.
type maintainStage struct {
	status            string
	encyclopediaDelta string
	runbookName       string
	runbookBody       string
	gotchas           []gotchaEntry
}

type gotchaEntry struct {
	severity string
	body     string
}

// extractMaintainStage walks task.yml content and extracts the maintain
// block's fields. The parser knows the C-02 schema's exact indent shape:
//
//	maintain:                 # 0-space (top-level key)
//	  status: done            # 2-space
//	  done_when: |            # 2-space, multi-line scalar 4-space body
//	    ...
//	  encyclopedia_delta: |   # 2-space, multi-line scalar 4-space body
//	    ### Heading
//	    ...
//	  runbook_delta:          # 2-space, nested map
//	    name: foo             # 4-space
//	    body: |               # 4-space, multi-line scalar 6-space body
//	      ...
//	  gotchas:                # 2-space, array
//	    - severity: warn      # 4-space + "- " marker
//	      body: |             # 6-space continuation
//	        ...               # 8-space scalar body
//
// Unknown fields under maintain are skipped (forward-compatible). Inline
// scalar values (no `|`) on encyclopedia_delta / runbook_delta.body / gotcha
// body are also accepted.
func extractMaintainStage(content string) maintainStage {
	ms := maintainStage{}
	lines := strings.Split(content, "\n")

	start := -1
	for i, line := range lines {
		if line == "maintain:" {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return ms
	}

	i := start
	for i < len(lines) {
		line := lines[i]
		if line == "" {
			i++
			continue
		}
		// Top-level boundary: any line that doesn't start with at least 2 spaces.
		if !strings.HasPrefix(line, "  ") {
			break
		}
		// Strip the 2-space maintain-block indent.
		body := line[2:]
		if strings.HasPrefix(body, " ") {
			// Deeper-indented stray line — skip (belongs to a previous multi-line scalar).
			i++
			continue
		}

		switch {
		case strings.HasPrefix(body, "status:"):
			ms.status = parseScalarValue(body[len("status:"):])
			i++
		case strings.HasPrefix(body, "encyclopedia_delta:"):
			val, next := readScalar(lines, i, "encyclopedia_delta:", 4)
			ms.encyclopediaDelta = val
			i = next
		case strings.HasPrefix(body, "runbook_delta:"):
			ms.runbookName, ms.runbookBody, i = readRunbookDelta(lines, i+1)
		case strings.HasPrefix(body, "gotchas:"):
			ms.gotchas, i = readGotchas(lines, i+1)
		default:
			i++
		}
	}
	return ms
}

// parseScalarValue strips inline comments + surrounding quotes from a
// trailing scalar like ` done`, ` "in-progress"`, ` foo # note`.
func parseScalarValue(s string) string {
	if hash := strings.Index(s, " #"); hash >= 0 {
		s = s[:hash]
	}
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		s = s[1 : len(s)-1]
	}
	return strings.TrimSpace(s)
}

// readScalar handles a `<key>: <inline-or-pipe>` field at `lines[i]`. If the
// value is `|` (block scalar) consumes subsequent lines indented at least
// bodyIndent spaces, dedents by bodyIndent, and returns the joined body.
// Returns the next line index to resume at.
func readScalar(lines []string, i int, prefix string, bodyIndent int) (string, int) {
	line := strings.TrimPrefix(strings.TrimSpace(lines[i]), prefix)
	val := strings.TrimSpace(line)
	if val != "|" && val != ">" && val != "|-" && val != ">-" {
		return parseScalarValue(line), i + 1
	}
	indentPrefix := strings.Repeat(" ", bodyIndent)
	var sb strings.Builder
	i++
	for i < len(lines) {
		l := lines[i]
		if l == "" {
			sb.WriteString("\n")
			i++
			continue
		}
		if !strings.HasPrefix(l, indentPrefix) {
			break
		}
		sb.WriteString(l[bodyIndent:])
		sb.WriteString("\n")
		i++
	}
	return sb.String(), i
}

// readRunbookDelta walks the indented runbook_delta children. Children at
// 4-space indent; body block scalar at 6-space. Returns name, body, next i.
func readRunbookDelta(lines []string, i int) (name, body string, next int) {
	for i < len(lines) {
		line := lines[i]
		if line == "" {
			i++
			continue
		}
		if !strings.HasPrefix(line, "    ") {
			break
		}
		// Stray deeper indent (multi-line scalar leftover) — skip.
		if strings.HasPrefix(line, "     ") && !strings.HasPrefix(line, "      ") {
			i++
			continue
		}
		sub := strings.TrimPrefix(line, "    ")
		switch {
		case strings.HasPrefix(sub, "name:"):
			name = parseScalarValue(sub[len("name:"):])
			i++
		case strings.HasPrefix(sub, "body:"):
			body, i = readScalar(lines, i, "body:", 6)
		default:
			i++
		}
	}
	return name, body, i
}

// readGotchas walks the gotchas array. Each entry starts with `    - severity:`
// (4-space + `- `); continuation fields at 6-space indent.
func readGotchas(lines []string, i int) ([]gotchaEntry, int) {
	var out []gotchaEntry
	var current *gotchaEntry
	flush := func() {
		if current != nil {
			out = append(out, *current)
			current = nil
		}
	}
	for i < len(lines) {
		line := lines[i]
		if line == "" {
			i++
			continue
		}
		// Boundary: any line not indented to at least 4 spaces ends the array.
		if !strings.HasPrefix(line, "    ") {
			break
		}
		if strings.HasPrefix(line, "    - ") {
			flush()
			current = &gotchaEntry{}
			first := strings.TrimPrefix(line, "    - ")
			parseGotchaField(first, current, lines, &i, 8)
			continue
		}
		if strings.HasPrefix(line, "      ") {
			if current == nil {
				i++
				continue
			}
			sub := strings.TrimPrefix(line, "      ")
			parseGotchaField(sub, current, lines, &i, 8)
			continue
		}
		// Stray indent within array body (deeper-than-4 but not exactly 6) — skip.
		i++
	}
	flush()
	return out, i
}

// parseGotchaField reads one `severity:` or `body:` field into the current
// entry. Advances `*i` past the field (handling block scalars).
func parseGotchaField(field string, g *gotchaEntry, lines []string, i *int, bodyIndent int) {
	switch {
	case strings.HasPrefix(field, "severity:"):
		g.severity = parseScalarValue(field[len("severity:"):])
		*i++
	case strings.HasPrefix(field, "body:"):
		val, next := readScalar(lines, *i, "body:", bodyIndent)
		g.body = val
		*i = next
	default:
		*i++
	}
}
