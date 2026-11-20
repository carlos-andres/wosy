# Installing wosy 3.0.0

macOS or Linux. The files are copied, not linked; there is no installer script.

Coming from 2.1.0: do not copy 3.0.0 over it. Read the upgrade note of `CHANGELOG.md` first, then follow every step below.

## 1. Prerequisites

`bash` 5+, `python3` 3.9+, `jq`, `rg` (ripgrep), `fd`, `yq`, `check-jsonschema`, Go 1.21+ for `brain`, and `~/.local/bin` on `PATH`. `gitleaks` is recommended.

## 2. Back up your Claude home

```bash
cp -Rp ~/.claude ~/.claude.bak-$(date +%Y%m%d)
```

## 3. Point the files at your machine, then copy them

The tools carry absolute paths of the form `/Users/you/...` and call `/opt/homebrew/bin/python3`. Rewrite both in this clone, before copying (`git checkout .` undoes it):

```bash
rg -l -uu -e '/Users/you' -e '/opt/homebrew/bin/python3' claude devwork \
  | xargs sed -i.bak -e "s#/Users/you#$HOME#g" -e "s#/opt/homebrew/bin/python3#$(command -v python3)#g"
fd -u -t f -e bak . claude devwork -x rm
cp -Rp claude/. ~/.claude/
mkdir -p ~/.devwork && cp -Rp devwork/. ~/.devwork/
```

`claude/CLAUDE.md` replaces your own global file in that copy. Merge the two by hand when you already have one, and keep no Subagent classifier table in it: the classifier is `~/.claude/hooks/classifier.tsv`.

After the rewrite, paths under `$HOME/work/...` name an optional work area that `protocolo`, `encuadre` and `briefing-check` read; set them to your own or leave them unused.

## 4. Wire the hooks

Merge `hooks`, `permissions` and `statusLine` from `claude/settings.example.json` into `~/.claude/settings.json`. Keep your own `env`, `model` and plugins. The example wires 13 hook scripts over 9 events; the task hooks do nothing in a session that is not bound to a task, except rules P90 and P91 of `orchestrator.py`.

## 5. Build brain

```bash
cd brain && go build -o "$HOME/.local/bin/brain" ./cmd/brain
```

## 6. Verify

```bash
bash -n ~/.claude/hooks/*.sh && echo hooks-parse-ok
python3 ~/.claude/hooks/secret_guard.py self-test          # passed N   failed 0
python3 -I ~/.claude/hooks/validate-bash.test.py           # exit 0
~/.devwork/bin/protocolo --selftest
~/.devwork/bin/ctx --selftest
brain version
```

In a new Claude Code session, run a harmless `git push --dry-run`: `validate-bash.sh` must refuse it. Dispatch a `general-purpose` subagent without `model: opus`: `secret_guard.py` must deny it.

## Customize

- `claude/hooks/classifier.tsv` is the owner's table. Change a row there, and keep an agent file under `~/.claude/agents/` for every agent a row names.
- `claude/hooks/pii_columns.txt` is an example subset. Generate yours from your own schema: `column<TAB>family`, one per line.
- `RTK.md` matters only if you use the rtk command proxy; add `@RTK.md` to `CLAUDE.md` then.

## Uninstall

Restore the backup from step 2 and remove `~/.devwork` and `~/.local/bin/brain`.
