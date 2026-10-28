# Installing wosy 2.0.0

macOS or Linux. The files are copied, not linked; there is no installer script.

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

After the rewrite, paths under `$HOME/work/...` name an optional work area that `protocolo`, `encuadre` and `briefing-check` read; set them to your own or leave them unused.

## 4. Wire the hooks

Merge `hooks`, `permissions` and `statusLine` from `claude/settings.example.json` into `~/.claude/settings.json`. Keep your own `env`, `model` and plugins.

## 5. Build brain

```bash
cd brain && go build -o "$HOME/.local/bin/brain" ./cmd/brain
```

## 6. Verify

```bash
python3 ~/.claude/hooks/secret_guard.py self-test     # passed N   failed 0
bash -n ~/.claude/hooks/*.sh && echo hooks-parse-ok
brain version
```

In a new Claude Code session, run a harmless `git push --dry-run`: `validate-bash.sh` must refuse it.

## Customize

- `claude/hooks/pii_columns.txt` is an example subset. Generate yours from your own schema: `column<TAB>family`, one per line.
- `RTK.md` matters only if you use the rtk command proxy; add `@RTK.md` to `CLAUDE.md` then.

## Uninstall

Restore the backup from step 2 and remove `~/.devwork` and `~/.local/bin/brain`.
