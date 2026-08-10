// Package promoter is the WATCHDOG's growth half. When an inline query is blocked
// with no library match, the PROMOTER captures the SQL, parameterizes its literals
// (psql :name convention, matching pilot-a's q.sh), and stages a reusable .sql so the
// library grows from 9 toward complete instead of the same query being re-typed.
package promoter

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	reInlineFlag = regexp.MustCompile(`(?s)(?:-e|-c|--execute|--command)\s+("([^"]*)"|'([^']*)')`)
	reHeredoc    = regexp.MustCompile(`<<-?\s*['"]?(\w+)`) // RE2 has no backrefs; delimiter matched in code
	rePipeEcho   = regexp.MustCompile(`(?s)echo\s+("([^"]*)"|'([^']*)')\s*\|`)
	reSQLStart   = regexp.MustCompile(`(?is)\b(select|insert|update|delete|with)\b`)
	// column = literal  ->  column = :column
	reColLit = regexp.MustCompile(`(?i)([a-z_][a-z0-9_.]*)\s*=\s*('[^']*'|"[^"]*"|-?\d+(?:\.\d+)?)`)
	reTable  = regexp.MustCompile(`(?is)\bfrom\s+([a-z_][a-z0-9_]*)`)
)

// Extract pulls the SQL out of an inline DB command.
func Extract(command string) (string, bool) {
	if sql, ok := extractHeredoc(command); ok {
		return sql, true
	}
	if m := reInlineFlag.FindStringSubmatch(command); m != nil {
		return strings.TrimSpace(firstNonEmpty(m[2], m[3])), true
	}
	if m := rePipeEcho.FindStringSubmatch(command); m != nil {
		return strings.TrimSpace(firstNonEmpty(m[2], m[3])), true
	}
	if reSQLStart.MatchString(command) {
		return strings.TrimSpace(command), true
	}
	return "", false
}

// Parameterize replaces literal values with :name placeholders (named after the
// column), returning the parameterized SQL and the distinct param names in order.
func Parameterize(sql string) (string, []string) {
	seen := map[string]int{}
	var order []string
	out := reColLit.ReplaceAllStringFunc(sql, func(s string) string {
		m := reColLit.FindStringSubmatch(s)
		col := m[1]
		base := col
		if i := strings.LastIndex(base, "."); i >= 0 {
			base = base[i+1:] // table.col -> col
		}
		name := base
		if n := seen[base]; n > 0 {
			name = fmt.Sprintf("%s%d", base, n+1)
		}
		seen[base]++
		order = append(order, name)
		return fmt.Sprintf("%s = :%s", col, name)
	})
	return out, order
}

// SuggestName proposes a library filename from the table + params.
func SuggestName(sql string, params []string) string {
	table := "query"
	if m := reTable.FindStringSubmatch(sql); m != nil {
		table = strings.ToLower(m[1])
	}
	if len(params) == 0 {
		return table
	}
	uniq := dedupe(params)
	sort.Strings(uniq)
	return table + "_by_" + strings.Join(uniq, "_")
}

// Render produces the staged .sql file contents (header + run hint + SQL).
func Render(name, sql string, params []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "-- %s.sql — staged by PROMOTER; review before promoting to the library.\n", name)
	hint := "-- run: q.sh queries/library/" + name + ".sql"
	for _, p := range dedupe(params) {
		hint += fmt.Sprintf(" -v %s=<value>", p)
	}
	b.WriteString(hint + "\n")
	b.WriteString(strings.TrimRight(sql, " \n;") + ";\n")
	return b.String()
}

// extractHeredoc finds `<<DELIM ... DELIM` and returns the body (RE2-safe).
func extractHeredoc(command string) (string, bool) {
	m := reHeredoc.FindStringSubmatchIndex(command)
	if m == nil {
		return "", false
	}
	delim := command[m[2]:m[3]]
	rest := command[m[1]:]
	nl := strings.IndexByte(rest, '\n')
	if nl < 0 {
		return "", false
	}
	var body []string
	for _, ln := range strings.Split(rest[nl+1:], "\n") {
		if strings.TrimSpace(ln) == delim {
			break
		}
		body = append(body, ln)
	}
	return strings.TrimSpace(strings.Join(body, "\n")), len(body) > 0
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func dedupe(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
