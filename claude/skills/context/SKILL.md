---
name: context
description: Print current Claude Code context-window usage (tokens used / total / percent) — same shape the statusline shows, but on demand. Reads the on-disk session transcript JSONL and pulls the last assistant message's usage record. Silent-callable form returns a structured single line (`pct=<int>`) consumed by `context-checkpoint.sh` (A-03 §4.4) at 60% nudge / 85% HARD STOP thresholds. Use when the user asks "how much context is left?", "what's the context usage?", or before deciding to stop/continue work that may overflow.
when_to_use: User asks "how much context is left?" / "what's the context usage?", types `/context`, or before deciding to stop vs continue work that may overflow. Also invoked silently by `context-checkpoint.sh` hook every N tool calls (Phase III) to drive the 60% nudge / 85% HARD STOP rule (C-02 Rule 1).
disable-model-invocation: true
---

# /context (on-demand · silent-callable · utility)

On-demand context-window usage check. Two callers: the user (interactive) and `context-checkpoint.sh` (silent, machine-parsed).

## When this fires

- **Interactive**: user asks "how much context is left?", "what's the context usage?", types `/context`, or before deciding to stop vs continue.
- **Silent**: `context-checkpoint.sh` hook invokes after every N tool calls (default N=10) and on every subagent dispatch return. The hook parses the `pct=` line and branches: <60% no-op, 60–85% emit nudge + fire `handoff-vigilante.sh`, >85% HARD STOP + force `/clear` directive.

## Workflow

Run `~/.claude/skills/context/bin/check.sh` via the Bash tool. The script:

1. Resolves the current session's transcript file under `~/.claude/projects/<cwd-encoded>/*.jsonl` (top-level — subagent jsonls excluded).
2. Reads the last line with a `usage` field (assistant-message turn).
3. Sums `input_tokens + cache_creation_input_tokens + cache_read_input_tokens` — the same formula `statusline.sh` uses.
4. Resolves the context-window size: first a per-session `~/.claude/state/context-window-<sid>` override (guards 1M-window sessions; accepted only within [100k, 2M]), else a model→size table (Opus 4.7/4.8/4.9 → 400k, `[1m]` variants → 1M, others → 200k default).
5. Prints `used/total (pct%)` plus a breakdown of input / cache_create / cache_read / output.

Append `--silent` for the hook-callable form (single `pct=<int>` line, machine-parseable).

```bash
bash ~/.claude/skills/context/bin/check.sh             # interactive
bash ~/.claude/skills/context/bin/check.sh --silent    # for context-checkpoint.sh
```

Report the single-line result back to the user verbatim when interactive.

## Output

Interactive (default):

```
246k/400k (61%) — input=1 cache_create=5k cache_read=240k output=2k — model=claude-opus-4-8
```

Silent (`--silent`) — one key=value line, `pct` only (no tier/model/total fields):

```
pct=61
```

The consumer (`context-checkpoint.sh`) classifies that `pct` itself (thresholds match C-02 Rule 1):
- `green`: pct < 60
- `yellow`: 60 ≤ pct < 85 → hook emits nudge + fires handoff-vigilante
- `red`: pct ≥ 85 → hook emits HARD STOP + force-`/clear` directive

## Caveats

- The on-disk record lags the live session by one assistant turn — what you read is the previous turn's tally. Good enough for "are we near the limit?" decisions; not exact-to-the-token.
- Context-window-size is inferred from the model name unless the per-session state-file override is present. If the user is on a model not in the table, the result defaults to 200k and the percentage will be wrong. Refine the case statement in `bin/check.sh` if a new model lands.
- The "current" session is the most recently modified top-level jsonl in the project dir. If two Claude sessions are open in the same cwd, this picks the more-recently-active one — usually correct, but worth noting if results look off.
- `--silent` output is the machine-readable contract: the single line `pct=<int>`. Changing that key or shape breaks `context-checkpoint.sh` parsing — version the format if it must change.
