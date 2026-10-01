-- Tables and views that the live store carried with no DDL in the binary
-- (factory-redesign-20260911 V1, 2026-10-01). Copied verbatim from the live
-- store's sqlite_master, IF NOT EXISTS added, tables before views, views in
-- creation order. Idempotent: Init applies it on every open.
CREATE TABLE IF NOT EXISTS lesson (
  id                TEXT NOT NULL,       -- lessons/<ISO-date>-<slug>.md
  project_id        TEXT REFERENCES project(id),  -- NULL = workspace-level
  node_type         TEXT NOT NULL CHECK (node_type IN ('failure','rule','decision')),
  symptom_effect    TEXT,                -- business-language effect; THE 1-line map entry
  mechanism         TEXT,                -- the WHY (service history, not code description)
  how_to_detect     TEXT,                -- executable query/command; NEVER the count
  silent            INTEGER,             -- bool; required when diagnostic
  misleading_signal TEXT,                -- what makes you look the wrong way; only if SOURCED
  location          TEXT,                -- stable symbol (Class::method); never :line
  fix_pointer       TEXT,                -- pointer to spec/fix surface (symbol or asset path)
  operating_rule    TEXT,
  forbidden         TEXT,
  canary_case       TEXT,                -- as a PATTERN; literal canary ROWS live in the md body
  env_scope         TEXT,                -- local | staging | prod | all
  decision_owner    TEXT,                -- WHO/WHAT locked it; NEVER a date
  prod_state        TEXT,
  last_verified_at  TEXT,
  superseded_by     TEXT,                -- successor pointer; the superseded row is PRESERVED
  origin_task       TEXT,
  created           TEXT NOT NULL,
  source_ref        TEXT NOT NULL,
  lang              TEXT,                -- 'en' | 'es'; written by the miner, not by build
  PRIMARY KEY (project_id, id)
);
CREATE TABLE IF NOT EXISTS ruled_out (
  id                INTEGER PRIMARY KEY,
  project_id        TEXT NOT NULL REFERENCES project(id),
  claim             TEXT NOT NULL,       -- the hypothesis that is dead
  negative_evidence TEXT NOT NULL,       -- why it is dead (dated evidence)
  scope             TEXT NOT NULL CHECK (scope IN ('task','project','workspace')),
  origin_task       TEXT NOT NULL,
  date              TEXT NOT NULL,
  lang              TEXT
);
CREATE TABLE IF NOT EXISTS synonym (
  term      TEXT NOT NULL,
  canonical TEXT NOT NULL,
  source    TEXT,
  PRIMARY KEY (term, canonical)
);
CREATE VIEW IF NOT EXISTS brief_project AS
SELECT p.id,
       p.display_name,
       p.team,
       p.status,
       p.root_path,
       p.updated,
       (SELECT group_concat(DISTINCT substr(se.value, 1, instr(se.value, '/') - 1))
          FROM scope_entry se
         WHERE se.project_id = p.id
           AND se.kind = 'file'
           AND instr(se.value, '/') > 0)                                     AS repos,
       (SELECT count(*) FROM scope_entry se WHERE se.project_id = p.id AND se.kind = 'file')  AS n_files,
       (SELECT count(*) FROM scope_entry se WHERE se.project_id = p.id AND se.kind = 'table') AS n_tables,
       (SELECT count(*) FROM gotcha g WHERE g.project_id = p.id)             AS n_gotchas
FROM project p;
CREATE VIEW IF NOT EXISTS brief_task AS
SELECT r.id,
       r.type,
       r.status,
       r.title,
       r.status_detail,
       r.updated,
       r.created,
       (SELECT e.to_id FROM edge e
         WHERE e.rel = 'part_of' AND e.from_id IN (r.id, 'task:' || r.id)
         LIMIT 1)                                                            AS parent,
       (SELECT count(*) FROM todo t WHERE t.record_id = r.id)                AS n_todo,
       (SELECT count(*) FROM todo t WHERE t.record_id = r.id AND t.status <> 'done') AS n_todo_open
FROM record r
WHERE r.type = 'task';
CREATE VIEW IF NOT EXISTS map AS
SELECT 'task'                                        AS kind,
       (SELECT e.to_id FROM edge e
         WHERE e.rel = 'part_of' AND e.from_id IN (r.id, 'task:' || r.id)
         LIMIT 1)                                    AS project_id,
       r.id                                          AS id,
       r.title                                       AS line,
       r.status                                      AS state,
       r.updated                                     AS touched,
       r.type                                        AS badge
