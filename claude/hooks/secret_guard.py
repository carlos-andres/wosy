#!/usr/bin/env python3
"""secret_guard.py - Claude Code credential guard (PreToolUse + UserPromptSubmit).

Layer 3 of the lockdown. Layers 1 (Seatbelt sandbox) and 2 (permissions.deny) hard-block
key material. This hook covers what they cannot: configuration files that hold credentials
must stay READABLE for work, so their values are REDACTED instead of blocked.

Mechanisms (verified on Claude Code 2.1.236, macOS, desktop app):
  * PostToolUse updatedToolOutput is ignored for Read and Bash -> everything happens in
    PreToolUse via updatedInput.
  * Read  : if the target file holds a credential, file_path is swapped for a redacted copy
            written under <cwd>/.claude/.secret-guard/ (readable under
            blockReadsOutsideWorkingDirectories).
  * Bash  : a command that would print a credential is wrapped so its output flows through
            `secret_guard.py --filter`; commands that dump the environment or expand a
            secret-named variable are denied (exit 2).
  * Grep  : content-mode searches are probed with rg; a search that would print a
            credential line is denied with the safe forms.
  * Edit/Write : denied when the new content carries a secret-looking literal.
  * UserPromptSubmit : @-mentions bypass hooks, so an @ of a credential-holding file is denied.

Decisions never print file contents or values. Any internal error -> exit 2 (fail closed).

CLI:
  secret_guard.py --filter            stdin -> stdout, values masked (used by the Bash wrap)
  secret_guard.py --redact FILE       print FILE with values masked
  secret_guard.py --check FILE...     print which rules each file triggers, no content
  secret_guard.py self-test           run the built-in battery (synthetic fixtures only)

This file is exempt from its own redaction rules: secret-guard:allow-file

Python 3.9+, stdlib only. Exec form:
  {"type":"command","command":"/opt/homebrew/bin/python3 <home>/hooks/secret_guard.py"}
"""
from __future__ import annotations

import csv
import hashlib
import hmac
import io
import json
import os
import re
import subprocess
import sys
import tempfile
import time

VERSION = "1.0.0"
HOME = os.path.expanduser("~")
SELF = os.path.realpath(__file__)
# The interpreter the redaction wrapper runs under. It followed a hardcoded /usr/bin/python3,
# so a guard launched with Homebrew python still injected the system one, putting two
# interpreters in one pipeline. The home declares PYTHON in settings.json env; honour it,
# and fall back to the interpreter actually running this file.
PYTHON = os.environ.get("PYTHON") or sys.executable or "/usr/bin/python3"
CONFIG_DIR = os.path.join(HOME, ".config", "secret-guard")
LOG_PATH = os.path.join(CONFIG_DIR, "guard.log")
KNOWN_HASHES_PATH = os.path.join(CONFIG_DIR, "known.sha256")
REDACTED_SUBDIR = os.path.join(".claude", ".secret-guard")

MAX_SCAN_BYTES = 2 * 1024 * 1024
MAX_CANDIDATES = 60
PROBE_TIMEOUT = 2  # was 5, which equalled the dispatcher's default STAGE_TIMEOUT: one
                   # probe at its cap consumed the whole stage budget on its own.
PROBE_BYTES = 2 * 1024 * 1024
MAX_PROBES = 3

NOLOG = False  # set by self-test

# ----------------------------------------------------------------------------------------
# Redaction engine (ported from redact.pl; values only, keys/structure survive)
# ----------------------------------------------------------------------------------------

META_TAIL = (
    r"[A-Za-z0-9_]*(?:_|(?-i:(?=[A-Z])))"
    r"(?:name|path|url|uri|env|var|file|field|header|prefix|suffix|pattern|regex|placeholder"
    r"|label|length|len|size|min|max|count|ttl|interval|timeout|column|algo|algorithm"
    r"|scheme|type|format|encoding)s?(?![A-Za-z0-9_])"
)
SECRET_KEY = (
    r"[A-Za-z0-9_]*(?:PASSWORD|PASSWD|PASSPHRASE"
    r"|(?:(?<![A-Za-z0-9])|(?<=[_.\-]))PASS(?![A-Za-z_])"
    r"|(?-i:Pass)(?![A-Za-z_])"
    r"|PWD(?![A-Za-z])"
    r"|SECRET|TOKEN|API_?KEY|PRIVATE_KEY|ACCESS_KEY|CLIENT_SECRET|SALT)"
    r"(?!" + META_TAIL + r")[A-Za-z0-9_]*"
)
HOST_KEY = r"[A-Za-z0-9_]*HOST(?:NAME)?(?![A-Za-z0-9_])"
NOT_A_LITERAL = (
    r"(?!['\"]?\$|getenv|env\(|\$_ENV|\$_SERVER|null|true|false|''|\"\"|\[|\)|new\s"
    r"|['\"]?<|['\"]?\{|process\.env|import\.meta\.env|[A-Za-z_][\w.\-]*\s*[\(\[])"
)
VALUE_END = r"[^\s'\",;)\]]+"
VALUE_IS_OPAQUE = (
    r"(?![0-9]+(?:\.[0-9]+)?[\s'\",;)\]]*(?:$|[\r\n]))"
    r"(?![^\sA-Za-z0-9]+[\s'\",;)\]]*(?:$|[\r\n]))"
    r"(?=[^\s'\",;)\]]{6,})"
)
ASSIGN_OP = r"((?:\\?[\]'\"]){0,2}\s*(?:=>|=(?!>)|:)\s*)"  # quotes may arrive escaped (jq -R, JSON)

RULE_URL = re.compile(
    r"\b(ftp|ftps|sftp|https?|mysql|mysqli|pgsql|postgres|postgresql|redis|rediss|amqp|amqps"
    r"|mongodb(?:\+srv)?)://([^:/@\s\"'<>]+):([^@\s\"'<>]+)@([^\s/:\"'<>?#]+)", re.I)
RULE_DSN = re.compile(r"\b(password|passwd|pwd)=([^;\"'\s&]+)", re.I)
RULE_AWS = re.compile(r"\b(AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16}\b")
RULE_MYSQL = re.compile(r"(mysql\w*\b[^\n]*?\s-p)(\S+)", re.I)
RULE_PDO = re.compile(
    r"(\bnew\s+\\?PDO\s*\(\s*[^,)]+,\s*)(['\"][^'\"]*['\"])(\s*,\s*)(['\"][^'\"]*['\"])", re.I | re.S)
RULE_CONNECT = re.compile(
    r"(\b(?:new\s+\\?\w*PDO|\\?\w+\s*(?:::|->)\s*connect|mysqli?_connect|pg_connect)\s*\()"
    r"([^{}]{0,400}?)(?=\))", re.I | re.S)
RULE_DEFINE = re.compile(
    r"(\bdefine\s*\(\s*(['\"])" + SECRET_KEY + r"\2\s*,\s*)" + NOT_A_LITERAL
    + r"((?:\\?['\"])?)" + VALUE_IS_OPAQUE + r"(" + VALUE_END + r")", re.I)
RULE_ASSIGN = re.compile(
    r"\b(" + SECRET_KEY + r")" + ASSIGN_OP + NOT_A_LITERAL + r"((?:\\?['\"])?)" + VALUE_IS_OPAQUE
    + r"(" + VALUE_END + r")", re.I)
RULE_HOST = re.compile(
    r"\b(" + HOST_KEY + r")" + ASSIGN_OP + NOT_A_LITERAL + r"((?:\\?['\"])?)"
    r"(?!(?:localhost|127\.0\.0\.1|::1|0\.0\.0\.0)(?![^\s'\",;)\]]))"
    r"(?=[^\s'\",;)\]]{4,})(" + VALUE_END + r")", re.I)

# A configuration key sits at the start of a line or after a structural delimiter. An English word
# sits after another word. Only one key in SECRET_KEY is also a common English word, and before
# 2026-09-17 this rule read ordinary prose such as "from the same pass" followed by a colon as a
# password assignment. It denied four consecutive documentation writes, and the message named a
# rule that, run over the same text on its own, matched nothing.
#
# The narrowing is deliberately the smallest one that removes the false positive:
#   - it applies to the bare key "pass" ONLY. "passwd" and "pwd" are never English prose.
#   - it applies to the colon form ONLY. Any "=" or "=>" assignment is untouched.
#   - it applies ONLY when the key follows a letter and a space, which is prose shape.
# A key at the start of a line, after indentation, after a quote, a brace or a comma still redacts,
# so every real YAML, JSON and dotenv shape keeps its old behaviour.
PROSE_WEAK_KEY = re.compile(r"^pass$", re.I)


def assignment_is_prose(m):
    key, op = m.group(1), m.group(2)
    if "=" in op or ":" not in op:
        return False
    if not PROSE_WEAK_KEY.match(key):
        return False
    lead = m.string[: m.start(1)]
    stripped = lead.rstrip(" \t")
    if len(stripped) == len(lead):
        return False
    return bool(stripped) and stripped[-1].isalpha()


def assignment_repl(m):
    if assignment_is_prose(m):
        return m.group(0)
    return m.group(1) + m.group(2) + m.group(3) + "***"


PROVIDER_RULES = [
    ("github-token", re.compile(r"\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{22,})\b"), 0),
    ("slack-token", re.compile(r"\bxox[abprs]-[A-Za-z0-9-]{10,}\b"), 0),
    ("stripe-key", re.compile(r"\b(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{16,}\b"), 0),
    ("stripe-webhook", re.compile(r"\bwhsec_[A-Za-z0-9]{16,}\b"), 0),
    ("openai-key", re.compile(r"\bsk-(?:proj-|ant-)?[A-Za-z0-9_-]{20,}\b"), 0),
    ("sendgrid-key", re.compile(r"\bSG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}\b"), 0),
    ("google-api-key", re.compile(r"\bAIza[0-9A-Za-z_-]{35}\b"), 0),
    ("twilio-sid", re.compile(r"\b(?:AC|SK)[0-9a-fA-F]{32}\b"), 0),
    ("jwt", re.compile(r"\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b"), 0),
    ("laravel-app-key", re.compile(r"\bbase64:[A-Za-z0-9+/]{40,}={0,2}"), 0),
    ("auth-header", re.compile(
        r"\b(?:authorization|x-api-key|api-key)\s*[:=]\s*(?:bearer\s+|basic\s+)?([A-Za-z0-9._~+/=-]{16,})", re.I), 1),
]
HIGH_CONFIDENCE = {"private-key", "url-credentials", "aws-access-key", "jwt", "github-token",
                   "slack-token", "stripe-key", "stripe-webhook", "openai-key", "sendgrid-key",
                   "google-api-key", "laravel-app-key", "known-secret"}

PEM_BEGIN = re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----")
PEM_END = re.compile(r"-----END [A-Z ]*PRIVATE KEY-----")
TOKEN_RE = re.compile(r"[^\s\"'`,;()<>\[\]{}|]{6,}")
KEEP_MARK = "redact:allow"


def mask_positional_literals(args: str) -> str:
    """Mask every argument that is entirely a quoted literal, when at least two are."""
    parts, cur, quote, escaped, depth = [], "", "", False, 0
    for ch in args:
        if escaped:
            cur += ch
            escaped = False
            continue
        if ch == "\\":
            cur += ch
            escaped = True
            continue
        if quote:
            cur += ch
            if ch == quote:
                quote = ""
            continue
        if ch in "'\"":
            quote = ch
            cur += ch
            continue
        if ch in "([":
            depth += 1
        elif ch in ")]":
            depth -= 1
        if ch == "," and depth == 0:
            parts.append(cur)
            cur = ""
            continue
        cur += ch
    parts.append(cur)
    lit = re.compile(r"\A\s*(['\"])((?:[^'\"\\]|\\.)*)\1\s*\Z", re.S)
    maskable = [i for i, p in enumerate(parts)
                if (m := lit.match(p)) and not re.search(r"://|=", m.group(2))]
    if len(maskable) < 2:
        return args
    for i in maskable:
        parts[i] = re.sub(r"(['\"])(?:[^'\"\\]|\\.)*(['\"])", r"\1***\2", parts[i], count=1, flags=re.S)
    return ",".join(parts)


def load_known_hashes() -> set:
    try:
        with open(KNOWN_HASHES_PATH) as f:
            return {ln.strip() for ln in f if ln.strip()}
    except OSError:
        return set()


