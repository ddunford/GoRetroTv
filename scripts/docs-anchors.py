#!/usr/bin/env python3
"""Bind doc sections to the code they describe, and fail when the code moves.

A doc section declares the code it is about:

    ## How orders settle

    <!-- anchor: src/orders/settlement.ts#^export class Settlement\b -->
    <!-- anchor: src/orders/types.ts -->
    <!-- fingerprint: sha256:4f3a...c1 @ 2026-09-22 -->

`check` recomputes the fingerprint and reports sections whose anchored code has
changed (STALE) or vanished (BROKEN), plus sections that make code claims and
anchor nothing (UNANCHORED) -- the vacuity guard, without which "all 0 anchored
sections are fresh" passes forever.

A section with no code claim says so explicitly:

    <!-- anchor: none - domain vocabulary, no code claim -->

Commands:
    check     [--strict] [--docs DIR] [--repo DIR]   exit 1 on STALE/BROKEN (+UNANCHORED with --strict)
    stamp     [--section HEADING]... [--all]         write fingerprints for sections you have just re-read
    touched   --since REF                            doc sections whose code changed, + undocumented changes
    selftest                                         prove the checker can fail, and does not cry wolf

No third-party dependencies. Python 3.9+.
"""

from __future__ import annotations

import argparse
import fnmatch
import hashlib
import os
import re
import subprocess
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path
from typing import Iterable

ANCHOR_RE = re.compile(r"<!--\s*anchor:\s*(.+?)\s*-->", re.I)
FINGERPRINT_RE = re.compile(r"<!--\s*fingerprint:\s*sha256:([0-9a-f]+)\s*(?:@\s*([0-9-]+))?\s*-->", re.I)
HEADING_RE = re.compile(r"^(#{1,6})\s+(.*\S)\s*$")
NONE_RE = re.compile(r"^none\b", re.I)

# A section that mentions any of these is making a claim about code.
CODE_CLAIM_RE = re.compile(
    r"`[^`]*(?:/|\.(?:ts|tsx|js|jsx|py|go|php|rb|rs|java|kt|sql|yml|yaml|json|sh)\b)[^`]*`"
    r"|`[A-Z][A-Za-z0-9_]+(?:\.[a-z][A-Za-z0-9_]*)?\(\)`"
)

SKIP_DIRS = {".git", "node_modules", "vendor", "dist", "build", ".venv", "__pycache__", ".next"}


# --------------------------------------------------------------------------- model


@dataclass
class Anchor:
    raw: str
    glob: str
    symbol: str | None = None

    @classmethod
    def parse(cls, raw: str) -> "Anchor | None":
        if NONE_RE.match(raw):
            return None
        glob, _, symbol = raw.partition("#")
        return cls(raw=raw, glob=glob.strip(), symbol=symbol.strip() or None)


@dataclass
class Section:
    path: Path
    heading: str
    level: int
    line: int
    body: str = ""
    anchors: list[Anchor] = field(default_factory=list)
    exempt_reason: str | None = None
    fingerprint: str | None = None
    stamped: str | None = None
    fp_line: int | None = None
    anchor_line: int | None = None
    inherited_from: "Section | None" = None

    @property
    def declares(self) -> bool:
        return bool(self.anchors) or bool(self.exempt_reason)

    @property
    def covered(self) -> bool:
        return self.declares or self.inherited_from is not None

    @property
    def ref(self) -> str:
        return f"{self.path}:{self.line} {self.heading}"

    @property
    def claims_code(self) -> bool:
        return bool(CODE_CLAIM_RE.search(self.body))


class Broken(Exception):
    pass


# ----------------------------------------------------------------------- normalise


# Languages where leading whitespace is decoration, not meaning. Everything else --
# Python, YAML, Sass, Markdown, anything unknown -- keeps its indentation, because
# under-firing is the worse failure: a doc that silently stops being true.
BRACE_EXT = {
    "ts", "tsx", "js", "jsx", "mjs", "cjs", "go", "php", "java", "kt", "kts", "rs",
    "c", "h", "cpp", "hpp", "cc", "cs", "swift", "scala", "groovy", "dart", "sql",
    "css", "scss", "less", "json", "proto", "graphql", "gql", "hcl", "tf", "zig",
}

