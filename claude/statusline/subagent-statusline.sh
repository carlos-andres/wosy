#!/usr/bin/env bash
#
# subagent-statusline.sh - one row per subagent, in ONE jq call.
#
#   description | model/effort | label
#
# It reads the payload and nothing else. No transcript, no cache, no second process.
#
# WHY THERE IS NO AGENT NAME. The official field list documents `tasks[].name` as a string.
# Measured against 40 real payloads captured on this machine on 2026-09-18, the key is ABSENT
# from every task: not null, absent, and `type` is `local_agent` every time. The predecessor
# recovered a name by scanning the session transcript twice with jq and caching the result to a
# TSV. That cost 123.0 ms on first sight of a new agent, against a 4.8 MB transcript, and 27.8 ms
# warm. What it recovered was the agent TYPE — `general-purpose` in every cached row — which says
# less about what is running than `description` does. So `description` is the identity here.
#
# Field reference: https://code.claude.com/docs/en/statusline.md#subagent-status-lines
# Output contract: one JSON object per line, {"id": "<task id>", "content": "<row>"}.
#
# `effort` is absent when the task inherits the session value, which is the common case, so it
# renders as `inherit` rather than being dropped. `model` arrives pre-formatted, e.g.
# `claude-opus-5[1m]`, and only the vendor prefix and a date suffix are removed.
set -uo pipefail

jq -c '
  # The row width the panel gives us. Absent in an older payload, so 100 stands in.
  (.columns // 100) as $w

  # description carries the identity and the scope. label carries the live activity, which
  # changes as the agent works, so it yields first when the row will not fit.
  | (($w * 40 / 100) | floor) as $dw
  | (($w * 35 / 100) | floor) as $lw

  | def cut($n): if $n < 8 then "" elif (. | length) > $n then (.[0:$n-1] + "…") else . end;
    def model: if . == null then "?"
               else (tostring | sub("^claude-"; "") | sub("-[0-9]{8}"; "")) end;

    .tasks[]? | {
      id: .id,
      content: ((.description // .label // "agent") | cut($dw))
             + "  " + (.model | model)
             + "/" + ((.effort // "inherit") | tostring)
             + (((.label // "") | cut($lw)) | if . == "" then "" else "  " + . end)
    }
' 2>/dev/null
