package cli

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const confirmUsage = `usage: brain confirm <gotcha|lesson>:<id> [--contradicted=<successor>]`

// cmdConfirm stamps one gotcha or lesson as re-verified: last_verified_at gets
// the UTC now. --contradicted=<successor> also sets superseded_by, so pallet
// and load stop serving the row while the row itself is preserved. A gotcha
// is keyed by its integer id; a lesson by its text id, which must name one
// project only (the lesson key is project_id+id). Every refusal is exit 1
// with nothing written.
func cmdConfirm(p args) int {
	if len(p.pos) != 1 {
		return fail("confirm: need exactly one <table>:<id> — %s", confirmUsage)
	}
	table, id, ok := strings.Cut(p.pos[0], ":")
	id = strings.TrimSpace(id)
	if !ok || id == "" {
		return fail("confirm: malformed %q, want <gotcha|lesson>:<id> — %s", p.pos[0], confirmUsage)
	}
	var key any = id
	switch table {
	case "gotcha":
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return fail("confirm: gotcha id must be an integer, got %q", id)
		}
		key = n
	case "lesson":
	default:
		return fail("confirm: unknown table %q (gotcha|lesson)", table)
	}
	successor, contradicted := p.flag["contradicted"]
	successor = strings.TrimSpace(successor)
	if contradicted && successor == "" {
		return fail("confirm: --contradicted needs a successor — %s", confirmUsage)
	}
	if c := secretClass(successor); c != "" {
		return refuseSecret("confirm", c)
	}

	s, err := openResolved(p)
	if err != nil {
		return fail("confirm: store: %v", err)
	}
	defer s.Close()

	rows, err := s.DB.Query(`SELECT project_id FROM `+table+` WHERE id=?`, key)
	if err != nil {
		return fail("confirm: %s lookup: %v", table, err)
	}
	var projects []sql.NullString
	for rows.Next() {
		var pid sql.NullString
		if err := rows.Scan(&pid); err != nil {
			rows.Close()
			return fail("confirm: %s lookup: %v", table, err)
		}
		projects = append(projects, pid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fail("confirm: %s lookup: %v", table, err)
	}
	switch len(projects) {
	case 0:
		return fail("confirm: no %s row with id %q", table, id)
	case 1:
	default:
		var names []string
		for _, pid := range projects {
			names = append(names, pidName(pid))
		}
		return fail("confirm: %s id %q is in %d projects (%s); refusing, nothing written", table, id, len(projects), strings.Join(names, ", "))
	}

	now := time.Now().UTC().Format(time.RFC3339)
	set, vals := "last_verified_at=?", []any{now}
	if contradicted {
		set += ", superseded_by=?"
		vals = append(vals, successor)
	}
	// IS, not =: a workspace-level lesson has project_id NULL.
	if _, err := s.DB.Exec(`UPDATE `+table+` SET `+set+` WHERE id=? AND project_id IS ?`,
		append(vals, key, projects[0])...); err != nil {
		return fail("confirm: update: %v", err)
	}
	if contradicted {
		fmt.Printf("confirm OK: %s:%s (%s) superseded_by=%s last_verified_at=%s\n", table, id, pidName(projects[0]), successor, now)
	} else {
		fmt.Printf("confirm OK: %s:%s (%s) last_verified_at=%s\n", table, id, pidName(projects[0]), now)
	}
	return 0
}

func pidName(pid sql.NullString) string {
	if !pid.Valid {
		return "workspace"
	}
	return pid.String
}
