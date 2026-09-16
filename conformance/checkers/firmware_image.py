#!/usr/bin/env python3
"""Refuse firmware bytes in tracked source or the built runtime image."""

from __future__ import annotations

import hashlib
import io
import os
import re
import subprocess
import tarfile
from pathlib import Path

from finding import EXIT_HARNESS, EXIT_OK, EXIT_VIOLATION, coverage, harness, violation

RULE = "ARCH-FW-1"
ROOT = Path(__file__).resolve().parents[2]
SUBJECTS = ("Dockerfile", ".dockerignore", "firmware/MANIFEST.md")
U202_OFFSET = 0x12584
U202_MAGIC = bytes.fromhex("4a42a007")


def command(*args: str) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(args, cwd=ROOT, capture_output=True, check=False)


def manifest() -> dict[str, tuple[int, str]]:
    rows = {}
    for line in (ROOT / "firmware/MANIFEST.md").read_text().splitlines():
        match = re.match(r"\|\s*`([^`]+\.bin)`\s*\|\s*(\d+)\s*\|\s*`([a-f0-9]{64})`", line)
        if match:
            rows[match[1]] = (int(match[2]), match[3])
    return rows


def is_firmware(path: str, data: bytes, known: dict[str, tuple[int, str]]) -> bool:
    if Path(path).name in known:
        return True
    for size, digest in known.values():
        if len(data) == size and hashlib.sha256(data).hexdigest() == digest:
            return True
    return len(data) == known["FLASH_U202.bin"][0] and data[U202_OFFSET:U202_OFFSET + 4] == U202_MAGIC


def main() -> int:
    absent = coverage(RULE, "firmware-subject-missing",
                      {path: int((ROOT / path).is_file()) for path in SUBJECTS})
    if absent:
        print(absent)
        return EXIT_VIOLATION
    known = manifest()
    if set(known) != {"FLASH_U202.bin", "FLASH_U203.bin", "application-ram-image.bin"}:
        print(harness(RULE, "firmware manifest no longer provides all three sizes and SHA-256 digests"))
        return EXIT_HARNESS

    tracked = command("git", "ls-files", "-z")
    if tracked.returncode:
        print(harness(RULE, f"cannot enumerate tracked files: {tracked.stderr.decode(errors='replace')}"))
        return EXIT_HARNESS
    findings = []
    for raw in tracked.stdout.split(b"\0"):
        if not raw:
            continue
        path = raw.decode()
        source = ROOT / path
        if not source.is_file():
            continue
        size = source.stat().st_size
        if Path(path).name not in known and size not in {item[0] for item in known.values()}:
            continue
        if is_firmware(path, source.read_bytes(), known):
            findings.append(violation(RULE, "firmware-in-repository",
                                      f"tracked file {path} contains a firmware image"))

    tag = f"goretrotv-conformance-fw-{os.getpid()}"
    built = command("docker", "build", "--target", "runtime", "-t", tag, ".")
    if built.returncode:
        print(harness(RULE, f"runtime image could not build: {built.stderr.decode(errors='replace')[-600:]}"))
        return EXIT_HARNESS
    container = ""
    try:
        created = command("docker", "create", tag)
        if created.returncode:
            print(harness(RULE, f"built image could not be inspected: {created.stderr.decode(errors='replace')}"))
            return EXIT_HARNESS
        container = created.stdout.decode().strip()
        exported = command("docker", "export", container)
        if exported.returncode:
            print(harness(RULE, f"built image could not be exported: {exported.stderr.decode(errors='replace')}"))
            return EXIT_HARNESS
        with tarfile.open(fileobj=io.BytesIO(exported.stdout)) as archive:
            for member in archive:
                if not member.isfile():
                    continue
                if Path(member.name).name not in known and member.size not in {item[0] for item in known.values()}:
                    continue
                stream = archive.extractfile(member)
                if stream is not None and is_firmware(member.name, stream.read(), known):
                    findings.append(violation(RULE, "firmware-in-image",
                                              f"runtime image contains {member.name}"))
    finally:
        if container:
            command("docker", "rm", "-f", container)
        command("docker", "image", "rm", "-f", tag)

    for finding in findings:
        print(finding)
    return EXIT_VIOLATION if findings else EXIT_OK


if __name__ == "__main__":
    raise SystemExit(main())
