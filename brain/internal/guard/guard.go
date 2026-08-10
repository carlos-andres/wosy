// Package guard is the WATCHDOG classifier (R10): the agnostic policeman. It
// decides whether a proposed operation may proceed raw, or must be ROUTED to its
// sanctioned tool. Three guarded classes only:
//
//	inline DB query  -> the query library / q.sh   (+ PROMOTER for misses)
//	inline ssh/infra -> the documented connection
//	KB-file write    -> the SCRIBE (brain set/append)
//
// Agnostic boundary (critical): SOURCE CODE, APP CONFIG, and TESTS flow free.
// This is not a general edit-blocker.
//
// Fail-open + provision-then-enable: no .devwork/wosy.flags (or wosy_enforce=0)
// => allow everything (prior behavior). A class is only BLOCKED when its flag is
// on AND its sanctioned tool already exists locally — so it can never self-lockout.
package guard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Action string

const (
	Allow Action = "allow"
	Block Action = "block"
)

type Decision struct {
	Action  Action `json:"action"`
	Class   string `json:"class,omitempty"`
	Reason  string `json:"reason"`
	Route   string `json:"route,omitempty"`
	FixHint string `json:"fix_hint,omitempty"`
}

type Input struct {
	Tool    string
	Command string
	Path    string
	Cwd     string
}

func allow(reason string) Decision { return Decision{Action: Allow, Reason: reason} }

// block returns a Decision with the structured route AND a per-class fix_hint
// the watchdog shim relays verbatim. The fix_hint lives here (not in the shim
// case statement) so the contract is single-source-of-truth (2026-05-28 D-1
// closed). Each call site passes the class-specific hint as the 4th arg.
func block(class, reason, route, fixHint string) Decision {
	return Decision{Action: Block, Class: class, Reason: reason, Route: route, FixHint: fixHint}
}

// brainHelpTail: SHADOW-ADOPTED remediation 2026-05-29. The brain --help feature
// shipped 2026-05-28 saw zero organic usage in 2 net-new sessions while the FIX
// hint drove 7/18 sessions → write-verb invocation. Promoting `brain --help`
// inside the FIX hint nudges agents toward the discovery surface they would
// otherwise miss. Single token cost; consistent across all 5 block classes.
const brainHelpTail = "  ·  `brain --help` for the full CLI surface"