# Formats where a blank line carries meaning (it separates paragraphs). Everywhere
# else a blank line is formatting, and treating it as drift is how a check earns a
# reputation for crying wolf and gets routed around.
BLANK_SIGNIFICANT_EXT = {"md", "markdown", "rst", "txt", "adoc", "asciidoc"}


def normalise(text: str, path: str = "") -> str:
    """Whitespace normalisation, language-aware about indentation.

    Tabs become spaces and trailing whitespace goes everywhere. Leading whitespace
    is stripped only for brace-delimited languages, where it carries no meaning; in
    Python, YAML or Markdown an indent change IS a change, so it is preserved and
    an unknown extension is treated as indent-significant. Blank lines are dropped
    outright except in prose formats, where they separate paragraphs.

    Deliberately does NOT strip comments: no comment syntax is safe across every
    language a repo holds (`#` is a heading in Markdown and a key in YAML), and a
    stripper with one vocabulary for every language erases real content. The cost
    is that a comment edit inside an anchored block reads as STALE. Answer that by
    anchoring narrowly -- a symbol, not a file -- not by widening the stripper.
    """
    ext = Path(path).suffix.lower().lstrip(".")
    brace, prose = ext in BRACE_EXT, ext in BLANK_SIGNIFICANT_EXT
    raw = text.replace("\r\n", "\n").replace("\r", "\n").split("\n")
    out: list[str] = []
    for ln in raw:
        ln = ln.replace("\t", "    ").rstrip()
        if brace:
            ln = ln.lstrip()
        if not ln:
            if not prose or (out and not out[-1]):
                continue
        out.append(ln)
    return "\n".join(out).strip("\n")


# ------------------------------------------------------------------ symbol capture


def _strip_noise(line: str) -> str:
    """Blank out string literals and line comments so brace counting is honest."""
    out, i, quote = [], 0, None
    while i < len(line):
        ch = line[i]
        if quote:
            if ch == "\\":
                out.append(" ")
                i += 2
                continue
            out.append(" " if ch != quote else ch)
            if ch == quote:
                quote = None
            i += 1
            continue
        if ch in "\"'`":
            quote = ch
            out.append(ch)
            i += 1
            continue
        if line.startswith("//", i) or line.startswith("#", i):
            break
        out.append(ch)
        i += 1
    return "".join(out)


def extract_symbol(text: str, pattern: str) -> str:
    """Return the block introduced by the first line matching `pattern`.

    Braced languages: walk brace depth (strings and line comments blanked) from the
    signature until depth returns to zero. Indented languages: take the signature
    plus every following line that is blank or indented deeper than it.

    Limit, stated rather than discovered: a brace inside a block comment or a
    template literal spanning lines can still mislead the walk. When a capture
    looks wrong, anchor the file instead and narrow the doc section.
    """
    rx = re.compile(pattern)
    lines = text.replace("\r\n", "\n").split("\n")
    for i, line in enumerate(lines):
        if not rx.search(line):
            continue
        head = _strip_noise(lines[i])
        lookahead = "\n".join(_strip_noise(l) for l in lines[i : i + 3])
        if "{" in lookahead:
            depth, block, started = 0, [], False
            for ln in lines[i:]:
                block.append(ln)
                clean = _strip_noise(ln)
                depth += clean.count("{") - clean.count("}")
                if "{" in clean:
                    started = True
                if started and depth <= 0:
                    break
            return "\n".join(block)
        base = len(head) - len(head.lstrip())
        block = [lines[i]]
        for ln in lines[i + 1 :]:
            if not ln.strip():
                block.append(ln)
                continue
            if len(ln) - len(ln.lstrip()) <= base:
                break
            block.append(ln)
        return "\n".join(block)
    raise Broken(f"symbol /{pattern}/ not found")