def redact_text(text: str, hashes=None):
    """Return (redacted_text, sorted rule ids). Structure and key names survive."""
    rules = set()
    lines = text.split("\n")
    keep = []
    in_pem = False
    for i, ln in enumerate(lines):
        if KEEP_MARK in ln:
            keep.append(ln)
            lines[i] = "\x00K%d\x00" % (len(keep) - 1)
            continue
        if in_pem or PEM_BEGIN.search(ln):
            rules.add("private-key")
            in_pem = not PEM_END.search(ln)
            lines[i] = "***PRIVATE KEY REDACTED***"
    text = "\n".join(lines)

    def apply(rid, rx, fn):
        nonlocal text
        n = [0]

        def repl(m):
            out = fn(m)
            # Count a redaction, not a match. A rule that inspected a match and deliberately left
            # it alone did not find a secret, and recording it as fired makes Write deny on it.
            if out != m.group(0):
                n[0] += 1
            return out
        text = rx.sub(repl, text)
        if n[0]:
            rules.add(rid)

    apply("url-credentials", RULE_URL, lambda m: m.group(1) + "://***:***@***")
    apply("dsn-password", RULE_DSN, lambda m: m.group(1) + "=***")
    apply("aws-access-key", RULE_AWS, lambda m: m.group(1) + "*" * 16)
    apply("mysql-cli-password", RULE_MYSQL, lambda m: m.group(1) + "***")
    apply("pdo-positional", RULE_PDO, lambda m: m.group(1) + "'***'" + m.group(3) + "'***'")

    def connect_repl(m):
        masked = mask_positional_literals(m.group(2))
        if masked == m.group(2):
            n_skip[0] += 1
        return m.group(1) + masked
    n_skip = [0]
    before = text
    text = RULE_CONNECT.sub(connect_repl, text)
    if text != before:
        rules.add("connect-positional")

    apply("define-literal", RULE_DEFINE, lambda m: m.group(1) + m.group(3) + "***")
    apply("assignment", RULE_ASSIGN, assignment_repl)
    apply("host", RULE_HOST, lambda m: m.group(1) + m.group(2) + m.group(3) + "***")
    for rid, rx, grp in PROVIDER_RULES:
        if grp == 0:
            apply(rid, rx, lambda m: "***")
        else:
            apply(rid, rx, lambda m, g=grp: m.group(0)[: m.start(g) - m.start(0)] + "***" + m.group(0)[m.end(g) - m.start(0):])

    if hashes is None:
        hashes = load_known_hashes()
    if hashes:
        for tok in set(TOKEN_RE.findall(text)):
            cand = tok.rstrip(".:")
            if hashlib.sha256(cand.encode()).hexdigest() in hashes:
                text = text.replace(cand, "***")
                rules.add("known-secret")

    text = re.sub(r"\x00K(\d+)\x00", lambda m: keep[int(m.group(1))], text)
    return text, sorted(rules)


# ----------------------------------------------------------------------------------------
# PII layer (2026-10-26). Spec: /Users/you/work/umbrella/.devwork/tasks/
# pii-redaction-map/spec.md. It runs only under `--filter --pii` (mysqlp, q.sh) and on reads of
# tabular files (.out .tsv .csv). The credential rules above run everywhere and stay unchanged.
# Two layers: a header cell named in pii_columns.txt masks its column; validated value rules
# catch what a column alias or a free-text cell hides. A masked value becomes a typed HMAC
# token, so equal inputs give equal tokens and joins and duplicate checks still work.
# ----------------------------------------------------------------------------------------

PII_COLUMNS_PATH = os.path.join(os.path.dirname(SELF), "pii_columns.txt")
PII_KEY_PATH = os.path.join(CONFIG_DIR, "pii.key")
PII_TABULAR = re.compile(r"\.(?:out|tsv|csv)$", re.I)
PII_EMAIL = re.compile(r"(?<![\w.+%/-])[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}(?![\w-])")
# Not an address: a git scp remote (git@host:path) or an asset name (logo@2x.png). Review 2026-10-26.
PII_EMAIL_NOT = re.compile(r"^git@|\.(?:png|jpe?g|gif|webp|svg|ico|css|js)$", re.I)
# SSA rules: no area 000, 666 or 9xx, no group 00, no serial 0000. Dashed only: a bare 9-digit
# number is a merchant id here, so only the column layer masks undashed SSNs.
PII_SSN = re.compile(r"(?<![\d-])(?!000|666|9\d\d)\d{3}-(?!00)\d{2}-(?!0000)\d{4}(?![\d-])")
# NANP with a separator or parentheses, or E.164 with its "+1" (Twilio payloads, phase 3 of
# 2026-10-26: 752 spans in website_lead.metadata). A bare 10-digit run is left to the column layer.
PII_PHONE = re.compile(r"(?<![\d-])(?:(?:\+?1[ .-]?)?(?:\([2-9]\d{2}\) ?[2-9]\d{2}[ .-]?|[2-9]\d{2}[ .-][2-9]\d{2}[ .-])\d{4}"
                       r"|\+1[2-9]\d{2}[2-9]\d{6})(?![\d-])")
# Grouped digits only: about 1 in 10 bare 16-digit ids passes Luhn (marketplace listing ids are
# that long), so a bare run is left to the column layer, like bare SSNs and phones.
PII_CARD = re.compile(r"(?<![\d-])(?:[2-6]\d{3}(?:[ -]\d{4}){3}|3[47]\d{2}[ -]\d{6}[ -]\d{5})(?![\d-])")
PII_IPV4 = re.compile(r"(?<![\d.])(?<![vV])(?<!version )(?<!Version )(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)(?![\d.])")
PII_PRIVATE_IP = re.compile(r"^(?:10\.|127\.|0\.|192\.168\.|172\.(?:1[6-9]|2\d|3[01])\.|169\.254\.)")
PII_HEADER_CELL = re.compile(r"^[A-Za-z_][A-Za-z0-9_ #()./-]*$")
PII_VERTICAL = re.compile(r"^(\s*)([A-Za-z_][A-Za-z0-9_]*): (.+)$")  # mysql \G output
PII_NULLS = {"", "NULL", "\\N"}

_pii_key = None
_pii_cols = None


def pii_key() -> bytes:
    global _pii_key
    if _pii_key is None:
        os.makedirs(CONFIG_DIR, exist_ok=True)
        try:
            fd = os.open(PII_KEY_PATH, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, "wb") as f:
                f.write(os.urandom(32))
        except FileExistsError:
            pass  # created earlier, or by a concurrent process
        for _ in range(50):  # a concurrent creator may not have written its 32 bytes yet
            with open(PII_KEY_PATH, "rb") as f:
                key = f.read()
            if len(key) == 32:
                _pii_key = key
                break
            time.sleep(0.01)
        else:
            raise RuntimeError("pii.key is not 32 bytes")  # fail closed: output withheld
    return _pii_key


def pii_columns() -> dict:
    global _pii_cols
    if _pii_cols is None:
        _pii_cols = {}
        try:
            with open(PII_COLUMNS_PATH) as f:
                for ln in f:
                    if ln.startswith("#") or "\t" not in ln:
                        continue
                    col, fam = ln.rstrip("\n").split("\t", 1)
                    _pii_cols[col] = fam
        except FileNotFoundError:
            pass  # the value layer still runs
    return _pii_cols


def luhn_ok(digits: str) -> bool:
    total = 0
    for i, ch in enumerate(reversed(digits)):
        d = int(ch)
        if i % 2:
            d = d * 2 - 9 if d > 4 else d * 2
        total += d
    return total % 10 == 0


def pii_token(family: str, value: str) -> str:
    v = value.strip()
    if family == "card":
        digits = re.sub(r"\D", "", v)
        return "****" + digits[-4:] if len(digits) >= 4 else "****"
    if family == "birth" and re.match(r"^\d{4}-\d\d-\d\d", v):
        return v[:4] + "-**-**"
    if family == "phone":
        canon = re.sub(r"\D", "", v)[-10:]
    else:
        canon = v.lower()
    digest = hmac.new(pii_key(), f"{family}:{canon}".encode(), hashlib.sha256).hexdigest()[:8]
    return f"<{family}:{digest}>"


def pii_values(text: str, rules: set) -> str:
    def sub(rid, rx, fam, valid=None):
        nonlocal text

        def repl(m):
            if valid and not valid(m.group(0)):
                return m.group(0)
            rules.add(rid)
            return pii_token(fam, m.group(0))
        text = rx.sub(repl, text)
    sub("pii-email", PII_EMAIL, "email", lambda s: not PII_EMAIL_NOT.search(s))
    sub("pii-ssn", PII_SSN, "natid")
    sub("pii-card", PII_CARD, "card", lambda s: luhn_ok(re.sub(r"\D", "", s)))
    sub("pii-phone", PII_PHONE, "phone")
    sub("pii-ip", PII_IPV4, "ip", lambda s: not PII_PRIVATE_IP.match(s))
    return text


# People rule for Jira (acli) and Bitbucket output (2026-10-26, Calo's rule): a person's email
# is removed and a person's name is cut to its first word. An Atlassian accountId becomes a token,
# so two people with the same first name stay distinct. Shapes measured on acli output that day:
# view --json (displayName, emailAddress, accountId), comment list --json ("author": "First Last"),
# plain view ("Assignee: First Last") and the plain comment table (an Author column).
PEOPLE_NAME_KEY = re.compile(
    r'("(?:displayName|display_name|author|authorName|assignee|reporter|creator)"\s*:\s*")((?:[^"\\]|\\.)*)(")')
PEOPLE_HANDLE_KEY = re.compile(r'("(?:nickname|username)"\s*:\s*")([^"]+)(")')  # Bitbucket user handle
PEOPLE_EMAIL_KEY = re.compile(r'("(?:emailAddress|email_address|email)"\s*:\s*")[^"]*(")')
PEOPLE_NAME_EMAIL = re.compile(r"\b([^\W\d_][\w'’.-]*)(?:[ \t]+[\w'’.-]+){0,4}[ \t]*<[^<>@\s]+@[^<>\s]+>")
PEOPLE_LABEL = re.compile(r"(?m)^(\s*(?:Assignee|Reporter|Creator|Author|Updated by|Created by)\s*:\s*)(\S+)[^\n]*$")
PEOPLE_ACCOUNT = re.compile(r"\b(?:\d{6}:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9a-f]{24})\b")
# git blame "(Jane Doe 2026-10-01 ...", git shortlog "Jane Doe (12):" and "    12\tJane Doe".
PEOPLE_BLAME = re.compile(r"(\()([^\W\d_][\w'’.-]*)(?:[ \t]+[\w'’.-]+){1,4}(?=[ \t]+\d{4}-\d\d-\d\d )")
PEOPLE_SHORTLOG = re.compile(r"(?m)^()([^\W\d_][\w'’.-]*)(?:[ \t]+[\w'’.-]+){1,4}(?= \(\d+\):$)")
PEOPLE_SHORTLOG_N = re.compile(r"(?m)^(\s*\d+\t)([^\W\d_][\w'’.-]*)(?:[ \t]+[\w'’.-]+){1,4}$")
PEOPLE_TABLE_COLS = {"author", "assignee", "reporter", "creator"}
ANSI = re.compile(r"\x1b\[[0-9;]*m")


def people_redact(text: str):
    """Return (text, rules). Applied before the PII value layer, so 'Name <email>' goes whole."""
    rules = set()

    def sub(rid, rx, fn):
        nonlocal text
        new = rx.sub(fn, text)
        if new != text:
            rules.add(rid)
            text = new
    sub("people-email", PEOPLE_EMAIL_KEY, lambda m: m.group(1) + "<removed>" + m.group(2))
    sub("people-name", PEOPLE_NAME_KEY, lambda m: m.group(1) + (m.group(2).split() or [""])[0].replace('\\"', "") + m.group(3))
    sub("people-handle", PEOPLE_HANDLE_KEY, lambda m: m.group(1) + pii_token("user", m.group(2)) + m.group(3))
    sub("people-name", PEOPLE_NAME_EMAIL, lambda m: m.group(1))
    sub("people-name", PEOPLE_LABEL, lambda m: m.group(1) + m.group(2))
    sub("people-name", PEOPLE_BLAME, lambda m: m.group(1) + m.group(2))
    sub("people-name", PEOPLE_SHORTLOG, lambda m: m.group(1) + m.group(2))
    sub("people-name", PEOPLE_SHORTLOG_N, lambda m: m.group(1) + m.group(2))
    sub("people-account", PEOPLE_ACCOUNT, lambda m: pii_token("acct", m.group(0)))
    # Box tables (│ separated): cut the people columns to the first word, keeping the width.
    out, cols = [], None
    for ln in text.split("\n"):
        if "│" in ln:
            cells = ln.split("│")
            names = [ANSI.sub("", c).strip().lower() for c in cells]
            if PEOPLE_TABLE_COLS & set(names):
                cols = {i for i, n in enumerate(names) if n in PEOPLE_TABLE_COLS}
                ncells = len(cells)
            elif cols and len(cells) == ncells:
                for i in cols:
                    words = cells[i].split()
                    if len(words) > 1:
                        cells[i] = (" " + words[0]).ljust(len(cells[i]))
                        rules.add("people-name")
                ln = "│".join(cells)
        out.append(ln)
    return "\n".join(out), sorted(rules)


def pii_delim(path: str) -> str:
    # ponytail: a quoted CSV cell spanning lines is read line by line; its continuation is a stray row.
    return "," if path.lower().endswith(".csv") else "\t"


def pii_split(line: str, delim: str):
    if delim == "\t":
        return line.split("\t")
    return next(csv.reader([line]))


def pii_join(cells, delim: str) -> str:
    if delim == "\t":
        return "\t".join(cells)
    buf = io.StringIO()
    csv.writer(buf, lineterminator="").writerow(cells)
    return buf.getvalue()


