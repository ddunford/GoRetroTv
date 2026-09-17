#!/usr/bin/env python3
"""Require an explicit decision before adding a Go module dependency."""

from __future__ import annotations

import json
import subprocess
from pathlib import Path

from finding import EXIT_HARNESS, EXIT_OK, EXIT_VIOLATION, coverage, harness, violation

RULE = "ARCH-MODULE-1"
ROOT = Path(__file__).resolve().parents[2]
SUBJECT = "go.mod"


def main() -> int:
    # An empty input is a violation of the rule's reach, not a green dependency graph.
    exists = (ROOT / SUBJECT).is_file()
    missing = coverage(RULE, "module-file-missing", {SUBJECT: int(exists)})
    if missing:
        print(missing)
        return EXIT_VIOLATION

    result = subprocess.run(["go", "mod", "edit", "-json"], cwd=ROOT,
                            capture_output=True, text=True, check=False)
    if result.returncode:
        print(harness(RULE, f"cannot read go.mod: {result.stderr.strip()}"))
        return EXIT_HARNESS
    try:
        document = json.loads(result.stdout)
        requirements = document.get("Require", [])
        module = document["Module"]["Path"]
    except (json.JSONDecodeError, KeyError, TypeError) as exc:
        print(harness(RULE, f"go.mod did not yield a module and requirements: {exc}"))
        return EXIT_HARNESS
    if not module:
        print(harness(RULE, "go.mod has an empty module path"))
        return EXIT_HARNESS
    if requirements:
        for requirement in requirements:
            print(violation(RULE, "external-module", f"go.mod requires {requirement['Path']}"))
        return EXIT_VIOLATION
    return EXIT_OK


if __name__ == "__main__":
    raise SystemExit(main())
