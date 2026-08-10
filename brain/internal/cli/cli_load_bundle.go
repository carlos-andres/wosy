package cli

import (
	"database/sql"
	"fmt"

	"wosy.local/brain/internal/store"
)

// loadProjectBundle assembles the 10-section bundle defined in
// brain/schema/bundle.schema.json from the per-team brain.db (C-03 §2). If
// `id` does not match a row in `project` (by id or slug), returns
// (nil, false, nil) — caller falls through to legacy single-record load.
//
// On hit, returns the bundle map ready for JSON marshalling. Section order
// is fixed (matches the schema's required array) for deterministic output.
//
// Empty sections are emitted as empty arrays, not omitted — the schema
// requires presence of every section key.
func loadProjectBundle(s *store.Store, id string) (map[string]any, bool, error) {
	var (
		projectID    string
		slug         string
		team         string
		displayName  sql.NullString
		rootPath     string
		statusVal    string
		createdAt    string
		updatedAt    string
		cacheVersion sql.NullString
	)
	err := s.DB.QueryRow(
		`SELECT id, slug, team, display_name, root_path, status, created, updated, cache_version
		   FROM project
		  WHERE id=? OR slug=?`,
		id, id,
	).Scan(&projectID, &slug, &team, &displayName, &rootPath, &statusVal, &createdAt, &updatedAt, &cacheVersion)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("project lookup: %w", err)
	}

	identity := map[string]any{
		"project_id": projectID,
		"slug":       slug,
		"team":       team,
		"root_path":  rootPath,
		"status":     statusVal,
		"created":    createdAt,
		"updated":    updatedAt,
	}
	if displayName.Valid {
		identity["display_name"] = displayName.String
	}

	stages, err := loadPipelineStages(s, projectID)
	if err != nil {
		return nil, true, err
	}
	scope, err := loadScopeEntries(s, projectID)
	if err != nil {
		return nil, true, err
	}
	conns, err := loadConnections(s, projectID)
	if err != nil {
		return nil, true, err
	}
	queries, err := loadQueryPointers(s, projectID)
	if err != nil {
		return nil, true, err
	}
	runbooks, err := loadRunbookPointers(s, projectID)
	if err != nil {
		return nil, true, err
	}
	sections, err := loadEncyclopediaSections(s, projectID)
	if err != nil {
		return nil, true, err
	}
	tasks, err := loadRecentTasks(s, projectID)
	if err != nil {
		return nil, true, err
	}
	commits, err := loadRecentCommits(s, projectID)
	if err != nil {
		return nil, true, err
	}
	gotchas, err := loadGotchas(s, projectID)
	if err != nil {
		return nil, true, err
	}

	cv := ""
	if cacheVersion.Valid {
		cv = cacheVersion.String
	}

	return map[string]any{
		"cache_version":         cv,
		"identity":              identity,
		"pipeline_stages":       stages,
		"scope":                 scope,
		"connections":           conns,
		"queries":               queries,
		"runbooks":              runbooks,
		"encyclopedia_sections": sections,
		"recent_tasks":          tasks,
		"recent_commits":        commits,
		"gotchas":               gotchas,
	}, true, nil
}

