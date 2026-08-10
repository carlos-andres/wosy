package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const queryUsage = `usage: brain query "<SELECT ...>" [--json]`

// cmdQuery is the read-only SQL escape hatch against the resolved store.
// Defense in depth: the statement gate rejects anything but a single
// SELECT/PRAGMA, AND the db opens with mode=ro so even a gate bypass cannot
// mutate. `brain schema --tables` prints the DDL these queries run against.
func cmdQuery(p args) int {
	if len(p.pos) == 0 || strings.TrimSpace(p.pos[0]) == "" {
		return usageFail("query: need one SQL statement\n%s", queryUsage)
	}
	q := p.pos[0]
	if err := vetReadOnlySQL(q); err != nil {
		return usageFail("query: %v", err)
	}

	db, err := openResolvedReadOnly(p)
	if err != nil {
		return fail("query: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(q)
	if err != nil {
		printDDLHint(db, err)
		return fail("query: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return fail("query: columns: %v", err)
	}
	var table [][]string
	var objects []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return fail("query: scan: %v", err)
		}
		cells := make([]string, len(cols))
		obj := map[string]any{}
		for i, v := range vals {
			// The driver hands TEXT back as []byte; everything downstream
			// (alignment, JSON) wants a string.
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			obj[cols[i]] = v
			if v == nil {
				cells[i] = ""
			} else {
				cells[i] = fmt.Sprint(v)
			}
		}
		table = append(table, cells)
		objects = append(objects, obj)
	}
	if err := rows.Err(); err != nil {
		printDDLHint(db, err)
		return fail("query: %v", err)
	}

	if _, jsonOut := p.flag["json"]; jsonOut {
		if objects == nil {
			objects = []map[string]any{}
		}
		out, _ := json.MarshalIndent(objects, "", "  ")
		fmt.Println(string(out))
		return 0
	}
	printAligned(cols, table)
	return 0
}

// vetReadOnlySQL admits exactly one SELECT or PRAGMA statement. The gate
// filters by statement type only — write-PRAGMAs (e.g. PRAGMA user_version=5)
// pass it and are stopped by the mode=ro open, not here. ATTACH is banned
// outright because it can graft a writable db onto a read-only handle.
func vetReadOnlySQL(q string) error {
	trimmed := strings.TrimSpace(q)
	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "PRAGMA") {
		return fmt.Errorf("only SELECT/PRAGMA statements are allowed")
	}
	if i := strings.Index(trimmed, ";"); i >= 0 && strings.TrimSpace(trimmed[i+1:]) != "" {
		return fmt.Errorf("multi-statement input rejected — pass exactly one statement")
	}
	if strings.Contains(upper, "ATTACH") {
		return fmt.Errorf("ATTACH is not allowed")
	}
	return nil
}

// openResolvedReadOnly opens the resolved store with mode=ro. The store must
// already exist — a read-only verb never creates or migrates.
func openResolvedReadOnly(p args) (*sql.DB, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cwd: %w", err)
	}
	path, _, err := resolveStore(p, cwd)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no store at %q — pass --store=<path> or set BRAIN_STORE", path)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	return db, nil
}

// printDDLHint surfaces the live table DDL on a missing column/table error —
// the usual cause is the caller writing against a remembered, drifted schema.
func printDDLHint(db *sql.DB, qErr error) {
	msg := qErr.Error()
	if !strings.Contains(msg, "no such column") && !strings.Contains(msg, "no such table") {
		return
	}
	rows, err := db.Query(`SELECT sql FROM sqlite_master WHERE type='table' AND sql IS NOT NULL ORDER BY name`)
	if err != nil {
		return
	}
	defer rows.Close()
	fmt.Fprintln(os.Stderr, "hint — live schema of this store:")
	for rows.Next() {
		var ddl string
		if rows.Scan(&ddl) == nil {
			fmt.Fprintln(os.Stderr, ddl+";")
		}
	}
}

// printAligned renders header + rows as space-padded columns.
func printAligned(cols []string, table [][]string) {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	for _, row := range table {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	line := func(cells []string) {
		var b strings.Builder
		for i, cell := range cells {
			if i > 0 {
				b.WriteString("  ")
			}
			fmt.Fprintf(&b, "%-*s", widths[i], cell)
		}
		fmt.Println(strings.TrimRight(b.String(), " "))
	}
	line(cols)
	for _, row := range table {
		line(row)
	}
	fmt.Printf("(%d rows)\n", len(table))
}
