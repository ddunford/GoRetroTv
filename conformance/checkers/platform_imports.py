#!/usr/bin/env python3
"""Keep internal/platform independent of emulator and transport domain packages."""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

from finding import EXIT_HARNESS, EXIT_OK, EXIT_VIOLATION, coverage, harness, violation

RULE = "ARCH-PLATFORM-1"
ROOT = Path(__file__).resolve().parents[2]
SUBJECT = "internal/platform"
FORBIDDEN = ("cpu", "device", "broadcast", "httpx", "web")


def command(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, cwd=ROOT, capture_output=True, text=True, check=False)


def packages(text: str) -> list[dict]:
    decoder = json.JSONDecoder()
    result = []
    pos = 0
    while pos < len(text):
        while pos < len(text) and text[pos].isspace():
            pos += 1
        if pos == len(text):
            break
        row, used = decoder.raw_decode(text[pos:])
        result.append(row)
        pos += used
    return result


def main() -> int:
    tracked = command("git", "ls-files", "--", SUBJECT)
    if tracked.returncode:
        print(harness(RULE, f"cannot enumerate tracked platform files: {tracked.stderr.strip()}"))
        return EXIT_HARNESS
    files = [line for line in tracked.stdout.splitlines()
             if line.endswith(".go") and (ROOT / line).is_file()]

    module = command("go", "list", "-m")
    if module.returncode or not module.stdout.strip():
        print(harness(RULE, f"cannot identify the Go module: {module.stderr.strip()}"))
        return EXIT_HARNESS
    prefix = module.stdout.strip() + "/internal/"

    rows = []
    if files:
        listed = command("go", "list", "-e", "-json", "./internal/platform/...")
        if listed.returncode:
            print(harness(RULE, f"go list cannot inspect platform packages: {listed.stderr.strip()}"))
            return EXIT_HARNESS
        try:
            rows = packages(listed.stdout)
        except (json.JSONDecodeError, TypeError, ValueError) as exc:
            print(harness(RULE, f"go list returned malformed package data: {exc}"))
            return EXIT_HARNESS
    absent = coverage(RULE, "platform-subject-missing",
                      {SUBJECT: min(len(files), len(rows))})
    if absent:
        print(absent)
        return EXIT_VIOLATION

    findings = []
    for row in rows:
        package = row.get("ImportPath", "<unknown>")
        for imported in set(row.get("Imports", []) + row.get("TestImports", []) + row.get("XTestImports", [])):
            if not imported.startswith(prefix):
                continue
            top = imported[len(prefix):].split("/", 1)[0]
            if top in FORBIDDEN:
                findings.append(violation(RULE, "platform-imports-domain", f"{package} imports {imported}"))
    for finding in sorted(findings):
        print(finding)
    return EXIT_VIOLATION if findings else EXIT_OK


if __name__ == "__main__":
    raise SystemExit(main())
