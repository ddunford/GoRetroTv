#!/usr/bin/env python3
"""Enforce the approved Go module and version without replacement."""

from __future__ import annotations

import json
import subprocess
from pathlib import Path

from finding import EXIT_HARNESS, EXIT_OK, EXIT_VIOLATION, coverage, harness, violation

RULE = "ARCH-MODULE-1"
ROOT = Path(__file__).resolve().parents[2]
SUBJECT = "go.mod"
APPROVED_PATH = "github.com/coder/websocket"
APPROVED_VERSION = "v1.8.15"


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
        replacements = document.get("Replace", [])
        module = document["Module"]["Path"]
    except (json.JSONDecodeError, KeyError, TypeError) as exc:
        print(harness(RULE, f"go.mod did not yield a module and requirements: {exc}"))
        return EXIT_HARNESS
    if not module:
        print(harness(RULE, "go.mod has an empty module path"))
        return EXIT_HARNESS
    failed = False
    if replacements:
        for replacement in replacements:
            print(violation(RULE, "module-replaced", f"go.mod replaces {replacement['Old']['Path']}"))
            failed = True

    approved = [requirement for requirement in requirements
                if requirement["Path"] == APPROVED_PATH]
    if not approved:
        print(violation(RULE, "approved-module-missing", f"go.mod does not require {APPROVED_PATH}"))
        failed = True
    for requirement in approved:
        if requirement["Version"] != APPROVED_VERSION:
            print(violation(RULE, "unapproved-version",
                            f"go.mod requires {APPROVED_PATH}@{requirement['Version']}, "
                            f"approved version is {APPROVED_VERSION}"))
            failed = True
    for requirement in requirements:
        if requirement["Path"] != APPROVED_PATH:
            print(violation(RULE, "external-module",
                            f"go.mod requires {requirement['Path']}@{requirement['Version']}"))
            failed = True
    return EXIT_VIOLATION if failed else EXIT_OK


if __name__ == "__main__":
    raise SystemExit(main())
