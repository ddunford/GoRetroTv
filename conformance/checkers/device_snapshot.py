#!/usr/bin/env python3
"""Enumerate hardware devices and exercise each package's snapshot contract tests."""

from __future__ import annotations

import json
import subprocess
from pathlib import Path

from finding import EXIT_HARNESS, EXIT_OK, EXIT_VIOLATION, coverage, harness, violation

RULE = "ARCH-SNAP-1"
ROOT = Path(__file__).resolve().parents[2]
SUBJECTS = ("internal/bus/device.go", "internal/bus/bustest/bustest.go",
            "conformance/checkers/devicecensus/main.go", "internal/memory")


def command(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, cwd=ROOT, capture_output=True, text=True, check=False)


def main() -> int:
    seen = {path: int((ROOT / path).exists()) for path in SUBJECTS}
    devices = []
    if all(seen.values()):
        census = command("go", "run", "./conformance/checkers/devicecensus")
        if census.returncode:
            print(harness(RULE, f"device census could not run: {census.stderr.strip()}"))
            return EXIT_HARNESS
        try:
            devices = json.loads(census.stdout)
        except (json.JSONDecodeError, TypeError) as exc:
            print(harness(RULE, f"device census returned invalid JSON: {exc}"))
            return EXIT_HARNESS
    seen["hardware devices"] = len(devices)
    found = coverage(RULE, "device-census-empty", seen)
    if found:
        print(found)
        return EXIT_VIOLATION

    findings = []
    for device in devices:
        name = f"{device['package']}.{device['type']}"
        missing = [method for method, present in zip(("Snapshot", "Restore"), device["methods"])
                   if not present]
        if missing:
            findings.append(violation(RULE, "device-missing-snapshot",
                                      f"{name} has no {', '.join(missing)} method"))
        if not missing and not device["contract"]:
            findings.append(violation(RULE, "device-missing-contract-test",
                                      f"{name} has no Test<Type>HoldsTheDeviceContract calling bustest.CheckSnapshot"))

    packages = sorted({device["package"] for device in devices
                       if device["contract"] and all(device["methods"])})
    for package in packages:
        tests = command("go", "test", "-json", "./" + package, "-run", "HoldsTheDeviceContract")
        try:
            events = [json.loads(line) for line in tests.stdout.splitlines() if line.strip()]
        except json.JSONDecodeError as exc:
            print(harness(RULE, f"go test for {package} emitted invalid JSON: {exc}"))
            return EXIT_HARNESS
        failed = {event.get("Test") for event in events
                  if event.get("Action") == "fail" and event.get("Test")}
        ran = {event.get("Test") for event in events
               if event.get("Action") in ("pass", "fail") and event.get("Test")}
        expected = {device["contract"]
                    for device in devices if device["package"] == package
                    and device["contract"] and all(device["methods"])}
        if not expected.issubset(ran):
            print(harness(RULE, f"go test for {package} did not run {sorted(expected-ran)}: {tests.stderr.strip()}"))
            return EXIT_HARNESS
        for test in sorted(failed):
            details = [event.get("Output", "").strip() for event in events
                       if event.get("Test") == test and event.get("Action") == "output"]
            findings.append(violation(RULE, "device-contract-failed",
                                      f"{package}/{test} failed: {' '.join(details)[-300:]}"))
        if tests.returncode and not failed:
            print(harness(RULE, f"go test for {package} failed without a contract assertion: {tests.stderr.strip()}"))
            return EXIT_HARNESS

    for finding in findings:
        print(finding)
    return EXIT_VIOLATION if findings else EXIT_OK


if __name__ == "__main__":
    raise SystemExit(main())
