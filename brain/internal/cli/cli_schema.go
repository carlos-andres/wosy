package cli

import (
	"fmt"
	"strings"

	brain "wosy.local/brain"
)

// cmdSchema lists record types (no flag), dumps the embedded JSON schema for
// one type (--type=<name>), or prints the live SQL DDL of the resolved store
// (--tables). The record-type paths are pure reads with no store access.
// Added to reduce fumbles on `brain schema --type=<name>` observed in
// session logs.
func cmdSchema(p args) int {
	if _, ok := p.flag["tables"]; ok {
		return cmdSchemaTables(p)
	}
	name := p.flag["type"]
	if name == "" {
		for _, t := range brain.SchemaTypes() {
			fmt.Println(t)
		}
		return 0
	}
	b, err := brain.SchemaJSON(name)
	if err != nil {
		return fail("schema: unknown type %q (available: %s)",
			name, strings.Join(brain.SchemaTypes(), ", "))
	}
	fmt.Print(string(b))
	return 0
}

// cmdSchemaTables prints the live CREATE TABLE DDL of the resolved store —
// the ground truth for `brain query` writers (embedded JSON schemas describe
// records, not the SQL layer, and the two can drift).
func cmdSchemaTables(p args) int {
	db, err := openResolvedReadOnly(p)
	if err != nil {
		return fail("schema --tables: %v", err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT sql FROM sqlite_master WHERE type='table' AND sql IS NOT NULL ORDER BY name`)
	if err != nil {
		return fail("schema --tables: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ddl string
		if err := rows.Scan(&ddl); err != nil {
			return fail("schema --tables: scan: %v", err)
		}
		fmt.Println(ddl + ";")
	}
	if err := rows.Err(); err != nil {
		return fail("schema --tables: %v", err)
	}
	return 0
}