def pii_redact(text: str, cols=None, delim=None):
    """Return (text, sorted rule ids). Column layer first, then the value layer.

    delim is the known separator of the whole text ("\t" for mysql -B, .out and .tsv; "," for
    .csv). Known, every line is a row, so a one-column result is still masked. Unknown (a wrapped
    command), each line guesses from its own tab or comma.
    """
    cols = pii_columns() if cols is None else cols
    rules = set()
    out = []
    active, cur = None, None
    for ln in text.split("\n"):
        d = delim or ("\t" if "\t" in ln else ("," if "," in ln else None))
        m = PII_VERTICAL.match(ln)  # mysql \G or -E output, also inside a known-delimiter stream
        if d and cols and ln and not m:
            try:
                cells = pii_split(ln, d)
            except csv.Error:
                cells = []
            ident = bool(cells) and all(PII_HEADER_CELL.match(c.strip() or "_") for c in cells)
            # A header is the first row, or a row of identifiers with a new width. A row of the
            # current width is data: before 2026-10-26 an all-identifier data row naming a listed
            # column ("view  username  varchar") became a header and masked what followed. A stray
            # line (a warning, prose with a comma) keeps the current header instead of replacing it.
            # ponytail: two result sets of equal width in one run share the first header's mapping.
            if active is None or (ident and (d != cur or len(cells) != len(active))):
                active, cur = [cols.get(c.strip().lower()) if ident else None for c in cells], d
                out.append(ln)
                continue
            if d == cur and len(cells) == len(active) and any(active):
                hit = False
                for i, fam in enumerate(active):
                    if fam and cells[i].strip() not in PII_NULLS:
                        cells[i] = pii_token(fam, cells[i])
                        hit = True
                if hit:
                    rules.add("pii-column")
                    ln = pii_join(cells, d)
                out.append(ln)
                continue
        if m and cols and m.group(2).lower() in cols and m.group(3).strip() not in PII_NULLS:
            rules.add("pii-column")
            ln = m.group(1) + m.group(2) + ": " + pii_token(cols[m.group(2).lower()], m.group(3))
        out.append(ln)
    return pii_values("\n".join(out), rules), sorted(rules)


# ----------------------------------------------------------------------------------------
# File tests
# ----------------------------------------------------------------------------------------

_file_cache: dict = {}


def file_rules(path: str):
    """Rules a file would trigger, or [] if clean / not testable. Never returns content."""
    if file_exempt(path):
        return []
    try:
        st = os.stat(path)
        key = (path, st.st_mtime_ns, st.st_size)
        if key in _file_cache:
            return _file_cache[key]
        if not os.path.isfile(path) or st.st_size == 0 or st.st_size > MAX_SCAN_BYTES:
            return []
        with open(path, "rb") as f:
            data = f.read(MAX_SCAN_BYTES)
        if b"\0" in data:
            return []
        text = data.decode("utf-8", errors="replace")
        _, rules = redact_text(text)
        if PII_TABULAR.search(path):
            rules = sorted(set(rules) | set(pii_redact(text, delim=pii_delim(path))[1]))
        _file_cache[key] = rules
        return rules
    except OSError:
        return []


KEY_MATERIAL_RE = re.compile(
    r"(?:(?:^|[\s/=:\"'`(])\.(?:ssh|aws|gnupg|kube)(?:/|$|[\"'\s;&|)])"
    r"|\.docker/config\.json|\.git-credentials|\.netrc\b|\.pgpass\b|\.mylogin\.cnf"
    r"|\.credentials\.json|\.claude\.json\b|\.config/gh(?:/|$|[\"'\s;&|)])|Library/Keychains"
    r"|\.(?:pem|p12|pfx|kdbx|jks|keystore)(?![A-Za-z0-9_.-])"
    r"|(?<![A-Za-z0-9_.])[A-Za-z0-9_-][A-Za-z0-9_.-]*\.key(?![A-Za-z0-9_.-])"  # a name, never a jq/yq .key accessor
    r"|(?:^|/)id_(?:rsa|ed25519|ecdsa|dsa)(?!\.pub)(?![A-Za-z0-9_-]))")


PRIVATE_KEY_MARK = re.compile(
    rb"PRIVATE KEY-----|BEGIN OPENSSH PRIVATE KEY|PuTTY-User-Key-File|BEGIN PGP PRIVATE")
PEM_EXT_RE = re.compile(r"\.(?:pem|key)$", re.I)
TOKEN_CHARS = set("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.~+-/")
FILE_ALLOW_MARK = "secret-guard:allow-file"


def token_at(s: str, idx: int) -> str:
    """The whole path-like token surrounding index idx."""
    i = idx
    while i > 0 and s[i - 1] in TOKEN_CHARS:
        i -= 1
    j = idx
    while j < len(s) and s[j] in TOKEN_CHARS:
        j += 1
    return s[i:j]


def pem_is_public(path: str) -> bool:
    """True when a .pem/.key file on disk carries no private key: a cert or CA bundle."""
    try:
        if not os.path.isfile(path) or os.path.getsize(path) > MAX_SCAN_BYTES:
            return False
        with open(path, "rb") as f:
            return not PRIVATE_KEY_MARK.search(f.read(MAX_SCAN_BYTES))
    except OSError:
        return False


def locate_secret(text: str):
    """Find the first line that trips a rule, and return (lineno, rule ids, redacted excerpt).

    Returns (0, [], "") when no single line reproduces the finding. A rule that only matches
    across a newline exists, so the caller must be able to say "not localised" rather than name
    the wrong line. Naming the wrong line is worse than naming none.

    The excerpt is the REDACTED form. The message exists to locate the literal, never to repeat it.
    """
    for i, ln in enumerate(text.split("\n"), 1):
        red, rules = redact_text(ln)
        if rules:
            red = red.strip()
            if len(red) > 96:
                red = red[:95] + "\u2026"
            return i, sorted(rules), red
    return 0, [], ""


def file_exempt(path: str) -> bool:
    """The guard's own source, and any file opting out with the whole-file marker.

    Without this the guard redacts its own patterns and fixtures, which makes it
    unmaintainable: a Read returns masked text and an Edit built on that text cannot match.

    THE MARKER IS NOT SYMMETRIC, and the asymmetry is deliberate. On a Write the marker is
    honoured from anywhere in the payload. Here, on the read path, only the first 4096 bytes are
    scanned, because reading a whole file is a cost paid on every Read, Grep and Glob call. So a
    marker below 4 KB in a long file lets the file be written and still denies reading it.
    Measured 2026-09-18 on an 8,147-byte file. The deny message now states this bound.
    """
    try:
        if os.path.realpath(path) == SELF:
            return True
        with open(path, "rb") as f:
            return FILE_ALLOW_MARK.encode() in f.read(4096)
    except OSError:
        return False


def is_key_material(s: str, cwd: str = "") -> bool:
    """A path naming private key material. A public certificate or CA bundle is not."""
    if not s:
        return False
    if "secret-guard/pii.key" in s or os.path.realpath(resolve(s, cwd or os.getcwd())) == os.path.realpath(PII_KEY_PATH):
        return True  # the HMAC key reverses tokens of small value spaces (phones, SSNs) by brute force
    for m in KEY_MATERIAL_RE.finditer(s):
        frag = m.group(0)
        if PEM_EXT_RE.search(frag):
            tok = token_at(s, m.start())
            if tok and pem_is_public(resolve(tok, cwd or os.getcwd())):
                continue  # a CA bundle such as /etc/ssl/cert.pem is public
        return True
    return False


def resolve(ref: str, cwd: str) -> str:
    ref = os.path.expanduser(ref)
    return ref if os.path.isabs(ref) else os.path.join(cwd, ref)


# ----------------------------------------------------------------------------------------
# Bash analysis
# ----------------------------------------------------------------------------------------

SEP_RE = re.compile(r"""'[^']*'|"(?:\\.|[^"])*"|\\.|\d?>&\d?|&>|<&\d?|&&|\|\||;|\n|\||&|.""", re.S)


def lift_substitutions(t: str) -> str:
    """Return t with $(...) and `...` replaced by SUBSTITUTION, bodies appended as segments."""
    bodies, out, i, n = [], "", 0, len(t)
    while i < n:
        c = t[i]
        if c == "'":
            j = t.find("'", i + 1)
            j = n - 1 if j < 0 else j
            out += t[i:j + 1]
            i = j + 1
        elif c == "\\":
            out += t[i:i + 2]
            i += 2
        elif c == "$" and t[i + 1:i + 2] == "(":
            depth, j, q = 1, i + 2, ""
            while j < n and depth > 0:
                d = t[j]
                if q:
                    if d == q:
                        q = ""
                elif d in "'\"":
                    q = d
                elif d == "(":
                    depth += 1
                elif d == ")":
                    depth -= 1
                j += 1
            bodies.append(t[i + 2:j - 1])
            out += " SUBSTITUTION "
            i = j
        elif c == "`":
            j = t.find("`", i + 1)
            j = n if j < 0 else j
            bodies.append(t[i + 1:j])
            out += " SUBSTITUTION "
            i = j + 1
        else:
            out += c
            i += 1
    for b in bodies:
        out += ";" + lift_substitutions(b)
    return out


def split_segments(cmd: str, pipes: bool = True):
    text = lift_substitutions(cmd)
    segs, cur = [], ""
    for m in SEP_RE.finditer(text):
        tok = m.group(0)
        if tok in (";", "&&", "||", "\n", "&") or (tok == "|" and pipes):
            segs.append(cur)
            cur = ""
        else:
            cur += tok
    segs.append(cur)
    return [s.strip() for s in segs if s.strip()]


WRAPPERS = {"sudo", "nohup", "time", "exec", "command", "builtin", "nice", "caffeinate", "stdbuf"}
ASSIGN_PREFIX = re.compile(r"^(?:[A-Za-z_][A-Za-z0-9_]*=\S*\s+)+")
# Keywords that introduce the real command: strip them so the command itself is judged.
# `do` and `then` must never be treated as safe verbs, or `do cat .env` would escape.
SHELL_PREFIX = {"if", "then", "else", "elif", "do", "while", "until", "!", "{", "(", "[["}
CASE_HEAD_RE = re.compile(r"^case\s+\S+\s+in\s*")
CASE_PAT_RE = re.compile(r"^[^\s;()|&]+\)\s*")


def head(seg: str):
    """(verb basename, args list) after stripping VAR=val prefixes and wrapper commands."""
    s = seg.lstrip(" \t({")
    s = CASE_HEAD_RE.sub("", s)
    s = CASE_PAT_RE.sub("", s)
    s = ASSIGN_PREFIX.sub("", s)
    words = s.split()
    while words:
        w = words[0]
        base = w.rsplit("/", 1)[-1]
        if base == "rtk":
            # rtk is a transparent proxy. The `rtk hook claude` PreToolUse hook runs first in
            # the Bash chain and rewrites every call to `rtk <cmd>` via updatedInput, so this
            # guard receives the rewritten form. Judge the wrapped command, not rtk itself.
            # `rtk read F` is rtk's spelling of `cat F`: rewrite it, or it falls through to the
            # SAFE_VERBS entry for the `read` shell builtin and the file test never runs.
            words = words[1:]
            while words and words[0].startswith("-"):
                words = words[1:]
            if words and words[0] == "proxy":
                words = words[1:]
            elif words and words[0] == "read":
                words = ["cat"] + words[1:]
            continue
        if base in WRAPPERS:
            words = words[1:]
            while words and words[0].startswith("-"):
                words = words[1:]
            continue
        if base in SHELL_PREFIX:
            words = ASSIGN_PREFIX.sub("", " ".join(words[1:])).split()
            continue
        if base == "env":
            rest = [x for x in words[1:] if not x.startswith("-") and "=" not in x]
            if rest:
                idx = words.index(rest[0])
                words = words[idx:]
                continue
        break
    if not words:
        return "", []
    return words[0].rsplit("/", 1)[-1], words[1:]


SAFE_VERBS = {
    "wc", "cmp", "stat", "ls", "eza", "fd", "find", "chmod", "chown", "cp", "mv", "mkdir", "touch",
    "ln", "rm", "rmdir", "shasum", "md5", "md5sum", "sha1sum", "sha256sum", "file", "dirname",
    "basename", "realpath", "readlink", "test", "[", "[[", "true", "false", "mktemp", "which", "type",
    "cd", "pwd", "date", "sleep", "tree", "du", "df", "ps", "kill", "pkill", "open", "code", "subl",
    "gzip", "gunzip", "tar", "zip", "unzip", "diff", "cloc", "dust", "jq",
    "for", "select", "done", "fi", "esac", "in", "break", "continue", "shift", "wait",
    "trap", "unset", "read", "return", "local", "printf", "echo"}
# jq is safe only when it does not name a credential-holding file; handled by the file test.
SAFE_VERBS.discard("jq")
KEY_OK_VERBS = {"ssh", "scp", "sftp", "rsync", "ssh-add", "ssh-keygen", "ssh-copy-id", "chmod",
                "chown", "ls", "eza", "stat", "test", "[", "rm", "mv", "cp", "git", "gh", "aws",
                "gcloud", "kubectl", "docker", "terraform", "file", "wc", "mkdir", "touch", "cd"}
INTERPRETERS = {"bash", "sh", "zsh", "php", "python", "python3", "perl", "ruby", "node", "deno", "bun"}
INLINE_FLAGS = {"-c", "-e", "-r", "-E", "--eval", "-p", "-pe", "-ne", "-lane", "-lne"}
SEARCH_VERBS = {"rg", "grep", "egrep", "fgrep", "ag", "ack"}
SEARCH_SAFE_FLAGS = {"-l", "-c", "-q", "--files-with-matches", "--count", "--quiet",
                     "--files-without-match", "--files", "-L"}
ENV_DUMP_INLINE = re.compile(
    r"os\.environ(?![\[.])|os\.environ\.(?:items|copy|values)\b|process\.env(?![.\[])"
    r"|\$_ENV(?!\[)|\$_SERVER(?!\[)|getenv\(\s*\)|ENV\.(?:to_h|each|map)|%ENV")
SECRET_VAR_RE = re.compile(r"\$\{?([A-Za-z_][A-Za-z0-9_]*)")
# A value under one of these names is a connection string: scheme://user:pass@host.
URLISH_VAR_NAME = re.compile(
    r"(?:^|_)(?:URL|URI|DSN|PROXY|ENDPOINT|CONN|CONNECTION|ADDR|ADDRESS|BROKER|SERVER)"
    r"(?:$|_)|(?:URL|URI|DSN|PROXY)$", re.I)