# ---------------------------------------------------------------------- resolution


def iter_repo_files(repo: Path) -> Iterable[Path]:
    for root, dirs, files in os.walk(repo):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        for name in files:
            yield Path(root, name)


def resolve(anchor: Anchor, repo: Path) -> list[tuple[str, str]]:
    matches: list[Path] = []
    direct = repo / anchor.glob
    if direct.is_file():
        matches = [direct]
    else:
        for path in iter_repo_files(repo):
            rel = path.relative_to(repo).as_posix()
            if fnmatch.fnmatch(rel, anchor.glob):
                matches.append(path)
    if not matches:
        raise Broken(f"no file matches '{anchor.glob}'")
    out = []
    for path in sorted(matches):
        rel = path.relative_to(repo).as_posix()
        try:
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError as exc:
            raise Broken(f"cannot read {path}: {exc}") from exc
        if anchor.symbol:
            text = extract_symbol(text, anchor.symbol)
        out.append((rel, normalise(text, rel)))
    return out


def fingerprint_of(section: Section, repo: Path) -> str:
    digest = hashlib.sha256()
    for anchor in section.anchors:
        digest.update(b"\x00anchor\x00" + anchor.raw.encode())
        for rel, text in resolve(anchor, repo):
            digest.update(b"\x00file\x00" + rel.encode() + b"\x00" + text.encode())
    return digest.hexdigest()


# --------------------------------------------------------------------------- parse


def parse_file(path: Path, repo: Path) -> list[Section]:
    lines = path.read_text(encoding="utf-8").split("\n")
    sections: list[Section] = []
    current: Section | None = None
    body: list[str] = []
    in_fence = False

    def close() -> None:
        if current is not None:
            current.body = "\n".join(body)
            sections.append(current)

    for idx, line in enumerate(lines, start=1):
        if line.lstrip().startswith("```"):
            in_fence = not in_fence
        heading = None if in_fence else HEADING_RE.match(line)
        if heading:
            close()
            body = []
            current = Section(
                path=path.relative_to(repo) if path.is_relative_to(repo) else path,
                heading=heading.group(2),
                level=len(heading.group(1)),
                line=idx,
            )
            continue
        if current is None:
            continue
        body.append(line)
        if in_fence:
            # An anchor shown as an EXAMPLE is not an anchor. Documentation about this
            # format is the obvious case, and it binds a section to code it only mentions.
            continue
        for match in ANCHOR_RE.finditer(line):
            raw = match.group(1)
            current.anchor_line = idx  # last anchor wins: the stamp covers the whole block
            if NONE_RE.match(raw):
                current.exempt_reason = raw
            else:
                parsed = Anchor.parse(raw)
                if parsed:
                    current.anchors.append(parsed)
        fp = FINGERPRINT_RE.search(line)
        if fp:
            current.fingerprint = fp.group(1)
            current.stamped = fp.group(2)
            current.fp_line = idx
    close()
    _inherit(sections)
    return sections


def _inherit(sections: list[Section]) -> None:
    """A subsection is covered by its nearest ancestor that declares.

    Without this, a reference document with 56 top-level findings and 260
    subsections under them demands 320 judgements, and the only way anyone gets
    through that is by rubber-stamping -- which is how a coverage number stops
    meaning anything. A subsection that needs its own binding still declares one;
    silence now means "same subject as my parent", which is what it already meant
    to every human reading the document.
    """
    for i, section in enumerate(sections):
        if section.declares:
            continue
        for ancestor in reversed(sections[:i]):
            if ancestor.level < section.level and ancestor.declares:
                section.inherited_from = ancestor
                break


def collect(docs: Path, repo: Path) -> list[Section]:
    sections: list[Section] = []
    for path in sorted(docs.rglob("*.md")):
        if any(part in SKIP_DIRS for part in path.parts):
            continue
        sections.extend(parse_file(path, repo))
    return sections


