---
name: query
description: Run database queries with schema awareness. Use when the user asks to query, count, inspect, or extract from a database. Reads .devwork/schema/ for column names and .devwork/connections.md for the right command. Defaults to read-only (SELECT, DESCRIBE, SHOW). Refuses to invent column names from training data.
when_to_use: User asks to query / count / inspect / extract from a database, types `/query` with a SQL-shaped request, or asks "show me rows of …" / "how many …". Defaults to read-only (SELECT / DESCRIBE / SHOW).
---

# Query (schema-first · read-only by default)

Schema-first DB queries. Verify before you SELECT.

## When this fires

User asks something like:
- "how many rows in jobs with status=processing"
- "show me the users table structure"
- "find the last 10 entries in orders where customer_id = 42"
- "query staging for X"

## Workflow

### Pre-flight

Before writing any SQL:

1. **Identify the connection AND engine.** Look at `.devwork/connections.md` for the right command (staging, prod, local) and the engine. If the engine isn't stated there, run `bash ~/.claude/hooks/lib/detect-db-engine.sh .` → `mysql|pgsql|sqlite`. If ambiguous, ask. Everything below branches on the engine.
2. **Identify the schema.** Look at `.devwork/schema/<table>.md`. Read the column list and types.
3. **If the table isn't in `.devwork/schema/`**, refuse to guess columns. Tell the user to either: (a) add the table to the schema doc, or (b) run the engine's schema-inspect first — `DESCRIBE <t>` (mysql) · `\d <t>` (pgsql) · `.schema <t>` / `PRAGMA table_info(<t>)` (sqlite).
4. **If the schema doc exists but is older than the migration directory's mtime**, warn: "schema doc may be stale — run schema refresh first?"

### Read-only by default

Every query is read-only by default. Read verbs per engine:
- **mysql:** `SELECT`, `SHOW`, `DESCRIBE`, `EXPLAIN`
- **pgsql:** `SELECT`, `EXPLAIN`, and `\d` / `\dt` / `\d+ <table>` meta-commands
- **sqlite:** `SELECT`, `EXPLAIN`, `.tables`, `.schema <table>`, `PRAGMA table_info(<table>)`

If the user asks for `INSERT`, `UPDATE`, `DELETE`, `ALTER`, `DROP`, `TRUNCATE`:

1. **Confirm explicitly.** "This is a write operation against `<connection>`. Confirm?"
2. Wait for explicit "yes" or "confirm".
3. Run with extra caution. For staging/prod, also confirm `WHERE` clause is bounded (no unbounded UPDATE/DELETE).

### Construction

Use the command **exactly as written in `connections.md`** — it already encodes the engine, host, db, and auth. Don't invent flags; don't inject credential flags the canonical line doesn't use.

**Runner first.** If `.devwork/wosy.flags` sets `enforce_inline_sql=1`, or the project has a `bin/q.sh` runner + `queries/library/`, do NOT run inline SQL — it will be blocked by the canonicalize hook. Put the query in a parameterized `.sql` file under `queries/library/` and run it through the runner (e.g. `.devwork/bin/q.sh queries/library/<name>.sql -v key=value`).

For the SQL:
- Use only columns present in `.devwork/schema/<table>.md`.
- Add `LIMIT 100` to any unbounded SELECT unless the user asks for a specific count.
- Tab-separated output flag per engine: mysql `-B` / `--batch` · pgsql `-A -F$'\t'` (or `\pset`) · sqlite `-batch` + `.mode tabs`.

Engine-branched heredoc shapes (use the canonical command from `connections.md` in place of the client below):
```bash
# mysql
mysql --login-path=<alias> --database=<db> -B <<'SQL'
SELECT id, status FROM <table> WHERE status = 'processing' LIMIT 100;
SQL

# pgsql  (or: psql "$DATABASE_URL")
psql -d <db> -A -F$'\t' <<'SQL'
SELECT id, status FROM <table> WHERE status = 'processing' LIMIT 100;
SQL

# sqlite
sqlite3 -batch <path/to.db> <<'SQL'
.mode tabs
SELECT id, status FROM <table> WHERE status = 'processing' LIMIT 100;
SQL
```

## Output

Show the query, run it, show the result. If the result is wide:
- Convert to a markdown-friendly format (one row per block, key:value pairs)
- For >20 rows, summarize: row count, sample of first 5, sample of last 5

If the result is empty, say "0 rows" — don't try a different query without asking.

## Hard rules

- Never invent column names. If the schema doc doesn't have it, refuse and ask.
- Never run a write query without explicit confirmation.
- Never run against prod without the user typing "prod" in the confirmation.
- Never `SELECT *` — always enumerate columns. Forces awareness of what's being read.
- Never paste passwords into the conversation, even if the user asks. Use the `connections.md` canonical shape per engine — mysql `--login-path`, pgsql trust-auth / `$DATABASE_URL`, sqlite file path — credentials never on the command line.
- Always read the connections cookbook for the canonical command shape — don't construct from memory.
