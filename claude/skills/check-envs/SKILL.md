---
name: check-envs
description: Diff a project's `.env` against its `.envrc` by KEY NAME only (never values) and report drift — keys present in one file but not the other. Walks up from cwd to the nearest pair, honors direnv's `dotenv`/`dotenv_if_exists` directive (which makes `.env` keys available in the direnv env by design, so those are not drift), and prints a three-bucket report: only-in-.env, only-in-.envrc, and in-both. Eliminates the recurring "I forgot to sync the .envrc" bite. Secret-safe: extracts names with grep, never cats or prints a value.
when_to_use: User types `/check-envs`, or asks to check / diff / reconcile `.env` against `.envrc`, or says "did I sync the envrc?", "env drift", "are my env files in sync?". Also run it proactively right after editing a `.env` value that a runbook says must live in both files.
disable-model-invocation: true
---

# check-envs — `.env` ↔ `.envrc` drift check (key names only)

Reports which env keys are defined in one file but not the other. Values are **never** read or printed — only the key names left of `=`. This closes the recurring "the value is in `.env` but I forgot the `.envrc`" drift bite.

## Secret hygiene (load-bearing — this skill touches credential files)

- **Names only, never values.** Every extraction cuts at the first `=` and keeps the left side. Do not `cat`, `head`, `less`, or otherwise dump either file. (The `secret-dump-guard` PreToolUse hook blocks `cat .env` anyway — use the grep forms below, which it allows.)
- If a command would print a value, it is the wrong command. Re-derive the key list, do not work around it.

## Workflow

### Step 1 — Locate the pair (read-only)

Walk up from cwd to the nearest directory that has **both** `.env` and `.envrc`. If the user named a directory or a specific file, use that. If only one of the two exists, say so and stop (nothing to diff — report the present file's key count and that its counterpart is absent). Echo the resolved paths before reading.

### Step 2 — Extract key names (secret-safe)

```bash
env_keys()   { grep -oE '^[[:space:]]*(export[[:space:]]+)?[A-Za-z_][A-Za-z0-9_]*[[:space:]]*=' "$1" \
                 | sed -E 's/^[[:space:]]*(export[[:space:]]+)?//; s/[[:space:]]*=.*$//' | sort -u; }
env_keys .env   > /tmp/_ck_env.keys
env_keys .envrc > /tmp/_ck_envrc.keys
```

`.env` lines are `KEY=value`; `.envrc` (direnv, sourced as bash) may use `export KEY=…` or bare `KEY=…`. The same extractor handles both because `export ` is optional in the regex. Only the name survives the `sed` — no value is ever captured.

### Step 3 — Detect the direnv `dotenv` bridge

```bash
grep -qE '^[[:space:]]*(dotenv|dotenv_if_exists)([[:space:]]|$)' .envrc && echo "BRIDGED"
```

If `.envrc` calls `dotenv` / `dotenv_if_exists` (optionally `dotenv .env`), then direnv loads every `.env` key into the environment automatically. In that case **only-in-.env keys are NOT drift** — say so explicitly and treat the bridge as the source of truth; still report only-in-.envrc keys (those add to, or override, the `.env` set).

### Step 4 — Report (three buckets, names only)

```bash
echo "== only in .env (missing from .envrc) =="; comm -23 /tmp/_ck_env.keys /tmp/_ck_envrc.keys
echo "== only in .envrc (missing from .env) =="; comm -13 /tmp/_ck_env.keys /tmp/_ck_envrc.keys
echo "== in both =="; comm -12 /tmp/_ck_env.keys /tmp/_ck_envrc.keys | wc -l | tr -d ' '
rm -f /tmp/_ck_env.keys /tmp/_ck_envrc.keys
```

Present it as a short table: **only-in-.env** (the drift that bites — a value set in `.env` a runbook wanted mirrored), **only-in-.envrc**, and the **in-both** count. If the `dotenv` bridge is present, prefix the only-in-.env bucket with "not drift — bridged by direnv `dotenv`". If both buckets are empty, state "in sync — N keys, no drift."

## Hard rules

- **Never print a value.** Only key names left of `=`. No `cat`/`head`/`less` on either file.
- **Read-only.** This skill diffs and reports; it never edits `.env` or `.envrc`. If drift is found, tell the user which keys and let them decide — do not auto-sync (values are user-owned secrets).
- **Bridge-aware.** A `.envrc` that `dotenv`s the `.env` is not drifting on shared keys — do not report those as missing.
- **One pair.** Diff the nearest `.env`/`.envrc` pair; if the user needs another dir, they name it. Do not fan out across the tree.
- **disable-model-invocation: true** — fires only on explicit `/check-envs` or the phrases above; never auto-runs mid-task.
