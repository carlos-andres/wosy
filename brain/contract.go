// Package brain (module root) carries the embedded contract: the canonical JSON
// schemas are the single source of truth. This validator is a Go port of
// schema/check.py's subset, consuming the SAME embedded schema files, so the
// write-time guard and the P0 harness can never drift. Enforces the completeness
// law (R1/R3) on every write.
package brain

import (
	"embed"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

//go:embed schema/*.schema.json
var schemaFS embed.FS

var docCache = map[string]map[string]any{}

func loadDoc(name string) (map[string]any, error) {
	if d, ok := docCache[name]; ok {
		return d, nil
	}
	b, err := schemaFS.ReadFile("schema/" + name)
	if err != nil {
		return nil, err
	}
	var d map[string]any
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	docCache[name] = d
	return d, nil
}

func pointer(doc map[string]any, ptr string) (any, error) {
	var node any = doc
	for _, part := range strings.Split(strings.TrimPrefix(ptr, "#"), "/") {
		if part == "" {
			continue
		}
		m, ok := node.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("pointer %q: not an object at %q", ptr, part)
		}
		node, ok = m[part]
		if !ok {
			return nil, fmt.Errorf("pointer %q: missing %q", ptr, part)
		}
	}
	return node, nil
}

func typeOK(v any, t string) bool {
	switch t {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "null":
		return v == nil
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == float64(int64(f))
	}
	return false
}

// sectionSchema returns the type-schema's properties map by resolving the
// per-type schema (which is allOf [_base.record, {type:object, properties:...}])
// and merging in _base.record's properties. Used by the SCRIBE write-gate to
// know which --section values are legal for a given type (G3) and which
// sections are id-keyed collections (G4).
func sectionSchema(recType string) (map[string]any, error) {
	root, err := loadDoc(recType + ".schema.json")
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	// Per-type properties (the type schema is an object with properties).
	if props, ok := root["properties"].(map[string]any); ok {
		for k, v := range props {
			out[k] = v
		}
	}
	// _base.record properties (id/type/status/updated/title/edges/...).
	if base, err := loadDoc("_base.schema.json"); err == nil {
		if defs, ok := base["$defs"].(map[string]any); ok {
			if rec, ok := defs["record"].(map[string]any); ok {
				if props, ok := rec["properties"].(map[string]any); ok {
					for k, v := range props {
						if _, present := out[k]; !present {
							out[k] = v
						}
					}
				}
			}
		}
	}
	return out, nil
}

// SectionAllowed reports whether `section` is a declared property of the
// per-type schema for `recType` (G3 write-gate). Returns (ok, sortedAllowed, err).
// When ok==false, the caller should reject the write BEFORE touching the store.
func SectionAllowed(recType, section string) (bool, []string, error) {
	props, err := sectionSchema(recType)
	if err != nil {
		return false, nil, err
	}
	allowed := make([]string, 0, len(props))
	for k := range props {
		allowed = append(allowed, k)
	}
	sort.Strings(allowed)
	_, ok := props[section]
	return ok, allowed, nil
}

// IsIDKeyed reports whether the given section on `recType` is an array of
// objects keyed by `id` (G4 duplicate-guard). Driven by the `x-id-keyed: true`
// annotation on the section's schema; absent annotation = false. Older code
// that doesn't know the annotation simply ignores it (additive only).
func IsIDKeyed(recType, section string) bool {
	props, err := sectionSchema(recType)
	if err != nil {
		return false
	}
	sub, ok := props[section].(map[string]any)
	if !ok {
		return false
	}
	v, ok := sub["x-id-keyed"].(bool)
	return ok && v
}

