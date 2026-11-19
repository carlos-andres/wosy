---
name: harness-work
description: Rules for work on the Claude harness itself. Use when you read or change a Claude home, its settings, hooks, rules, skills or memory, or the work home's limits, or when you judge whether Claude Code is outdated.
---

# Harness work

## Which home

- Resolve the Claude home from `$CLAUDE_CONFIG_DIR`, or from the directory the transcript path sits in. **Never assume `~/.claude`.** A session can run against another home, and reading the wrong one reports a configuration that is not in force.
- The work home's limit is a blocklist (deny rules), not an allowlist. A runtime-built path in an interpreter is held only by the auto classifier.
- Claude Code on the stable channel — compare with the npm `stable` dist-tag, not `latest`, before calling it outdated.

## Design

- Do not enforce a convention the adoption data says is dead. A rule that lives only in a document is a wish.
- Save tokens, keep the essence — measure where the weight is; cut structure losslessly, prove it byte-identical.
