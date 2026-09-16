#!/usr/bin/env python3
"""Reject wall-clock scheduling and goroutines from instruction-time packages."""

from __future__ import annotations

import json
import subprocess
from pathlib import Path

from finding import EXIT_HARNESS, EXIT_OK, EXIT_VIOLATION, coverage, harness, violation

ROOT = Path(__file__).resolve().parents[2]
RULE = "ARCH-DET-1"
SUBJECTS = ("internal/platform/clock", "conformance/checkers/determinismcensus/main.go")


def main() -> int:
    seen = {path: int((ROOT / path).exists()) for path in SUBJECTS}
    result = {"scanned": 0, "found": []}
    if all(seen.values()):
        proc = subprocess.run(["go", "run", "./conformance/checkers/determinismcensus"],
                              cwd=ROOT, capture_output=True, text=True, check=False)
        if proc.returncode:
            print(harness(RULE, f"determinism census could not run: {proc.stderr.strip()}"))
            return EXIT_HARNESS
        try:
            result = json.loads(proc.stdout)
        except (json.JSONDecodeError, TypeError) as exc:
            print(harness(RULE, f"determinism census returned invalid JSON: {exc}"))
            return EXIT_HARNESS
    seen["instruction-time files"] = result["scanned"]
    absent = coverage(RULE, "instruction-subject-missing", seen)
    if absent:
        print(absent)
        return EXIT_VIOLATION
    findings = []
    for row in result["found"]:
        if row["kind"] == "wall-clock-source":
            findings.append(violation(RULE, "wall-clock-source",
                                      f"{row['file']}:{row['line']} calls wall-clock time"))
        if row["kind"] == "goroutine-in-core":
            findings.append(violation(RULE, "goroutine-in-core",
                                      f"{row['file']}:{row['line']} starts a goroutine"))
    for finding in findings:
        print(finding)
    return EXIT_VIOLATION if findings else EXIT_OK


if __name__ == "__main__":
    raise SystemExit(main())
