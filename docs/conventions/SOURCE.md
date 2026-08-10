---
title: SOURCE.md convention — data-snapshot provenance
version: 1.0.0
status: shipped
updated: 2026-05-26
applies_to: every `.devwork/data/<source>/` directory
enforces: data provenance is explicit; stale snapshots cannot be silently confused with prod truth
owner: WOSY / brain detect (R8)
---

# SOURCE.md — the data-snapshot provenance convention

## 1. Why this exists (W-03 evidence)

A CSV under `.devwork/data/acme/marketfeed/<dealer>.csv` had an `mtime` of today but did NOT reflect production truth — prod uses a different path on a different worker host. The operator drew a wrong-direction conclusion from a fresh-looking local snapshot.

**Failure mode:** `ls -la` shows recent files → "data must be current" → wrong conclusion. Mtime is not provenance. A local copy is not prod truth.

**Fix:** every `.devwork/data/<source>/` directory carries a `SOURCE.md` declaring where the data came from, when, and whether it is prod truth. Without it, `brain detect` flags the directory as `missing SOURCE.md`. Stale snapshots are tagged or quarantined; new data dirs must declare provenance before content lands.

## 2. Required shape

`SOURCE.md` is a markdown file with YAML front matter. The front matter MUST include the keys below. Body is free text.

```markdown
---
origin: <URL | host | vendor name>          # where the data came from
captured_at: <ISO-8601 date or datetime>    # when this snapshot was taken
captured_by: <operator | process | cron>    # who or what captured it
is_production: <true | false>               # does this directory represent prod truth?
prod_truth_table: <db.table.column | null>  # if is_production=false, where prod truth lives
encoding: <utf-8 | latin1 | ...>            # text encoding (csv-read.sh consumes this)
staleness_threshold_days: <int>             # OPTIONAL; default 7
notes: <free text or omit>                  # OPTIONAL
---

# <human-readable title>

<free-form body — origin link, how to refresh, gotchas, etc.>
```

### Field semantics

| field | required | notes |
|---|---|---|
| `origin` | yes | A URL is preferred when available. Otherwise the host or vendor name (e.g. `acme.example`, `worker-host-3:/var/feeds/`). |
| `captured_at` | yes | ISO-8601. Date-only is acceptable (`2026-05-26`); datetime is preferred when known. |
| `captured_by` | yes | Operator name, process name, or cron job identifier. "manual download by operator", `feed-sync.service`, `cron:nightly-feed-mirror` are all valid. |
| `is_production` | yes | `false` for local snapshots, dev mirrors, and one-off captures. `true` ONLY when the directory IS the prod source (rare in `.devwork/data/`). |
| `prod_truth_table` | yes when `is_production=false` | `db.table.column` shape (e.g. `feed_item_hash.last_seen_at`). If prod truth is a service/endpoint, use `service:<name>` or `url:<url>`. Use `null` only when prod truth genuinely does not exist. |
| `encoding` | yes | Consumed by `.devwork/bin/csv-read.sh` (N-08). Common values: `utf-8`, `latin1`, `cp1252`. Field name MUST match exactly (`encoding:` — lowercase, colon). |
| `staleness_threshold_days` | optional | Defaults to `7`. `brain detect` flags entries older than this. |
| `notes` | optional | Free text. Refresh instructions, gotchas, links to runbooks. |

## 3. Rationale (the rules this encodes)

1. **Stale snapshots must be tagged or quarantined.** A `SOURCE.md` with `captured_at` older than `staleness_threshold_days` is flagged by `brain detect`. The operator sees the stale tag before they make a conclusion.
2. **New data dirs need a SOURCE.md before content lands.** A directory under `.devwork/data/` without `SOURCE.md` is flagged `missing SOURCE.md` — content is not blocked, but the directory is loudly unprovenanced.
3. **`is_production` is the one-line truth signal.** A local mirror is `false`; prod truth lives at `prod_truth_table`. The operator follows the pointer instead of trusting the mirror.
4. **Encoding is explicit.** `csv-read.sh` reads `encoding:` from SOURCE.md before falling back to its path-heuristic. One source of truth.

## 4. Integration points

- **`brain detect <path>`** — surfaces a `data_provenance:` block: one entry per `.devwork/data/<source>/` directory found, with `is_production`, `staleness_threshold_days`, and days-since-`captured_at`. Stale entries are flagged. Missing-SOURCE.md dirs emit `status: "missing SOURCE.md"`.
- **`.devwork/bin/csv-read.sh`** (N-08) — reads `encoding:` from the sibling SOURCE.md before falling back to path heuristics.
- **WOSY §3 architecture** — the convention is listed under "Architecture / data provenance".

## 5. Worked example

```markdown
---
origin: https://acme.example/dealer-export
captured_at: 2026-05-26
captured_by: manual download by operator
is_production: false
prod_truth_table: feed_item_hash.last_seen_at
encoding: latin1
staleness_threshold_days: 7
notes: |
  Local copies of per-dealer Acme exports. Prod consumes these via
  the worker host at /var/spool/feeds/ — those mtimes are authoritative,
  not these. Refresh with: scripts/download-acme-exports.sh
---

# Acme dealer exports — local snapshot

These CSVs are dealer-keyed exports from acme.example. Each file is named
`<dealer_stock_id>.csv`. Latin-1 encoded.

Prod truth: `feed_item_hash.last_seen_at` (per item, not per file).
```

## 6. Authoring checklist

- [ ] File lives at `.devwork/data/<source>/SOURCE.md` (sibling to data files).
- [ ] YAML front matter parses (open `---`, close `---`).
- [ ] All required fields present.
- [ ] `captured_at` is ISO-8601.
- [ ] `is_production` is a bare boolean (no quotes).
- [ ] If `is_production: false`, `prod_truth_table` is set.
- [ ] `encoding` matches what `csv-read.sh` expects (lowercase, no quotes).
- [ ] Re-run `brain detect $PWD | jq .data_provenance` — entry shows up green (not stale, not missing).

## 7. Backfill policy

When you find a `.devwork/data/<source>/` directory without `SOURCE.md`:

1. Inspect the contents (`ls`, sample a CSV header).
2. Ask the operator for `origin` + `captured_at` if not derivable from mtime / commit log.
3. Default `is_production: false` unless you have evidence otherwise.
4. Default `encoding: utf-8` unless the file is binary-different (`file <csv>` shows non-utf-8).
5. Write the SOURCE.md before doing any analytical work on the directory.

## 8. Anti-patterns

- ❌ Trusting `mtime` as provenance. Mtime is when the file was touched on this disk, not when the data was sourced.
- ❌ Treating a local snapshot as prod truth because "it looks fresh".
- ❌ Putting encoding in code (`iconv -f latin1` in a script) when it could live next to the data.
- ❌ Silently overwriting a SOURCE.md to "refresh" — bump `captured_at` and `captured_by`, keep the history visible in git.