// ReadFlag walks up from dir to the nearest .devwork/wosy.flags and returns the
// trimmed, comment-stripped value for key (matching wosy-flags.sh). Empty if absent.
func ReadFlag(key, dir string) string {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=(.*)$`)
	for dir != "" && dir != "/" {
		f := filepath.Join(dir, ".devwork", "wosy.flags")
		if b, err := os.ReadFile(f); err == nil {
			if m := re.FindSubmatch(b); m != nil {
				v := string(m[1])
				if i := strings.IndexByte(v, '#'); i >= 0 {
					v = v[:i]
				}
				return strings.TrimSpace(v)
			}
			return "" // flags file found but key absent
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

// existsUp reports whether any ancestor's .devwork/<rel> exists (file or non-empty dir).
func existsUp(dir, rel string, wantDir bool) bool {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	for dir != "" && dir != "/" {
		p := filepath.Join(dir, ".devwork", rel)
		if fi, err := os.Stat(p); err == nil {
			if !wantDir {
				return true
			}
			if fi.IsDir() {
				if es, _ := os.ReadDir(p); len(es) > 0 {
					return true
				}
			}
		}
		dir = filepath.Dir(dir)
	}
	return false
}

var (
	reSQLClient = regexp.MustCompile(`(?i)^(mysql|mariadb|psql|sqlite3)$`)
	reSQLInline = regexp.MustCompile(`(?i)(\s-e\b|\s-c\b|--execute|--command)`)
	reSQLPipe   = regexp.MustCompile(`(?i)\|\s*(mysql|mariadb|psql|sqlite3)\b`)
	reSQLVerb   = regexp.MustCompile(`(?i)["']?\s*\b(select|insert|update|delete|create|drop|alter|truncate)\b`)
	reHeredoc   = regexp.MustCompile(`<<-?\s*['"]?\w+`)
	// reSSHRemote: ssh user@host cmd OR ssh host cmd — first non-flag token after ssh
	// is the target; next non-flag token implies a remote command.
	reSSHRemote   = regexp.MustCompile(`(?i)(^|[\s|&;(])ssh\s+[^\s-]\S*\s+\S`)
	reSSHQuotedFn = regexp.MustCompile(`(?i)\bssh\b[^|;&]*("[^"]*"|'[^']*')`)
)

// extractBashC (R53): if cmd is `bash -c <inner>` (sh/zsh accepted, optional
// path prefix like /bin/bash), return the inner command. Empty string if no
// match. Uses the tokenizer (RE2 has no backreferences, can't pair quotes).
func extractBashC(cmd string) string {
	toks := tokenize(cmd)
	for i, t := range toks {
		base := filepath.Base(t)
		if base != "bash" && base != "sh" && base != "zsh" {
			continue
		}
		if i+2 < len(toks) && toks[i+1] == "-c" {
			return toks[i+2]
		}
	}
	return ""
}

// firstVerb returns the first whitespace-separated token of cmd (the command verb),
// stripped of leading env-prefixes like `FOO=bar`, with its basename only. Used by
// isInlineSQL/isSanctioned/isBashKBWrite to decide what command is being executed
// without false-positives on paths that *contain* the verb name as a substring
// (R55: `sqlite3 .devwork/brain/store.db ...` previously matched the old regex
// reSanctioned on the literal substring `/brain` in the path).
func firstVerb(cmd string) string {
	toks := tokenize(cmd)
	for _, t := range toks {
		if strings.Contains(t, "=") && !strings.HasPrefix(t, "-") {
			// env-prefix like PGUSER=foo — skip
			continue
		}
		return filepath.Base(t)
	}
	return ""
}

// isSanctioned: the COMMAND VERB is one of the sanctioned brain tools.
// Stricter than the old substring regex; verb is the first non-env token's basename.
func isSanctioned(cmd string) bool {
	v := firstVerb(cmd)
	return v == "q.sh" || v == "brain"
}

// isInlineSQL: per-clause scan so multi-statement Bash like `ls && psql -c X`
// is still caught (lost when reSQLClient was anchored ^…$ to fix the R55
// false-positive). reSQLPipe is checked on the WHOLE command — splitClauses
// strips the `|` separator so it can't be detected per-clause.
func isInlineSQL(cmd string) bool {
	// Whole-command pipe match — anchored on the `|` that splitClauses removes.
	if reSQLPipe.MatchString(cmd) {
		return true
	}
	for _, clause := range splitClauses(cmd) {
		if isSanctioned(clause) {
			continue // q.sh / brain are the sanctioned tools — never flagged
		}
		verb := firstVerb(clause)
		// R55: sqlite3 acting on a brain store is ALWAYS inline SQL, even
		// without an explicit verb (interactive shell, .dump, etc.).
		if verb == "sqlite3" {
			for _, t := range tokenize(clause)[1:] {
				if strings.HasSuffix(t, "store.db") || strings.Contains(t, "/.devwork/brain/") {
					return true
				}
			}
		}
		// Scan all tokens for a SQL-client basename — catches eval-wrapped
		// (`eval psql -c X`) and command-substituted (`echo $(psql -c X)`)
		// forms where the client isn't the first verb. Token cleanup strips
		// `$(` / `)` / backticks so the substituted form's `psql` still matches.
		hasSQLClient := false
		for _, tok := range tokenize(clause) {
			cleaned := strings.TrimFunc(tok, func(r rune) bool {
				return r == '$' || r == '(' || r == ')' || r == '`'
			})
			if reSQLClient.MatchString(filepath.Base(cleaned)) {
				hasSQLClient = true
				break
			}
		}
		if hasSQLClient {
			if reSQLInline.MatchString(clause) || reHeredoc.MatchString(clause) || reSQLVerb.MatchString(clause) {
				return true
			}
		}
	}
	return false
}

func isInlineSSH(cmd string) bool {
	for _, clause := range splitClauses(cmd) {
		if isSanctioned(clause) {
			continue
		}
		if reSSHRemote.MatchString(clause) || reSSHQuotedFn.MatchString(clause) {
			return true
		}
	}
	return false
}

// kbBase: brain-OWNED operational data filenames. These are managed by the
// brain CLI (set/append/import) and should NOT be hand-edited. 2026-05-29 (B1
// policy): removed lowercase `status.md`, `context.md`, and `consolidated.md`
// from this set — they are now task-folder canonical filenames that flow free
// under .devwork/tasks/<id>/ (files-canonical policy per 3-repo empirical
// audit: 1/11 brain task adoption on pilot-a dogfood, 3/14 on Globex).
// The consolidated.md shipped-marker convention is deprecated (2% adoption
// across 82 pilot-a archived tasks). STATUS.md (uppercase) remains because it's
// the brain-home keyed-yaml status file, not a task-folder file.
// kbBase: SOFT-block basenames — block under .devwork/<subdir>/ unless the
// subdir is in kbExcludeSegments (so a draft state.yml in _scratch/ flows free).
// Per C-02 (2026-06-08) plan.yml + task.yml moved to kbBaseHard — they have NO
// legitimate draft form (canonical paths are 1:1 per plan/task).
var kbBase = map[string]bool{
	"STATUS.md":  true, // brain-home keyed status (master design)
	"ledger.yml": true, // brain transaction ledger
	"state.yml":  true, // brain state snapshot
}

// kbBaseHard: ALWAYS block under .devwork/ regardless of subdir. C-02 (2026-06-08):
// plan.yml + task.yml are single-canonical-path YAML — no draft variant exists
// (drafts go to _scratch/<anything>.md, not _scratch/plan.yml). The exclude-segment
// gate would otherwise let .devwork/<team>/projects/<slug>/plans/<plan-id>/tasks/<task-id>/task.yml
// flow free because /tasks/ is excluded for task-folder NOTES.
var kbBaseHard = map[string]bool{
	"plan.yml": true,
	"task.yml": true,
}

// kbBaseAllow: ALWAYS allow under .devwork/ regardless of subdir. C-02 (2026-06-08):
// handoff.md is plan-level vigilante-rendered markdown — written by /handoff skill
// directly OR by handoff-vigilante.sh hook. Under the widened kbExt rule it would
// otherwise be caught when nested under .devwork/<team>/projects/<slug>/plans/<plan-id>/.
var kbBaseAllow = map[string]bool{
	"handoff.md": true,
}

// 2026-05-29 (B1 policy): removed reKBTaskFile var. Task-folder files
// (.devwork/tasks/<id>/{status,context,scope}.md AND custom names like
// findings.md / verification.md / inventory.md) flow free under the
// files-canonical policy. Brain task records become opt-in for cross-ticket
// queries, not the canonical store. Evidence:
// audits/2026-05-29-classifier-portability-fixes.md §6 + 3-repo discovery.

// scribe-poc-b (2026-05-27): widened KB scope. ANY .md/.yml/.yaml/.jsonl under
// .devwork/ is KB EXCEPT the excluded operational subdirs (scratch/archive/
// research/data/logs/metrics/reports/flows/queries) AND .devwork/tasks/<id>/**
// (added 2026-05-29 B1). Catches the loose folders the master design names as
// records (decisions/, specs/, plans/, intake/) and any project-nested
// .devwork/<project-id>/**.{md,yml,yaml,jsonl}. Queries is excluded because the
// inline-SQL flow owns that path via q.sh.
var kbExt = map[string]bool{
	".md": true, ".yml": true, ".yaml": true, ".jsonl": true,
}

// kbExcludeSegments: subdir slugs that flow free under .devwork/. Matched with
// strings.Contains (not HasPrefix) since 2026-06-08 so /tasks/ matches at ANY
// depth — closes the C-02 path-shape case where tasks/<task-id>/notes.md sits
// under <team>/projects/<slug>/plans/<plan-id>/tasks/<task-id>/.
// /interview/ added 2026-06-08 to close the documented gap (Phase IB I-6):
// .devwork/interview/<id>.md is the C-01 storage path; agent-scribe spec lists
// it as direct-write but the classifier blocked until this commit.
var kbExcludeSegments = []string{
	"/_scratch/", "/_archive/", "/research/", "/data/",
	"/logs/", "/metrics/", "/reports/", "/flows/", "/queries/",
	"/tasks/",     // 2026-05-29 B1: task folders are files-canonical (user-edited)
	"/interview/", // 2026-06-08 Phase IB I-6: C-01 storage, agent-scribe direct-write
}

// srcExt: source/code/config/test extensions. The agnostic boundary (master §B1) is
// that these ALWAYS flow free — even when they live under a .devwork/brain/ tree, e.g.
// the brain's own Go source or a repo checked out beneath such a path (fix G9: the old
// unanchored substring match mis-classified all of them as KB).
var srcExt = map[string]bool{
	".go": true, ".php": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".py": true, ".rb": true, ".rs": true, ".java": true, ".kt": true, ".swift": true,
	".c": true, ".h": true, ".cpp": true, ".cc": true, ".cs": true, ".css": true,
	".scss": true, ".html": true, ".sh": true, ".sql": true, ".vue": true, ".mod": true,
}

// canonicalize (R56 + R59a): make path absolute (anchor relative paths on root),
// then resolve symlinks. Fail-OPEN if EvalSymlinks errors — keeps the system
// robust against broken/dangling links instead of returning false negatives by
// crashing classification.
//
// Empty path returns "" (sentinel for "no path to classify").
func canonicalize(path, root string) string {
	if path == "" {
		return ""
	}
	abs := resolvePath(path, root)
	// EvalSymlinks requires the target to EXIST. For Write/Edit on paths that
	// don't exist yet, eval the parent then re-append the basename.
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	if realDir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(realDir, filepath.Base(abs))
	}
	return abs // fail-OPEN: keep absolute even when nothing resolves
}