SECRET_VAR_NAME = re.compile(
    r"(?:PASSWORD|PASSWD|PASSPHRASE|SECRET|TOKEN|API_?KEY|PRIVATE_KEY|ACCESS_KEY|CLIENT_SECRET"
    r"|(?:^|_)PASS(?:$|_)|_PWD$|(?:^|_)HOST(?:NAME)?$)", re.I)
PATH_TOKEN_RE = re.compile(r"[A-Za-z0-9_.~+\-/]+")
GIT_CONTENT_SUBS = {"diff", "show", "grep"}
GIT_STAT_FLAGS = {"--stat", "--numstat", "--shortstat", "--name-only", "--name-status", "--summary"}


def is_secret_var(name: str) -> bool:
    if name in ("PWD", "OLDPWD", "HOSTNAME", "HOST"):
        return name in ("HOSTNAME", "HOST") and False
    return bool(SECRET_VAR_NAME.search(name))


def candidate_files(seg: str, cwd: str):
    out, seen = [], set()
    for tok in PATH_TOKEN_RE.findall(seg):
        tok = tok.rstrip(".:")
        if len(tok) < 2 or tok.startswith("-") or tok in seen:
            continue
        seen.add(tok)
        p = resolve(tok, cwd)
        if os.path.isfile(p):
            out.append(p)
            if len(out) >= MAX_CANDIDATES:
                break
    return out


def run_probe(argv, cwd):
    """Run a read-only probe; return (output_text or None, ok)."""
    try:
        r = subprocess.run(argv, cwd=cwd, capture_output=True, timeout=PROBE_TIMEOUT,
                           stdin=subprocess.DEVNULL)
    except (subprocess.SubprocessError, OSError):
        return None, False
    out = r.stdout[:PROBE_BYTES]
    if r.returncode > 1 and not out:
        return None, False
    return out.decode("utf-8", errors="replace"), True


def probe_would_leak(argv, cwd, pii=False) -> bool:
    text, ok = run_probe(argv, cwd)
    if not ok:
        return True  # cannot verify -> treat as leaking (wrap/deny is the safe direction)
    _, rules = redact_text(text)
    return bool(rules) or (pii and bool(pii_redact(text)[1]))


class Decision:
    def __init__(self):
        self.deny = None      # str reason
        self.wrap = []        # list of reasons
        self.rules = set()
        self.pii = False      # wrap with --pii: the output carries tabular PII
        self.people = False   # wrap with --people: Jira or Bitbucket output names people

    def block(self, reason, rules=()):
        if self.deny is None:
            self.deny = reason
        self.rules.update(rules)

    def filter(self, reason, rules=(), pii=False, people=False):
        self.wrap.append(reason)
        self.rules.update(rules)
        self.pii |= pii or any(r.startswith("pii-") for r in rules)
        self.people |= people


# ----------------------------------------------------------------------------------------
# Database password typed into the command
# ----------------------------------------------------------------------------------------
# A password typed into the command lands in three places at once: this transcript, the
# process table where any local user reads it with `ps`, and the shell history file. None
# of the canonical forms cost anything. Bare `-p` prompts, `--login-path` reads
# ~/.mylogin.cnf, and a shell expansion keeps the literal out of the command text, so the
# rule denies only a literal and always names the replacement for that client.
MYSQL_PW_CLIENTS = {"mysql", "mysqldump", "mysqladmin", "mysqlshow", "mysqlsh",
                    "mariadb", "mariadb-dump", "mariadb-admin"}
PG_PW_CLIENTS = {"psql", "pg_dump", "pg_dumpall", "pg_restore"}
MONGO_PW_CLIENTS = {"mongo", "mongosh", "mongodump", "mongorestore", "mongoexport"}
REDIS_PW_CLIENTS = {"redis-cli"}
DB_PW_CLIENTS = MYSQL_PW_CLIENTS | PG_PW_CLIENTS | MONGO_PW_CLIENTS | REDIS_PW_CLIENTS

# VAR=value carrying a database password. These names are DB-specific, so matching them
# anywhere in the segment cannot collide with unrelated work.
DB_PW_ENV_RE = re.compile(
    r"(?:^|[\s;&|(])(PGPASSWORD|MYSQL_PWD|MARIADB_PWD|REDISCLI_AUTH|MONGO_PASSWORD"
    r"|MONGODB_PASSWORD)=([^\s;&|)]+)")

# Per client family, because the same letter means different things. `-p` is the password
# for mysql and mongo and the PORT for psql and redis-cli, so a single -p rule would deny
# `psql -p 5432`. mysql reads `-p secret` as a prompt plus a database named secret, so only
# the attached form carries a value there.
DB_PW_RE = {
    "mysql": [re.compile(r"(?:^|\s)-p([^\s;&|)]+)"),
              re.compile(r"(?:^|\s)--password=([^\s;&|)]+)")],
    "pg":    [re.compile(r"(?:^|\s)--password=([^\s;&|)]+)")],
    "mongo": [re.compile(r"(?:^|\s)-p\s*([^\s;&|)\-][^\s;&|)]*)"),
              re.compile(r"(?:^|\s)--password[=\s]+([^\s;&|)]+)")],
    "redis": [re.compile(r"(?:^|\s)-a\s*([^\s;&|)\-][^\s;&|)]*)"),
              re.compile(r"(?:^|\s)--pass(?:word)?[=\s]+([^\s;&|)]+)")],
}

DB_PW_ADVICE = {
    "mysql": "use --login-path=<alias> (mysql_config_editor set) or ~/.my.cnf",
    "pg": "use ~/.pgpass, or export PGPASSWORD from .envrc rather than typing it",
    "mongo": "read it from a config file, or let the client prompt",
    "redis": "export REDISCLI_AUTH from .envrc, or let the client prompt",
}


def is_shell_expansion(tok: str) -> bool:
    """True when the token defers to the shell, so no literal reaches the command text."""
    t = tok.strip("'\"")
    return bool(t) and (t[0] in "$`" or "$(" in t or "${" in t)


def db_password_literal(seg: str, verb: str) -> str:
    """Reason when a database password is typed in the clear in seg, else ''."""
    for m in DB_PW_ENV_RE.finditer(seg):
        if not is_shell_expansion(m.group(2)):
            return ("%s= carries a database password in the clear, which reaches the "
                    "transcript, the process table and your shell history" % m.group(1))
    if verb not in DB_PW_CLIENTS:
        return ""
    family = ("mysql" if verb in MYSQL_PW_CLIENTS else
              "pg" if verb in PG_PW_CLIENTS else
              "mongo" if verb in MONGO_PW_CLIENTS else "redis")
    for rx in DB_PW_RE[family]:
        for m in rx.finditer(seg):
            if not is_shell_expansion(m.group(1)):
                return ("'%s' carries its password in the command, which reaches the "
                        "transcript, the process table and your shell history: %s"
                        % (verb, DB_PW_ADVICE[family]))
    return ""


def search_prints_no_content(verb: str, args: list, seg: str) -> bool:
    """A search that prints names, counts or a fixed replacement, never a line of the file.
    These are the forms the denial message recommends (factory logbook 26, H6)."""
    if verb not in SEARCH_VERBS:
        return False
    flags = [a for a in args if a.startswith("-") and not a.startswith("--")]
    if any(a in SEARCH_SAFE_FLAGS for a in args) or any(re.fullmatch(r"-[A-Za-z]*[lcqL][A-Za-z]*", f) for f in flags):
        return True
    only = "-o" in args or "--only-matching" in args or any(re.fullmatch(r"-[A-Za-z]*o[A-Za-z]*", f) for f in flags)
    replace = verb == "rg" and ("--replace" in args or any(a.startswith("--replace=") for a in args))
    return only and replace and "$" not in seg


PII_TABULAR_ADVICE = ("Read the file with the Read tool: it arrives as a redacted copy whose path the "
                      "message names, and rg, sed or awk on that copy are allowed.")


def analyze_bash(cmd: str, cwd: str) -> Decision:
    d = Decision()
    if SELF in cmd or "secret_guard.py" in cmd:
        return d  # already wrapped, or a deliberate --redact / --check call
    probes = 0
    for seg in split_segments(cmd, pipes=True):
        verb, args = head(seg)
        if not verb:
            continue
        plain = [a for a in args if not a.startswith("-")]

        # --- denials -------------------------------------------------------------------
        if verb in ("env", "printenv") and not plain:
            d.block(f"'{verb}' dumps the whole environment, secrets included")
        elif verb == "printenv" and any(is_secret_var(a) for a in plain):
            d.block("printenv of a secret-named variable")
        elif verb == "set" and not args:
            d.block("bare 'set' prints every shell variable, secrets included")
        elif verb in ("declare", "typeset") and any(a in ("-p", "-x") for a in args) and not any(is_secret_var(a) or not a.startswith("-") for a in args) :
            d.block(f"'{verb} -p' dumps variables, secrets included")
        elif verb in ("declare", "typeset") and any(a in ("-p", "-x") for a in args) and any(is_secret_var(a) for a in plain):
            d.block(f"'{verb}' of a secret-named variable")
        elif verb == "export" and (not args or args == ["-p"]):
            d.block("bare 'export' lists exported variables, secrets included")
        elif verb == "launchctl" and args[:1] == ["getenv"] and any(is_secret_var(a) for a in plain):
            d.block("launchctl getenv of a secret-named variable")
        elif verb == "security" and any(a in ("find-generic-password", "find-internet-password", "dump-keychain", "export") for a in args):
            d.block("keychain read prints a credential")
        elif verb == "gh" and args[:2] == ["auth", "token"]:
            d.block("'gh auth token' prints the GitHub token")
        elif verb == "git" and args[:1] == ["credential"] or (verb.startswith("git-credential")):
            if any(a in ("fill", "get") for a in args):
                d.block("git credential helper prints a credential")
        elif verb == "op" and (args[:2] == ["item", "get"] or args[:1] == ["read"]):
            d.block("1Password read prints a credential")
        elif verb == "vault" and (args[:1] == ["read"] or args[:2] == ["kv", "get"]):
            d.block("vault read prints a credential")
        elif "/proc/" in seg and "environ" in seg:
            d.block("/proc environ dumps the environment")

        db_pw = db_password_literal(seg, verb)
        if db_pw:
            d.block(db_pw)

        if verb in ("echo", "printf", "print", "printenv"):
            names = SECRET_VAR_RE.findall(seg)
            if verb == "printenv":
                names += plain
            bad = [n for n in names if is_secret_var(n)]
            if bad:
                d.block("printing a secret-named variable puts its bare value in the transcript")
            elif any(URLISH_VAR_NAME.search(n) for n in names):
                d.filter("a URL-shaped variable can carry a credential in its userinfo")

        if is_key_material(seg, cwd):
            if verb not in KEY_OK_VERBS and not search_prints_no_content(verb, args, seg):
                d.block("names key material (ssh/aws/gh/netrc/pem/key/id_*), which is never printed")
            continue  # a KEY_OK verb uses the key without printing it; never scan its content

        if d.deny:
            continue

        # --- filtering (wrap) ------------------------------------------------------------
        if verb in INTERPRETERS and any(a in INLINE_FLAGS for a in args):
            code = seg
            if ENV_DUMP_INLINE.search(code):
                d.block("inline code dumps the environment")
                continue
            d.filter("inline interpreter code can print any file it names")
            continue

        if verb in ("curl", "wget") and any(a in ("-v", "--verbose", "-i", "--include", "-I") or a.startswith("--trace") for a in args):
            d.filter("verbose HTTP output prints headers such as Authorization")

        if verb in SEARCH_VERBS:
            flags = [a for a in args if a.startswith("-")]
            if any(f in SEARCH_SAFE_FLAGS for f in flags) or any(re.fullmatch(r"-[A-Za-z]*[lcqL][A-Za-z]*", f) for f in flags):
                continue
            if "--replace" in args or any(a.startswith("--replace=") for a in args) or "-r" in args:
                if "-o" in args or "--only-matching" in args or any(re.fullmatch(r"-[A-Za-z]*o[A-Za-z]*", f) for f in flags):
                    continue
            if "$" in seg or probes >= MAX_PROBES:
                d.filter("content-mode search whose output cannot be verified")
                continue
            pattern, paths = None, []
            i = 0
            words = [w.strip("'\"") for w in args]
            for w in words:
                if w.startswith("-"):
                    continue
                if os.path.exists(resolve(w, cwd)):
                    paths.append(w)
                elif pattern is None:
                    pattern = w
            if pattern is None:
                continue
            if not paths:
                # No path means the search reads stdin, so it searches the OUTPUT of the pipeline
                # and not the filesystem. Probing "." asked a different question, and under a home
                # of 1.08M files it answered none: the probe hit its own PROBE_TIMEOUT, which was
                # the dispatcher's entire stage budget. Measured 2026-09-19 with cwd=/Users/you:
                # 5108 ms for one such segment and 15248 ms for three, against 76 ms for the same
                # search with a path named. The stage then blocked a legitimate command, and a gate
                # that denies because it ran out of its own budget decides nothing.
                # The verdict is unchanged: this is the same conservative filter the branch above
                # already applies to a segment nobody can verify.
                d.filter("content-mode search whose output cannot be verified")
                continue
            argv = ["rg", "--no-heading", "--color", "never", "--max-filesize", "5M", "-i"]
            for f in flags:
                m = re.fullmatch(r"-([ABC])(\d+)", f)
                if m:
                    argv += [f]
            argv += ["-e", pattern, "--"] + paths   # paths is non-empty, guarded above
            pii_file = next((p for p in paths if "pii-column" in file_rules(resolve(p, cwd))), None)
            if pii_file:
                d.block(f"{os.path.basename(pii_file)} has PII columns and a search prints rows without "
                        "their header, so the columns cannot be masked. " + PII_TABULAR_ADVICE)
                continue
            probes += 1
            tabular = any(PII_TABULAR.search(p) for p in paths)
            if probe_would_leak(argv, cwd, pii=tabular):
                d.filter("this search would print at least one credential line", pii=tabular)
            continue

        if verb == "acli":
            d.filter("acli output names people: emails removed, names cut to the first word", pii=True, people=True)
            continue

        # git history prints "Author: Full Name <email>". rtk excludes these subcommands since
        # 2026-10-26 (rtk config.toml exclude_commands), so the wrap survives. If that exclusion
        # is lost, rtk_would_rewrite turns this wrap into a deny: loud, never a silent leak.
        if verb == "git":
            i = 0
            while i < len(args) and args[i].startswith("-"):
                i += 2 if args[i] in ("-C", "-c") else 1
            explicit = any(a.startswith(("--format", "--pretty")) for a in args)
            if i < len(args) and args[i] in ("log", "show", "blame", "shortlog", "whatchanged") \
                    and ("--oneline" not in args or explicit):
                d.filter("git history names people: emails removed, names cut to the first word", pii=True, people=True)
                continue

        if verb == "git" and args:
            sub = args[0]
            if sub in GIT_CONTENT_SUBS or (sub == "log" and any(a in ("-p", "--patch", "-u") or re.fullmatch(r"-[a-zA-Z]*p[a-zA-Z]*", a) for a in args)):
                if any(a in GIT_STAT_FLAGS for a in args) and not any(a in ("-p", "--patch") for a in args):
                    continue
                if any(a.startswith("--output") for a in args) or "$" in seg or probes >= MAX_PROBES:
                    d.filter("git patch output cannot be verified")
                    continue
                probes += 1
                if probe_would_leak(["git"] + [a.strip("'\"") for a in args], cwd):
                    d.filter("this patch prints a credential that a change removed or added")
                continue

        if verb in SAFE_VERBS:
            continue

        for p in candidate_files(seg, cwd):
            rules = file_rules(p)
            if "pii-column" in rules:
                d.block(f"{os.path.basename(p)} has PII columns, and a Bash command can print its rows "
                        "without the header row the column mask needs. " + PII_TABULAR_ADVICE)
                break
            if rules:
                d.filter(f"{os.path.basename(p)} holds a credential or PII ({', '.join(rules)})", rules)
                break
    return d