# ------------------------------------------------------------------------ commands


def cmd_check(args) -> int:
    repo, docs = Path(args.repo).resolve(), Path(args.docs).resolve()
    sections = collect(docs, repo)
    stale, broken, unanchored, fresh, exempt, unstamped = [], [], [], [], [], []

    inherited = []
    for section in sections:
        if section.exempt_reason and not section.anchors:
            exempt.append(section)
            continue
        if not section.anchors:
            if section.inherited_from is not None:
                inherited.append(section)
            elif section.claims_code:
                unanchored.append(section)
            continue
        try:
            actual = fingerprint_of(section, repo)
        except Broken as exc:
            broken.append((section, str(exc)))
            continue
        if section.fingerprint is None:
            unstamped.append(section)
        elif actual != section.fingerprint:
            stale.append(section)
        else:
            fresh.append(section)

    def show(label: str, items, fmt=lambda s: s.ref) -> None:
        if items:
            print(f"\n{label} ({len(items)})")
            for item in items:
                print(f"  {fmt(item)}")

    show("BROKEN   anchored code is gone", broken, lambda p: f"{p[0].ref}\n           -> {p[1]}")
    show("STALE    anchored code changed since the doc was verified", stale)
    show("UNSTAMPED anchored but never fingerprinted", unstamped)
    show("UNANCHORED section makes code claims and anchors nothing", unanchored)

    total = len(sections)
    print(
        f"\n{len(fresh)} fresh, {len(stale)} stale, {len(broken)} broken, "
        f"{len(unstamped)} unstamped, {len(unanchored)} unanchored, "
        f"{len(exempt)} exempt, {len(inherited)} inherited, {total} sections in {docs}"
    )

    if total and not (fresh or stale or broken or unstamped):
        print("\nVACUOUS: no section in this docs tree is anchored to anything. "
              "A green check here guarantees nothing.", file=sys.stderr)
        return 1

    failed = bool(stale or broken or unstamped)
    if args.strict and unanchored:
        failed = True
    return 1 if failed else 0


def cmd_stamp(args) -> int:
    repo, docs = Path(args.repo).resolve(), Path(args.docs).resolve()
    if not args.all and not args.section:
        print("stamp needs --section HEADING (repeatable) or --all", file=sys.stderr)
        return 2
    if args.all:
        print("WARNING: --all stamps every section as verified. A fingerprint you did not\n"
              "         earn by re-reading the section is a false currency signal, and it\n"
              "         deprioritises the file for good.", file=sys.stderr)
    wanted = {s.lower() for s in (args.section or [])}
    today = subprocess.run(["date", "+%Y-%m-%d"], capture_output=True, text=True).stdout.strip()
    changed = 0

    for path in sorted(docs.rglob("*.md")):
        sections = parse_file(path, repo)
        targets = [
            s for s in sections
            if s.anchors and (args.all or s.heading.lower() in wanted)
        ]
        if not targets:
            continue
        lines = path.read_text(encoding="utf-8").split("\n")
        for section in sorted(targets, key=lambda s: s.line, reverse=True):
            try:
                digest = fingerprint_of(section, repo)
            except Broken as exc:
                print(f"  SKIP {section.ref}: {exc}", file=sys.stderr)
                continue
            stamp = f"<!-- fingerprint: sha256:{digest} @ {today} -->"
            if section.fp_line:
                lines[section.fp_line - 1] = stamp
            else:
                insert_at = (section.anchor_line or section.line)
                lines.insert(insert_at, stamp)
            changed += 1
            print(f"  stamped {section.ref}")
        path.write_text("\n".join(lines), encoding="utf-8")

    print(f"{changed} section(s) stamped")
    return 0 if changed else 1


