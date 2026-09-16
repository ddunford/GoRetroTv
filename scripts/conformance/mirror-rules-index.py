#!/usr/bin/env python3
"""Generate the catalogue's rules index from the registry, and refuse to let them drift.

THE INDEX IS A MIRROR, NOT A SOURCE. `conformance/rules.toml` is what the harness runs; the table
at the top of the catalogue is what a person reads first, and a hand-maintained copy of machine
state is a copy that is wrong. This regenerates it, and `--check` fails when the file on disk does
not match what the registry says — which is how the gate catches a rule whose state changed and
whose summary row did not.

`check_registry.py` already asserts that the two documents name the SAME RULES, in both directions.
That is the check that stops a rule existing in one and not the other. It says nothing about whether
the row DESCRIBES the rule correctly: a rule can be flipped from `pending` to `enforced`, or from
ARMED to DEFERRED, with the table still stating the old answer — and the table is what gets quoted
into a phase close-out.

Same shape as the beads phase mirror: generated at close-out boundaries, never hand-edited.

    scripts/conformance/mirror-rules-index.py            rewrite the table
    scripts/conformance/mirror-rules-index.py --check    exit 1 if it is stale
"""

from __future__ import annotations

import re
import subprocess
import sys
import tomllib
from pathlib import Path

ROOT = Path(subprocess.run(["git", "rev-parse", "--show-toplevel"],
                           capture_output=True, text=True, check=True).stdout.strip())
REGISTRY = ROOT / "conformance" / "rules.toml"

HEADING = "## Rules"
HEADER = ("| Rule | Invariant | Source decision | Class | Arming | State |\n"
          "|---|---|---|---|---|---|\n")

# The catalogue names the decision, not the file it lives in — the file is the same for all of them
# and repeating it twenty times is noise in a column that exists to be scanned.
SECTION = re.compile(r"§\s*([^(;]+)")


def short_source(source: str) -> str:
    found = SECTION.findall(source)
    return " + ".join(s.strip() for s in found) if found else source


def table(rules: list[dict]) -> str:
    rows = []
    for r in rules:
        state = r["state"]
        held = "**enforced**" if state == "enforced" else f"{state} — {r.get('owner_task', '?')}"
        rows.append(
            f"| `{r['id']}` | {r['invariant']} | {short_source(r['source'])} | "
            f"{r['classification']} | {r['arming']} | {held} |"
        )
    return HEADER + "\n".join(rows) + "\n"


def replace(text: str, generated: str) -> str:
    start = text.index(HEADING)
    body = text.index("\n", start) + 1
    # up to the next heading of any level
    m = re.search(r"^#{2,3} ", text[body:], re.MULTILINE)
    end = body + (m.start() if m else len(text) - body)
    return text[:body] + "\n" + generated + "\n" + text[end:]


def main() -> int:
    doc = tomllib.loads(REGISTRY.read_text())
    catalogue = ROOT / doc["harness"]["catalogue"]
    generated = table(doc["rule"])
    current = catalogue.read_text()
    updated = replace(current, generated)

    if "--check" in sys.argv:
        if updated != current:
            print(f"{catalogue.relative_to(ROOT)}: the rules index does not match "
                  f"conformance/rules.toml.")
            print("  It is a MIRROR of the registry and is regenerated, never hand-edited:")
            print("    scripts/conformance/mirror-rules-index.py")
            return 1
        print(f"{catalogue.relative_to(ROOT)}: rules index matches the registry")
        return 0

    if updated == current:
        print("rules index already current")
        return 0
    catalogue.write_text(updated)
    print(f"rewrote the rules index in {catalogue.relative_to(ROOT)} "
          f"({len(doc['rule'])} rules)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
