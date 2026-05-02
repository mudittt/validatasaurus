#!/usr/bin/env python3
"""
SQL Syntax Checker
==================
A user-friendly SQL syntax analyser that checks .sql files for common
syntax mistakes and displays results clearly so that anyone — even
non-technical users — can review them.

Usage:
    python sql_checker.py <path_to_sql_file>

    Or import and call:
        from sql_checker import check_sql_file
        check_sql_file("/path/to/file.sql")
"""

import re
import json
import sys
import os
from dataclasses import dataclass, field
from enum import Enum
from typing import Optional


# ──────────────────────────────────────────────────────────────────────
# Data structures
# ──────────────────────────────────────────────────────────────────────

class Severity(Enum):
    ERROR = "ERROR"
    WARNING = "WARNING"
    INFO = "INFO"


@dataclass
class Issue:
    severity: Severity
    line_number: int
    column: Optional[int]
    phrase: str
    message: str
    suggestion: str = ""


@dataclass
class StatementInfo:
    """Metadata for a single SQL statement extracted from the file."""
    text: str
    start_line: int
    end_line: int
    line_offset: int  # char offset of the first line in the file


@dataclass
class Report:
    file_path: str
    total_statements: int = 0
    issues: list = field(default_factory=list)


# ──────────────────────────────────────────────────────────────────────
# Helpers
# ──────────────────────────────────────────────────────────────────────

def _line_col_at(text: str, pos: int):
    """Return (1-based line, 1-based column) for a character position."""
    line = text.count("\n", 0, pos) + 1
    last_nl = text.rfind("\n", 0, pos)
    col = pos - last_nl  # 1-based
    return line, col


