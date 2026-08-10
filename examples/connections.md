# connections.md (example)

Drop this file into your project's `.devwork/` as the **canonical reference** for how Claude (and Claude Code subagents) reach external systems: databases, remote hosts, message queues, anything that needs a connection.

The wosy agents (`agent-query`, `agent-server`, `agent-scribe`) and the `/query` skill read this file before running. Keep it accurate; they trust it.

---

## Databases

### Primary database

- **Engine:** `<mysql | pgsql | sqlite>`
- **Canonical command:** `<EXAMPLE_DB_COMMAND>`

#### Examples

**MySQL with login-path (no creds on the command line):**

```bash
mysql --login-path=<ALIAS_NAME> -D <DATABASE_NAME>
# Read-only example:
mysql --login-path=<ALIAS_NAME> -D <DATABASE_NAME> -e "SELECT 1"
```

Set up the login-path once:

```bash
mysql_config_editor set --login-path=<ALIAS_NAME> --host=<HOSTNAME> --user=<USERNAME> --password
```

**PostgreSQL with `.pgpass`:**

```bash
psql -h <HOSTNAME> -U <USERNAME> -d <DATABASE_NAME>
# Read-only example:
psql -h <HOSTNAME> -U <USERNAME> -d <DATABASE_NAME> -c "SELECT 1"
```

Credentials in `~/.pgpass` (mode 600):

```
<HOSTNAME>:<PORT>:<DATABASE_NAME>:<USERNAME>:<PASSWORD>
```

**SQLite:**

```bash
sqlite3 <RELATIVE_PATH_TO_DB_FILE>
```

### Replicas / read-only mirrors (optional)

| Name | Command | When to use |
|---|---|---|
| `<replica-name>` | `<COMMAND_WITH_PLACEHOLDERS>` | reporting reads — never writes |

### Default rules

- **Read-only by default** — `SELECT`, `DESCRIBE`, `SHOW`, `EXPLAIN`. Anything that mutates (`INSERT`, `UPDATE`, `DELETE`, `DROP`, `TRUNCATE`, DDL) requires explicit user confirmation in chat.
- **Credentials never in queries** — they live in `~/.my.cnf`, `~/.pgpass`, login-path, or `.envrc`. The query itself stays portable.
- **Schema before queries** — Claude reads `.devwork/schema/<table>.md` before composing SQL. Never invent column names.

---

## Remote hosts (SSH)

### Primary remote

- **Alias:** `<SSH_ALIAS>`
- **Defined at:** `~/.ssh/config`
- **Verify alias resolves:** `ssh -G <SSH_ALIAS> | head -5`

Example `~/.ssh/config` entry:

```
Host <SSH_ALIAS>
    HostName <REMOTE_HOSTNAME>
    User <REMOTE_USERNAME>
    IdentityFile ~/.ssh/<KEY_FILENAME>
    Port 22
```

### One-shot remote commands

```bash
ssh <SSH_ALIAS> "<COMMAND>"
ssh <SSH_ALIAS> "tail -n 200 /var/log/<SERVICE_NAME>/<LOG_FILE>"
```

### Default rules

- **Read-only by default.** Confirm in chat before any write/restart/deploy command.
- **Never `ssh root@<host>` directly** — go through the configured alias which uses a non-root user.

---

## Object stores / queues / other services

Add a section per service the project actually uses. Examples:

### Redis

```bash
redis-cli -h <HOSTNAME> -p <PORT> [-a <PASSWORD_FROM_ENV>]
# or via env var:
REDISCLI_AUTH="<PASSWORD>" redis-cli -h <HOSTNAME>
```

### S3-compatible object store

- **Endpoint:** `https://<BUCKET_HOSTNAME>`
- **Bucket:** `<BUCKET_NAME>`
- **Credentials:** `aws-cli` profile `<PROFILE_NAME>` (configured in `~/.aws/credentials`)

```bash
aws --profile <PROFILE_NAME> s3 ls s3://<BUCKET_NAME>/<PREFIX>/
```

---

## Drift check

This file silently drifts when infrastructure changes. Before treating any entry as authoritative:

```bash
# DB
mysql --login-path=<ALIAS_NAME> -e "SELECT 1" || echo "alias broken"
psql -h <HOSTNAME> -U <USERNAME> -d <DATABASE_NAME> -c "SELECT 1" || echo "creds/host broken"

# SSH
ssh -G <SSH_ALIAS> | grep -E '^(hostname|user) ' || echo "alias not in ~/.ssh/config"
ssh -o ConnectTimeout=5 <SSH_ALIAS> "echo ok" || echo "alias unreachable"
```

If any check fails, **update this file first**, then proceed. Don't claim "the alias is missing" until you've verified — the labels lag the OS config.

---

## Placeholders used in this template

Replace all `<UPPERCASE_PLACEHOLDERS>` with project-specific values before committing:

- `<HOSTNAME>`, `<PORT>`, `<USERNAME>`, `<PASSWORD>` — connection coords
- `<ALIAS_NAME>`, `<SSH_ALIAS>`, `<PROFILE_NAME>` — local aliases for the above
- `<DATABASE_NAME>`, `<BUCKET_NAME>`, `<SERVICE_NAME>` — resource names
- `<KEY_FILENAME>`, `<LOG_FILE>`, `<RELATIVE_PATH_TO_DB_FILE>` — file references
- `<COMMAND>`, `<COMMAND_WITH_PLACEHOLDERS>`, `<EXAMPLE_DB_COMMAND>` — command templates