// inKBExcludedSegment reports whether path lies under one of the operational
// excluded subdirs (_scratch/, _archive/, research/, data/, logs/, metrics/,
// reports/, flows/, queries/, tasks/) of some .devwork/. Used by isKBPath so
// that even kbBase-basename matches (status.md, context.md, etc.) flow free
// when written under _scratch/ — the documented universal-write path. 2026-05-29
// added /tasks/ per B1 policy. Caller passes clean (trailing-slash-stripped) path.
func inKBExcludedSegment(clean string) bool {
	idx := strings.Index(clean, "/.devwork/")
	if idx < 0 {
		return false
	}
	rel := clean[idx+len("/.devwork"):] // leading "/"
	// Contains (not HasPrefix) since 2026-06-08 — see kbExcludeSegments comment.
	// The leading + trailing slashes in each segment ensure literal-subdir match
	// (no false positives on basenames like `research-notes.md`).
	for _, ex := range kbExcludeSegments {
		if strings.Contains(rel, ex) {
			return true
		}
	}
	return false
}

// isKBPath: only the brain's OWN operational data files. Source/config/tests flow free.
// Caller MUST pre-canonicalize (R56 + R59a fix is done by canonicalize() at call sites).
func isKBPath(path string) bool {
	if path == "" {
		return false
	}
	// G9: source/code/config never count as KB, regardless of where they live.
	if srcExt[strings.ToLower(filepath.Ext(path))] {
		return false
	}
	// EvalSymlinks normalizes away trailing slashes, so a Write to the brain
	// directory itself ($PROJ/.devwork/brain/) becomes $PROJ/.devwork/brain
	// and the substring check fails. Catch the directory case explicitly.
	clean := strings.TrimRight(path, "/")
	if strings.HasSuffix(clean, "/.devwork/brain") || strings.Contains(clean, "/.devwork/brain/") {
		return true
	}
	if strings.HasSuffix(clean, "store.db") {
		return true
	}
	// 2026-06-08 Phase IB I-6: per-team brain.db block — new substrate per C-03.
	if strings.HasSuffix(clean, "brain.db") {
		return true
	}
	// 2026-06-08 Phase IB I-6: explicit-allow basenames (handoff.md). Short-circuit
	// the widened-rule block when basename is plan-level vigilante-managed markdown.
	if kbBaseAllow[filepath.Base(clean)] && strings.Contains(clean, "/.devwork/") {
		return false
	}
	// 2026-06-08 Phase IB I-6: hard-block basenames (plan.yml / task.yml) — see
	// kbBaseHard comment. Never gated by exclude-segments because no draft variant
	// exists for these canonical YAML files.
	if kbBaseHard[filepath.Base(clean)] && strings.Contains(clean, "/.devwork/") {
		return true
	}
	// 2026-05-29 fix: kbBase basename match must honor kbExcludeSegments.
	// Previously status.md / context.md / consolidated.md etc. were blocked even
	// under .devwork/_scratch/ because the basename rule fired before any exclude
	// check. Verified bug: writing .devwork/_scratch/status.md returned block(kb_write)
	// even though _scratch/ is the documented universal-write path. The fix lets
	// drafts named after the canonical task files flow free under _scratch/.
	if kbBase[filepath.Base(clean)] && strings.Contains(clean, "/.devwork/") {
		if !inKBExcludedSegment(clean) {
			return true
		}
	}
	// 2026-05-29 (B1 policy): reKBTaskFile rule REMOVED. Task-folder files
	// (.devwork/tasks/<id>/**) now flow free per the files-canonical policy.
	// The /tasks/ entry in kbExcludeSegments handles the widened-rule branch
	// below so .devwork/tasks/<id>/<custom>.md (findings.md, verification.md,
	// etc.) also flow free. See audits/2026-05-29-intake-policy-execution.md.

	// scribe-poc-b widened rule: any kbExt under .devwork/ except excluded subdirs.
	// 2026-05-29 fix: ROOT-LEVEL .devwork/foo.md files (depth 1, no subfolder)
	// are NOT brain-managed records — they're human-edited operational docs like
	// connections.md, INDEX.md, BACKLOG.md, IMPLEMENTATION_PLAN.md. Only the
	// explicit kbBase entries (consolidated.md / STATUS.md / state.yml / etc.)
	// should trigger at root; everything else at root flows free. The widened
	// rule applies only to NESTED paths under .devwork/<subdir>/. Verified bug:
	// writing .devwork/connections.md returned block(kb_write) even though
	// connections.md is not a brain record — `brain set` errors on it and Write
	// is blocked, producing a deadlock observed in PROJ-1414 work.
	if kbExt[strings.ToLower(filepath.Ext(clean))] {
		if idx := strings.Index(clean, "/.devwork/"); idx >= 0 {
			rel := clean[idx+len("/.devwork"):] // leading "/"
			// Require depth ≥ 2 (i.e. at least one subfolder under .devwork/).
			// Root-level files fall back to the explicit kbBase check above.
			if strings.Count(rel, "/") < 2 {
				return false
			}
			// 2026-06-08: delegate to inKBExcludedSegment so the Contains-match
			// semantics (not HasPrefix) apply uniformly. This is what lets
			// /tasks/ AND /interview/ flow free at ANY depth under .devwork/,
			// e.g. <team>/projects/<slug>/plans/<plan-id>/tasks/<task-id>/notes.md.
			if inKBExcludedSegment(clean) {
				return false
			}
			return true
		}
	}
	return false
}