// Validate checks an unmarshalled record against its type schema. Returns all
// violations (nil = valid).
func Validate(rec map[string]any) []string {
	t, _ := rec["type"].(string)
	if t == "" {
		return []string{"record: missing/empty 'type' — cannot select schema"}
	}
	root, err := loadDoc(t + ".schema.json")
	if err != nil {
		return []string{fmt.Sprintf("no schema for type %q: %v", t, err)}
	}
	id, _ := rec["id"].(string)
	if id == "" {
		id = t
	}
	var errs []string
	validate(rec, root, root, id, &errs)
	return errs
}

func validate(value any, schema, root map[string]any, path string, errs *[]string) {
	if ref, ok := schema["$ref"].(string); ok {
		file, ptr, _ := strings.Cut(ref, "#")
		target := root
		if file != "" {
			d, err := loadDoc(file)
			if err != nil {
				*errs = append(*errs, fmt.Sprintf("%s: cannot load $ref %s", path, ref))
				return
			}
			target = d
		}
		sub, err := pointer(target, "#"+ptr)
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s: %v", path, err))
			return
		}
		validate(value, sub.(map[string]any), target, path, errs)
		return
	}

	if all, ok := schema["allOf"].([]any); ok {
		for _, sub := range all {
			validate(value, sub.(map[string]any), root, path, errs)
		}
	}
	if any_, ok := schema["anyOf"].([]any); ok {
		matched := false
		for _, sub := range any_ {
			var e []string
			validate(value, sub.(map[string]any), root, path, &e)
			if len(e) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			*errs = append(*errs, fmt.Sprintf("%s: matches none of anyOf (%d branches)", path, len(any_)))
		}
	}

	if c, ok := schema["const"]; ok && !reflect.DeepEqual(value, c) {
		*errs = append(*errs, fmt.Sprintf("%s: const expected %v, got %v", path, c, value))
	}
	if en, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range en {
			if reflect.DeepEqual(value, e) {
				found = true
				break
			}
		}
		if !found {
			*errs = append(*errs, fmt.Sprintf("%s: %v not in enum %v", path, value, en))
		}
	}

	if tv, ok := schema["type"]; ok {
		var types []string
		switch t := tv.(type) {
		case string:
			types = []string{t}
		case []any:
			for _, x := range t {
				types = append(types, x.(string))
			}
		}
		ok := false
		for _, t := range types {
			if typeOK(value, t) {
				ok = true
				break
			}
		}
		if !ok {
			*errs = append(*errs, fmt.Sprintf("%s: type %v, got %T", path, types, value))
		}
	}

	if s, ok := value.(string); ok {
		if p, ok := schema["pattern"].(string); ok {
			if m, _ := regexp.MatchString(p, s); !m {
				*errs = append(*errs, fmt.Sprintf("%s: %q fails pattern %s", path, s, p))
			}
		}
		if ml, ok := schema["minLength"].(float64); ok && float64(len(s)) < ml {
			*errs = append(*errs, fmt.Sprintf("%s: shorter than minLength %.0f", path, ml))
		}
	}

	if arr, ok := value.([]any); ok {
		if mi, ok := schema["minItems"].(float64); ok && float64(len(arr)) < mi {
			*errs = append(*errs, fmt.Sprintf("%s: fewer than minItems %.0f", path, mi))
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, it := range arr {
				validate(it, items, root, fmt.Sprintf("%s[%d]", path, i), errs)
			}
		}
	}

	if obj, ok := value.(map[string]any); ok {
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				if _, present := obj[r.(string)]; !present {
					*errs = append(*errs, fmt.Sprintf("%s: MISSING required field '%s'", path, r))
				}
			}
		}
		props, _ := schema["properties"].(map[string]any)
		for k, sub := range props {
			if v, present := obj[k]; present {
				validate(v, sub.(map[string]any), root, path+"."+k, errs)
			}
		}
		if ap, ok := schema["additionalProperties"]; ok {
			if b, isBool := ap.(bool); isBool && !b {
				for k := range obj {
					if _, declared := props[k]; !declared {
						*errs = append(*errs, fmt.Sprintf("%s: additional property '%s' not allowed", path, k))
					}
				}
			}
		}
	}
}