def cmd_touched(args) -> int:
    repo, docs = Path(args.repo).resolve(), Path(args.docs).resolve()
    diff = subprocess.run(
        ["git", "-C", str(repo), "diff", "--name-only", f"{args.since}..HEAD"],
        capture_output=True, text=True,
    )
    if diff.returncode != 0:
        print(diff.stderr.strip(), file=sys.stderr)
        return 2
    changed = [f for f in diff.stdout.split("\n") if f.strip()]
    if not changed:
        print(f"no files changed since {args.since}")
        return 0

    sections = collect(docs, repo)
    hit: dict[str, list[str]] = {}
    covered: set[str] = set()
    for section in sections:
        for anchor in section.anchors:
            for f in changed:
                if f == anchor.glob or fnmatch.fnmatch(f, anchor.glob):
                    hit.setdefault(section.ref, []).append(f)
                    covered.add(f)

    print(f"{len(changed)} file(s) changed since {args.since}\n")
    print(f"DOC SECTIONS TO RE-READ ({len(hit)})")
    for ref, files in sorted(hit.items()):
        print(f"  {ref}\n      <- {', '.join(sorted(set(files)))}")

    orphans = [f for f in changed if f not in covered and not f.startswith(docs.name + "/")]
    print(f"\nCHANGED CODE NO DOC SECTION CLAIMS ({len(orphans)})")
    for f in sorted(orphans):
        print(f"  {f}")
    print("\nNot every orphan deserves a doc. The ones that do are where the WHY changed.")
    return 0