def wrap_command(cmd: str, pii: bool = False, people: bool = False) -> str:
    return ("set -o pipefail\n{\n" + cmd + "\n} 2>&1 | " + PYTHON + " " + SELF + " --filter"
            + (" --pii" if pii else "") + (" --people" if people else ""))


RTK = "/opt/homebrew/bin/rtk"
RTK_CHECK = True  # cleared by self-test so the battery does not depend on a local rtk
RTK_CLOBBER = (
    "rtk rewrites this command, and its PreToolUse hook returns permissionDecision:allow. "
    "That settles the call, so every later hook's updatedInput is discarded and the redaction "
    "wrapper never runs. Measured on 2026-09-15: cat of a credential file printed raw values "
    "in both hook orderings while this guard fired. A denial is the only control rtk cannot "
    "overrule, so this is denied instead of redacted. Read the file with the Read tool, which "
    "arrives redacted, or use a verb rtk leaves alone such as sed or rg."
)


def rtk_would_rewrite(cmd: str) -> bool:
    """True when rtk will rewrite cmd, so a wrap emitted here would be silently discarded.

    `rtk hook check` exits 0 and prints the rewrite when it rewrites, and exits 1 when it does
    not. Measured at 5ms, and only ever reached on the wrap path, so the cost is not on the
    common route. If rtk is not installed it cannot be in the hook chain either, so nothing can
    clobber the wrap and False is correct. Any other failure returns True, which downgrades the
    outcome to a denial: a denial that was not strictly needed costs a prompt, while a wrap that
    vanishes costs the credential.
    """
    if not RTK_CHECK:
        return False
    try:
        r = subprocess.run([RTK, "hook", "check", cmd], stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL, timeout=PROBE_TIMEOUT)
        return r.returncode == 0
    except (FileNotFoundError, NotADirectoryError):
        return False
    except Exception:
        return True


# ----------------------------------------------------------------------------------------
# Hook events
# ----------------------------------------------------------------------------------------

SAFE_FORMS = (
    "Values are masked as *** to keep them out of the transcript. Safe forms: "
    "`rg -l PATTERN PATH`, `rg -c PATTERN PATH`, `rg -o --replace HIT PATTERN PATH`, "
    "`git diff --stat`, `git diff --name-only`, or read the file with the Read tool "
    "(it arrives redacted). Never try to recover a masked value."
)


def log(event, tool, action, rules=(), note=""):
    if NOLOG:
        return
    try:
        os.makedirs(CONFIG_DIR, exist_ok=True)
        with open(LOG_PATH, "a") as f:
            f.write(json.dumps({"ts": int(time.time()), "event": event, "tool": tool,
                                "action": action, "rules": sorted(rules), "note": note}) + "\n")
    except OSError:
        pass


class Outcome:
    def __init__(self, action, message="", payload=None):
        self.action = action      # "allow" | "deny" | "rewrite"
        self.message = message
        self.payload = payload or {}


def redacted_copy(path: str, cwd: str) -> str:
    dest_dir = os.path.join(cwd, REDACTED_SUBDIR)
    os.makedirs(dest_dir, exist_ok=True)
    gi = os.path.join(dest_dir, ".gitignore")
    if not os.path.exists(gi):
        with open(gi, "w") as f:
            f.write("*\n")
    tag = hashlib.sha256(path.encode()).hexdigest()[:8]
    dest = os.path.join(dest_dir, f"{tag}-{os.path.basename(path)}")
    with open(path, "rb") as f:
        data = f.read(MAX_SCAN_BYTES)
    text, _ = redact_text(data.decode("utf-8", errors="replace"))
    if PII_TABULAR.search(path):
        text, _ = pii_redact(text, delim=pii_delim(path))
    with open(dest, "w") as f:
        f.write(text)
    os.chmod(dest, 0o600)
    return dest


def evaluate(data: dict) -> Outcome:
    event = data.get("hook_event_name") or ("UserPromptSubmit" if "prompt" in data else "PreToolUse")
    cwd = data.get("cwd") or os.getcwd()
    tool = data.get("tool_name", "")
    ti = data.get("tool_input") or {}

    if event == "UserPromptSubmit":
        prompt = data.get("prompt") or ""
        for m in re.finditer(r"(?<!\S)@(\S+)", prompt):
            ref = m.group(1).rstrip(".,;:)\"'")
            if is_key_material(ref, cwd):
                return Outcome("deny", f"@{ref} is key material; @-mentions bypass every hook.")
            p = resolve(ref, cwd)
            if os.path.isfile(p):
                rules = file_rules(p)
                if rules:
                    return Outcome("deny", f"@{ref} holds a credential or PII ({', '.join(rules)}) and @ expansion "
                                           "bypasses hooks. Ask for an explicit Read: it arrives redacted.", {"rules": rules})
        _, rules = redact_text(prompt)
        hard = [r for r in rules if r in HIGH_CONFIDENCE]
        if hard:
            return Outcome("deny", f"the prompt carries a secret-looking value ({', '.join(hard)}); "
                                   "replace it with a placeholder.", {"rules": hard})
        return Outcome("allow")

    if event != "PreToolUse":
        return Outcome("allow")

    if tool == "Bash":
        cmd = ti.get("command") or ""
        if not cmd:
            return Outcome("allow")
        d = analyze_bash(cmd, cwd)
        if d.deny:
            return Outcome("deny", d.deny + ". " + SAFE_FORMS, {"rules": d.rules})
        if d.wrap:
            if rtk_would_rewrite(cmd):
                if d.people and re.search(r"\bgit\b", cmd):
                    return Outcome("deny", "git history names people, and rtk rewrites this form, which would drop the "
                                           "people filter. rtk excludes git log/show/blame/shortlog only when the command "
                                           "starts with them: run it as `cd <repo> && git log ...`, not `git -C <repo> log`.",
                                   {"rules": d.rules})
                return Outcome("deny", "; ".join(d.wrap) + ". " + RTK_CLOBBER,
                               {"rules": d.rules})
            new = dict(ti, command=wrap_command(cmd, d.pii, d.people))
            return Outcome("rewrite", "; ".join(d.wrap), {"updatedInput": new, "rules": d.rules})
        return Outcome("allow")

    if tool in ("Read", "Edit", "Write", "MultiEdit", "NotebookEdit", "Grep", "Glob"):
        target = ti.get("file_path") or ti.get("notebook_path") or ti.get("path") or ""
        if is_key_material(target, cwd):
            return Outcome("deny", f"{tool} of key material is never allowed: {os.path.basename(target)}")

    if tool == "Read":
        target = ti.get("file_path") or ""
        p = resolve(target, cwd) if target else ""
        rules = file_rules(p) if p else []
        if rules:
            copy = redacted_copy(p, cwd)
            new = dict(ti, file_path=copy)
            return Outcome("rewrite", f"{os.path.basename(p)} holds credentials or PII ({', '.join(rules)}); "
                                      "you are reading a redacted copy. Values shown as *** are masked "
                                      "on purpose; never try to recover them.", {"updatedInput": new, "rules": rules})
        return Outcome("allow")

    if tool in ("Edit", "Write", "MultiEdit", "NotebookEdit"):
        tgt = ti.get("file_path") or ti.get("notebook_path") or ""
        if tgt and file_exempt(resolve(tgt, cwd)):
            return Outcome("allow")
        payload = "\n".join(str(v) for k, v in ti.items() if k in ("content", "new_string", "new_source"))
        if ti.get("edits"):
            payload += "\n" + "\n".join(str(e.get("new_string", "")) for e in ti["edits"] if isinstance(e, dict))
        if FILE_ALLOW_MARK in payload:
            return Outcome("allow")
        _, rules = redact_text(payload)
        if rules:
            # Name the line. Before 2026-09-18 this message named only the rule, and localising a
            # single line in a 60-line write cost four blocked attempts and a dozen probes. The
            # excerpt is redacted: the message locates the literal and never repeats it.
            lineno, line_rules, excerpt = locate_secret(payload)
            where = (f" at line {lineno}: {excerpt}" if lineno
                     else " (no single line reproduces it, so the match spans lines)")
            return Outcome("deny", f"the new content carries a secret-looking literal "
                                   f"({', '.join(rules)}){where}. "
                                   "Reference the environment or a vault instead. For a dummy value "
                                   "in docs or tests add 'redact:allow' on that line, or "
                                   "'secret-guard:allow-file' anywhere in the content to exempt the "
                                   "whole file. On a Read that whole-file marker only counts inside "
                                   "the first 4096 bytes.", {"rules": rules})
        return Outcome("allow")

    if tool == "Grep":
        if ti.get("output_mode") != "content":
            return Outcome("allow")
        pattern = ti.get("pattern") or ""
        path = ti.get("path") or "."
        argv = ["rg", "--no-heading", "--color", "never", "--max-filesize", "5M"]
        if ti.get("-i"):
            argv.append("-i")
        if ti.get("glob"):
            argv += ["-g", str(ti["glob"])]
        if ti.get("type"):
            argv += ["-t", str(ti["type"])]
        for k in ("-A", "-B", "-C"):
            if ti.get(k) is not None:
                argv += [k, str(ti[k])]
        if ti.get("multiline"):
            argv += ["-U", "--multiline-dotall"]
        argv += ["-e", pattern, "--", path]
        if os.path.isfile(resolve(path, cwd)) and "pii-column" in file_rules(resolve(path, cwd)):
            return Outcome("deny", f"{os.path.basename(path)} has PII columns and a content search prints "
                                   "rows without their header. " + PII_TABULAR_ADVICE)
        tabular = bool(PII_TABULAR.search(path) or PII_TABULAR.search(str(ti.get("glob") or ""))
                       or ti.get("type") in ("csv", "tsv"))
        if probe_would_leak(argv, cwd, pii=tabular):
            return Outcome("deny", "this content-mode search would print a credential line. Use "
                                   "output_mode files_with_matches or count, narrow the path, or run "
                                   "rg through Bash where the output is redacted.")
        return Outcome("allow")

    return Outcome("allow")


QUIET_REASON = "content-mode search whose output cannot be verified"


def emit(outcome: Outcome, data: dict) -> int:
    event = data.get("hook_event_name", "PreToolUse")
    tool = data.get("tool_name", "")
    rules = outcome.payload.get("rules", ())
    if outcome.action == "deny":
        log(event, tool, "deny", rules, outcome.message[:120])
        sys.stderr.write(f"BLOCKED by secret-guard: {outcome.message}\n")
        return 2
    if outcome.action == "rewrite":
        log(event, tool, "rewrite", rules, outcome.message[:120])
        out = {"hookEventName": "PreToolUse", "updatedInput": outcome.payload["updatedInput"]}
        # The unverifiable-search reason only announces the redaction wrapper, which runs anyway.
        # It fired on 78 of 122 sessions with no decision behind it (factory logbook 23, H4).
        said = "; ".join(r for r in outcome.message.split("; ") if r != QUIET_REASON)
        if said:
            out["additionalContext"] = f"secret-guard: {said}"
        print(json.dumps({"hookSpecificOutput": out}))
        return 0
    return 0


# ----------------------------------------------------------------------------------------
# CLI modes
# ----------------------------------------------------------------------------------------

