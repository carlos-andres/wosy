package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("record not found")

func str(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// Put upserts a full record (single source of truth = the doc JSON) and
// re-derives the edge + todo index tables transactionally, so the graph (R2)
// and to-do lists (R9) never drift from the record. The LINKER pattern: edges
// are extracted on write, never hand-maintained.
func (s *Store) Put(rec map[string]any) error {
	id := str(rec, "id")
	if id == "" {
		return errors.New("record has no id")
	}
	doc, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Guard against silent type-clobber: one id == one entity (F1/R1). Re-importing
	// the same id with a different type is almost always an id collision (e.g. an
	// umbrella-of-one sharing its project's name) — refuse rather than overwrite.
	var existingType string
	switch err := tx.QueryRow(`SELECT type FROM record WHERE id=?`, id).Scan(&existingType); err {
	case nil:
		if newType := str(rec, "type"); existingType != newType {
			return fmt.Errorf("id collision: %q already exists as %q, refusing to overwrite with %q",
				id, existingType, newType)
		}
	case sql.ErrNoRows:
		// new record, fine
	default:
		return err
	}

	if _, err := tx.Exec(
		`INSERT INTO record(id,type,status,updated,created,title,status_detail,doc)
		 VALUES(?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
		   type=excluded.type, status=excluded.status, updated=excluded.updated,
		   created=excluded.created, title=excluded.title,
		   status_detail=excluded.status_detail, doc=excluded.doc`,
		id, str(rec, "type"), str(rec, "status"), str(rec, "updated"),
		nullable(str(rec, "created")), nullable(str(rec, "title")),
		nullable(str(rec, "status_detail")), string(doc),
	); err != nil {
		return fmt.Errorf("upsert record: %w", err)
	}

	// Rebuild edges from doc.edges (adjacency -> join table).
	if _, err := tx.Exec(`DELETE FROM edge WHERE from_id=?`, id); err != nil {
		return err
	}
	if edges, ok := rec["edges"].([]any); ok {
		for _, e := range edges {
			em, _ := e.(map[string]any)
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO edge(from_id,rel,to_id,note) VALUES(?,?,?,?)`,
				id, str(em, "rel"), str(em, "to"), nullable(str(em, "note")),
			); err != nil {
				return fmt.Errorf("insert edge: %w", err)
			}
		}
	}

	// Rebuild todo index from doc.todo.
	if _, err := tx.Exec(`DELETE FROM todo WHERE record_id=?`, id); err != nil {
		return err
	}
	if todos, ok := rec["todo"].([]any); ok {
		for i, t := range todos {
			tm, _ := t.(map[string]any)
			itemID := str(tm, "id")
			if itemID == "" {
				itemID = fmt.Sprintf("#%d", i)
			}
			if _, err := tx.Exec(
				`INSERT OR REPLACE INTO todo(record_id,item_id,ord,descr,status) VALUES(?,?,?,?,?)`,
				id, itemID, i, str(tm, "desc"), str(tm, "status"),
			); err != nil {
				return fmt.Errorf("insert todo: %w", err)
			}
		}
	}
	return tx.Commit()
}

// Get returns the full record by id.
func (s *Store) Get(id string) (map[string]any, error) {
	var doc string
	err := s.DB.QueryRow(`SELECT doc FROM record WHERE id=?`, id).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(doc), &rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
