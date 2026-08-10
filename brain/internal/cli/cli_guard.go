package cli

import (
	"encoding/json"
	"fmt"

	"wosy.local/brain/internal/guard"
)

// guard is the WATCHDOG decision (R10). Test mode: flags --tool/--command/--path
// [--cwd] -> prints the decision JSON; exit 0 allow, exit 2 block (PreToolUse
// convention). Source/config/tests flow free; only inline DB/ssh + KB writes are
// guarded; fail-open with no flags.
func cmdGuard(p args) int {
	d := guard.Classify(guard.Input{
		Tool:    p.tool,
		Command: p.command,
		Path:    p.path,
		Cwd:     p.cwd,
	})
	out, _ := json.Marshal(d)
	fmt.Println(string(out))
	if d.Action == guard.Block {
		return 2
	}
	return 0
}
