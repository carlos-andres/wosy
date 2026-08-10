package store

import "fmt"

// Neighbor is one graph edge from the perspective of a queried node.
type Neighbor struct {
	Rel, Dir, Other string // Dir: "out" (this->other) or "in" (other->this)
}

// Neighbors answers "what connects to X" with a query, not a scan (R2).
func (s *Store) Neighbors(id string) ([]Neighbor, error) {
	var out []Neighbor
	rows, err := s.DB.Query(
		`SELECT rel, to_id, 'out' FROM edge WHERE from_id=?
		 UNION ALL
		 SELECT rel, from_id, 'in' FROM edge WHERE to_id=?
		 ORDER BY 3, 1`, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var n Neighbor
		if err := rows.Scan(&n.Rel, &n.Other, &n.Dir); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

type RecordRow struct{ ID, Type, Status, Updated, Title string }

var recordCols = map[string]bool{"type": true, "status": true}

// ListRecords lists record summaries, optionally filtered on a whitelisted column.
func (s *Store) ListRecords(col, val string) ([]RecordRow, error) {
	q := `SELECT id,type,status,updated,COALESCE(title,'') FROM record`
	var arg []any
	if col != "" {
		if !recordCols[col] {
			return nil, fmt.Errorf("cannot filter records on %q (allowed: type,status)", col)
		}
		q += " WHERE " + col + "=?"
		arg = append(arg, val)
	}
	q += " ORDER BY id"
	rows, err := s.DB.Query(q, arg...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecordRow
	for rows.Next() {
		var r RecordRow
		if err := rows.Scan(&r.ID, &r.Type, &r.Status, &r.Updated, &r.Title); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// BadEdge is an edge that breaks graph integrity (R2/F2): an endpoint that doesn't
// resolve to a record (dangling), or a self-loop (from == to).
type BadEdge struct {
	From, Rel, To          string
	MissingFrom, MissingTo bool
}

// Dangling returns edges whose from_id or to_id is not a record. The LINKER projects
// hand-authored edges verbatim, so an edge can point at a record that was never
// imported — this surfaces them rather than letting the graph silently break (R2/F2).
func (s *Store) Dangling() ([]BadEdge, error) {
	rows, err := s.DB.Query(
		`SELECT from_id, rel, to_id,
		   (from_id NOT IN (SELECT id FROM record)),
		   (to_id   NOT IN (SELECT id FROM record))
		 FROM edge
		 WHERE from_id NOT IN (SELECT id FROM record)
		    OR to_id   NOT IN (SELECT id FROM record)
		 ORDER BY from_id, rel, to_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BadEdge
	for rows.Next() {
		var e BadEdge
		var mf, mt int
		if err := rows.Scan(&e.From, &e.Rel, &e.To, &mf, &mt); err != nil {
			return nil, err
		}
		e.MissingFrom, e.MissingTo = mf != 0, mt != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// SelfLoops returns edges where from_id == to_id (G13: a record pointing at itself).
func (s *Store) SelfLoops() ([]BadEdge, error) {
	rows, err := s.DB.Query(
		`SELECT from_id, rel, to_id FROM edge WHERE from_id = to_id ORDER BY from_id, rel`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BadEdge
	for rows.Next() {
		var e BadEdge
		if err := rows.Scan(&e.From, &e.Rel, &e.To); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type TodoRow struct{ RecordID, ItemID, Status, Descr string }

// ListTodos lists to-do items across records, optionally filtered by status (R9).
func (s *Store) ListTodos(status string) ([]TodoRow, error) {
	q := `SELECT record_id,item_id,status,descr FROM todo`
	var arg []any
	if status != "" {
		q += " WHERE status=?"
		arg = append(arg, status)
	}
	q += " ORDER BY record_id, ord"
	rows, err := s.DB.Query(q, arg...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TodoRow
	for rows.Next() {
		var t TodoRow
		if err := rows.Scan(&t.RecordID, &t.ItemID, &t.Status, &t.Descr); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
