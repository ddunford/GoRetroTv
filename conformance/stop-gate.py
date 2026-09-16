#!/usr/bin/env python3
"""STOP-GATE: neutralise one detector at a time and confirm the estate reacts correctly.

A RULE IS NOT ARMED UNTIL THIS HAS BEEN RUN FOR EVERY DETECTOR IT DECLARES, and the reason is that
a probe caught by a SIBLING detector counts as caught. Such a probe has silently stopped testing
what it was written for: delete the detector it names and the run stays green. Nothing else in this
harness can see that — `run.py` sees a caught probe, `check_registry.py` sees a declared one.

For each detector in turn this disables ONLY that detector — by renaming the id it reports, so
every sibling detector keeps working normally — and requires BOTH directions:

    the probe(s) declaring it go UNCAUGHT      else the catch was never that detector's
    every sibling probe stays CAUGHT           else the detectors are not independent

Both are findings. The first is the one people expect; the second catches a checker that fails for
one reason and gets credited for several, which is how a rule ends up with four probes and one
assertion.

WHY IT RUNS THE CHECKER DIRECTLY rather than through run-probes.sh: the subject here is the
DETECTORS, not the runner. run.py already proves on every run that the runner catches each probe;
this proves each catch belongs to the detector that claims it. Keeping the two instruments separate
means neither can mask a defect in the other.

It is deliberately NOT part of `./ctl.sh conformance`: it copies the tree once per probe, mutates
checkers, and answers a question that only changes when a detector or a probe changes. Run it when
arming a rule, and when changing one.
"""

from __future__ import annotations

import ast
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
NEUTRALISED = "neutralised-for-the-stop-gate"
EXIT_HARNESS = 2

GREEN, RED, YELLOW, RESET = "\033[32m", "\033[31m", "\033[33m", "\033[0m"



def _discard(ws: Path) -> None:
    """Remove a workspace, and SAY SO if it cannot be. `ignore_errors=True` hid 73 leftover copies
    on this host — the quiet version of the same defect the probe runner had."""
    shutil.rmtree(ws, ignore_errors=True)
    if ws.exists():
        print(f"      WARNING: could not remove {ws} — it is orphaned and needs removing by hand")



def workspace() -> Path:
    """A throwaway copy of the WORKING TREE.

    The tree, not the index — this instrument is asked about the checker as it stands right now,
    which is the state an author is iterating on. (run-probes.sh builds from the INDEX instead, and
    that difference has already cost an afternoon: an unstaged checker is simply absent from a
    worktree built that way, so every probe exits 127 and the rule reports `pass` on a clean corpus
    it never inspected.)
    """
    tmp = Path(tempfile.mkdtemp(prefix="stop-gate-"))
    subprocess.run(
        ["rsync", "-a", "--exclude", "node_modules", "--exclude", ".git", "--exclude", ".venv",
         "--exclude", "dist", "--exclude", "__pycache__", f"{ROOT}/", f"{tmp}/"],
        check=True, capture_output=True,
    )
    # IT MUST BE A GIT REPOSITORY. A probe that adds a file has to stage it or the corpus cannot
    # see it, so `git add` inside an apply.sh is normal and correct — and in a plain copy it exits
    # 128. The first version of this file rsynced without one and scored three apply failures as
    # "the sibling went quiet", reporting a defect in ARCH-QUEUE-2 that did not exist. A probe that
    # could not be applied is not a result.
    for cmd in (["git", "init", "-q", "."], ["git", "add", "-A"]):
        subprocess.run(cmd, cwd=tmp, check=True, capture_output=True)
    return tmp


def violation_sites(checker: Path) -> list[tuple[str, str, int, int, int]]:
    """Every `violation(rule, "detector", ...)` call site: (rule, detector, line, col, end col).

    Sites, not ids — and that distinction is the whole instrument. A detector id written at TWO
    call sites is two mechanisms wearing one name, and the failure that hides behind it is precisely
    the one this gate exists to find: disable one branch and the probe is still caught by the other,
    so the report stays green over an assertion nobody is testing. Renaming every occurrence at once
    would make that pair look healthy. Each site is therefore disabled on its own, which is what a
    person commenting out a single branch by hand actually does.
    """
    src = checker.read_text()
    sites: list[tuple[str, str, int, int, int]] = []
    consts = {
        t.id: n.value.value
        for n in ast.parse(src, filename=str(checker)).body
        if isinstance(n, ast.Assign) and isinstance(n.value, ast.Constant)
        and isinstance(n.value.value, str)
        for t in n.targets if isinstance(t, ast.Name)
    }

    def literal(node):
        if isinstance(node, ast.Constant) and isinstance(node.value, str):
            return node.value
        if isinstance(node, ast.Name):
            return consts.get(node.id)
        return None

    for node in ast.walk(ast.parse(src, filename=str(checker))):
        if not isinstance(node, ast.Call):
            continue
        func = node.func
        called = (func.id if isinstance(func, ast.Name)
                  else func.attr if isinstance(func, ast.Attribute) else None)
        # Both emitters in the finding vocabulary — a coverage detector must be
        # blindable like any other, or it is a detector nothing proves.
        if called not in {"violation", "coverage"} or len(node.args) < 2:
            continue
        rule_id, arg = literal(node.args[0]), node.args[1]
        if rule_id is None or not (isinstance(arg, ast.Constant) and isinstance(arg.value, str)):
            raise SystemExit(
                f"stop-gate: {checker.name}:{node.lineno} raises a finding whose rule or detector "
                f"id is not written at the call site, so that branch cannot be disabled on its own "
                f"— and a branch that cannot be disabled cannot be proven."
            )
        sites.append((rule_id, arg.value, arg.lineno, arg.col_offset, arg.end_col_offset))
    return sites


