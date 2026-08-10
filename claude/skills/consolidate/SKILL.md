---
name: consolidate
description: Archive a SHIPPED plan as a single atomic operation per C-02 maintain stage + A-01 §1 verdict. Pre-checks every task.yml in `plans/<plan-id>/tasks/*/task.yml` is `status: done` or explicit `status: skipped` with notes; collects `maintain.encyclopedia_delta` + `maintain.runbook_delta` from each task; invokes `brain reconcile <project>` to merge deltas into `<team>/projects/<slug>/{encyclopedia.md, runbooks/<name>.md}`; then ONE atomic `mv <team>/projects/<slug>/plans/<plan-id>/ <team>/projects/<slug>/_archive/<date>-<plan-id>/`. Replaces the legacy 4-file fanout (`mv tasks/<id>/{context,status,scope,consolidated}.md`). Manual only — never auto-fires.
when_to_use: User explicitly types `/consolidate [<plan-id>]` after a plan ships (all tasks done, PR merged, work landed). Manual only — never auto-fires; never invoked from a hook.
disable-model-invocation: true
---

# Consolidate (manual · plan-level atomic archive · brain reconcile pickup)

One plan, one atomic mv. The plan ships; its maintain-stage deltas merge into project encyclopedia + runbooks via `brain reconcile`; the plan folder lands in `_archive/<date>-<plan-id>/`.

## When this fires

User types `/consolidate <plan-id>` after a plan is genuinely shipped. **Never** trigger from a hook — the user knows when a plan is done; hooks don't.

## Inputs

- `<plan-id>` (optional but recommended). If absent, scan `<team>/projects/*/plans/*/plan.yml` with `status: open` AND all child task.yml `status ∈ {done, skipped}` → list + ask which to consolidate.

## Workflow

### Phase A — Pre-checks (read-only, refuse-fast)

In order:

1. **Plan exists**: `<team>/projects/<slug>/plans/<plan-id>/plan.yml` resolves. Refuse otherwise.
2. **Plan not already archived**: `plan.yml.status: open`. Refuse if `archived`.
3. **All tasks shipped or explicitly skipped**: for each `tasks/<task-id>/task.yml`:
   - `task.yml.status` MUST be `done` OR `skipped`.
   - If `skipped`, the task.yml MUST carry `notes: <reason>` on the task or on at least one stage (refuse on bare skip).
   - Refuse with: `task <task-id> status=<x> — not done/skipped; finish or explicitly skip before consolidating`.
4. **Maintain deltas non-empty for every done task**: each `task.yml` with `status: done` MUST carry at least one of `maintain.encyclopedia_delta` OR `maintain.runbook_delta` non-empty (audit T-finding: shipped tasks must leave durable knowledge). Refuse: `task <task-id> shipped without maintain.encyclopedia_delta or maintain.runbook_delta — fill before consolidating, or convert to status=skipped with notes`.
5. **Optional git check**: if a branch is named in `plan.yml.links.branch`, check `git log --oneline | grep <plan-id>` — if zero matches, warn: `no commits reference plan-id — this might be premature; consolidate anyway? (yes/no)`. Wait for `yes` before continuing.

Emit one block:

```
PRE-CHECK:
  plan:     <team>/projects/<slug>/plans/<plan-id>/plan.yml (status=open)
  tasks:    <N> total · <D> done · <S> skipped
  deltas:   <E> encyclopedia_delta · <R> runbook_delta (across done tasks)
  branch:   <branch-name> · commits-reference=<yes|no>
ASK: lock / edit / abort?
```

User confirms `lock`. Otherwise abort.

### Phase B — Collect deltas + dry-run mv plan

Walk `tasks/*/task.yml`. Build:

- `encyclopedia_appends[]` — each `maintain.encyclopedia_delta` (markdown snippet) tagged with `task-id` for traceability.
- `runbook_writes[]` — each `maintain.runbook_delta` `{name, body}` tagged with `task-id`.
- `gotchas[]` — each `maintain.gotchas[]` item across tasks (merged; severity preserved).

Emit the dry-run move plan:

```
MOVE PLAN (dry-run):
  encyclopedia: <team>/projects/<slug>/encyclopedia.md
    + append section "<heading-from-delta-1>" (from task <task-id-1>)
    + append section "<heading-from-delta-2>" (from task <task-id-2>)
    ...
  runbooks:     <team>/projects/<slug>/runbooks/
    + write <runbook-name-1>.md (from task <task-id-1>)
    + write <runbook-name-2>.md (from task <task-id-2>)
    ...
  gotchas:      <K> entries promoted to brain.db gotcha table (via brain reconcile)
  archive mv:   <team>/projects/<slug>/plans/<plan-id>/ → <team>/projects/<slug>/_archive/<date>-<plan-id>/
ASK: type the plan-id to confirm, or `abort`.
```

