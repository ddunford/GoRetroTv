#!/usr/bin/env python3
"""Exercise the real config loader and the container's effective published port."""

from __future__ import annotations

import json
import re
import subprocess
from pathlib import Path

from finding import EXIT_HARNESS, EXIT_OK, EXIT_VIOLATION, coverage, harness, violation

ROOT = Path(__file__).resolve().parents[2]
RULE = "ARCH-DEV-1"
SUBJECTS = ("internal/config/config.go", "docker-compose.yml", ".env.example",
            "conformance/checkers/bindcheck/main.go")
SETTING = re.compile(r"^\s*(?:-\s*)?(?:export\s+)?GORETROTV_BIND_ALL_INTERFACES\s*[:=]", re.MULTILINE)


def command(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, cwd=ROOT, capture_output=True, text=True, check=False)


def main() -> int:
    seen = {path: int((ROOT / path).is_file()) for path in SUBJECTS}
    absent = coverage(RULE, "bind-subject-missing", seen)
    if absent:
        print(absent)
        return EXIT_VIOLATION

    findings = []
    tracked = command("git", "ls-files", "--", "*.yml", "*.yaml", "*.sh", "*.toml", "*.json")
    if tracked.returncode:
        print(harness(RULE, f"cannot enumerate config files: {tracked.stderr.strip()}"))
        return EXIT_HARNESS
    for path in tracked.stdout.splitlines():
        if path.startswith("conformance/probes/"):
            continue
        source = ROOT / path
        if not source.is_file():
            continue
        lines = "\n".join(line for line in source.read_text().splitlines()
                          if not line.lstrip().startswith("#"))
        if SETTING.search(lines) and path != "docker-compose.yml":
            findings.append(violation(RULE, "bind-override-elsewhere",
                                      f"{path} sets the container-only override"))
    compose_source = (ROOT / "docker-compose.yml").read_text()
    if not re.search(r'^\s*GORETROTV_BIND_ALL_INTERFACES:\s*["\']true["\']\s*$',
                     compose_source, re.MULTILINE):
        findings.append(violation(RULE, "container-override-not-literal",
                                  "docker-compose.yml must set the override literally to true"))

    loader = command("go", "run", "./conformance/checkers/bindcheck")
    if loader.returncode:
        print(harness(RULE, f"cannot exercise config.Load: {loader.stderr.strip()}"))
        return EXIT_HARNESS
    try:
        actual = {row["name"]: row["allowed"] for row in json.loads(loader.stdout)}
    except (json.JSONDecodeError, KeyError, TypeError) as exc:
        print(harness(RULE, f"bindcheck emitted malformed results: {exc}"))
        return EXIT_HARNESS
    expected = {"loopback": True, "ipv6-loopback": True, "empty-host": False,
                "ipv4-any": False, "ipv6-any": False, "public-ip": False,
                "container-override": True}
    if actual != expected:
        findings.append(violation(RULE, "non-loopback-accepted",
                                  f"config.Load bind outcomes {actual}, expected {expected}"))

    compose = command("docker", "compose", "--env-file", ".env.example", "config", "--format", "json")
    if compose.returncode:
        print(harness(RULE, f"cannot inspect effective compose config: {compose.stderr.strip()}"))
        return EXIT_HARNESS
    try:
        service = json.loads(compose.stdout)["services"]["goretrotv"]
        ports = service["ports"]
    except (json.JSONDecodeError, KeyError, TypeError) as exc:
        print(harness(RULE, f"compose returned no usable goretrotv port mapping: {exc}"))
        return EXIT_HARNESS
    if not ports or any(port.get("host_ip") != "127.0.0.1" for port in ports):
        findings.append(violation(RULE, "container-port-public",
                                  f"effective compose ports are {ports}, expected host loopback only"))
    for finding in findings:
        print(finding)
    return EXIT_VIOLATION if findings else EXIT_OK


if __name__ == "__main__":
    raise SystemExit(main())