def _snippet(text: str, pos: int, context: int = 40) -> str:
    """Return a short snippet around *pos* for display."""
    start = max(0, pos - context // 2)
    end = min(len(text), pos + context // 2)
    raw = text[start:end].replace("\n", "↵")
    if start > 0:
        raw = "…" + raw
    if end < len(text):
        raw = raw + "…"
    return raw


def _sql_unescape(s: str) -> str:
    """Unescape a MySQL single-quoted string (backslash escapes + doubled quotes)."""
    result = []
    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            result.append(s[i + 1])
            i += 2
        elif s[i] == "'" and i + 1 < len(s) and s[i + 1] == "'":
            result.append("'")
            i += 2
        else:
            result.append(s[i])
            i += 1
    return ''.join(result)


# ──────────────────────────────────────────────────────────────────────
# Splitting the file into individual statements
# ──────────────────────────────────────────────────────────────────────

def split_statements(sql_text: str) -> list[StatementInfo]:
    """
    Split raw SQL text on top-level semicolons (ignoring semicolons
    inside single-quoted strings or backtick identifiers) and return
    StatementInfo objects.

    Double-quoted regions are intentionally NOT tracked here. MySQL uses
    `'` for string literals and `"` for identifiers (under ANSI_QUOTES);
    `;` essentially never appears inside an identifier. Tracking `"`
    here breaks splitting on CSV-exported files where each statement is
    wrapped in `"…"` — the closing/opening `"` between blocks would
    leave the parser inside a "string" across the `;` boundary, gluing
    statements together. `check_csv_wrapped_statements` and
    `check_unmatched_quotes` still flag `"`-related issues independently.
    """
    statements: list[StatementInfo] = []
    current_start = 0
    i = 0
    in_single = False
    in_backtick = False
    length = len(sql_text)

    while i < length:
        ch = sql_text[i]

        # Handle escape sequences inside single-quoted strings
        if in_single:
            if ch == "\\" and i + 1 < length:
                i += 2
                continue
            if ch == "'" and i + 1 < length and sql_text[i + 1] == "'":
                i += 2
                continue
            if ch == "'":
                in_single = False
            i += 1
            continue

        if in_backtick:
            if ch == '`':
                in_backtick = False
            i += 1
            continue

        if ch == "'":
            in_single = True
        elif ch == '`':
            in_backtick = True
        elif ch == '-' and i + 1 < length and sql_text[i + 1] == '-':
            # Line comment — skip to end of line
            nl = sql_text.find("\n", i)
            i = nl if nl != -1 else length
            continue
        elif ch == '#':
            # MySQL line comment
            nl = sql_text.find("\n", i)
            i = nl if nl != -1 else length
            continue
        elif ch == '/' and i + 1 < length and sql_text[i + 1] == '*':
            # Block comment
            end = sql_text.find("*/", i + 2)
            i = end + 2 if end != -1 else length
            continue
        elif ch == ';':
            stmt_text = sql_text[current_start:i + 1].strip()
            if stmt_text and stmt_text != ";":
                start_line = sql_text.count("\n", 0, current_start) + 1
                end_line = sql_text.count("\n", 0, i) + 1
                statements.append(StatementInfo(
                    text=stmt_text,
                    start_line=start_line,
                    end_line=end_line,
                    line_offset=current_start,
                ))
            current_start = i + 1

        i += 1

    # Check for trailing text without a semicolon
    leftover = sql_text[current_start:].strip()
    if leftover:
        start_line = sql_text.count("\n", 0, current_start) + 1
        end_line = sql_text.count("\n") + 1
        statements.append(StatementInfo(
            text=leftover,
            start_line=start_line,
            end_line=end_line,
            line_offset=current_start,
        ))

    return statements


# ──────────────────────────────────────────────────────────────────────
# Individual checks
# ──────────────────────────────────────────────────────────────────────

def check_unmatched_quotes(sql_text: str, file_lines: list[str]) -> list[Issue]:
    """Detect unmatched single-quotes, double-quotes, and backticks."""
    issues: list[Issue] = []

    # We scan once, tracking which quote-type we are inside.
    # When inside one quote-type, other quote characters are ignored.
    i = 0
    length = len(sql_text)
    in_single = False
    in_double = False
    in_backtick = False
    open_pos = -1

    while i < length:
        ch = sql_text[i]

        # --- skip comments (only when outside all quotes) ---
        if not in_single and not in_double and not in_backtick:
            if ch == '-' and i + 1 < length and sql_text[i + 1] == '-':
                nl = sql_text.find("\n", i)
                i = nl + 1 if nl != -1 else length
                continue
            if ch == '#':
                nl = sql_text.find("\n", i)
                i = nl + 1 if nl != -1 else length
                continue
            if ch == '/' and i + 1 < length and sql_text[i + 1] == '*':
                end = sql_text.find("*/", i + 2)
                i = end + 2 if end != -1 else length
                continue

        # --- inside single-quoted string ---
        if in_single:
            if ch == '\\' and i + 1 < length:
                i += 2
                continue
            if ch == "'" and i + 1 < length and sql_text[i + 1] == "'":
                i += 2
                continue
            if ch == "'":
                in_single = False
            i += 1
            continue

        # --- inside double-quoted string ---
        if in_double:
            if ch == '\\' and i + 1 < length:
                i += 2
                continue
            if ch == '"':
                in_double = False
            i += 1
            continue

        # --- inside backtick identifier ---
        if in_backtick:
            if ch == '`':
                in_backtick = False
            i += 1
            continue

        # --- outside all quotes: detect opening ---
        if ch == "'":
            in_single = True
            open_pos = i
        elif ch == '"':
            in_double = True
            open_pos = i
        elif ch == '`':
            in_backtick = True
            open_pos = i

        i += 1

    # If we ended still inside a quote, report it
    if in_single:
        ln, col = _line_col_at(sql_text, open_pos)
        issues.append(Issue(
            severity=Severity.ERROR,
            line_number=ln,
            column=col,
            phrase=_snippet(sql_text, open_pos, 60),
            message="Unmatched single-quote (') — opened here but never closed.",
            suggestion="Add a matching closing single-quote or remove this one.",
        ))
    if in_double:
        ln, col = _line_col_at(sql_text, open_pos)
        issues.append(Issue(
            severity=Severity.ERROR,
            line_number=ln,
            column=col,
            phrase=_snippet(sql_text, open_pos, 60),
            message='Unmatched double-quote (") — opened here but never closed.',
            suggestion='Add a matching closing double-quote or remove this one.',
        ))
    if in_backtick:
        ln, col = _line_col_at(sql_text, open_pos)
        issues.append(Issue(
            severity=Severity.ERROR,
            line_number=ln,
            column=col,
            phrase=_snippet(sql_text, open_pos, 60),
            message="Unmatched backtick (`) — opened here but never closed.",
            suggestion="Add a matching closing backtick or remove this one.",
        ))

    return issues


def check_unmatched_brackets(sql_text: str) -> list[Issue]:
    """Check for unmatched parentheses, square brackets, and curly braces."""
    issues: list[Issue] = []
    pairs = {'(': ')', '[': ']', '{': '}'}
    closing = {v: k for k, v in pairs.items()}
    stack: list[tuple[str, int]] = []

    in_single = False
    in_double = False
    in_backtick = False
    i = 0
    length = len(sql_text)

    while i < length:
        ch = sql_text[i]

        # String / identifier tracking (simplified)
        if in_single:
            if ch == '\\':
                i += 2
                continue
            if ch == "'" and i + 1 < length and sql_text[i + 1] == "'":
                i += 2
                continue
            if ch == "'":
                in_single = False
            i += 1
            continue
        if in_double:
            if ch == '"':
                in_double = False
            i += 1
            continue
        if in_backtick:
            if ch == '`':
                in_backtick = False
            i += 1
            continue

        if ch == "'":
            in_single = True
        elif ch == '"':
            in_double = True
        elif ch == '`':
            in_backtick = True
        elif ch == '-' and i + 1 < length and sql_text[i + 1] == '-':
            nl = sql_text.find("\n", i)
            i = nl + 1 if nl != -1 else length
            continue
        elif ch == '#':
            nl = sql_text.find("\n", i)
            i = nl + 1 if nl != -1 else length
            continue
        elif ch == '/' and i + 1 < length and sql_text[i + 1] == '*':
            end = sql_text.find("*/", i + 2)
            i = end + 2 if end != -1 else length
            continue
        elif ch in pairs:
            stack.append((ch, i))
        elif ch in closing:
            expected_open = closing[ch]
            if stack and stack[-1][0] == expected_open:
                stack.pop()
            else:
                ln, col = _line_col_at(sql_text, i)
                if stack:
                    issues.append(Issue(
                        severity=Severity.ERROR,
                        line_number=ln,
                        column=col,
                        phrase=_snippet(sql_text, i),
                        message=f"Unexpected closing '{ch}' — expected closing '{pairs[stack[-1][0]]}' first.",
                        suggestion="Check that every opening bracket has a matching close in the right order.",
                    ))
                else:
                    issues.append(Issue(
                        severity=Severity.ERROR,
                        line_number=ln,
                        column=col,
                        phrase=_snippet(sql_text, i),
                        message=f"Unexpected closing '{ch}' with no matching opening bracket.",
                        suggestion=f"Remove this '{ch}' or add a matching opening '{expected_open}' before it.",
                    ))
        i += 1

    for open_ch, pos in stack:
        ln, col = _line_col_at(sql_text, pos)
        issues.append(Issue(
            severity=Severity.ERROR,
            line_number=ln,
            column=col,
            phrase=_snippet(sql_text, pos),
            message=f"Opening '{open_ch}' is never closed with '{pairs[open_ch]}'.",
            suggestion=f"Add a matching '{pairs[open_ch]}' somewhere after this point.",
        ))

    return issues


def check_statement_structure(stmt: StatementInfo) -> list[Issue]:
    """
    Lightweight structural checks per statement:
    - Recognised statement type (UPDATE, INSERT, SELECT, DELETE, etc.)
    - UPDATE must have SET and WHERE
    - INSERT must have INTO and VALUES / SET / SELECT
    - Missing trailing semicolon
    """
    issues: list[Issue] = []
    text = stmt.text.strip()
    upper = text.upper()

    # Remove leading comments to find the keyword
    cleaned = re.sub(r'--[^\n]*', '', text)
    cleaned = re.sub(r'#[^\n]*', '', cleaned)
    cleaned = re.sub(r'/\*.*?\*/', '', cleaned, flags=re.DOTALL)
    cleaned = cleaned.strip()
    first_word = cleaned.split()[0].upper().strip('`"') if cleaned.split() else ""

    recognised = {"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER",
                   "DROP", "TRUNCATE", "REPLACE", "SET", "USE", "SHOW",
                   "DESCRIBE", "EXPLAIN", "GRANT", "REVOKE", "BEGIN",
                   "COMMIT", "ROLLBACK", "CALL", "MERGE", "WITH", "LOCK",
                   "UNLOCK", "RENAME", "LOAD", "START", "FLUSH"}

    if first_word and first_word not in recognised:
        issues.append(Issue(
            severity=Severity.WARNING,
            line_number=stmt.start_line,
            column=1,
            phrase=first_word,
            message=f"Statement starts with unrecognised keyword '{first_word}'.",
            suggestion="Expected a SQL keyword like SELECT, INSERT, UPDATE, DELETE, etc.",
        ))

    # ----- UPDATE checks -----
    if first_word == "UPDATE":
        # Must contain SET
        if not _keyword_outside_strings(upper, text, "SET"):
            issues.append(Issue(
                severity=Severity.ERROR,
                line_number=stmt.start_line,
                column=1,
                phrase=text[:80] + "…" if len(text) > 80 else text,
                message="UPDATE statement is missing the SET keyword.",
                suggestion="An UPDATE statement must have: UPDATE <table> SET <column>=<value> WHERE …",
            ))
        # Should contain WHERE (warning, not error — some mass updates are intentional)
        if not _keyword_outside_strings(upper, text, "WHERE"):
            issues.append(Issue(
                severity=Severity.WARNING,
                line_number=stmt.start_line,
                column=1,
                phrase=text[:80] + "…" if len(text) > 80 else text,
                message="UPDATE statement has no WHERE clause — this would update ALL rows.",
                suggestion="Add a WHERE clause to limit which rows are updated, e.g. WHERE id = 123.",
            ))

    # ----- INSERT checks -----
    if first_word == "INSERT":
        if not _keyword_outside_strings(upper, text, "INTO"):
            issues.append(Issue(
                severity=Severity.ERROR,
                line_number=stmt.start_line,
                column=1,
                phrase=text[:80] + "…" if len(text) > 80 else text,
                message="INSERT statement is missing the INTO keyword.",
                suggestion="Use: INSERT INTO <table> (columns) VALUES (…)",
            ))

    # ----- Missing semicolon -----
    if not text.endswith(";"):
        issues.append(Issue(
            severity=Severity.WARNING,
            line_number=stmt.end_line,
            column=None,
            phrase=text[-60:] if len(text) > 60 else text,
            message="Statement does not end with a semicolon (;).",
            suggestion="Add a semicolon at the end of the statement.",
        ))

    return issues


def _keyword_outside_strings(upper_text: str, raw_text: str, keyword: str) -> bool:
    """
    Return True if *keyword* appears in *upper_text* outside of quoted
    strings.  Simplistic but effective for the patterns we see.
    """
    pattern = rf'\b{keyword}\b'
    for m in re.finditer(pattern, upper_text):
        pos = m.start()
        # Count unescaped quotes before this position
        before = raw_text[:pos]
        # Rough check: if the number of unescaped single-quotes is even,
        # we are outside a string.
        count = 0
        j = 0
        while j < len(before):
            if before[j] == '\\':
                j += 2
                continue
            if before[j] == "'":
                count += 1
            j += 1
        if count % 2 == 0:
            return True
    return False


def check_json_in_strings(stmt: StatementInfo) -> list[Issue]:
    """
    For large SET values that look like JSON (start with { or [), try to
    parse them and report JSON syntax errors with precise locations.
    """
    issues: list[Issue] = []
    # Find patterns like SET `col` = '{...}' or SET `col` = '[...]'
    # We look for = '<json>' patterns
    text = stmt.text
    i = 0
    length = len(text)

    while i < length:
        # Find = '{ or = '[
        if text[i] == '=' and i + 1 < length:
            # Skip whitespace
            j = i + 1
            while j < length and text[j] in ' \t\n\r':
                j += 1
            if j < length and text[j] == "'":
                # Check if next non-whitespace inside the string is { or [
                k = j + 1
                while k < length and text[k] in ' \t\n\r':
                    k += 1
                if k < length and text[k] in ('{', '['):
                    # Find the end of this single-quoted string
                    str_start = j + 1
                    m = j + 1
                    while m < length:
                        if text[m] == '\\':
                            m += 2
                            continue
                        if text[m] == "'" and m + 1 < length and text[m + 1] == "'":
                            m += 2
                            continue
                        if text[m] == "'":
                            break
                        m += 1
                    if m < length:
                        json_str = text[str_start:m]
                        # Unescape SQL escapes for JSON parsing
                        json_str_clean = _sql_unescape(json_str)
                        try:
                            json.loads(json_str_clean)
                        except json.JSONDecodeError as e:
                            abs_pos = str_start + e.pos if e.pos else str_start
                            ln, col = _line_col_at(text, min(abs_pos, length - 1))
                            ln = ln + stmt.start_line - 1

                            # Get a useful snippet around the error
                            err_start = max(0, (e.pos or 0) - 30)
                            err_end = min(len(json_str_clean), (e.pos or 0) + 30)
                            err_snippet = json_str_clean[err_start:err_end].replace("\n", "↵")

                            issues.append(Issue(
                                severity=Severity.ERROR,
                                line_number=ln,
                                column=col,
                                phrase=err_snippet,
                                message=f"Invalid JSON inside string value: {e.msg} (position {e.pos}).",
                                suggestion="Check for missing commas, extra commas, unmatched braces/brackets, or malformed values in the JSON.",
                            ))
                        i = m + 1
                        continue
        i += 1

    return issues


def check_common_typos(sql_text: str) -> list[Issue]:
    """Flag common SQL keyword typos outside of quoted strings."""
    issues: list[Issue] = []
    typos = {
        r'\bUPDATE\s+SET\b': ("Missing table name", "UPDATE should be followed by a table name before SET."),
        r'\bWHERE\s*;': ("Empty WHERE clause", "WHERE clause has no conditions."),
        r'\bSET\s*WHERE\b': ("Missing SET assignments", "SET must have column=value assignments before WHERE."),
        r'\bLIMIT\s+[^0-9]': ("Invalid LIMIT value", "LIMIT should be followed by a number."),
        r'\bVALUES\s*\(\s*\)': ("Empty VALUES", "VALUES() is empty — provide values to insert."),
    }
    for pattern, (title, suggestion) in typos.items():
        for m in re.finditer(pattern, sql_text, re.IGNORECASE):
            pos = m.start()
            # Check we're outside strings (rough)
            before = sql_text[:pos]
            sq = before.count("'") - before.count("\\'")
            if sq % 2 != 0:
                continue
            ln, col = _line_col_at(sql_text, pos)
            issues.append(Issue(
                severity=Severity.ERROR,
                line_number=ln,
                column=col,
                phrase=m.group(),
                message=title,
                suggestion=suggestion,
            ))
    return issues


def check_unescaped_apostrophes(sql_text: str) -> list[Issue]:
    """
    Detect apostrophes inside string literals that are not escaped.

    In MySQL, an apostrophe inside a single-quoted string must be either
    doubled ('') or backslash-escaped (\\').  An apostrophe sitting between
    two letters (e.g. ``o'clock``, ``Year's``, ``D'Oyly``) prematurely
    terminates the string and the rest is parsed as raw SQL — producing
    cryptic errors at execution time.

    This check works on the raw text rather than the statement splitter
    output, because the splitter itself is thrown off by exactly this bug.
    The pattern ``[A-Za-z]'[A-Za-z]`` is unambiguous: in legitimate SQL a
    single-quote is always preceded or followed by a non-letter
    (whitespace, ``=``, ``,``, ``)``, ``;``, digit, or another ``'``).
    """
    issues: list[Issue] = []
    # Strip comments first so apostrophes in -- / # / /* */ comments aren't flagged.
    pattern = re.compile(r"[A-Za-z]'[A-Za-z]")
    for m in pattern.finditer(sql_text):
        pos = m.start() + 1  # point at the apostrophe itself
        # Skip if this position falls inside a SQL line/block comment
        if _pos_inside_comment(sql_text, pos):
            continue
        ln, col = _line_col_at(sql_text, pos)
        issues.append(Issue(
            severity=Severity.ERROR,
            line_number=ln,
            column=col,
            phrase=_snippet(sql_text, pos, 60),
            message="Unescaped apostrophe (') inside a string literal — will fail to execute.",
            suggestion="Double the apostrophe ('') or backslash-escape it (\\') so the string isn't terminated mid-word.",
        ))
    return issues


def check_csv_wrapped_statements(sql_text: str) -> list[Issue]:
    """
    Detect SQL statements wrapped in outer double-quotes — a common
    artefact of pasting from a CSV / Excel export.  The pattern looks like::

        "UPDATE `t` SET `c` = '...' WHERE id = 1;"

    The leading and trailing ``"`` make the whole thing not a valid SQL
    statement.
    """
    issues: list[Issue] = []
    pattern = re.compile(
        r'^\s*"\s*(UPDATE|INSERT|DELETE|SELECT|REPLACE|CREATE|ALTER|DROP|MERGE|TRUNCATE)\b',
        re.IGNORECASE | re.MULTILINE,
    )
    for m in pattern.finditer(sql_text):
        pos = m.start()
        ln, col = _line_col_at(sql_text, pos)
        issues.append(Issue(
            severity=Severity.ERROR,
            line_number=ln,
            column=col,
            phrase=_snippet(sql_text, m.start(1), 60),
            message='Statement is wrapped in outer double-quotes ("…") — not valid SQL.',
            suggestion='Remove the surrounding " characters; this usually comes from a CSV/Excel export.',
        ))
    return issues


def check_doubled_double_quotes(sql_text: str) -> list[Issue]:
    """
    Detect ``""`` (two double-quotes in a row).  Inside CSV-exported text
    this is the escape for an embedded ``"``, but in raw SQL the inner
    string content is normally unquoted or single-quoted, so ``""`` almost
    always means the file came from a CSV export and the data has been
    corrupted.

    Reported once per line to avoid drowning the user in matches when a
    single statement contains many of them.
    """
    issues: list[Issue] = []
    seen_lines: set[int] = set()
    for m in re.finditer(r'""', sql_text):
        pos = m.start()
        ln, col = _line_col_at(sql_text, pos)
        if ln in seen_lines:
            continue
        seen_lines.add(ln)
        issues.append(Issue(
            severity=Severity.WARNING,
            line_number=ln,
            column=col,
            phrase=_snippet(sql_text, pos, 60),
            message='Found "" (doubled double-quote) — likely a CSV-escaped " that should be a single ".',
            suggestion='Replace "" with " — this usually means the SQL was pasted from a CSV/Excel export.',
        ))
    return issues


def check_invalid_html_br(sql_text: str) -> list[Issue]:
    """
    Flag the invalid ``</br>`` HTML tag.  ``<br>`` is a void element and
    has no closing form; the correct alternatives are ``<br>`` or
    ``<br/>``.  Storing ``</br>`` in content is a data-quality issue
    even though the SQL itself parses fine.
    """
    issues: list[Issue] = []
    seen_lines: set[int] = set()
    for m in re.finditer(r'</br\s*>', sql_text, re.IGNORECASE):
        pos = m.start()
        ln, col = _line_col_at(sql_text, pos)
        if ln in seen_lines:
            continue
        seen_lines.add(ln)
        issues.append(Issue(
            severity=Severity.WARNING,
            line_number=ln,
            column=col,
            phrase=_snippet(sql_text, pos, 40),
            message="Invalid </br> HTML tag — <br> is a void element with no closing form.",
            suggestion="Replace </br> with <br> or <br/>.",
        ))
    return issues


def _pos_inside_comment(sql_text: str, pos: int) -> bool:
    """Cheap check: is *pos* inside a -- / # line comment or /* */ block comment?"""
    # Find the start of the line containing pos
    line_start = sql_text.rfind("\n", 0, pos) + 1
    line_prefix = sql_text[line_start:pos]
    # Line comments
    if "--" in line_prefix or "#" in line_prefix:
        return True
    # Block comment: count /* and */ before pos
    opens = sql_text.count("/*", 0, pos)
    closes = sql_text.count("*/", 0, pos)
    return opens > closes


def check_duplicate_keywords(stmt: StatementInfo) -> list[Issue]:
    """Warn if certain keywords appear more than once at the top level."""
    issues: list[Issue] = []
    text = stmt.text
    upper = text.upper()

    for kw in ("SET", "WHERE", "LIMIT"):
        positions = []
        for m in re.finditer(rf'\b{kw}\b', upper):
            pos = m.start()
            before = text[:pos]
            sq = 0
            depth = 0
            j = 0
            while j < len(before):
                if before[j] == '\\':
                    j += 2
                    continue
                if before[j] == "'":
                    sq += 1
                elif sq % 2 == 0:
                    if before[j] == '(':
                        depth += 1
                    elif before[j] == ')':
                        depth -= 1
                j += 1
            if sq % 2 == 0 and depth == 0:
                positions.append(pos)
        if len(positions) > 1:
            for p in positions[1:]:
                ln, col = _line_col_at(text, p)
                ln = ln + stmt.start_line - 1
                issues.append(Issue(
                    severity=Severity.WARNING,
                    line_number=ln,
                    column=col,
                    phrase=_snippet(text, p),
                    message=f"Keyword '{kw}' appears more than once in this statement.",
                    suggestion=f"A statement usually has only one {kw} clause. Check if this is intentional.",
                ))

    return issues


# ──────────────────────────────────────────────────────────────────────
# Report display
# ──────────────────────────────────────────────────────────────────────

SEVERITY_SYMBOLS = {
    Severity.ERROR:   "❌ ERROR  ",
    Severity.WARNING: "⚠️  WARNING",
    Severity.INFO:    "ℹ️  INFO   ",
}

SEVERITY_COLORS = {
    Severity.ERROR:   "\033[91m",  # red
    Severity.WARNING: "\033[93m",  # yellow
    Severity.INFO:    "\033[96m",  # cyan
}
RESET = "\033[0m"
BOLD = "\033[1m"
DIM = "\033[2m"


def _supports_color() -> bool:
    return hasattr(sys.stdout, "isatty") and sys.stdout.isatty()


def print_report(report: Report):
    color = _supports_color()

    def c(code, text):
        return f"{code}{text}{RESET}" if color else text

    separator = "─" * 80
    print()
    print(c(BOLD, separator))
    print(c(BOLD, f"  SQL SYNTAX CHECK REPORT"))
    print(c(BOLD, separator))
    print(f"  File : {report.file_path}")
    print(f"  Statements found : {report.total_statements}")
    print(separator)

    if not report.issues:
        print()
        print(c("\033[92m", "  ✅ No syntax issues found! The SQL file looks good."))
        print()
        print(separator)
        return

    errors = [i for i in report.issues if i.severity == Severity.ERROR]
    warnings = [i for i in report.issues if i.severity == Severity.WARNING]
    infos = [i for i in report.issues if i.severity == Severity.INFO]

    print(f"  Found: {c(SEVERITY_COLORS[Severity.ERROR], f'{len(errors)} error(s)')}"
          f"  |  {c(SEVERITY_COLORS[Severity.WARNING], f'{len(warnings)} warning(s)')}"
          f"  |  {c(SEVERITY_COLORS[Severity.INFO], f'{len(infos)} info(s)')}")
    print(separator)

    # Sort by line number, then severity
    severity_order = {Severity.ERROR: 0, Severity.WARNING: 1, Severity.INFO: 2}
    sorted_issues = sorted(report.issues, key=lambda i: (i.line_number, severity_order[i.severity]))

    for idx, issue in enumerate(sorted_issues, 1):
        sev = SEVERITY_SYMBOLS[issue.severity]
        sev_color = SEVERITY_COLORS[issue.severity]

        loc = f"Line {issue.line_number}"
        if issue.column:
            loc += f", Column {issue.column}"

        print()
        print(f"  {c(sev_color, sev)}  #{idx}")
        print(f"  {c(BOLD, 'Location')}: {loc}")
        print(f"  {c(BOLD, 'Problem')} : {issue.message}")
        # Show the problematic phrase in a box
        phrase_display = issue.phrase[:120]
        if len(issue.phrase) > 120:
            phrase_display += "…"
        print(f"  {c(BOLD, 'Found')}   : {c(DIM, phrase_display)}")
        if issue.suggestion:
            print(f"  {c(BOLD, 'Fix')}     : {issue.suggestion}")

    print()
    print(separator)
    summary_color = SEVERITY_COLORS[Severity.ERROR] if errors else (
        SEVERITY_COLORS[Severity.WARNING] if warnings else "\033[92m")
    status = "FAILED" if errors else ("PASSED with warnings" if warnings else "PASSED")
    print(f"  {c(summary_color, f'RESULT: {status}')}")
    print(separator)
    print()


# ──────────────────────────────────────────────────────────────────────
# Main entry point
# ──────────────────────────────────────────────────────────────────────

def report_to_dict(report: Report) -> dict:
    return {
        "file_path": report.file_path,
        "total_statements": report.total_statements,
        "issues": [
            {
                "severity": i.severity.value,
                "line_number": i.line_number,
                "column": i.column,
                "phrase": i.phrase,
                "message": i.message,
                "suggestion": i.suggestion,
            }
            for i in report.issues
        ],
    }


def check_sql_file(file_path: str, print_output: bool = True) -> Report:
    """
    Analyse the SQL file at *file_path* and return a Report.

    Parameters
    ----------
    file_path : str
        Absolute or relative path to the .sql file to check.

    Returns
    -------
    Report
        Contains every issue found, with line numbers, phrases, and
        suggestions.
    """
    file_path = os.path.abspath(file_path)
    report = Report(file_path=file_path)

    if not os.path.isfile(file_path):
        report.issues.append(Issue(
            severity=Severity.ERROR,
            line_number=0,
            column=None,
            phrase=file_path,
            message="File not found.",
            suggestion="Check that the file path is correct.",
        ))
        print_report(report)
        return report

    with open(file_path, "r", encoding="utf-8", errors="replace") as f:
        sql_text = f.read()

    file_lines = sql_text.splitlines()

    # 1. Split into statements
    statements = split_statements(sql_text)
    report.total_statements = len(statements)

    # 2. File-level checks
    report.issues.extend(check_unmatched_quotes(sql_text, file_lines))
    report.issues.extend(check_unmatched_brackets(sql_text))
    report.issues.extend(check_common_typos(sql_text))
    report.issues.extend(check_unescaped_apostrophes(sql_text))
    report.issues.extend(check_csv_wrapped_statements(sql_text))
    report.issues.extend(check_doubled_double_quotes(sql_text))
    report.issues.extend(check_invalid_html_br(sql_text))

    # 3. Per-statement checks
    for stmt in statements:
        report.issues.extend(check_statement_structure(stmt))
        report.issues.extend(check_json_in_strings(stmt))
        report.issues.extend(check_duplicate_keywords(stmt))

    # De-duplicate issues at the same location with the same message
    seen = set()
    unique = []
    for issue in report.issues:
        key = (issue.line_number, issue.column, issue.message)
        if key not in seen:
            seen.add(key)
            unique.append(issue)
    report.issues = unique

    if print_output:
        print_report(report)
    return report


# ──────────────────────────────────────────────────────────────────────
# CLI
# ──────────────────────────────────────────────────────────────────────

if __name__ == "__main__":
    json_mode = "--json" in sys.argv
    args = [a for a in sys.argv[1:] if a != "--json"]
    if len(args) < 1:
        print("Usage: python sql_checker.py [--json] <path_to_sql_file>")
        sys.exit(1)

    report = check_sql_file(args[0], print_output=not json_mode)
    if json_mode:
        print(json.dumps(report_to_dict(report)))
    sys.exit(1 if any(i.severity == Severity.ERROR for i in report.issues) else 0)
