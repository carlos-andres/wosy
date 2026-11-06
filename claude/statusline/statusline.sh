#!/usr/bin/env bash
#
# statusline.sh - the main Claude Code status line: one jq call, one git call. Requires bash 5.
#
#   model | session | dir | [branch |] context | effort [| 5h | 7d | spend] [| corp]
#
# Both homes run this same file. The corp tag prints only when it runs from ~/.claude-work,
# so the Obra reads apart from the personal home.
#
# Every field but the branch comes from the documented stdin payload. The payload has no branch
# field, so git reads it. Measured 2026-10-26 with hyperfine: 15.4 ms inside a repo.
#
# Field reference: https://code.claude.com/docs/en/statusline.md
#
# Absence is the normal case and is rendered, never guessed:
#   - `session_name` is absent until /rename, --name or an AI title, so the short session id stands in.
#   - `rate_limits` exists only on Pro/Max or behind a spend-limit gateway. The enterprise home
#     receives `null` (payload captured 2026-10-26, v2.1.308), so an absent window is not printed.
#   - `context_window.used_percentage` can be null early, so it is recomputed from
#     `total_input_tokens`, the documented input-only numerator (output tokens never count).
#   - `effort.level` is absent on a model with no reasoning effort.
#
# jq stderr is not silenced: Claude Code shows only stdout, and `claude --debug` logs stderr.
set -uo pipefail

corp=
[[ ${BASH_SOURCE[0]} == */.claude-work/* ]] && corp=1

# jq prints five lines: session id, "used size", cwd, then the halves on either side of the branch.
# One value per line, because `read` with a tab IFS collapses an empty leading field.
mapfile -t f < <(jq -r --arg corp "$corp" '
  def esc: "\u001b[";
  def dim:   esc + "2m";
  def off:   esc + "0m";
  def teal:  "46;149;153";
  def red:   esc + "38;2;255;85;85m";
  def amber: esc + "38;2;230;200;0m";
  def green: esc + "38;2;0;160;0m";
  def sep:   " " + dim + "|" + off + " ";
  def corp:  if $corp == "" then null else esc + "38;2;" + teal + "mcorp" + off end;

  # Names come from the user or an AI title. Strip control characters so none can inject an escape.
  def clean: tostring | explode | map(select(. >= 32 and . != 127)) | implode;

  # A percentage colours by band. 80 and over is red, 50 and over is amber.
  def pct($n): ($n | floor) as $p
    | (if $p >= 80 then red elif $p >= 50 then amber else green end) + "\($p)%" + off;

  # Token counts read as 111k, not 111000.
  def human:
    if . >= 1000000 then "\(. / 1000000 * 10 | floor / 10)m"
    elif . >= 1000 then "\(. / 1000 | floor)k"
    else tostring end;

  # A limit window, or null when it is absent. $fmt is the strftime of its reset, or null.
  def window($label; $w; $fmt):
    if $w.used_percentage == null then null
    else "\($label) " + pct($w.used_percentage)
      + (if $fmt == null or $w.resets_at == null then ""
         else dim + "@" + ($w.resets_at | strflocaltime($fmt)) + off end)
    end;

  . as $d
  | ($d.context_window // {}) as $cw
  | ($d.rate_limits // {}) as $rl
  | ($cw.context_window_size // 200000) as $size
  | ($cw.total_input_tokens // 0) as $used
  | ($cw.used_percentage // (if $size > 0 then $used * 100 / $size else 0 end)) as $cpct
  | ($d.workspace.current_dir // $d.cwd // "") as $cwd

  # display_name already carries the version, e.g. "Opus 5.5 (1M context)".
  | (esc + "38;2;" + teal + "m" + ($d.model.display_name // "Claude" | clean) + off) as $model
  | ($d.session_name // (($d.session_id // "-") | split("-")[0]) | clean) as $sess
  | ($cwd | split("/") | last // "-" | clean) as $dir
  | ("\($used | human)/\($size | human) " + pct($cpct)) as $ctx
  | ($d.effort.level // "-" | clean) as $eff
  # spend_limit is the apps-gateway spend limit. Dollars arrive up to 5 min after the percentage.
  | (window("spend"; $rl.spend_limit; null)
     | if . != null and $rl.spend_limit.used_usd != null
       then . + dim + " $\($rl.spend_limit.used_usd | floor)/$\($rl.spend_limit.limit_usd)" + off
       else . end) as $spend

  | ($d.session_id // ""), "\($used | floor) \($size)", $cwd,
    ([$model, $sess, $dir] | join(sep)),
    ([$ctx, $eff,
      window("5h"; $rl.five_hour; "%H:%M"),
      window("7d"; $rl.seven_day; "%a %d %H:%M"),
      $spend, corp] | map(select(. != null)) | join(sep))
')
sid=${f[0]:-} usz=${f[1]:-} cwd=${f[2]:-} left=${f[3]:-} right=${f[4]:-}

# Relay "used size" for `ctx` (skills/context/bin/check.sh): only this stdin payload knows the real
# window, the transcript drops the [1m] suffix. Each home writes to its own <home>/state.
state=${CLAUDE_CONFIG_DIR:-$HOME/.claude}/state
if [[ -n $sid && $usz =~ ^[0-9]+\ [0-9]+$ ]]; then
  [[ -d $state ]] || mkdir -p "$state"
  printf '%s\n' "$usz" > "$state/context-window-${sid//[^a-zA-Z0-9_-]/_}"
fi 2>/dev/null

[[ -n $left ]] || { printf 'Claude'; exit 0; }

# Empty outside a repo and on a detached HEAD, so nothing prints.
# --no-optional-locks keeps this read from taking index.lock while a git command runs.
branch=
[[ -n $cwd ]] && branch=$(git -C "$cwd" --no-optional-locks branch --show-current 2>/dev/null)

sep=$' \e[2m|\e[0m '
printf '%s' "$left${branch:+$sep$'\e[38;2;180;140;220m'$branch$'\e[0m'}$sep$right"
