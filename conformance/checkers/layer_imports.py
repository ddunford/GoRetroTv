#!/usr/bin/env python3
"""Keep hardware and oracle packages independent of transport and application wiring."""

from __future__ import annotations

from pathlib import Path

from finding import EXIT_HARNESS, EXIT_OK, EXIT_VIOLATION, coverage, harness, violation
from platform_imports import command, packages

RULE = "ARCH-LAYER-1"
ROOT = Path(__file__).resolve().parents[2]
SUBJECTS = ("internal/bus", "internal/memory")
CORE = {"bus", "memory", "cpu", "device", "broadcast", "oracle", "platform"}
OUTWARD = {"httpx", "web", "app"}


def main() -> int:
    tracked = command("git", "ls-files", "--", *SUBJECTS)
    if tracked.returncode:
        print(harness(RULE, f"cannot enumerate core source: {tracked.stderr.strip()}"))
        return EXIT_HARNESS
    paths = tracked.stdout.splitlines()
    counts = {subject: sum(path.startswith(subject + "/") and path.endswith(".go")
                           and (ROOT / path).is_file() for path in paths)
              for subject in SUBJECTS}
    absent = coverage(RULE, "core-subject-missing", counts)
    if absent:
        print(absent)
        return EXIT_VIOLATION

    module = command("go", "list", "-m")
    if module.returncode or not module.stdout.strip():
        print(harness(RULE, f"cannot identify the Go module: {module.stderr.strip()}"))
        return EXIT_HARNESS
    prefix = module.stdout.strip() + "/internal/"

    listed = command("go", "list", "-e", "-json", "./internal/...")
    if listed.returncode:
        print(harness(RULE, f"go list cannot inspect internal packages: {listed.stderr.strip()}"))
        return EXIT_HARNESS
    try:
        rows = packages(listed.stdout)
    except (TypeError, ValueError) as exc:
        print(harness(RULE, f"go list returned malformed package data: {exc}"))
        return EXIT_HARNESS

    findings = []
    for row in rows:
        package = row.get("ImportPath", "<unknown>")
        if not package.startswith(prefix):
            continue
        top = package[len(prefix):].split("/", 1)[0]
        if top not in CORE:
            continue
        for imported in set(row.get("Imports", []) + row.get("TestImports", []) + row.get("XTestImports", [])):
            if not imported.startswith(prefix):
                continue
            dest = imported[len(prefix):].split("/", 1)[0]
            if dest in OUTWARD:
                findings.append(violation(RULE, "core-imports-transport",
                                          f"{package} imports {imported}"))
    for finding in sorted(findings):
        print(finding)
    return EXIT_VIOLATION if findings else EXIT_OK


if __name__ == "__main__":
    raise SystemExit(main())
