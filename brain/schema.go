package brain

import (
	"sort"
	"strings"
)

// SchemaTypes returns the sorted list of public record types whose JSON
// schemas ship embedded in the binary. Excludes "_base" (internal $ref base)
// and "bundle" (the `brain load` output contract, not a record type).
// Used by the `brain schema` CLI verb (1.3.3) to answer "what types exist?"
// at the surface agents reach for.
func SchemaTypes() []string {
	entries, err := schemaFS.ReadDir("schema")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".schema.json") {
			continue
		}
		base := strings.TrimSuffix(n, ".schema.json")
		if base == "_base" || base == "bundle" {
			continue
		}
		out = append(out, base)
	}
	sort.Strings(out)
	return out
}

// SchemaJSON returns the raw embedded bytes for <name>.schema.json. Accepts
// the 7 public types AND "_base" (advanced use; not hidden). Unknown name
// returns the embed.FS error verbatim — callers can match on fs.ErrNotExist
// or just surface the message.
func SchemaJSON(name string) ([]byte, error) {
	return schemaFS.ReadFile("schema/" + name + ".schema.json")
}
