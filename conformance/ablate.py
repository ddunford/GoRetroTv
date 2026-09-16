#!/usr/bin/env python3
"""SUBJECT ABLATION: remove one subject at a time and require the coverage guard to fire.

WHAT THE OTHER INSTRUMENTS CANNOT SEE. The stop-gate proves a probe is testing its own detector.
`check_registry.py` proves every detector is declared and every half is probe-backed. Both are about
FALSIFIABILITY — can this check fail? — and both are satisfied by a rule inspecting an EMPTY SET
through a detector that is perfectly capable of firing. Nothing asked the other question: is the
rule looking at anything at all?

It was not academic. Three rules counted their subjects together — `len(models) + len(migrations)`
— so removing one left that half inspecting nothing while the other kept the total above zero. Eight
subjects across those rules could vanish in a refactor with the rule still green, and every probe
missed it because each "the subject vanishes" probe removed ALL of them at once. A guard fed by a
sibling is not a guard.

So: for each subject a rule declares, move it aside, leave everything else, and require the rule's
coverage detector to fire. A rule that still says `ok` has a half that is inspecting nothing.

SUBJECTS ARE DECLARED IN THE REGISTRY, not discovered, for the same reason detectors are: a subject
the checker reads and the registry does not name is one nobody has agreed is load-bearing, and
`check_registry.py` refuses an enforced rule that declares none.

Not part of `./ctl.sh conformance`: it copies the tree once per subject and answers a question that
only changes when a checker's reach changes. Run it when arming a rule, and when changing what one
reads.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
import tempfile
import tomllib
from pathlib import Path

ROOT = Path(subprocess.run(["git", "rev-parse", "--show-toplevel"],
                           capture_output=True, text=True, check=True).stdout.strip())
EXIT_HARNESS = 2
GREEN, RED, YELLOW, DIM, RESET = "\033[32m", "\033[31m", "\033[33m", "\033[2m", "\033[0m"


def workspace() -> Path:
    tmp = Path(tempfile.mkdtemp(prefix="ablate-"))
    subprocess.run(
        ["rsync", "-a", "--exclude", "node_modules", "--exclude", ".git", "--exclude", ".venv",
         "--exclude", "dist", "--exclude", "__pycache__", f"{ROOT}/", f"{tmp}/"],
        check=True, capture_output=True,
    )
    for cmd in (["git", "init", "-q", "."], ["git", "add", "-A"]):
        subprocess.run(cmd, cwd=tmp, check=True, capture_output=True)
    return tmp


def ablate(ws: Path, subject: str) -> str:
    """Move a subject aside. Returns what was done, for the report.

    MOVED, NOT DELETED, and that is the realistic case: a subject does not usually disappear, it
    gets renamed in a refactor by somebody who has no idea a rule was reading it. A deletion is the
    version a reviewer would notice.
    """
    target = ws / subject
    if not target.exists():
        raise SystemExit(f"ablate: {subject} does not exist, so removing it proves nothing. "
                         f"Either the registry names a subject the checker no longer reads, or the "
                         f"path is wrong — both are findings.")
    target.rename(target.with_name(target.name + "_moved_by_ablation"))
    return f"moved {subject} aside"


def run_checker(ws: Path, rule: dict) -> tuple[int, str]:
    env = dict(os.environ)
    env.setdefault("CONFORMANCE_BACKEND_VENV", str(ROOT / "backend" / ".venv"))
    command = [str(ws / rule["command"][0]), *rule["command"][1:]]
    r = subprocess.run(["python3", *command], cwd=ws, capture_output=True, text=True, env=env)
    return r.returncode, r.stdout + r.stderr


def main() -> int:
    doc = tomllib.loads((ROOT / "conformance" / "rules.toml").read_text())
    only = sys.argv[1:]
    rules = [r for r in doc.get("rule", [])
             if r.get("state") == "enforced" and (not only or r["id"] in only)]
    if not rules:
        print("ablate: no enforced rule selected. Nothing here was proven.")
        return 2

    ok = True
    for rule in rules:
        rid = rule["id"]
        subjects = rule.get("subjects") or []
        if not subjects:
            print(f"\n{rid}  {RED}declares no subjects{RESET} — nothing can be ablated, so nothing "
                  f"establishes that this rule reads anything.")
            ok = False
            continue
        guard = rule.get("coverage_detector")
        print(f"\n{rid}  ({len(subjects)} subjects, coverage detector {guard!r})")
        for subject in subjects:
            ws = workspace()
            try:
                ablate(ws, subject)
                rc, out = run_checker(ws, rule)
            finally:
                shutil.rmtree(ws, ignore_errors=True)
            clean = re.sub(r"\033\[[0-9;]*m", "", out).strip()
            first = clean.splitlines()[0][:90] if clean else "(no output)"
            if rc == EXIT_HARNESS:
                # Honest: the checker says it could not do its job without that subject. That is a
                # guard of a different kind, and it is not a hole.
                print(f"  {YELLOW}says so{RESET}  {subject:38} exit 2 — could not run without it")
            elif rc == 0:
                ok = False
                print(f"  {RED}HOLE{RESET}     {subject:38} exit 0 — {first}")
                print(f"           {DIM}the rule still passes with this subject gone, so whichever "
                      f"half reads it is inspecting nothing{RESET}")
            elif guard and f"[{guard}]" in clean:
                print(f"  {GREEN}guarded{RESET}  {subject:38} coverage detector fired")
            else:
                ok = False
                print(f"  {RED}WRONG{RESET}    {subject:38} exit {rc}, but not via {guard!r}")
                print(f"           {DIM}{first}{RESET}")

    print(f"\n{GREEN}EVERY SUBJECT IS GUARDED{RESET}" if ok
          else f"\n{RED}A SUBJECT CAN VANISH UNNOTICED{RESET}")
    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
