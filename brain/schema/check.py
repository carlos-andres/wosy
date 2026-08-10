#!/usr/bin/env python3
"""P0 verification harness — stdlib-only JSON-Schema checker for the brain contract.

Covers exactly the subset used by brain/schema/*.schema.json:
  $ref (local '#/...' and cross-file 'file.json#/...'), allOf, anyOf,
  type, enum, const, required, properties, additionalProperties (false),
  items, pattern, minLength, minItems.

Not a general validator — a transparent gate that proves the required-field
definition-of-done holds, with zero external deps (no node, no jsonschema lib).

Usage:
  check.py                      # validate every examples/*.json against its type schema, + lint all schemas
  check.py <instance.json>      # validate one instance (schema chosen by its "type" field)
"""
import json, re, sys
from pathlib import Path

SCHEMA_DIR = Path(__file__).resolve().parent
_DOC_CACHE: dict[str, dict] = {}


def load_doc(name: str) -> dict:
    if name not in _DOC_CACHE:
        _DOC_CACHE[name] = json.loads((SCHEMA_DIR / name).read_text())
    return _DOC_CACHE[name]


def pointer(doc: dict, ptr: str):
    node = doc
    for part in ptr.lstrip("#/").split("/"):
        if part == "":
            continue
        node = node[part]
    return node


JSON_TYPES = {
    "object": dict, "array": list, "string": str,
    "number": (int, float), "integer": int, "boolean": bool, "null": type(None),
}


def type_ok(value, t: str) -> bool:
    if t == "integer":
        return isinstance(value, int) and not isinstance(value, bool)
    if t == "number":
        return isinstance(value, (int, float)) and not isinstance(value, bool)
    if t == "boolean":
        return isinstance(value, bool)
    py = JSON_TYPES[t]
    return isinstance(value, py)


def validate(value, schema: dict, root: dict, path: str, errs: list):
    # $ref — resolve (cross-file or local) and recurse with the correct root document.
    if "$ref" in schema:
        ref = schema["$ref"]
        file_part, _, ptr = ref.partition("#")
        target_doc = load_doc(file_part) if file_part else root
        sub = pointer(target_doc, ptr)
        validate(value, sub, target_doc, path, errs)
        return

    if "allOf" in schema:
        for sub in schema["allOf"]:
            validate(value, sub, root, path, errs)
    if "anyOf" in schema:
        branch_errs = []
        for sub in schema["anyOf"]:
            e: list = []
            validate(value, sub, root, path, e)
            if not e:
                break
            branch_errs.append(e)
        else:
            errs.append(f"{path}: matches none of anyOf ({len(schema['anyOf'])} branches)")

    if "const" in schema and value != schema["const"]:
        errs.append(f"{path}: const expected {schema['const']!r}, got {value!r}")
    if "enum" in schema and value not in schema["enum"]:
        errs.append(f"{path}: {value!r} not in enum {schema['enum']}")

    if "type" in schema:
        types = schema["type"] if isinstance(schema["type"], list) else [schema["type"]]
        if not any(type_ok(value, t) for t in types):
            errs.append(f"{path}: type {types}, got {type(value).__name__} ({value!r:.40})")

    if isinstance(value, str):
        if "pattern" in schema and not re.search(schema["pattern"], value):
            errs.append(f"{path}: {value!r} fails pattern {schema['pattern']}")
        if "minLength" in schema and len(value) < schema["minLength"]:
            errs.append(f"{path}: shorter than minLength {schema['minLength']}")

    if isinstance(value, list):
        if "minItems" in schema and len(value) < schema["minItems"]:
            errs.append(f"{path}: fewer than minItems {schema['minItems']}")
        if "items" in schema:
            for i, item in enumerate(value):
                validate(item, schema["items"], root, f"{path}[{i}]", errs)

    if isinstance(value, dict):
        for req in schema.get("required", []):
            if req not in value:
                errs.append(f"{path}: MISSING required field '{req}'")
        props = schema.get("properties", {})
        for k, sub in props.items():
            if k in value:
                validate(value[k], sub, root, f"{path}.{k}", errs)
        if schema.get("additionalProperties") is False:
            for k in value:
                if k not in props:
                    errs.append(f"{path}: additional property '{k}' not allowed")


def lint_schema(name: str, errs: list):
    """Cheap structural lint: valid JSON, declares $schema + $id, refs resolve."""
    try:
        doc = load_doc(name)
    except json.JSONDecodeError as e:
        errs.append(f"{name}: invalid JSON — {e}")
        return
    if "$schema" not in doc:
        errs.append(f"{name}: missing $schema")
    if "$id" not in doc:
        errs.append(f"{name}: missing $id")

    def walk(node):
        if isinstance(node, dict):
            if "$ref" in node:
                fp, _, ptr = node["$ref"].partition("#")
                try:
                    pointer(load_doc(fp) if fp else doc, ptr)
                except (KeyError, FileNotFoundError):
                    errs.append(f"{name}: unresolvable $ref {node['$ref']}")
            for v in node.values():
                walk(v)
        elif isinstance(node, list):
            for v in node:
                walk(v)
    walk(doc)


def schema_for(instance: dict) -> str:
    t = instance.get("type")
    if not t:
        raise SystemExit("instance has no 'type' field — cannot select schema")
    return f"{t}.schema.json"


def main(argv):
    failures = 0

    # 1) lint every schema file
    print("== schema lint ==")
    for sf in sorted(SCHEMA_DIR.glob("*.schema.json")):
        errs: list = []
        lint_schema(sf.name, errs)
        ok = not errs
        print(f"  {'PASS' if ok else 'FAIL'}  {sf.name}")
        for e in errs:
            print(f"        - {e}")
        failures += 0 if ok else 1

    # 2) validate instances
    print("== instance validation ==")
    if len(argv) > 1:
        targets = [Path(argv[1])]
    else:
        targets = sorted((SCHEMA_DIR / "examples").glob("*.json"))
    for tf in targets:
        inst = json.loads(tf.read_text())
        sname = schema_for(inst)
        root = load_doc(sname)
        errs = []
        validate(inst, root, root, inst.get("id", inst.get("type", "?")), errs)
        ok = not errs
        print(f"  {'PASS' if ok else 'FAIL'}  {tf.name}  -> {sname}")
        for e in errs:
            print(f"        - {e}")
        failures += 0 if ok else 1

    print(f"\n{'ALL GREEN' if failures == 0 else f'{failures} FAILURE(S)'}")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
