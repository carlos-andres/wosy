// Phase III-A4b (Fork 4 lock, 2026-06-08): this file previously held cmdAppend
// + cmdFind alongside cmdList. Both were killed:
//   - `brain append` → callers migrate to RMW with
//     `brain set --<type>=<id> --section=<f> --input=<full-array-json>`
//     (read with `brain get --<type>=<id> --section=<f>`, splice client-side,
//     write back).
//   - `brain find` → graph neighbor lookups have zero adoption in audit data;
//     direct sqlite3 query against the `edge` table is the documented escape.
//
// Only cmdList remains — the operational lists (records / todos) verb.
package cli

import (
	"fmt"
	"strings"
)

// list produces operational lists (R9): records / todos.
func cmdList(p args) int {
	col := "records"
	if len(p.pos) > 0 {
		col = p.pos[0]
	}
	var wkey, wval string
	if p.where != "" {
		wkey, wval, _ = strings.Cut(p.where, "=")
	}
	s, err := open(p)
	if err != nil {
		return fail("list: store: %v", err)
	}
	defer s.Close()

	switch col {
	case "records":
		rows, err := s.ListRecords(wkey, wval)
		if err != nil {
			return fail("list: %v", err)
		}
		for _, r := range rows {
			fmt.Printf("%-32s %-9s %-9s %s\n", r.ID, r.Type, r.Status, r.Title)
		}
		fmt.Printf("(%d records)\n", len(rows))
	case "todos":
		status := ""
		if wkey == "status" {
			status = wval
		}
		rows, err := s.ListTodos(status)
		if err != nil {
			return fail("list: %v", err)
		}
		for _, t := range rows {
			fmt.Printf("[%s] %-12s %-8s %s\n", t.RecordID, t.ItemID, t.Status, t.Descr)
		}
		fmt.Printf("(%d todos)\n", len(rows))
	default:
		return fail("list: unknown collection %q (records|todos)", col)
	}
	return 0
}