def neutralise(ws: Path, checker: str, site: tuple[str, str, int, int, int]) -> None:
    """Blind exactly ONE call site, by renaming the detector it reports.

    Located by syntax node rather than by string replacement, because the same text appears in
    prose — a comment explaining why two mechanisms are separate detectors names both of them — and
    a replacement that lands in a comment neutralises nothing while reporting that it did. That
    failure is indistinguishable from a blind detector, so the anchor is the AST.
    """
    path = ws / checker
    _, _, lineno, start, end = site
    # BYTES, NOT CHARACTERS. `col_offset` and `end_col_offset` are UTF-8 byte offsets, and these
    # checkers use em dashes everywhere — one on the same line before a violation() call would have
    # sliced mid-character and produced a SyntaxError, which the scoring above would then have
    # reported as a detector-independence finding. Every current call site happens to have an ASCII
    # prefix; that is luck, not design.
    lines = path.read_bytes().splitlines(keepends=True)
    line = lines[lineno - 1]
    lines[lineno - 1] = line[:start] + f'"{NEUTRALISED}"'.encode() + line[end:]
    path.write_bytes(b"".join(lines))


def run_probe(ws: Path, rule: dict, probe: str) -> tuple[int, str]:
    apply = ws / "conformance" / "probes" / rule["id"] / probe / "apply.sh"
    r = subprocess.run([str(apply)], cwd=ws, capture_output=True, text=True)
    if r.returncode != 0:
        # NOT a verdict. A probe that could not be applied introduced no violation, so the checker
        # says nothing and the run would score it as "uncaught" or "went quiet" — a defect report
        # about code that is fine. Die instead, naming the probe.
        raise SystemExit(
            f"stop-gate: {rule['id']}/{probe}/apply.sh exited {r.returncode} and introduced no "
            f"violation, so nothing here can be scored.\n{r.stderr.strip()[:400]}"
        )
    command = [str(ws / rule["command"][0]), *rule["command"][1:]]
    env = dict(os.environ)
    # The isolated workspace is an rsync'd copy with no link back to this checkout, so anything a
    # checker needs from the PROJECT rather than from the tree under test has to be handed to it.
    # Today that is the backend virtualenv a RUNTIME rule boots the application with.
    env.setdefault("CONFORMANCE_BACKEND_VENV", str(ROOT / "backend" / ".venv"))
    image = subprocess.run(["python3", "-c",
                            "import sys; sys.path.insert(0, 'conformance'); "
                            "import runtime; print(runtime._image())"],
                           cwd=ROOT, capture_output=True, text=True)
    if image.returncode == 0 and image.stdout.strip():
        env.setdefault("CONFORMANCE_APP_IMAGE", image.stdout.strip())
    c = subprocess.run(["python3", *command], cwd=ws, capture_output=True, text=True, env=env)
    out = c.stdout + c.stderr
    if c.returncode == EXIT_HARNESS or ": FAIL — HARNESS:" in out:
        # NOT a verdict, and emphatically not "the sibling went quiet". A checker that could not do
        # its job says nothing about any detector, and scoring it as silence reports a defect in
        # detectors that are perfectly independent. Observed on the first RUNTIME rule: the copy had
        # no virtualenv, every run exited 2, and the gate reported four detectors as coupled.
        raise SystemExit(
            f"stop-gate: {rule['id']}'s checker COULD NOT RUN in the isolated workspace "
            f"(exit {c.returncode}), so nothing here can be scored.\n"
            f"{out.strip()[:500]}"
        )
    return c.returncode, out


def caught(rc: int, out: str, rule_id: str, detectors: list[str]) -> bool:
    """The runner's own definition: exit exactly 1, and every declared detector attributed."""
    return rc == 1 and all(f"{rule_id}: FAIL [{d}] " in out for d in detectors)


