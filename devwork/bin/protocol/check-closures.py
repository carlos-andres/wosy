#!/usr/bin/env python3
"""Source-driven closure checker for a record document.

A form checker reads only the record and therefore cannot see a closure that was deleted rather
than struck: round 33 measured exit 0 on a record with one closure removed. This instrument
enumerates closures from the files that OWN them and asserts each one appears in the record,
struck and dated. The source set is derived from the record's own backticked references, so no
closure list is hardcoded here.

Usage: check-closures.py <record.md> [hub]
Exit 0 when every derived closure is present. Exit 1 naming each one that is not.
"""
import os, re, sys

STRUCK = re.compile(r"~~(.+?)~~")
CLOSED_WORD = re.compile(r"\b(Closed|CLOSED|Answered|Cerrada|CERRADA)\b")
DATE = re.compile(r"\d{4}-\d{2}-\d{2}")
PROBLEM_HEAD = re.compile(r"^#{2,4}\s*(P\d+)\s*·")
MD_REF = re.compile(r"`([A-Za-z0-9_./-]+\.md)`")


def outside_code(line):
    """Offsets of `line` that sit inside a backticked span.

    A struck span quoted inside backticks is evidence, not a closure this document owns. The donor's
    heading is quoted that way in the record spec and it was the instrument's only false positive.
    """
    masked, inside, start = [], False, 0
    for i, ch in enumerate(line):
        if ch == "`":
            if inside:
                masked.append((start, i))
            else:
                start = i
            inside = not inside
    return masked


def struck_spans(line):
    """Struck spans that are not enclosed by a backticked span."""
    masked = outside_code(line)
    return [m.group(1) for m in STRUCK.finditer(line)
            if not any(a < m.start() < b for a, b in masked)]


def normalize(text):
    text = re.sub(r"[`*_]", "", text)
    return re.sub(r"\s+", " ", text).strip().lower()


def source_set(record_path, hub):
    """The record's own backticked .md references, resolved against the hub.

    Round-fixture directories are excluded: a bundle copy is evidence of a past run, not an
    authority over the live record.
    """
    text = open(record_path, encoding="utf-8").read()
    resolved, unresolved = [], []
    for ref in sorted(set(MD_REF.findall(text))):
        path = ref if os.path.isabs(ref) else os.path.join(hub, ref)
        if os.path.realpath(path) == os.path.realpath(record_path):
            continue
        if "/round3" in path or "/_scratch/round" in path:
            continue
        (resolved if os.path.isfile(path) else unresolved).append(path if os.path.isfile(path) else ref)
    return resolved, unresolved


def closures_from(path):
    """Every closure this file owns, as (key, description).

    Rule A — a problem heading whose next lines carry a bold CLOSED marker.
    Rule B — a struck span on a line that also carries a closing word and a date.
    """
    found = []
    lines = open(path, encoding="utf-8").read().splitlines()
    for i, line in enumerate(lines):
        head = PROBLEM_HEAD.match(line)
        if head:
            window = " ".join(lines[i + 1:i + 4])
            if re.search(r"\*\*CLOSED\b", window):
                found.append((head.group(1).lower(), "%s closed in %s" % (head.group(1), os.path.basename(path))))
        for span in struck_spans(line):
            if CLOSED_WORD.search(line) and DATE.search(line):
                found.append((normalize(span), "struck closure in %s:%d" % (os.path.basename(path), i + 1)))
    return found


def record_closures(record_path):
    """Struck spans in the record that carry a date on the same line."""
    keys = []
    for line in open(record_path, encoding="utf-8").read().splitlines():
        for span in struck_spans(line):
            if DATE.search(line):
                keys.append(normalize(span))
    return keys


def main():
    if len(sys.argv) < 2:
        sys.exit("usage: check-closures.py <record.md> [hub]")
    record = sys.argv[1]
    hub = sys.argv[2] if len(sys.argv) > 2 else "/Users/you/.devwork"
    if not os.path.isfile(record):
        sys.exit("FATAL: no record at %s" % record)

    resolved, unresolved = source_set(record, hub)
    present = record_closures(record)

    owed, seen = [], set()
    for path in resolved:
        for key, where in closures_from(path):
            if key in seen:
                continue
            seen.add(key)
            owed.append((key, where))

    missing = [(k, w) for k, w in owed if not any(k in p for p in present)]

    print("record:        %s" % record)
    print("source files:  %d resolved" % len(resolved))
    print("closures owed: %d" % len(owed))
    print("struck in rec: %d" % len(present))
    for ref in unresolved:
        print("WARN unresolvable reference: %s" % ref)
    if not owed:
        print("FAIL: no closure was derived from any source. A checker with nothing to assert is a baseline, not a test.")
        return 1
    for key, where in missing:
        print("MISSING: %-46s owed by %s" % (key[:46], where))
    if missing:
        print("CLOSURES BREACH: %d of %d missing from the record" % (len(missing), len(owed)))
        return 1
    print("CLOSURES OK: %d of %d present" % (len(owed), len(owed)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
