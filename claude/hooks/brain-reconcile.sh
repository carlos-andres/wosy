#!/usr/bin/env bash
# brain-reconcile.sh — RETIRED 2026-07-30 (was: SessionEnd hook, non-blocking).
#
# What it did: on every SessionEnd it detected the active project from cwd (or a
# .devwork/wosy.yml satellite pointer) and ran `brain reconcile <slug>`, whose
# CLI walks plans/*/tasks/*/task.yml and merges the C-02 maintain.* deltas into
# the project encyclopedia/runbooks. That plans/<plan-id>/tasks/<task-id>/task.yml
# layer never materialized on disk, so the walk always found zero task.yml
# (matches=0) and both brain *_pointer rows it would populate stayed empty — the
# hook was dormant on every fire. Retired to a no-op rather than run project
# detection plus a CLI subprocess on every session close for nothing.
#
# The `brain reconcile` CLI SUBCOMMAND is unaffected and still available — the
# /consolidate skill invokes it explicitly (its Phase C). Only this automatic
# SessionEnd invocation is gone.
#
# Reversal: the full original logic is in brain-reconcile.sh.bak.20260730-125756
# beside this file — restore it verbatim if a real
# plans/<plan-id>/tasks/<task-id>/task.yml layout is ever adopted. This file's
# settings.json SessionEnd registration is now dead weight; unregistering it is
# a separate settings.json change.

exit 0
