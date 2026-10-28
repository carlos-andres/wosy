#!/usr/bin/env python3
"""Does L1 name every artifact it operates on, or does it point with a bare noun?

Every other checker reads the paths a document WROTE. None of them can see an artifact the author
referred to and never wrote as a path. On 2026-09-16 a handover passed four checkers and said "run
two blind judges over the old candidate" with no path, and the receiving session could not start.

The rule this enforces: L1 is self-contained. A definite noun phrase whose head is an artifact word
carries an absolute path inside its own clause.
"""
import re
import sys

# Artifact words this project actually uses for things that live on disk. A noun outside this set is
# not inspected, so the check under-reports rather than inventing defects.
NOUNS = [
    "candidate", "handover", "document", "record", "prereg", "seal", "chain", "plan", "spec",
    "report", "register", "control", "fixture", "arm", "round", "bundle", "transcript", "log",
    "answer", "key", "runner", "checker", "instrument", "script", "ledger", "graph", "step",
    "artifact", "battery", "judge", "copy", "version", "draft", "file", "folder", "one",
]
ARTICLES = r"(?:the|The|el|El|la|La|los|Los|las|Las)"
NOUN_RE = re.compile(
    r"\b" + ARTICLES + r"\s+(?:[A-Za-z0-9-]+\s+){0,2}?(" + "|".join(NOUNS) + r")s?\b"
)
# A phrase that points inside this same document resolves without a path.
DEICTIC = re.compile(r"^\s*(above|below|arriba|abajo|anterior|siguiente)\b")


def l1_block(lines):
    """L1 and its continuation lines, up to L2 or a blank line."""
    out, started = [], False
    for ln in lines:
        if re.match(r"^\s*L1\.", ln):
            started, out = True, [ln]
            continue
        if started:
            if re.match(r"^\s*L[2-7]\.", ln) or not ln.strip():
                break
            out.append(ln)
    return out


def mask_spans(text):
    """Replace each backticked span with a marker that says whether it was an absolute path."""
    def repl(m):
        return " PATHSPAN " if m.group(1).startswith("/") else " CODESPAN "
    return re.sub(r"`([^`]+)`", repl, text)


def main():
    if len(sys.argv) < 2:
        print("usage: check-referents.py <document.md>")
        return 2
    try:
        lines = open(sys.argv[1], encoding="utf-8").read().splitlines()
    except OSError as exc:
        print(f"FATAL: {exc}")
        return 2

    block = l1_block(lines)
    if not block:
        print("L1   ABSENT    no line starts with 'L1.' - nothing to resolve")
        print("\nREFERENTS BROKEN")
        return 1

    text = mask_spans(" ".join(block))
    clauses = [c for c in re.split(r"[,.;:]", text) if c.strip()]

    inspected, unresolved = 0, []
    for clause in clauses:
        has_path = "PATHSPAN" in clause
        for m in NOUN_RE.finditer(clause):
            phrase = m.group(0).strip()
            if DEICTIC.match(clause[m.end():]):
                continue
            inspected += 1
            if not has_path:
                unresolved.append((phrase, clause.strip()))

    for phrase, clause in unresolved:
        shown = clause if len(clause) <= 62 else clause[:59] + "..."
        print(f"L1   UNRESOLVED  {phrase:<28} in: {shown}")

    print(f"\nL1 clauses {len(clauses)} - artifact phrases {inspected} - unresolved {len(unresolved)}")
    if unresolved:
        print("REFERENTS BROKEN")
        return 1
    print("REFERENTS OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