def scoreable(rc: int, out: str, rule_id: str) -> bool:
    """Whether this run said anything at all about the detectors.

    A CRASH IS NOT A VERDICT ABOUT INDEPENDENCE. A traceback exits 1 with no FAIL line, an exit 0
    says the corpus is clean, a signal death says nothing — and each was scored as "goes uncaught
    (correct)" for the blinded probe and "WENT QUIET — the detectors are not independent" for every
    sibling. A false diagnosis of coupling in detectors that are perfectly fine, and for a rule with
    ONE probe the same crash read as a clean STOP-GATE PASSED.

    The blinded run has a positive assertion available and unused: the neutralised id itself. If the
    checker is working, SOMETHING fires under the new name, or the corpus is clean and it exits 0
    cleanly. A non-zero exit with no attributed finding at all is a checker that fell over.
    """
    if rc == 0:
        return True
    return f"{rule_id}: FAIL [" in out


def main() -> int:
    doc = tomllib.loads((ROOT / "conformance" / "rules.toml").read_text())
    only = sys.argv[1:]
    rules = [r for r in doc.get("rule", [])
             if r.get("state") == "enforced" and (not only or r["id"] in only)]
    if not rules:
        print("stop-gate: no enforced rule to neutralise. Nothing here was proven.")
        return 2

    ok = True
    for rule in rules:
        rid = rule["id"]
        probes = {p["name"]: list(p.get("detectors") or []) for p in rule.get("probes") or []}
        checker = ROOT / rule["command"][0]
        sites = [s for s in violation_sites(checker) if s[0] == rid]
        if not sites:
            print(f"\n{rid}  {RED}no violation in {checker.name} belongs to this rule{RESET}")
            ok = False
            continue
        seen = [s[1] for s in sites]
        print(f"\n{rid}  ({len(sites)} call sites, {len(set(seen))} detectors, "
              f"{len(probes)} probes)")
        for site in sites:
            detector, lineno = site[1], site[2]
            own = [n for n, ds in probes.items() if detector in ds]
            siblings = [n for n in probes if n not in own]
            where = f"{detector}" + (f"  ({checker.name}:{lineno})" if seen.count(detector) > 1
                                     else "")
            if not own:
                # DECLARED BUT UNHELD is a state this harness already has a word for, and the two
                # instruments disagreed about it: `check_registry` accepts a detector claimed by a
                # half that is `pending` with an owner_task, or by a negative control, and the
                # stop-gate called the same thing a failure. A detector nobody has AGREED is unheld
                # is still a failure; one that is declared unheld, in writing, with an owner, is a
                # statement — and the run's own PARTLY HELD line already says so.
                claim = next((h for h in rule.get("halves") or []
                              if detector in (h.get("detectors") or [])), None)
                if claim and claim.get("held_by") in ("pending", "negative control"):
                    owed = f" (owed by {claim['owner_task']})" if claim.get("owner_task") else ""
                    print(f"  {YELLOW}unheld{RESET}  {where}")
                    print(f"      declared {claim['held_by']!r} by the half "
                          f"{claim.get('half', '?')!r}{owed} — no probe, and the catalogue says so.")
                    continue
                print(f"  {RED}FAIL{RESET}  {where}")
                print(f"      no probe declares this detector, and no half admits it is unheld. "
                      f"Nothing requires it to fire.")
                ok = False
                continue
            lines, good = [], True

            for probe in own:
                ws = workspace()
                try:
                    neutralise(ws, rule["command"][0], site)
                    rc, out = run_probe(ws, rule, probe)
                    if not scoreable(rc, out, rid):
                        raise SystemExit(
                            f"stop-gate: with {detector!r} blinded, {rid}'s checker exited {rc} "
                            f"and attributed NO finding to any detector — it fell over rather than "
                            f"answering. Nothing here can be scored.\n{out.strip()[:500]}")
                    still = caught(rc, out, rid, probes[probe])
                finally:
                    _discard(ws)
                good = good and not still
                lines.append(f"      own probe {probe}: "
                             + ("STILL CAUGHT — this branch is not what catches it"
                                if still else "goes uncaught (correct)"))

            for probe in siblings:
                ws = workspace()
                try:
                    neutralise(ws, rule["command"][0], site)
                    rc, out = run_probe(ws, rule, probe)
                    if not scoreable(rc, out, rid):
                        raise SystemExit(
                            f"stop-gate: with {detector!r} blinded, sibling probe {probe} made "
                            f"{rid}'s checker exit {rc} with no finding attributed — a crash, not "
                            f"a statement about independence.\n{out.strip()[:500]}")
                    still = caught(rc, out, rid, probes[probe])
                finally:
                    _discard(ws)
                good = good and still
                lines.append(f"      sibling {probe}: "
                             + ("stays caught (correct)" if still else
                                "WENT QUIET — the detectors are not independent"))

            ok = ok and good
            colour = GREEN if good else RED
            print(f"  {colour}{'PASS' if good else 'FAIL'}{RESET}  {where}")
            for line in lines:
                print(line)

    print(f"\n{GREEN}STOP-GATE PASSED{RESET}" if ok else f"\n{RED}STOP-GATE FAILED{RESET}")
    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