// isKBTarget: G2 destructive-op path test. Broader than isKBPath because a
// destructive op can target a DIRECTORY (no trailing slash, no extension) — e.g.
// `rm -rf .devwork/brain` or `mv .devwork/brain /tmp/`. Caller MUST pre-canonicalize.
// 2026-05-29 (B1): does NOT include task-folder files. Removing a task folder
// (rm -rf .devwork/tasks/<id>/) is the user's prerogative — task records are
// files-canonical, owned by the user, not the brain. The brain store itself
// (.devwork/brain/) remains protected.
func isKBTarget(path string) bool {
	if path == "" {
		return false
	}
	if srcExt[strings.ToLower(filepath.Ext(path))] {
		return false
	}
	clean := strings.TrimRight(path, "/")
	if strings.HasSuffix(clean, "/.devwork/brain") || strings.Contains(clean, "/.devwork/brain/") {
		return true
	}
	if strings.HasSuffix(clean, "/.devwork") {
		return true // deleting the whole .devwork tree
	}
	if strings.HasSuffix(clean, "store.db") {
		return true
	}
	if kbBase[filepath.Base(clean)] && strings.Contains(clean, "/.devwork/") {
		return true
	}
	return false
}

// resolvePath: makes path absolute. Relative paths anchor on root (cwd of the op).
// Does NOT call filepath.Abs (which would consult the process cwd, irrelevant here).
func resolvePath(path, root string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if root == "" {
		root, _ = os.Getwd()
	}
	return filepath.Clean(filepath.Join(root, path))
}

