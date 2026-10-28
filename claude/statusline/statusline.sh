#!/usr/bin/env bash
#
# statusline.sh - the main Claude Code status line, in ONE jq call.
#
#   model | session | context | effort | 5h | 7d | extra
#
# Every field comes from the documented stdin payload. Nothing is looked up, computed by another
# process, or read off disk. The predecessor paid two `awk` calls to humanise token counts and two
# `date` calls to format the two reset epochs. jq 1.8 does both: integer arithmetic for the counts
# and `strflocaltime` for local time. Measured 2026-09-18: 20.1 ms before, and the extraction floor
# for this payload is 7.2 ms.
#
# Field reference: https://code.claude.com/docs/en/statusline.md
#
# Absence is the normal case and is rendered, never guessed:
#   - `session_name` is absent unless set with /rename, so the short session id stands in.
#   - `rate_limits` is absent before the first API response and outside Pro/Max.
#   - `context_window.used_percentage` can be null early, so it is recomputed from the counts.
#   - `effort.level` is absent on a model with no reasoning effort.
#
# jq reads stdin directly. Capturing it into a variable first cost a `cat` process and a second
# subshell for no gain, and the empty-input case is the same test either way.
set -uo pipefail

out=$(jq -r '
  # --- presentation helpers -------------------------------------------------
  def esc: "[";
  def dim:   esc + "2m";
  def off:   esc + "0m";
  def cyan:  esc + "38;2;46;149;153m";
  def red:   esc + "38;2;255;85;85m";
  def amber: esc + "38;2;230;200;0m";
  def green: esc + "38;2;0;160;0m";

  # A percentage colours by band. 80 and over is red, 50 and over is amber.
  def band: if . == null then dim elif . >= 80 then red elif . >= 50 then amber else green end;
  def pct($n): ($n | floor) as $p | ($p | band) + "\($p)%" + off;

  # Token counts read as 111k, not 111000.
  def human:
    if . == null then "?"
    elif . >= 1000000 then "\(. / 1000000 * 10 | floor / 10)m"
    elif . >= 1000 then "\(. / 1000 | floor)k"
    else tostring end;

  def sep: " " + dim + "|" + off + " ";

  # --- the payload ----------------------------------------------------------
  . as $d
  | ($d.context_window // {}) as $cw
  | ($d.rate_limits // {}) as $rl
  | (($cw.current_usage // {}) | (.input_tokens // 0) + (.output_tokens // 0)
        + (.cache_creation_input_tokens // 0) + (.cache_read_input_tokens // 0)) as $sum
  | ($cw.context_window_size // 200000) as $size
  | (($cw.total_input_tokens // 0) as $t | if $t > 0 then $t else $sum end) as $used
  | ($cw.used_percentage // (if $size > 0 then $used * 100 / $size else null end)) as $cpct

  # 1. model. display_name already carries the version, e.g. "Opus 5", "Haiku 4.5".
  | (cyan + ($d.model.display_name // "Claude") + off) as $model

  # 2. session. The id stands in when no name was set, shortened to its first segment.
  | ($d.session_name // (($d.session_id // "") | split("-")[0]) // "-") as $sess

  # 3. context.
  | ("\($used | human)/\($size | human) " + pct($cpct)) as $ctx

  # 4. effort.
  | (($d.effort.level // "-") | tostring) as $eff

  # 5, 6, 7. the three limit windows. Absent is printed as n/a, never as zero.
  | (if $rl.five_hour.used_percentage == null then dim + "5h n/a" + off
     else "5h " + pct($rl.five_hour.used_percentage)
          + (if $rl.five_hour.resets_at == null then ""
             else dim + "@" + ($rl.five_hour.resets_at | strflocaltime("%H:%M")) + off end)
     end) as $fh
  | (if $rl.seven_day.used_percentage == null then dim + "7d n/a" + off
     else "7d " + pct($rl.seven_day.used_percentage)
          + (if $rl.seven_day.resets_at == null then ""
             else dim + "@" + ($rl.seven_day.resets_at | strflocaltime("%a %d %H:%M")) + off end)
     end) as $sd
  | (if $rl.spend_limit.used_percentage == null then dim + "extra n/a" + off
     else "extra " + pct($rl.spend_limit.used_percentage) end) as $ex

  | (($d.session_id // "") + "\t" + ($used | floor | tostring) + " " + ($size | tostring)), ([$model, $sess, $ctx, $eff, $fh, $sd, $ex] | join(sep))
' 2>/dev/null) || out=""
# Relay "used size" for `ctx` (skills/context/bin/check.sh): only this stdin payload knows the real
# window, the transcript drops the [1m] suffix. Both homes write to ~/.claude/state.
{ IFS=$'\t' read -r sid usz; IFS= read -r out; } <<< "$out"
[[ -n "$sid" && "$usz" =~ ^[0-9]+\ [0-9]+$ ]] && { mkdir -p "$HOME/.claude/state"; printf '%s\n' "$usz" > "$HOME/.claude/state/context-window-${sid//[^a-zA-Z0-9_-]/_}"; } 2>/dev/null
[ -n "$out" ] || out="Claude"
printf '%s' "$out"
