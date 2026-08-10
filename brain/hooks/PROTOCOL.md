# PROTOCOL — the gated loop + opt-in wiring (P3.4)

The brain's protocol upgrades the working keys so each stage has a **gate** enforced by the tools.
Everything here is **provided but DISABLED** — provision-then-enable, fail-open.

## The gated loop (master §C)
| stage | gate | enforced by |
|---|---|---|
| **KICKOFF** | a record with `context·status·to-do·done_when·scope` | `brain import` rejects an incomplete task (P0 schema; proven: a task missing `done_when` is rejected) |
| **PLAN** | whole-scope: every step a checkpoint, design/wire/docs | `done_when[]` + `todo[]` on the task |
| **AUTO** | each checkpoint green; **zero new ceremony files**; KB writes via SCRIBE | WATCHDOG (`enforce_kb_scribe`) → `brain set/append` |
| **FEEDBACK** | no correction lost in the transcript | corrections stored as `decision` records via `brain import` |
| **WRAP** | `done` rejected unless scope met; then consolidate + remove stale | DONE-GATE (`brain gate`) blocks while to-dos/scope unmet |

Inline ops are continuously guarded by the WATCHDOG: inline DB query → query library / `q.sh` (+ PROMOTER
grows it), inline ssh → documented connection, KB-file write → SCRIBE. **Source/config/tests flow free.**

## Enable wiring (do this only when you opt in)
1. **Install the binary** on PATH:
   ```
   CGO_ENABLED=0 go build -o ~/.local/bin/brain ./cmd/brain   # from brain/
   ```
2. **Per-project opt-in** — write `.devwork/wosy.flags` (fail-open: absent = no enforcement):
   ```
   enforce_inline_sql=1     # block inline SQL; routes to q.sh   (fires only if queries/library exists)
   enforce_inline_ssh=1     # block inline ssh; routes to connections.md   (fires only if connections.md exists)
   enforce_kb_scribe=1      # block hand-writing KB files; routes to SCRIBE   (fires only if brain/store.db exists)
   # wosy_enforce=0         # global kill switch for this project
   ```
   Each rule is gated on its sanctioned tool existing locally → **cannot self-lockout**.
3. **Register the PreToolUse hook** in `~/.claude/settings.json` (alongside the existing ones):
   ```json
   { "matcher": "Bash|Write|Edit|MultiEdit",
     "hooks": [{ "type": "command", "command": "$HOME/.claude/hooks/watchdog.sh" }] }
   ```
   The canonical watchdog shim lives at `<repo-root>/claude/hooks/watchdog.sh`. Run
   `<repo-root>/install.sh --apply` to deploy it to `~/.claude/hooks/` (the script
   handles backup + idempotent matcher registration). It fails open if `brain` is absent.
4. **WRAP gate** — run `brain gate --task=<id>` before declaring done (or wire into a Stop hook later).

## Verify before enabling (isolation)
```
echo '{"tool_name":"Bash","tool_input":{"command":"mysql -e \"SELECT 1\""},"cwd":"<enforced-proj>"}' \
  | BRAIN_BIN=./brain ../claude/hooks/watchdog.sh ; echo "exit=$?"   # -> WATCHDOG ... ; exit=2
```

## R11 (no main-terminal diff) — note
`enforce_kb_scribe` routes KB writes to `brain set/append`, which mutate SQLite (no markdown diff on
screen). For long writes the master also pairs a `subagent-diff` route (isolated subagent) — that piece
is future work; the SCRIBE path already removes hand-written status/context md from the main terminal.