// tokenize: minimal shell-aware splitter. Handles single/double quotes; everything
// else is whitespace-separated. NOT a full shell parser — destructive ops with
// command substitution or eval are inherently un-classifiable and fall through.
func tokenize(cmd string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if quote != 0 {
			if c == quote {
				quote = 0
				continue
			}
			cur.WriteByte(c)
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case ' ', '\t', '\n':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// splitClauses: split a Bash one-liner on ; && || | & so each verb is classified
// independently. Quotes are honored. Returns non-empty trimmed clauses.
func splitClauses(cmd string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if quote != 0 {
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			cur.WriteByte(c)
		case ';', '|', '&':
			// also consumes a paired && or ||
			if i+1 < len(cmd) && (cmd[i+1] == '&' || cmd[i+1] == '|') {
				i++
			}
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

// isKBMutating (was isDestructiveKB; renamed R54): detect raw mutating ops
// against KB paths via the Bash channel.
//
// Verbs flagged: rm, rmdir, mv (source side or KB dest), cp/rsync (KB dest),
// find <PATH...> -delete.
//
// Operates per clause so `cd /tmp && rm -rf .devwork/brain` is still caught.
func isKBMutating(cmd, root string) bool {
	for _, clause := range splitClauses(cmd) {
		toks := tokenize(clause)
		if len(toks) < 2 {
			continue
		}
		// strip a leading `sudo` so `sudo rm -rf …` still classifies
		if toks[0] == "sudo" {
			toks = toks[1:]
			if len(toks) < 2 {
				continue
			}
		}
		verb := filepath.Base(toks[0])
		args := toks[1:]
		switch verb {
		case "rm", "rmdir":
			for _, a := range args {
				if strings.HasPrefix(a, "-") {
					continue
				}
				if isKBTarget(canonicalize(a, root)) {
					return true
				}
			}
		case "mv":
			var positional []string
			for _, a := range args {
				if strings.HasPrefix(a, "-") {
					continue
				}
				positional = append(positional, a)
			}
			for _, a := range positional {
				if isKBTarget(canonicalize(a, root)) {
					return true
				}
			}
		case "cp", "rsync": // R54 — rsync added; dest-side check
			var positional []string
			for _, a := range args {
				if strings.HasPrefix(a, "-") {
					continue
				}
				positional = append(positional, a)
			}
			if len(positional) >= 2 {
				dest := positional[len(positional)-1]
				if isKBTarget(canonicalize(dest, root)) {
					return true
				}
			}
		case "find":
			hasDelete := false
			var paths []string
			seenFlag := false
			for _, a := range args {
				if strings.HasPrefix(a, "-") {
					seenFlag = true
					if a == "-delete" {
						hasDelete = true
					}
					continue
				}
				if !seenFlag {
					paths = append(paths, a)
				}
			}
			if !hasDelete {
				continue
			}
			if len(paths) == 0 {
				paths = []string{"."}
			}
			for _, p := range paths {
				if isKBTarget(canonicalize(p, root)) {
					return true
				}
			}
		}
	}
	return false
}

// isBashKBWrite (R52): detect writes to KB paths through the Bash channel —
// redirect (`>`, `>>`) and `tee`. Independent of isKBMutating (which covers
// cp/mv/rsync/rm/find). One classifier per write channel.
//
// Tokenizer treats `>` and `>>` as standalone tokens when whitespace-separated.
// We also handle the glued form `>foo` / `>>foo` (no space) for robustness.
func isBashKBWrite(cmd, root string) bool {
	for _, clause := range splitClauses(cmd) {
		toks := tokenize(clause)
		// tee <flags?> <path>
		for i, t := range toks {
			if filepath.Base(t) == "tee" {
				for _, a := range toks[i+1:] {
					if strings.HasPrefix(a, "-") {
						continue
					}
					if isKBPath(canonicalize(a, root)) {
						return true
					}
				}
			}
		}
		// >, >>, >foo, >>foo
		for i, t := range toks {
			var dest string
			switch {
			case t == ">" || t == ">>":
				if i+1 < len(toks) {
					dest = toks[i+1]
				}
			case strings.HasPrefix(t, ">>") && len(t) > 2:
				dest = t[2:]
			case strings.HasPrefix(t, ">") && len(t) > 1:
				dest = t[1:]
			}
			if dest != "" && isKBPath(canonicalize(dest, root)) {
				return true
			}
		}
	}
	return false
}

// lookupRoot picks the anchor whose .devwork/ tree owns this operation. For
// write-class tools the file's parent directory is the right anchor — Claude Code
// invokes PreToolUse hooks with cwd = session-cwd (often $HOME), not dirname(path),
// so cwd-anchored walk-up misses flags that live alongside the target file. For
// Bash and others, the session cwd is the right anchor (no path to key off).
func lookupRoot(in Input) string {
	if in.Path != "" {
		switch in.Tool {
		case "Write", "Edit", "MultiEdit", "NotebookEdit":
			// R56: canonicalize so symlinks are followed BEFORE we walk up
			// looking for .devwork/wosy.flags — otherwise a symlink in /tmp
			// dodges pilot-a's flags even though the real path lands inside pilot-a.
			return filepath.Dir(canonicalize(in.Path, in.Cwd))
		}
	}
	return in.Cwd
}

// classifyBash holds the Bash branch of Classify so we can recurse into bash -c.
// Returns the decision plus a hint whether the input was a sanctioned tool we
// should NEVER block (q.sh / brain itself).
func classifyBash(cmd, root string, depth int) Decision {
	// R53 — bash -c <inner>: peel the wrapper and re-classify the inner.
	// Cap recursion at depth=2 to bound adversarial inputs.
	if depth < 2 {
		if inner := extractBashC(cmd); inner != "" {
			d := classifyBash(inner, root, depth+1)
			if d.Action == Block {
				return d // propagate the inner block
			}
			// Inner allowed — fall through to also classify the OUTER (could
			// contain its own SQL/SSH after the bash -c block).
		}
	}
	if isInlineSQL(cmd) {
		if ReadFlag("enforce_inline_sql", root) == "1" && existsUp(root, "queries/library", true) {
			return block("inline_sql",
				"inline DB query — route through the query library so it's reusable + grows (PROMOTER)",
				"q.sh queries/library/<name>.sql  (or `brain` query)",
				"Use `.devwork/bin/q.sh queries/library/<name>.sql` for the query. New query? `brain promote --kind=sql --command=<sql>` captures it into the library."+brainHelpTail)
		}
		return allow("inline DB query, but enforcement off or no query library yet — fail-open (nudge)")
	}
	if isInlineSSH(cmd) {
		if ReadFlag("enforce_inline_ssh", root) == "1" && existsUp(root, "connections.md", false) {
			return block("inline_ssh",
				"inline ssh — use the documented connection/alias",
				".devwork/connections.md",
				"Use the alias documented in `.devwork/connections.md` (e.g. `ssh <alias> '<cmd>'`). Raw `ssh host cmd` shapes are blocked; aliases pass through.")
		}
		return allow("inline ssh, but enforcement off or no connections doc — fail-open")
	}
	if isKBMutating(cmd, root) {
		if ReadFlag("enforce_destructive_kb", root) == "1" && existsUp(root, "brain/store.db", false) {
			return block("destructive_kb",
				"raw mutation on KB store — route to SCRIBE (see CLAUDE.md »SCRIBE routing«)",
				"snapshot via brain CLI; never rm/mv/cp/rsync on .devwork/brain/",
				"Use brain CLI to snapshot; task folders (.devwork/tasks/<id>/) are user-owned — not protected."+brainHelpTail)
		}
		return allow("destructive op on KB, but enforcement off or no brain store yet — fail-open")
	}
	if isBashKBWrite(cmd, root) {
		if ReadFlag("enforce_bash_kb_writes", root) == "1" && existsUp(root, "brain/store.db", false) {
			return block("bash_kb_write",
				"Bash redirect to KB file — route to SCRIBE (see CLAUDE.md »SCRIBE routing«)",
				"Task(subagent_type=agent-scribe)",
				"Drafts→_scratch/ · Tasks→tasks/<id>/ · KB→brain set/append, brain import"+brainHelpTail)
		}
		return allow("Bash write to KB, but enforcement off or no brain store yet — fail-open")
	}
	return allow("source/ops command — flows free")
}

// Classify is the WATCHDOG decision.
func Classify(in Input) Decision {
	root := lookupRoot(in)
	if ReadFlag("wosy_enforce", root) == "0" {
		return allow("wosy_enforce=0 (kill switch) — fail-open")
	}
	// 2026-05-29: removed the WOSY_SCRIBE_BYPASS env-var POC (scribe-poc-b).
	// Verified dead code — Claude Code subagent tool calls do not propagate env
	// vars set via `Bash export …` across to subsequent Write/Edit calls (each
	// tool call gets a fresh process env from CC, not from the previous tool's
	// subprocess). The env var was unreachable from inside a session and the
	// 2026-05-28 audit observed agents repeatedly citing it without success.
	// Removed; the contract is: brain CLI for KB-protected paths, Write to
	// _scratch/ for drafts. See claude/agents/agent-scribe.md.
	switch in.Tool {
	case "Bash", "":
		return classifyBash(in.Command, root, 0)
	case "Write", "Edit", "MultiEdit", "NotebookEdit": // R59b — NotebookEdit added
		path := canonicalize(in.Path, root)
		if isKBPath(path) {
			if ReadFlag("enforce_kb_scribe", root) == "1" && existsUp(root, "brain/store.db", false) {
				return block("kb_write",
					"KB file — route to SCRIBE (see CLAUDE.md »SCRIBE routing«)",
					"Task(subagent_type=agent-scribe)",
					"Drafts→_scratch/ · Tasks→tasks/<id>/ · KB→brain set, brain import"+brainHelpTail)
			}
			return allow("KB file, but enforcement off or no brain store yet — fail-open")
		}
		return allow("source/config/test file — flows free")
	default:
		return allow("unguarded tool — flows free")
	}
}
