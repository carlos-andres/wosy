package cli

import "regexp"

// secretClass is srs F1 for the store: a planted credential in a candidate
// write is refused and the refusal names its class, never the value. It runs
// before every INSERT that carries free text (note, promote). Four classes,
// each a shape that never belongs in a knowledge row: a route to where the
// secret lives is what belongs there (connection.credential_ref is the model).
var secretPatterns = []struct {
	class string
	re    *regexp.Regexp
}{
	{"url_credentials", regexp.MustCompile(`://[^/\s:@]+:[^@\s/]+@`)},
	{"aws_access_key", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"private_key_block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY`)},
	{"known_token_format", regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{30,}|xox[baprs]-[A-Za-z0-9-]{20,}|sk-[A-Za-z0-9]{32,})\b`)},
}

// credential_assignment needs two conditions so that prose like "password: <value>"
// passes: a secret-looking key, and a value of 8+ chars that is not a plain word.
var (
	assignRe   = regexp.MustCompile(`(?i)\b(pass(word|wd)?|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|credential)\b\s*[:=]\s*['"]?([^\s'"]{8,})`)
	notAWordRe = regexp.MustCompile(`[0-9]|[^A-Za-z]`)
)

func secretClass(s string) string {
	for _, p := range secretPatterns {
		if p.re.MatchString(s) {
			return p.class
		}
	}
	for _, m := range assignRe.FindAllStringSubmatch(s, -1) {
		if notAWordRe.MatchString(m[len(m)-1]) {
			return "credential_assignment"
		}
	}
	return ""
}

// refuseSecret is the shared refusal: exit 1, class shown, value never echoed.
func refuseSecret(verb, class string) int {
	return fail("%s: refused — text carries a %s. The store keeps the ROUTE to a secret (connection.credential_ref), never the secret", verb, class)
}
