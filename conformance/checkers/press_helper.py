#!/usr/bin/env python3
"""Keep the screen-settle loop in one helper, so no probe reads a menu mid-paint."""

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path

from finding import EXIT_HARNESS, EXIT_OK, EXIT_VIOLATION, coverage, harness, violation

RULE = "ARCH-PRESS-1"
ROOT = Path(__file__).resolve().parents[2]
SUBJECT = "internal/multiplex/firmwaretests"
HELPER = f"{SUBJECT}/rununtil_test.go"
# The helper the probes must call, named here so a tree that has renamed it fails as coverage
# rather than passing over a corpus with no settle loop left to find.
HELPER_FUNC = "func pressAndLetItFinishWatching("
WINDOW = 14  # lines either side of a screen read that a settle loop fits inside

SCREEN = re.compile(r"\bscreen(?:Body)?Now\s*\(")
BUMP = re.compile(r"\b([A-Za-z_]\w*)\+\+")
SETTLE = re.compile(r"\b([A-Za-z_]\w*)\s*>=\s*(?:4|stableSamples)\b")


def command(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, cwd=ROOT, capture_output=True, text=True, check=False)


def settle_loops(lines: list[str]) -> list[int]:
    """Line numbers where a counter is bumped and compared beside a screen read.

    STRUCTURAL, NOT SEMANTIC. It does not ask whether a loop is "really" a settle loop; it asks
    whether a file contains the three tokens that make one -- a screen read, a counter increment
    and a comparison against the stability threshold -- close enough together to be the same loop.
    """
    hits = []
    for i, line in enumerate(lines):
        if not SCREEN.search(line):
            continue
        lo, hi = max(0, i - WINDOW), min(len(lines), i + WINDOW + 1)
        window = lines[lo:hi]
        bumped = {m.group(1) for row in window for m in BUMP.finditer(row)}
        for row_index in range(lo, hi):
            match = SETTLE.search(lines[row_index])
            if match and match.group(1) in bumped:
                hits.append(row_index + 1)
    return sorted(set(hits))


def main() -> int:
    tracked = command("git", "ls-files", "--", SUBJECT)
    if tracked.returncode:
        print(harness(RULE, f"cannot enumerate tracked probes: {tracked.stderr.strip()}"))
        return EXIT_HARNESS
    files = [line for line in tracked.stdout.splitlines()
             if line.endswith("_test.go") and (ROOT / line).is_file()]

    helper = ROOT / HELPER
    helpers = 0
    if helper.is_file() and HELPER_FUNC in helper.read_text(encoding="utf-8"):
        helpers = 1
    absent = coverage(RULE, "press-subject-missing",
                      {SUBJECT: len(files), HELPER: helpers})
    if absent:
        print(absent)
        return EXIT_VIOLATION

    findings = []
    for name in files:
        if name == HELPER:
            continue
        lines = (ROOT / name).read_text(encoding="utf-8").splitlines()
        for at in settle_loops(lines):
            findings.append(violation(
                RULE, "press-loop-outside-helper",
                f"{name}:{at} settles a screen by hand; call pressAndLetItFinish"))
    for finding in sorted(findings):
        print(finding)
    return EXIT_VIOLATION if findings else EXIT_OK


if __name__ == "__main__":
    raise SystemExit(main())