func loadPipelineStages(s *store.Store, projectID string) ([]map[string]any, error) {
	rows, err := s.DB.Query(
		`SELECT ord, name, description, status, source_quote FROM pipeline_stage WHERE project_id=? ORDER BY ord ASC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("pipeline_stage: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var ord int
		var name string
		var desc, sourceQuote sql.NullString
		var status string
		if err := rows.Scan(&ord, &name, &desc, &status, &sourceQuote); err != nil {
			return nil, err
		}
		row := map[string]any{"ord": ord, "name": name, "status": status}
		if desc.Valid {
			row["description"] = desc.String
		}
		if sourceQuote.Valid {
			row["source_quote"] = sourceQuote.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadScopeEntries(s *store.Store, projectID string) ([]map[string]any, error) {
	rows, err := s.DB.Query(
		`SELECT kind, value, health, last_seen, notes, promoted FROM scope_entry WHERE project_id=?`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("scope_entry: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var kind, value, health, lastSeen string
		var notes sql.NullString
		var promoted int
		if err := rows.Scan(&kind, &value, &health, &lastSeen, &notes, &promoted); err != nil {
			return nil, err
		}
		row := map[string]any{"kind": kind, "value": value, "health": health, "last_seen": lastSeen, "promoted": promoted != 0}
		if notes.Valid {
			row["notes"] = notes.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadConnections(s *store.Store, projectID string) ([]map[string]any, error) {
	rows, err := s.DB.Query(
		`SELECT alias, kind, target, credential_ref, notes FROM connection WHERE project_id=?`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("connection: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var alias, kind, target string
		var credRef, notes sql.NullString
		if err := rows.Scan(&alias, &kind, &target, &credRef, &notes); err != nil {
			return nil, err
		}
		row := map[string]any{"alias": alias, "kind": kind, "target": target}
		if credRef.Valid {
			row["credential_ref"] = credRef.String
		}
		if notes.Valid {
			row["notes"] = notes.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadQueryPointers(s *store.Store, projectID string) ([]map[string]any, error) {
	rows, err := s.DB.Query(
		`SELECT name, path, description FROM query_pointer WHERE project_id=?`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("query_pointer: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var name, path string
		var desc sql.NullString
		if err := rows.Scan(&name, &path, &desc); err != nil {
			return nil, err
		}
		row := map[string]any{"name": name, "path": path}
		if desc.Valid {
			row["description"] = desc.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadRunbookPointers(s *store.Store, projectID string) ([]map[string]any, error) {
	rows, err := s.DB.Query(
		`SELECT name, path, kind, purpose FROM runbook_pointer WHERE project_id=?`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("runbook_pointer: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var name, path, kind string
		var purpose sql.NullString
		if err := rows.Scan(&name, &path, &kind, &purpose); err != nil {
			return nil, err
		}
		row := map[string]any{"name": name, "path": path, "kind": kind}
		if purpose.Valid {
			row["purpose"] = purpose.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadEncyclopediaSections(s *store.Store, projectID string) ([]map[string]any, error) {
	rows, err := s.DB.Query(
		`SELECT heading, body_md, updated FROM encyclopedia_section WHERE project_id=?`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("encyclopedia_section: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var heading, body, updated string
		if err := rows.Scan(&heading, &body, &updated); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"heading": heading, "body_md": body, "updated": updated})
	}
	return out, rows.Err()
}

func loadRecentTasks(s *store.Store, projectID string) ([]map[string]any, error) {
	rows, err := s.DB.Query(
		`SELECT plan_id, task_id, status, updated FROM recent_task WHERE project_id=? ORDER BY updated DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("recent_task: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var planID, taskID, status, updated string
		if err := rows.Scan(&planID, &taskID, &status, &updated); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"plan_id": planID, "task_id": taskID, "status": status, "updated": updated})
	}
	return out, rows.Err()
}

func loadRecentCommits(s *store.Store, projectID string) ([]map[string]any, error) {
	rows, err := s.DB.Query(
		`SELECT sha, message, files_changed_n, ts FROM recent_commit WHERE project_id=? ORDER BY ts DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("recent_commit: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var sha, ts string
		var msg sql.NullString
		var nChanged int
		if err := rows.Scan(&sha, &msg, &nChanged, &ts); err != nil {
			return nil, err
		}
		row := map[string]any{"sha": sha, "files_changed_n": nChanged, "ts": ts}
		if msg.Valid {
			row["message"] = msg.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadGotchas(s *store.Store, projectID string) ([]map[string]any, error) {
	rows, err := s.DB.Query(
		`SELECT severity, body_md, source_ref, created FROM gotcha WHERE project_id=?`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("gotcha: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var severity, body, created string
		var srcRef sql.NullString
		if err := rows.Scan(&severity, &body, &srcRef, &created); err != nil {
			return nil, err
		}
		row := map[string]any{"severity": severity, "body_md": body, "created": created}
		if srcRef.Valid {
			row["source_ref"] = srcRef.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