def cli_filter(pii: bool = False, people: bool = False, tsv: bool = False) -> int:
    data = sys.stdin.buffer.read()
    text, _ = redact_text(data.decode("utf-8", errors="replace"))
    if people:
        text, _ = people_redact(text)
    if pii:
        text, _ = pii_redact(text, delim="\t" if tsv else None)
    sys.stdout.write(text)
    sys.stdout.flush()
    return 0


def cli_redact(path: str) -> int:
    with open(path, "rb") as f:
        data = f.read()
    text, _ = redact_text(data.decode("utf-8", errors="replace"))
    sys.stdout.write(text)
    return 0


def cli_check(paths) -> int:
    for p in paths:
        rules = file_rules(p)
        print(f"{p}: {', '.join(rules) if rules else 'clean'}")
    return 0


# ----------------------------------------------------------------------------------------
# Self-test (synthetic values only; nothing here ever existed)
# ----------------------------------------------------------------------------------------

def self_test() -> int:
    global NOLOG, RTK_CHECK
    NOLOG = True
    # The battery must not depend on a local rtk, so the wrap path is exercised with the check
    # off. The rtk-clobber downgrade is asserted separately below, with the check forced on.
    RTK_CHECK = False
    failures = []
    total = 0

    def check(name, got, want, extra=""):
        nonlocal total
        total += 1
        ok = got == want
        print(f"{'PASS' if ok else 'FAIL'}  {name:<48} want={want} got={got} {extra}")
        if not ok:
            failures.append(name)

    # ---- redaction rules: MASK (token must vanish) / KEEP (byte-identical) ------------
    mask_cases = [
        ("array key 'password'", "$c = ['password' => 'DUMMYA01'];", "DUMMYA01"),
        ("array key 'pass'", "$c = ['pass' => 'DUMMYA02'];", "DUMMYA02"),
        ("array key 'passwd'", "$c = ['passwd' => 'DUMMYA03'];", "DUMMYA03"),
        ("array key 'pwd'", "$c = ['pwd' => 'DUMMYA04'];", "DUMMYA04"),
        ("var $password", "$password = 'DUMMYA05';", "DUMMYA05"),
        ("var $pass", "$pass = 'DUMMYA06';", "DUMMYA06"),
        ("var $dbPass camelCase", "$dbPass = 'DUMMYA07';", "DUMMYA07"),
        ("const DB_PASS", "define('DB_PASS', 'DUMMYA08');", "DUMMYA08"),
        ("key MYSQL_PASS", "'MYSQL_PASS' => 'DUMMYA09',", "DUMMYA09"),
        ("passphrase", "'passphrase' => 'DUMMYA10',", "DUMMYA10"),
        ("new PDO positional", "$d = new PDO('mysql:host=h;dbname=n', 'user', 'DUMMYA11');", "DUMMYA11"),
        ("custom ::connect() 4 args", "$d = \\XPDO::connect(\n    'host.local',\n    'dbname',\n    'user',\n    'DUMMYA12'\n);", "DUMMYA12"),
        ("mysqli_connect()", "$d = mysqli_connect('host.local', 'user', 'DUMMYA13', 'dbname');", "DUMMYA13"),
        ("instance ->connect()", "$d = $mgr->connect('host.local', 'user', 'DUMMYA14');", "DUMMYA14"),
        ("inline URL credential", "$u = 'mysql://user:DUMMYA15@host.local/db';", "DUMMYA15"),
        ("inline URL host", "$u = 'mysql://user:DUMMYA15@dummyhost.local/db';", "dummyhost.local"),
        ("DSN password field", "$dsn = 'host=h;password=DUMMYA16;dbname=n';", "DUMMYA16"),
        ("mysql CLI -p", "system('mysql -uuser -pDUMMYA17 dbname');", "DUMMYA17"),
        ("AWS access key id", "$k = 'AKIA" "IOSFODNN7EXAMPLE';", "AKIA" "IOSFODNN7EXAMPLE"),
        ("commented-out connection", "//$d = new PDO('mysql:host=h;dbname=n', 'user', 'DUMMYA19');", "DUMMYA19"),
        ("dotenv DB_PASSWORD", "DB_PASSWORD=DUMMYA20\n", "DUMMYA20"),
        ("dotenv quoted", 'APP_SECRET="DUMMYA21"\n', "DUMMYA21"),
        ("dotenv DB_HOST", "DB_HOST=db.dummy.internal\n", "db.dummy.internal"),
        ("dotenv REDIS_HOSTNAME", "REDIS_HOSTNAME=cache.dummy.internal\n", "cache.dummy.internal"),
        ("ini password", "[db]\npassword = DUMMYA22\n", "DUMMYA22"),
        ("ini host", "[db]\nhost = db.dummy.local\n", "db.dummy.local"),
        ("yaml password", "database:\n  password: DUMMYA23\n", "DUMMYA23"),
        ("json password", '{"password": "DUMMYA24"}', "DUMMYA24"),
        ("php array host", "'host' => 'db.example.local',", "db.example.local"),
        ("github token", "token = ghp_DUMMYabcdefghijklmnopqrstuvwxyz0123456789", "ghp_DUMMY"),
        ("private key block", "-----BEGIN PRIVATE KEY-----\nDUMMYKEYLINE\n-----END PRIVATE KEY-----\n", "DUMMYKEYLINE"),
        ("jwt", "auth eyJ" "hbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.DUMMYsignature_value", "DUMMYsignature"),
        ("bearer header", "Authorization: Bearer DUMMYbearer0123456789", "DUMMYbearer"),
        ("host header", "Host: api.dummy.internal", "api.dummy.internal"),
        ("jq -R escaped quotes", '"API_TOKEN=\\"DUMMYJQ01\\""', "DUMMYJQ01"),  # redact:allow
        ("json string escaped", '"{\\"password\\":\\"DUMMYJQ02\\"}"', "DUMMYJQ02"),  # redact:allow
        ("json host escaped", '"{\\"host\\":\\"db.dummy.escaped\\"}"', "db.dummy.escaped"),  # redact:allow
    ]
    keep_cases = [
        ("env read via $_ENV", "'password' => $_ENV['TC_DB_PASSWORD'],"),
        ("env read, key 'pass'", "'pass' => $_ENV['TC_DB_PASSWORD'],"),
        ("env read via getenv", "'password' => getenv('TC_DB_PASSWORD'),"),
        ("empty value", "'password' => '',"),
        ("declared key, no value", "TC_DB_PASSWORD="),
        ("dbname literal", "'dbname' => 'shopfront',"),
        ("single-literal connect", "$redis->connect('127.0.0.1');"),
        ("boolean value", "'bypass' => true,"),
        ("variable in PDO slots", "$d = new PDO($dsn, $user, $pass);"),
        ("territory abbrev table", '$states = ["OR" => "Oregon", "PW" => "Palau", "PA" => "Pennsylvania"];'),
        ("array key not a value", "$d = $this->connect($parts['user'], $parts['pass']);"),
        ("env key name in getenv", "$d = new PDO($dsn, getenv('TC_DB_USERNAME'), getenv('TC_DB_PASSWORD'));"),
        ("PASS_THROUGH enum", "const PASS_THROUGH = 'PASS THROUGH';"),
        ("PassThrough camel enum", "const PassThrough = 'passthrough';"),
        ("call as the value", "$basic_pass = isset($_SERVER['PHP_AUTH_PW']) ? $_SERVER['PHP_AUTH_PW'] : '';"),
        ("constructor as the value", "$secret = new SecretBag($args);"),
        ("localhost host", "DB_HOST=localhost\n"),
        ("loopback host", "host: 127.0.0.1\n"),
        ("short numeric token", "MIN_CORE_TOKEN_LEN = 2\n"),
        ("token name meta", "TOKEN_NAME = 'csrf_token'\n"),
        ("hosts plural", "ALLOWED_HOSTS=localhost,example.com\n"),
        ("redact:allow marker", "password = 'DUMMYDOC01'  # redact:allow\n"),
        ("process.env", "const pw = process.env.DB_PASSWORD;\n"),
        ("plain prose", "The password reset flow sends an email.\n"),
    ]
    for name, body, token in mask_cases:
        out, _ = redact_text(body, hashes=set())
        check(f"MASK {name}", token not in out, True)
    for name, body in keep_cases:
        out, _ = redact_text(body, hashes=set())
        check(f"KEEP {name}", out == body, True)

    # ---- tool decisions on fixtures ---------------------------------------------------
    tmp = tempfile.mkdtemp(prefix="secret-guard-selftest-")
    try:
        fix = os.path.join(tmp, "proj")
        os.makedirs(os.path.join(fix, "config"))
        C = os.path.join(fix, "config", "app-config.php")
        with open(C, "w") as f:
            f.write("<?php\nreturn ['db' => ['host' => 'db.example.local', 'dbname' => 'appdb',\n"
                    "    'username' => 'appuser', 'password' => 'DUMMYLEAK01']];\n")
        CLEAN = os.path.join(fix, "config", "clean.php")
        with open(CLEAN, "w") as f:
            f.write("<?php\nreturn ['db' => ['host' => $_ENV['TC_DB_HOST'], 'password' => $_ENV['TC_DB_PASSWORD']]];\n")
        ENV = os.path.join(fix, ".env")
        env_ok = True
        try:
            with open(ENV, "w") as f:
                f.write("APP_NAME=demo\nDB_HOST=db.dummy.internal\nDB_PASSWORD=DUMMYLEAK02\n")
        except PermissionError:
            env_ok = False  # sandbox still denies .env (layer 1 not yet relaxed)
            print("SKIP  .env fixture cannot be created under the current sandbox; .env cases skipped")
        INI = os.path.join(fix, "app.ini")
        with open(INI, "w") as f:
            f.write("[ftp]\nhost = ftp.dummy.internal\npassword = DUMMYLEAK03\n")
        README = os.path.join(fix, "README.md")
        with open(README, "w") as f:
            f.write("# demo\nSet DB_PASSWORD in your .env (see the vault).\n")
        SCRIPT = os.path.join(fix, "script.py")
        with open(SCRIPT, "w") as f:
            f.write("print('hello')\n")
        # git repo with a committed credential and a pending change (patch prints it)
        gitok = subprocess.run(["git", "init", "-q", fix], capture_output=True).returncode == 0
        if gitok:
            env = dict(os.environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@t", GIT_COMMITTER_NAME="t",
                       GIT_COMMITTER_EMAIL="t@t", HOME=tmp)
            subprocess.run(["git", "-C", fix, "add", "-A"], capture_output=True, env=env)
            subprocess.run(["git", "-C", fix, "commit", "-q", "-m", "init"], capture_output=True, env=env)
            with open(C, "a") as f:
                f.write("// rotated\n")

        def bash(cmd, cwd=fix):
            return evaluate({"hook_event_name": "PreToolUse", "tool_name": "Bash",
                             "tool_input": {"command": cmd}, "cwd": cwd}).action

        bash_cases = [
            ("cat a config with a value", f"cat {C}", "rewrite"),
            ("head", f"head -20 {C}", "rewrite"),
            ("tail", f"tail -5 {C}", "rewrite"),
            ("sed range", f"sed -n '1,20p' {C}", "rewrite"),
            ("awk", f"awk 'NR<20' {C}", "rewrite"),
            ("strings", f"strings {C}", "rewrite"),
            ("base64", f"base64 {C}", "rewrite"),
            ("dd", f"dd if={C} bs=1 count=400", "rewrite"),
            ("php one-liner", f"php -r 'echo file_get_contents(\"{C}\");'", "rewrite"),
            ("python one-liner", f"python3 -c 'print(open(\"{C}\").read())'", "rewrite"),
            ("python env dump", "python3 -c 'import os; print(os.environ)'", "deny"),
            ("rg on the file", f"rg 'password' {C}", "rewrite"),
            ("rg on a directory", f"rg -n 'pass' {fix}", "rewrite"),
            ("grep -r on a directory", f"grep -r 'password' {fix}", "rewrite"),
            ("rg -o without --replace", f"rg -o 'password.*' {C}", "rewrite"),
            ("rg relative cwd", "rg password .", "rewrite"),
            ("bare env", "env", "deny"),
            ("env piped", "env | grep DB", "deny"),
            ("printenv bare", "printenv", "deny"),
            ("printenv secret", "printenv DB_PASSWORD", "deny"),
            ("printenv HOME", "printenv HOME", "allow"),
            ("bare set", "set", "deny"),
            ("set -e", "set -e", "allow"),
            ("declare -p", "declare -p", "deny"),
            ("export bare", "export", "deny"),
            ("export -p", "export -p", "deny"),
            ("export assignment", "export FOO=1", "allow"),
            ("env as wrapper", "env FOO=1 ls", "allow"),
            ("echo secret var", "echo $DB_PASSWORD", "deny"),
            ("echo braces", 'echo "${API_TOKEN}"', "deny"),
            ("echo host var", "echo $DB_HOST", "deny"),
            ("echo PWD", "echo $PWD", "allow"),
            ("printenv proxy url", "printenv HTTP_PROXY", "rewrite"),
            ("echo database url", "echo $DATABASE_URL", "rewrite"),
            ("echo redis uri", 'echo "${REDIS_URI}"', "rewrite"),
            ("printenv plain name", "printenv LANG", "allow"),
            ("echo a url-named var", "echo $PROJECT_URL", "rewrite"),
            ("cat .env", "cat .env", "rewrite"),
            ("cat ini", "cat app.ini", "rewrite"),
            ("source .env", "source .env && npm test", "rewrite"),
            ("dump in a later segment", f"cd /tmp && cat {C}", "rewrite"),
            ("substitution", f"echo $(cat {C})", "rewrite"),
            ("cat ssh key", "cat ~/.ssh/id_rsa", "deny"),
            ("ssh with key", "ssh -i ~/.ssh/id_rsa host uptime", "allow"),
            ("keychain", "security find-generic-password -w -s foo", "deny"),
            ("gh auth token", "gh auth token", "deny"),
            ("op read", "op read op://vault/item/password", "deny"),
            ("curl verbose", "curl -v https://example.com", "rewrite"),
            ("curl plain", "curl -sS https://example.com", "allow"),
            ("rg with --replace", f"rg -H -n -o --replace 'HIT' 'password' {C}", "allow"),
            ("rg -l", f"rg -l 'password' {fix}", "allow"),
            ("rg -c", f"rg -c 'password' {fix}", "allow"),
            ("rg clean match", f"rg -n 'hello' {SCRIPT}", "allow"),
            ("redacted view", f"{PYTHON} {SELF} --redact {C} | sed -n '1,20p'", "allow"),
            ("php -l", f"php -l {C}", "rewrite"),
            ("wc", f"wc -l {C}", "allow"),
            ("cp to a backup", f"cp {C} /tmp/probe-backup.php", "allow"),
            ("cat a clean file", f"cat {CLEAN}", "allow"),
            ("cat README", "cat README.md", "allow"),
            ("run a script", "python3 script.py", "allow"),
            ("git status", "git status", "allow"),
            ("ls", f"ls -la {fix}", "allow"),
            ("npm test", "npm test", "allow"),
            ("mysql login-path", 'mysql --login-path=local -e "select 1"', "allow"),
            ("already wrapped", wrap_command("cat .env"), "allow"),
        ]
        if gitok:
            bash_cases += [
                ("git diff", "git diff", "rewrite"),
                ("git show", "git show HEAD", "rewrite"),
                ("git log -p", "git log -p -1", "rewrite"),
                ("git diff --stat", "git diff --stat", "allow"),
                ("git diff --name-only", "git diff --name-only", "allow"),
                ("git log plain", "git log --oneline -3", "allow"),
            ]
        # public CA bundle: named, not key material
        CA = os.path.join(fix, "ca-bundle.pem")
        with open(CA, "w") as f:
            f.write('-----BEGIN CERTIFICATE-----\\nZHVtbXk=\\n-----END CERTIFICATE-----\\n')
        PRIV = os.path.join(fix, "server.pem")
        with open(PRIV, "w") as f:
            f.write('-----BEGIN PRIVATE KEY-----\\nZHVtbXk=\\n-----END PRIVATE KEY-----\\n')
        EXEMPT = os.path.join(fix, "fixtures.py")
        with open(EXEMPT, "w") as f:
            f.write("# secret-guard:allow-file\\nPASSWORD = 'DUMMYEXEMPT01'\\n")
        try:
            with open(CA, "rb") as f:
                pem_ok = b"CERT" in f.read(64)
        except OSError:
            pem_ok = False  # layer 1 denies /**/*.pem here; the hook runs outside the sandbox
        if not pem_ok:
            print("SKIP  .pem fixtures unreadable under the current sandbox; cert cases skipped")
        cert_cases = [
            ("cat a public CA bundle", f"cat {CA}", "allow"),
            ("openssl on a CA bundle", f"openssl x509 -in {CA} -noout -subject", "allow"),
            ("for loop over paths", f"for f in {CA} {C}; do echo x; done", "allow"),
        ]
        bash_cases += cert_cases if pem_ok else []
        bash_cases += [
            ("cat a private key pem", f"cat {PRIV}", "deny"),
            ("cat a missing pem", "cat /nonexistent/server.pem", "deny"),
            ("do segment still judged", f"for f in a; do cat {C}; done", "rewrite"),
            ("if segment still judged", f"if true; then cat {C}; fi", "rewrite"),
            ("while segment judged", f"while read l; do cat {C}; done", "rewrite"),
            ("exempt file by marker", f"cat {EXEMPT}", "allow"),
            ("cat the guard itself", f"cat {SELF}", "allow"),
        ]
        # rtk composition. `rtk hook claude` runs first in the PreToolUse Bash chain and
        # rewrites every call to `rtk <cmd>` through updatedInput, so this guard receives the
        # rewritten form. Every deny/rewrite case below was a live miss before head() learned
        # to strip rtk. The allow cases guard against over-stripping rtk's own meta commands.
        bash_cases += [
            ("rtk env", "rtk env", "deny"),
            ("rtk printenv", "rtk printenv", "deny"),
            ("rtk echo secret var", "rtk echo $DB_PASSWORD", "deny"),
            ("rtk git patch", "rtk git diff", "rewrite"),
            ("rtk read credential", f"rtk read {C}", "rewrite"),
            ("rtk proxy cat credential", f"rtk proxy cat {C}", "rewrite"),
            ("rtk gain still allowed", "rtk gain", "allow"),
            ("rtk discover still allowed", "rtk discover", "allow"),
            ("rtk ls still allowed", "rtk ls -la", "allow"),
            ("rtk git status still allowed", "rtk git status", "allow"),
            ("rtk git stat flag allowed", "rtk git diff --stat", "allow"),
        ]
        # --- database password typed into the command ------------------------------------
        # The deny rows and the CONTROL rows are one pair and both halves must hold. A guard
        # that denies every -p would pass the deny rows and break `psql -p 5432`, where -p is
        # the port. A guard that denies nothing would pass the CONTROL rows and leak.
        bash_cases += [
            ("mysql -p attached literal", "mysql -u root -pDUMMYPW01 -e 'SELECT 1'", "deny"),
            ("mysql --password= literal", "mysql --password=DUMMYPW02 -e 'SELECT 1'", "deny"),
            ("mysqldump -p attached", "mysqldump -pDUMMYPW03 appdb", "deny"),
            ("PGPASSWORD literal prefix", "PGPASSWORD=DUMMYPW04 psql -h db -U app", "deny"),
            ("MYSQL_PWD literal export", "export MYSQL_PWD=DUMMYPW05", "deny"),
            ("redis-cli -a literal", "redis-cli -a DUMMYPW06 ping", "deny"),
            ("mongosh --password literal", "mongosh --password DUMMYPW07 --host db", "deny"),
            ("mariadb -p attached", "mariadb -pDUMMYPW08 appdb", "deny"),
            ("CONTROL mysql --login-path", "mysql --login-path=staging -e 'SELECT 1'", "allow"),
            ("CONTROL mysql bare -p prompts", "mysql -u root -p -e 'SELECT 1'", "allow"),
            ("CONTROL mysql -p from a variable", "mysql -p\"$DB_PASS\" -e 'SELECT 1'", "allow"),
            ("CONTROL PGPASSWORD from a variable", "PGPASSWORD=\"$DB_PASS\" psql -h db", "allow"),
            ("CONTROL psql -p is a PORT", "psql -p 5432 -h db -U app -c 'SELECT 1'", "allow"),
            ("CONTROL redis-cli -p is a PORT", "redis-cli -p 6379 ping", "allow"),
            ("CONTROL redis-cli no auth", "redis-cli -n 2 ping", "allow"),
            ("CONTROL pg_dump plain", "pg_dump -h db -U app appdb", "allow"),
        ]
        for name, cmd, want in bash_cases:
            if not env_ok and ".env" in cmd:
                continue
            check(f"BASH {name}", bash(cmd), want)

        # The rtk clobber downgrade. rtk's hook returns permissionDecision:allow, which settles
        # the call and discards this guard's updatedInput, so a wrap on an rtk-rewritten command
        # never runs. The outcome must fall back to a denial, which rtk cannot overrule. Asserting
        # it needs a real rtk, because the verdict comes from `rtk hook check`.
        if os.path.exists(RTK):
            RTK_CHECK = True
            try:
                check("BASH rtk verb denies not wraps", bash(f"cat {C}"), "deny")
                check("BASH non-rtk verb still wraps", bash(f"sed -n 1p {C}"), "rewrite")
            finally:
                RTK_CHECK = False
        else:
            print("SKIP  rtk not installed; the clobber-downgrade cases need `rtk hook check`")

        # php -l is a lint: the file test flags it because php is not in SAFE_VERBS. Accepted:
        # a wrap costs a prompt, never a block.

        def tool(tname, ti):
            return evaluate({"hook_event_name": "PreToolUse", "tool_name": tname, "tool_input": ti, "cwd": fix})

        o = tool("Read", {"file_path": C})
        check("READ credential file rewritten", o.action, "rewrite")
        if o.action == "rewrite":
            copy = o.payload["updatedInput"]["file_path"]
            check("READ copy inside cwd/.claude", copy.startswith(os.path.join(fix, REDACTED_SUBDIR)), True)
            with open(copy) as f:
                body = f.read()
            check("READ copy has no value", "DUMMYLEAK01" not in body and "db.example.local" not in body, True)
            check("READ copy keeps keys", "'password'" in body and "'username' => 'appuser'" in body, True)
        if env_ok:
            check("READ .env rewritten", tool("Read", {"file_path": ".env"}).action, "rewrite")
        check("READ clean file", tool("Read", {"file_path": CLEAN}).action, "allow")
        check("READ the guard itself", tool("Read", {"file_path": SELF}).action, "allow")
        check("READ exempt fixture", tool("Read", {"file_path": EXEMPT}).action, "allow")
        if pem_ok:
            check("READ public CA bundle", tool("Read", {"file_path": CA}).action, "allow")
        check("READ private key pem", tool("Read", {"file_path": PRIV}).action, "deny")
        check("EDIT the guard itself", tool("Edit", {"file_path": SELF, "old_string": "a",
              "new_string": "PASSWORD = 'x123456'"}).action, "allow")
        check("READ ssh key", tool("Read", {"file_path": os.path.join(HOME, ".ssh", "id_rsa")}).action, "deny")
        check("READ pem", tool("Read", {"file_path": "/tmp/server.pem"}).action, "deny")
        check("READ keys.json not key material", tool("Read", {"file_path": os.path.join(fix, "settings.key.json")}).action, "allow")
        check("WRITE secret literal", tool("Write", {"file_path": "x.php", "content": "return ['password' => 'hunter22xx'];"}).action, "deny")
        check("WRITE with marker", tool("Write", {"file_path": "x.php", "content": "return ['password' => 'hunter22xx']; // redact:allow"}).action, "allow")
        check("WRITE env read", tool("Write", {"file_path": "x.php", "content": "return ['password' => getenv('DB_PASSWORD')];"}).action, "allow")

        # D2, 2026-09-18. The deny message used to name the rule and nothing else, so a writer
        # had to bisect their own file to find the line. These six rows are a pair set: three
        # assert the message locates and explains, three are the controls that catch a regression.
        _d2 = "\n".join(["clean one", "clean two", "password = 'hunter22xx'", "clean four"])
        _d2msg = tool("Write", {"file_path": "x.php", "content": _d2}).message
        check("locate names the offending line", "line 3" in _d2msg, True)
        check("CONTROL locate does not name a clean line", ("line 1" in _d2msg or "line 2" in _d2msg), False)
        check("locate shows a masked excerpt", "***" in _d2msg, True)
        check("CONTROL locate never repeats the literal", 'hunter22xx' in _d2msg, False)
        check("the message names the whole-file marker too", "allow-file" in _d2msg, True)
        check("CONTROL a clean payload localises nothing",
              locate_secret("alpha\nbeta\ngamma")[0], 0)
        # The bare key "pass" is the only SECRET_KEY name that is also a common English word, and
        # before 2026-09-17 prose reached the assignment shape and denied ordinary documentation.
        # These six rows are a pair set: the narrowing must hold AND every real shape must still
        # deny, or the fix has widened the hole instead of closing a false positive.
        PROSE = "Still open from the same pass: decisions.md carries the stale row."
        check("WRITE prose pass-colon", tool("Write", {"file_path": "x.md", "content": PROSE}).action, "allow")
        check("WRITE prose Pass-colon", tool("Write", {"file_path": "x.md", "content": "One more Pass: decisions.md and we stop."}).action, "allow")
        check("control: yaml pass at line start", tool("Write", {"file_path": "x.yml", "content": "db:\n  pass: hunter22xx\n"}).action, "deny")
        check("control: dotenv PASS equals", tool("Write", {"file_path": "x.env", "content": "PASS=hunter22xx\n"}).action, "deny")
        check("control: json quoted pass", tool("Write", {"file_path": "x.json", "content": '{"pass": "hunter22xx"}'}).action, "deny")
        check("control: passwd keeps colon", tool("Write", {"file_path": "x.md", "content": "the same passwd: hunter22xx"}).action, "deny")
        check("EDIT secret literal", tool("Edit", {"file_path": "x.php", "old_string": "a", "new_string": "token = 'DUMMYEDIT99'"}).action, "deny")
        check("EDIT key material", tool("Edit", {"file_path": os.path.join(HOME, ".aws", "credentials"), "old_string": "a", "new_string": "b"}).action, "deny")
        check("GREP content leaks", tool("Grep", {"pattern": "password", "path": fix, "output_mode": "content"}).action, "deny")
        check("GREP files_with_matches", tool("Grep", {"pattern": "password", "path": fix}).action, "allow")
        check("GREP content clean", tool("Grep", {"pattern": "hello", "path": SCRIPT, "output_mode": "content"}).action, "allow")
        check("GLOB key material", tool("Glob", {"pattern": "*", "path": os.path.join(HOME, ".ssh")}).action, "deny")

        # ---- PII layer on tool decisions (2026-10-26) ----------------------------------
        global _pii_cols
        saved_cols, _pii_cols = _pii_cols, {"email": "email", "phone": "phone", "first_name": "name"}
        PO = os.path.join(fix, "leads.out")
        with open(PO, "w") as f:
            f.write("merchant_id\tfirst_name\temail\n600030031\tJane\tjane.dummy@example.com\n")
        CO = os.path.join(fix, "counts.out")
        with open(CO, "w") as f:
            f.write("merchant_id\tunits\n600030031\t42\n")
        MD = os.path.join(fix, "notes.md")
        with open(MD, "w") as f:
            f.write("Ping jane.dummy@example.com about the feed.\n")
        r = tool("Read", {"file_path": PO})
        check("PII READ .out with email is redacted", r.action, "rewrite")
        if r.action == "rewrite":
            with open(r.payload["updatedInput"]["file_path"]) as f:
                body = f.read()
            check("PII READ copy hides the email", "jane.dummy@example.com" in body, False)
            check("PII READ copy keeps the merchant id", "600030031" in body, True)
        check("CONTROL PII READ clean .out", tool("Read", {"file_path": CO}).action, "allow")
        check("CONTROL PII READ prose .md is not tabular", tool("Read", {"file_path": MD}).action, "allow")
        b = tool("Bash", {"command": f"sed -n 2,5p {PO}"})
        check("PII BASH sed of a .out with PII columns is denied toward Read", b.action == "deny" and "Read" in b.message, True)
        check("PII GREP content on a .out with PII columns is denied",
              tool("Grep", {"pattern": "Jane", "path": PO, "output_mode": "content"}).action, "deny")
        check("PII READ of the HMAC key is denied", tool("Read", {"file_path": PII_KEY_PATH}).action, "deny")
        b = tool("Bash", {"command": f"sed -n 1,5p {CO}"})
        check("CONTROL PII BASH clean .out is not wrapped", b.action, "allow")
        _pii_cols = saved_cols

        def prompt(p):
            return evaluate({"hook_event_name": "UserPromptSubmit", "prompt": p, "cwd": fix}).action

        if env_ok:
            check("PROMPT @.env", prompt("revisa @.env y dime"), "deny")
        check("PROMPT @config", prompt("mira @config/app-config.php"), "deny")
        check("PROMPT @clean", prompt("mira @config/clean.php"), "allow")
        check("PROMPT @ssh", prompt("lee @~/.ssh/config"), "deny")
        check("PROMPT pasted token", prompt("usa ghp_DUMMYabcdefghijklmnopqrstuvwxyz0123456789"), "deny")
        check("PROMPT plain", prompt("añade un test al servicio de facturas"), "allow")
        check("PROMPT email is fine", prompt("escribe a user@example.com"), "allow")

    finally:
        import shutil
        shutil.rmtree(tmp, ignore_errors=True)

    # ---- filter mode round trip ---------------------------------------------------------
    proxy_url = "http://user:DUMMYPX01@localhost:54361"  # redact:allow
    masked, _ = redact_text("HTTP_PROXY=" + proxy_url + "\n", hashes=set())
    check("FILTER masks proxy userinfo", "DUMMYPX01" not in masked, True)

    out, rules = redact_text("DB_PASSWORD=DUMMYF01\nok line\n", hashes=set())
    check("FILTER masks and keeps lines", out == "DB_PASSWORD=***\nok line\n", True)

    # ---- PII layer: MASK (value must vanish) / KEEP (byte-identical) ---------------------
    # Synthetic values only: example.com mail, the 555-01xx fiction range, the public test
    # card 4111..., TEST-NET-3 addresses. The keep set is H1 of the task plan: working data.
    cols = {"email": "email", "phone": "phone", "first_name": "name", "ssn": "natid", "dob": "birth",
            "card_number": "card"}
    pii_mask = [
        ("email value", "contact jane.dummy@example.com today", "jane.dummy@example.com"),
        ("email plus tag", "x jane+feed@sub.example.com y", "jane+feed@sub.example.com"),
        ("ssn dashed", "ssn 234-56-7890 on file", "234-56-7890"),
        ("phone parens", "call (312) 555-0142", "555-0142"),
        ("phone dashed +1", "call +1 312-555-0142", "312-555-0142"),
        ("phone dotted", "call 312.555.0142", "312.555.0142"),
        ("phone e164", '{"Caller": "+13125550142"}', "3125550142"),
        ("phone parens joined", "call (312) 5550142", "5550142"),
        ("card luhn spaced", "card 4111 1111 1111 1111 ok", "4111 1111 1111"),
        ("column bare card", "id\tcard_number\n7\t4111111111111111\n", "411111111111"),
        ("single column under a known delimiter", "\x00TSVfirst_name\nJanedummy\n", "Janedummy"),
        ("stray comma line keeps the header", "id\tfirst_name\n1\tAnadummy\nNote: a, b\n2\tBobdummy\n", "Bobdummy"),
        ("public ipv4", "from 203.0.113.7 at noon", "203.0.113.7"),
        ("column email", "merchant_id\temail\n600030031\tjane.dummy@example.com\n", "jane.dummy@example.com"),
        ("column bare phone", "merchant_id\tphone\n600030031\t3125550142\n", "3125550142"),
        ("column first name", "first_name\tlast_seen\nJanedummy\t2026-10-26\n", "Janedummy"),
        ("column undashed ssn", "id\tssn\n7\t234567890\n", "234567890"),
        ("column csv quoted", 'merchant_id,first_name,email\n1,"Doe, Janedummy",x@example.com\n', "Janedummy"),
        ("vertical \\G", "*** 1. row ***\n  first_name: Janedummy\n", "Janedummy"),
        ("vertical under a known delimiter", "\x00TSV*** 1. row ***\n  first_name: Janedummy\n", "Janedummy"),
        ("dob keeps year only", "id\tdob\n7\t1984-03-09\n", "03-09"),
    ]
    pii_keep = [
        ("merchant id", "merchant_id 600030031\n"),
        ("vin", "vin 1FTFW1ET5DFC10312\n"),
        ("stock", "stock SP7X16-5334\n"),
        ("price and date", "price 12999.00 created_at 2026-10-26 08:31:00\n"),
        ("epoch seconds and ms", "t=1729900000 t_ms=1729900000123\n"),
        ("private ipv4", "host 10.4.1.39 and 192.168.1.20 and 127.0.0.1\n"),
        ("version string", "Server version 8.0.44\n"),
        ("card fails luhn", "ref 4111 1111 1111 1112\n"),
        ("bare 16-digit id", "listing 4000000000000002\n"),
        ("asset name with @", "img/logo@2x.png\n"),
        ("git scp remote", "git@bitbucket.org:team/repo.git\n"),
        ("four-part version", "version 5.7.44.1\n"),
        ("ssn invalid area 666", "ref 666-12-3456\n"),
        ("ssn invalid group 00", "ref 234-00-4567\n"),
        ("phone exchange 1xx", "ref 312-155-0142\n"),
        ("bare 10 digits outside a phone column", "ref 3125550142\n"),
        ("11 digits with no plus", "ref 13125550142\n"),
        ("dashed area with joined rest", "ref 312-5550142\n"),
        ("NULL in a pii column", "id\temail\n7\tNULL\n"),
        ("unlisted column name", "id\tname\n7\tGoodrich Listings\n"),
        ("prose with commas", "Calo said: first, check the feed, then the queue.\n"),
        ("information_schema column names",
         "TABLE_NAME\tCOLUMN_NAME\tDATA_TYPE\nusers\tusername\tvarchar\nusers\tid\tint\nleads\tphone\tvarchar\nleads\tstatus\tvarchar\n"),
    ]
    for name, body, token in pii_mask:
        tsv = body.startswith("\x00TSV")
        out, _ = pii_redact(body[4:] if tsv else body, cols, delim="\t" if tsv else None)
        check(f"PII MASK {name}", token not in out, True)
    for name, body in pii_keep:
        out, _ = pii_redact(body, cols)
        check(f"PII KEEP {name}", out == body, True)
    a, _ = pii_redact("x jane.dummy@example.com", cols)
    b, _ = pii_redact("y JANE.DUMMY@example.com", cols)
    c, _ = pii_redact("x john.dummy@example.com", cols)
    check("PII token is deterministic and case-folded", a[2:] == b[2:], True)
    check("CONTROL PII token differs for another value", a[2:] == c[2:], False)
    check("PII card keeps the last four", pii_redact("card 4111 1111 1111 1111", cols)[0], "card ****1111")
    check("CONTROL credential layer alone keeps a PII email",
          "jane.dummy@example.com" in redact_text("contact jane.dummy@example.com", hashes=set())[0], True)

    # ---- people rule (Jira, Bitbucket): REWRITE to the expected form / KEEP byte-identical ---
    acct = "5b10a2844c20165700ede21f"
    people_cases = [
        ("json displayName", '"displayName": "Janedummy Q Smithers",', '"displayName": "Janedummy",'),
        ("json emailAddress", '"emailAddress": "jane.dummy@example.com",', '"emailAddress": "<removed>",'),
        ("json comment author", '{"author": "Janedummy Smithers", "body": "ok"}', '{"author": "Janedummy", "body": "ok"}'),
        ("name with email", "Signed-off: Janedummy de Tal <jane.dummy@example.com>", "Signed-off: Janedummy"),
        ("plain label", "Assignee: Janedummy Smithers", "Assignee: Janedummy"),
        ("bitbucket display_name", '"display_name": "Janedummy Smithers"', '"display_name": "Janedummy"'),
        ("bitbucket commit raw", '"raw": "Janedummy Smithers <jane.dummy@example.com>"', '"raw": "Janedummy"'),
        ("json escaped quote", '"displayName": "Janedummy \\"JJ\\" Smithers"', '"displayName": "Janedummy"'),
        ("lowercase name with email", "Author: janedummy smithers <j@example.com>", "Author: janedummy"),
        ("git blame", "4f1c2e9a (Janedummy Smithers 2026-10-01 12:00:00 +0000 12) code",
         "4f1c2e9a (Janedummy 2026-10-01 12:00:00 +0000 12) code"),
        ("git shortlog", "Janedummy Smithers (12):\n      fix the feed\n", "Janedummy (12):\n      fix the feed\n"),
        ("git shortlog -sn", "    12\tJanedummy Smithers", "    12\tJanedummy"),
        ("box table author", "│ ID  │ Author            │ Body │\n│ 7   │ Janedummy Smithers │ hi   │",
         "│ ID  │ Author            │ Body │\n│ 7   │ Janedummy          │ hi   │"),
    ]
    for name, body, want in people_cases:
        check(f"PEOPLE {name}", people_redact(body)[0], want)
    out, _ = people_redact(f'"accountId": "{acct}"')
    check("PEOPLE accountId becomes a token", acct not in out and "<acct:" in out, True)
    out, _ = people_redact('"account_id": "557058:0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b", "nickname": "jsmithdummy"')
    check("PEOPLE bitbucket account_id and nickname become tokens",
          "557058:" not in out and "jsmithdummy" not in out and "<user:" in out, True)
    people_keep = [
        ("status label", "Status: In Progress\n"),
        ("entity name key", '"name": "Goodrich Listings"\n'),
        ("ticket key and git sha", "TICKET-1442 at 4f1c2e9a7b3d5e6f8a9b0c1d2e3f4a5b6c7d8e9f\n"),
        ("prose with Author", "The Author field is empty in Nova.\n"),
        ("prose lines are not shortlog", "Fix the feed queue\nRetry failed downloads\n"),
        ("single-word name", '"displayName": "Janedummy"\n'),
    ]
    for name, body in people_keep:
        check(f"PEOPLE KEEP {name}", people_redact(body)[0], body)
    for cmd in ("git log -5", "git -C /tmp/repo show HEAD", "git blame -e x.php", "git log --oneline --format=%an"):
        a = analyze_bash(cmd, "/tmp")
        check(f"PEOPLE BASH wraps '{cmd}'", a.people and bool(a.wrap), True)
    check("CONTROL PEOPLE BASH git status is not wrapped", analyze_bash("git status", "/tmp").people, False)
    check("CONTROL PEOPLE BASH git log --oneline is not wrapped", analyze_bash("git log --oneline -5", "/tmp").people, False)
    check("CONTROL pii layer alone keeps a full displayName",
          "Smithers" in pii_redact('"displayName": "Janedummy Smithers"', {})[0], True)

    print("-" * 72)
    print(f"passed {total - len(failures)}   failed {len(failures)}   total {total}")
    for f in failures:
        print("  FAIL:", f)
    return 0 if not failures else 1


# ----------------------------------------------------------------------------------------
# main
# ----------------------------------------------------------------------------------------

def main() -> int:
    argv = sys.argv[1:]
    if argv[:1] == ["--filter"]:
        return cli_filter(pii="--pii" in argv[1:], people="--people" in argv[1:], tsv="--tsv" in argv[1:])
    if argv[:1] == ["--redact"] and len(argv) == 2:
        return cli_redact(argv[1])
    if argv[:1] == ["--check"]:
        return cli_check(argv[1:])
    if argv[:1] == ["self-test"]:
        return self_test()
    if argv[:1] == ["--version"]:
        print(VERSION)
        return 0
    data = json.load(sys.stdin)
    return emit(evaluate(data), data)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except SystemExit:
        raise
    except Exception as exc:  # fail closed, never echo data
        mode = sys.argv[1] if len(sys.argv) > 1 else "hook"
        if mode == "--filter":
            sys.stdout.write("[secret-guard: filter error, output withheld]\n")
        sys.stderr.write(f"BLOCKED by secret-guard: internal error ({type(exc).__name__}); call blocked.\n")
        sys.exit(2)
