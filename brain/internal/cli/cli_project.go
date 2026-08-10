package cli

import (
	"fmt"
	"os"
	"time"
)

const projectUsage = "usage: brain project register --id=<slug> --team=<team> --root=<path> [--status=active|paused|archived] [--display-name=<name>]"

// usageFail prints a usage error and returns exit 2 (bad invocation), distinct
// from fail's exit 1 (runtime error) so callers can tell the two apart.
func usageFail(msg string, a ...any) int {
	fmt.Fprintf(os.Stderr, msg+"\n", a...)
	return 2
}

// cmdProject routes `brain project <subaction>`. Only `register` exists today.
func cmdProject(p args) int {
	if len(p.pos) == 0 {
		return usageFail(projectUsage)
	}
	switch p.pos[0] {
	case "register":
		return cmdProjectRegister(p)
	default:
		return usageFail("project: unknown subaction %q\n%s", p.pos[0], projectUsage)
	}
}

// cmdProjectRegister upserts the project identity row so satellite tables have
// a parent; re-registering an existing id is the idempotent refresh path.
func cmdProjectRegister(p args) int {
	id, team, root := p.flag["id"], p.flag["team"], p.flag["root"]
	if id == "" || team == "" || root == "" {
		return usageFail("project register: need --id, --team and --root\n%s", projectUsage)
	}
	status := p.flag["status"]
	if status == "" {
		status = "active"
	}
	switch status {
	case "active", "paused", "archived":
	default:
		return usageFail("project register: invalid --status %q (active|paused|archived)", status)
	}
	var displayName any
	if dn := p.flag["display-name"]; dn != "" {
		displayName = dn
	}

	s, err := open(p)
	if err != nil {
		return fail("project register: store: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	// On re-register: identity fields refresh, created/cache_version stay,
	// display_name only changes when the flag is passed.
	if _, err := s.DB.Exec(
		`INSERT INTO project(id, slug, team, display_name, root_path, created, updated, status)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   team         = excluded.team,
		   root_path    = excluded.root_path,
		   status       = excluded.status,
		   updated      = excluded.updated,
		   display_name = COALESCE(excluded.display_name, project.display_name)`,
		id, id, team, displayName, root, now, now, status,
	); err != nil {
		return fail("project register: %v", err)
	}
	fmt.Printf("project register OK: %s (team=%s, root=%s, status=%s)\n", id, team, root, status)
	return 0
}