FROM record r
WHERE r.type = 'task'
UNION ALL
SELECT 'edge'                                        AS kind,
       CASE WHEN e.rel = 'part_of'
                 AND e.to_id IN (SELECT id FROM project) THEN e.to_id
            ELSE NULL END                            AS project_id,
       e.from_id                                     AS id,
       e.from_id || ' ' || e.rel || ' ' || e.to_id    AS line,
       NULL                                          AS state,
       NULL                                          AS touched,
       e.rel                                         AS badge
FROM edge e;
CREATE VIEW IF NOT EXISTS lesson_index AS
SELECT l.project_id, l.id,
       COALESCE(l.symptom_effect, l.operating_rule, l.forbidden) AS line,
       l.node_type,
       CASE WHEN (l.symptom_effect IS NOT NULL OR l.silent IS NOT NULL)
             AND (l.operating_rule IS NOT NULL OR l.forbidden IS NOT NULL)
            THEN 'diagnostic+imperative'
            WHEN (l.operating_rule IS NOT NULL OR l.forbidden IS NOT NULL)
            THEN 'imperative' ELSE 'diagnostic' END AS mode,
       l.silent,
       l.misleading_signal,
       CASE WHEN l.superseded_by IS NULL THEN 'current' ELSE 'superseded' END AS lifecycle,
       COALESCE(l.last_verified_at, l.created) AS verified,
       CASE WHEN julianday('now') - julianday(COALESCE(l.last_verified_at, l.created))
                 < (SELECT CAST(value AS REAL) FROM meta WHERE key='freshness_high_days')
            THEN 'high'
            WHEN julianday('now') - julianday(COALESCE(l.last_verified_at, l.created))
                 < (SELECT CAST(value AS REAL) FROM meta WHERE key='freshness_medium_days')
            THEN 'medium' ELSE 'SIN DATOS' END AS confidence,
       l.created
FROM lesson l;
CREATE VIEW IF NOT EXISTS ruled_out_index AS
SELECT r.project_id,
       r.id,
       r.claim,
       r.negative_evidence,
       r.scope,
       r.origin_task,
       r.date,
       CASE WHEN julianday('now') - julianday(r.date)
                 < (SELECT CAST(value AS REAL) FROM meta WHERE key='freshness_medium_days')
            THEN 'medium' ELSE 'SIN DATOS' END AS confidence
FROM ruled_out r;
CREATE VIEW IF NOT EXISTS task_index AS
SELECT r.id,
       r.title,
       r.status,
       r.updated,
       (SELECT e.to_id FROM edge e
         WHERE e.rel = 'part_of' AND e.from_id IN (r.id, 'task:' || r.id) LIMIT 1)   AS parent,
       (SELECT group_concat(replace(e.to_id, 'dealer:', ''), ' ') FROM edge e
         WHERE e.rel = 'about' AND e.from_id = 'task:' || r.id
           AND e.to_id LIKE 'dealer:%')                                             AS dealers,
       (SELECT group_concat(replace(e.to_id, 'ticket:', ''), ' ') FROM edge e
         WHERE e.rel = 'about' AND e.from_id = 'task:' || r.id
           AND e.to_id LIKE 'ticket:%')                                             AS tickets,
       json_extract(r.doc, '$.root')                                                AS root,
       json_array_length(json_extract(r.doc, '$.gaps'))                             AS n_gaps
FROM record r
WHERE r.type = 'task';
CREATE VIEW IF NOT EXISTS locate AS
SELECT term, canonical, source FROM synonym
UNION ALL
SELECT id AS term, id AS canonical, 'project.id · un nombre canonico se resuelve a si mismo' AS source FROM project;
CREATE VIEW IF NOT EXISTS continent AS
SELECT to_id AS world, from_id AS continent, note
FROM edge
WHERE rel = 'part_of' AND from_id LIKE 'concept:%';
CREATE VIEW IF NOT EXISTS stage_anchors AS
SELECT s.project_id,
       s.ord,
       s.name,
       s.description,
       s.status,
       s.source_quote                                                        AS anchor_quote,
       s.source_quote IS NOT NULL AND trim(s.source_quote) <> ''             AS has_anchor
FROM pipeline_stage s;
CREATE VIEW IF NOT EXISTS gotcha_located AS
SELECT l.term AS project_id, g.project_id AS mundo,
       g.id, g.severity, g.body_md, g.source_ref, g.created
FROM gotcha g
JOIN locate l ON l.canonical = g.project_id;