Confirm-by-name discipline (matches Solomonic-reset shape in /wire-up): user must TYPE `<plan-id>` verbatim. No other confirmation accepted. Refuse `yes` / `lock` / `go` alone.

### Phase C — Brain reconcile (deltas merge)

Invoke `brain reconcile <project>` per C-03 §5.3 — the CLI verb walks `<team>/projects/<slug>/plans/*/tasks/*/task.yml` ITSELF, reads each `maintain.*` field, performs the merge, and writes to disk:

```bash
BRAIN_STORE=.devwork/<team>/brain.db brain reconcile <slug>
```

The CLI walks task.yml files; this skill's Phase B collection is a DRY-RUN preview for user-facing transparency, NOT input to the reconcile verb. `brain reconcile` does NOT accept stdin / a pre-computed delta JSON — it is a single-arg verb that takes the project slug and re-walks the disk. The Phase B preview and the reconcile output should match; if they diverge it indicates a race (file changed between collection and reconcile), which is a refuse-and-retry signal.

What `brain reconcile <slug>` does internally per C-03:
- Appends each `maintain.encyclopedia_delta` markdown snippet to `<team>/projects/<slug>/encyclopedia.md` under matching `## <heading>` (or creates the heading at the bottom if no match).
- Writes each `maintain.runbook_delta.body` to `<team>/projects/<slug>/runbooks/<runbook_delta.name>.md` (runbooks/ is NOT KB-protected so direct file write works).
- Inserts each `maintain.gotchas[]` item into `brain.db.gotcha` rows with `source_ref=<plan-id>/<task-id>` for traceability.

If `brain reconcile` returns non-zero, ABORT the move — leave the plan folder in place and surface the error. The mv only happens after deltas successfully land.

### Phase D — Atomic mv + status flip

ONE shell command:

```bash
mv "<team>/projects/<slug>/plans/<plan-id>/" "<team>/projects/<slug>/_archive/$(date -u +%Y-%m-%d)-<plan-id>/"
```

Then update `plan.yml.status: archived` in the archived copy via ONE atomic agent-scribe edit:

```
Agent(subagent_type="agent-scribe",
      prompt="Edit <team>/projects/<slug>/_archive/<date>-<plan-id>/plan.yml. old_string: 'status: open' · new_string: 'status: archived'.")
```

That single field flip is the only post-mv mutation. The `updated:` date is not auto-flipped here (intentionally — the canonical `updated` is preserved as the last actual edit date for audit trail; archive date is captured in the parent folder name `<date>-<plan-id>/`).

### Phase E — Return ≤3-line summary

1. **Archived:** `<team>/projects/<slug>/_archive/<date>-<plan-id>/`.
2. **Knowledge merged:** `<E>` encyclopedia sections, `<R>` runbooks, `<K>` gotchas (via `brain reconcile <slug>`).
3. **Active plans remaining:** `<list of other plans with status: open under this project>` or "none — project idle".

Don't paste the consolidated content back. The disk is the artifact.

## Recovery (manual, no skill support)

If the user wants to undo: the entire plan tree is at `_archive/<date>-<plan-id>/` with `plan.yml.status: archived`. `mv` back to `plans/<plan-id>/` and flip status to `open` via agent-scribe. The encyclopedia/runbook merges from Phase C are NOT auto-reversed — manual git-revert if the project encyclopedia is git-tracked, or hand-edit.

## Hard rules

- **Plan-level atomic only.** Never partial-consolidate (e.g. archive 3 of 5 tasks). All tasks must be done/skipped before the plan archives.
- **Maintain deltas are MANDATORY for done tasks.** Empty deltas = refuse. Skipped tasks may have empty deltas (with `notes` explaining the skip).
- **Confirm-by-name.** User types `<plan-id>` verbatim. No `yes` / `lock` / `go` shortcuts.
- **Reconcile BEFORE mv.** If `brain reconcile` fails, the mv does NOT happen. Knowledge merge is the gate; the archive is the consequence.
- **Single mv command.** The whole plan tree moves atomically — never per-file. `mv` is POSIX-atomic on same-volume operations.
- **Manual only.** Never auto-fire from a hook. The user's "this plan shipped" call is the only trigger.
- **No legacy `.devwork/tasks/<id>/` consolidation.** This skill targets C-02 plans only. Existing legacy `tasks/<id>/` folders stay in place per files-canonical 1.3.2 B1; consolidate them manually or with the pre-Wave-C legacy `/consolidate` (kept in git history at `claude/skills/consolidate/SKILL.md` before commit `e09290f` if needed).
- **No source-code edits.** This skill only writes to encyclopedia.md / runbooks/ / brain.db / plan.yml — never to project source.
- **disable-model-invocation: true** — manual user trigger only.