def cmd_selftest(args) -> int:
    """Prove the checker can fail, and prove it does not fire on cosmetic edits."""
    with tempfile.TemporaryDirectory() as tmp:
        repo = Path(tmp)
        (repo / "src").mkdir()
        code = repo / "src" / "widget.ts"
        code.write_text(
            "export class Widget {\n"
            "  // a comment\n"
            "  price(): number {\n"
            "    return 42;\n"
            "  }\n"
            "}\n",
            encoding="utf-8",
        )
        docs = repo / "docs"
        docs.mkdir()
        doc = docs / "explanation.md"
        doc.write_text(
            "# Widgets\n\n## Why widgets cost what they cost\n\n"
            "<!-- anchor: src/widget.ts#^export class Widget -->\n\n"
            "Widgets are priced in `src/widget.ts`.\n",
            encoding="utf-8",
        )

        ns = argparse.Namespace(repo=str(repo), docs=str(docs), strict=False, section=None, all=True)
        results = {}

        print("1. unstamped section  ->", end=" ")
        results["unstamped"] = cmd_check(ns) == 1
        print("FAILS (correct)" if results["unstamped"] else "PASSES (BUG)")

        print("\n2. stamping           ->", end=" ")
        cmd_stamp(ns)
        results["clean"] = cmd_check(ns) == 0
        print("PASSES (correct)" if results["clean"] else "FAILS (BUG)")

        print("\n3. CONTROL: reindent, rename nothing ->", end=" ")
        code.write_text(code.read_text(encoding="utf-8").replace("    return 42;", "        return 42;\n\n"), encoding="utf-8")
        results["control"] = cmd_check(ns) == 0
        print("PASSES (correct - whitespace is not drift)" if results["control"] else "FAILS (cries wolf)")

        print("\n3b. CONTROL GUARD: reindent an INDENT-SIGNIFICANT file ->", end=" ")
        pycode = repo / "src" / "rules.py"
        pycode.write_text("def price():\n    if True:\n        return 42\n", encoding="utf-8")
        doc2 = docs / "rules.md"
        doc2.write_text(
            "# Rules\n\n## How prices are decided\n\n"
            "<!-- anchor: src/rules.py -->\n\nSee `src/rules.py`.\n",
            encoding="utf-8",
        )
        cmd_stamp(ns)
        pycode.write_text("def price():\n    if True:\n            return 42\n", encoding="utf-8")
        results["indent_guard"] = cmd_check(ns) == 1
        print("FAILS (correct - indentation is meaning in Python)" if results["indent_guard"]
              else "PASSES (BUG - blind to a real Python change)")
        doc2.unlink()
        pycode.unlink()

        print("\n4. ABLATION: change the anchored logic ->", end=" ")
        code.write_text(code.read_text(encoding="utf-8").replace("42", "43"), encoding="utf-8")
        results["stale"] = cmd_check(ns) == 1
        print("FAILS (correct)" if results["stale"] else "PASSES (BUG - the check is decoration)")

        print("\n5. BROKEN: delete the anchored file ->", end=" ")
        code.unlink()
        results["broken"] = cmd_check(ns) == 1
        print("FAILS (correct)" if results["broken"] else "PASSES (BUG)")

        print("\n5b. FENCED EXAMPLE: an anchor inside a code block is not an anchor ->", end=" ")
        fenced = docs / "howto.md"
        fenced.write_text(
            "# How to anchor\n\n## Keeping these true\n\nWrite one like this:\n\n"
            "```\n<!-- anchor: src/does-not-exist.ts -->\n```\n",
            encoding="utf-8",
        )
        parsed = parse_file(fenced, repo)
        results["fenced"] = not any(s.anchors for s in parsed)
        print("IGNORED (correct)" if results["fenced"] else "PARSED (BUG - binds docs to code they only mention)")
        fenced.unlink()

        print("\n5c. INHERITANCE: a subsection under an anchored parent ->", end=" ")
        code.write_text("export class Widget {\n  price(): number {\n    return 42;\n  }\n}\n", encoding="utf-8")
        inh = docs / "inherit.md"
        inh.write_text(
            "# Widgets\n\n## Pricing\n\n<!-- anchor: src/widget.ts -->\n\nSee `src/widget.ts`.\n\n"
            "### Rounding\n\nA detail about `src/widget.ts`.\n\n"
            "## Unrelated\n\nClaims `src/widget.ts` but declares nothing.\n",
            encoding="utf-8",
        )
        parsed = parse_file(inh, repo)
        by = {s.heading: s for s in parsed}
        results["inherit_covers"] = by["Rounding"].inherited_from is not None
        results["inherit_no_invent"] = by["Unrelated"].inherited_from is None
        print(("COVERED, and a SIBLING is not (correct)"
               if results["inherit_covers"] and results["inherit_no_invent"]
               else f"BUG - covers={by['Rounding'].inherited_from}, sibling={by['Unrelated'].inherited_from}"))
        inh.unlink()

        print("\n6. VACUITY: a docs tree anchoring nothing ->", end=" ")
        doc.write_text("# Widgets\n\n## Vision\n\nWidgets are good.\n", encoding="utf-8")
        results["vacuous"] = cmd_check(ns) == 1
        print("FAILS (correct)" if results["vacuous"] else "PASSES (BUG)")

    bad = [k for k, v in results.items() if not v]
    print("\n" + ("SELFTEST PASSED - the checker can fail, and does not cry wolf"
                  if not bad else f"SELFTEST FAILED: {', '.join(bad)}"))
    return 1 if bad else 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", default=".", help="repository root (default: cwd)")
    ap.add_argument("--docs", default="docs", help="docs root (default: docs/)")
    sub = ap.add_subparsers(dest="cmd", required=True)

    c = sub.add_parser("check", help="fail on stale, broken or unstamped sections")
    c.add_argument("--strict", action="store_true", help="also fail on UNANCHORED sections")
    c.set_defaults(func=cmd_check)

    s = sub.add_parser("stamp", help="write fingerprints for sections you have just re-read")
    s.add_argument("--section", action="append", help="section heading (repeatable)")
    s.add_argument("--all", action="store_true")
    s.set_defaults(func=cmd_stamp)

    t = sub.add_parser("touched", help="doc sections whose anchored code changed")
    t.add_argument("--since", required=True, help="git ref, e.g. HEAD~10 or a tag")
    t.set_defaults(func=cmd_touched)

    st = sub.add_parser("selftest", help="prove the checker can fail")
    st.set_defaults(func=cmd_selftest)

    args = ap.parse_args()
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
