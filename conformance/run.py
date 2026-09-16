#!/usr/bin/env python3
"""The conformance entry point. One command, every rule, one verdict.

Three things happen, in this order, and the order matters:

  1. registry integrity — the catalogue and the registry must agree about which rules exist,
     every enforced rule must have a checker on disk and every probe it declares, and no rule
     may sit un-enforced without naming a task that still exists. Nothing below is meaningful if
     this fails, so it runs first and its failure is fatal.
  2. every enforced rule's checker, run against the working tree.
  3. every probe every enforced rule declares, each in its own isolated worktree — each
     violation must still be caught INDEPENDENTLY, because a rule's detectors can sit in
     parallel and one probe cannot hold two of those. See conformance/run-probes.sh.

Then a line per rule, for EVERY id the catalogue names. A rule that is not enforced is reported
as not enforced, next to the task that owes it. That is the difference between a suite that is
green and a suite that is complete, and conflating the two is how a catalogue becomes decoration.
"""

from __future__ import annotations

import argparse
import subprocess
import re
import textwrap
import sys
import tomllib
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from task_source import unchecked_notice  # noqa: E402


ROOT = Path(subprocess.run(["git", "rev-parse", "--show-toplevel"],
                           capture_output=True, text=True, check=True).stdout.strip())
REGISTRY = ROOT / "conformance" / "rules.toml"

RED, GREEN, YELLOW, DIM, RESET = "\033[31m", "\033[32m", "\033[33m", "\033[2m", "\033[0m"


def paint(colour: str, text: str) -> str:
    return f"{colour}{text}{RESET}" if sys.stdout.isatty() else text


def load() -> tuple[dict, list[dict]]:
    with REGISTRY.open("rb") as fh:
        doc = tomllib.load(fh)
    return doc.get("harness", {}), sorted(doc.get("rule", []), key=lambda r: r["id"])


def run(argv: list[str], echo: bool) -> tuple[int, str]:
    proc = subprocess.run(argv, cwd=ROOT, capture_output=True, text=True)
    out = proc.stdout + proc.stderr
    if echo and out.strip():
        print("\n".join(f"    {line}" for line in out.rstrip().splitlines()))
    return proc.returncode, out


# A rule id at the head of a probe-runner line, so the report can attribute a failure to the
# rule it belongs to rather than to all of them.
RULE_ID = re.compile(r"[A-Z][A-Z0-9]*(?:-[A-Z0-9]+)+")


def strip_ansi(text: str) -> str:
    return re.sub(r"\033\[[0-9;]*m", "", text)


# The checker vocabulary, restated rather than imported: `finding.py` lives beside the
# checkers and this file must not depend on a path it computes later in its own body.
# It is one integer and it is part of the protocol — if it ever changes, it changes here
# too, and the probe runner asserts the same value.
EXIT_HARNESS = 2


def stream(argv: list[str], *, seen: set[str] | None = None) -> int:
    """Run a command with its output reaching the terminal as it arrives, indented to match `run`.

    The probe pass is minutes long. Capturing it and printing it at the end leaves a developer
    watching a blank terminal for the whole of it, unable to tell a slow probe from a hung one —
    which is its own reason to stop running the gate, and a gate nobody runs is a rule that cannot
    fail by a slower route.
    """
    proc = subprocess.Popen(argv, cwd=ROOT, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT, text=True, bufsize=1)
    seen = seen if seen is not None else set()
    if proc.stdout is not None:
        for line in proc.stdout:
            print(f"    {line.rstrip()}", flush=True)
            # Which RULES the probe pass actually failed on, remembered as it goes. Without this
            # the report had only the overall exit code, so ONE failing probe rendered every
            # enforced rule UNPROVEN — nine of them, when eight were fine and the one that mattered
            # was named three screens earlier. An over-stated verdict is a different dishonesty
            # from an over-stated pass, and it is worse for finding the fault.
            clean = strip_ansi(line).strip()
            head = clean.split(":", 1)[0].strip() if ":" in clean else ""
            if RULE_ID.fullmatch(head) and "caught by" not in clean:
                # Attribute by EXCLUSION rather than by listing the failure phrasings. The first
                # version matched two of them and missed "could not create the probe worktree",
                # so a real per-rule failure fell through to "blame every rule". There is exactly
                # one success sentence and many ways to fail, so the success sentence is what to
                # recognise.
                seen.add(head)
    return proc.wait()


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--only", metavar="RULE-ID", action="append", default=[],
                    help="run just these rules (checker AND every probe they declare — there is "
                         "no way to run one without the other, because either alone proves "
                         "nothing)")
    ap.add_argument("--classification", metavar="CLASS", action="append", default=[],
                    help="run just the rules of this classification (STATIC, RUNTIME, SCHEMA). "
                         "A gate is often split across environments — a fast job that needs "
                         "nothing, and a slower one that has a built image and a running stack — "
                         "and a RUNTIME rule reports honestly that it could not run rather than "
                         "passing, so it must be RUN where its subject exists. Rules not selected "
                         "are counted as `skipped` and named, because a rule nobody runs anywhere "
                         "is the decoration this harness exists to prevent.")
    args = ap.parse_args()

    harness, rules = load()
    catalogue = harness.get("catalogue", "plan/architecture-rules.md")
    known = {r["id"] for r in rules}
    for rid in args.only:
        if rid not in known:
            print(paint(RED, f"no such rule: {rid}"))
            return 2

    wanted = {c.upper() for c in args.classification}
    known_classes = {r["classification"].upper() for r in rules}
    for c in wanted - known_classes:
        print(paint(RED, f"no rule is classified {c}; the catalogue has {sorted(known_classes)}"))
        return 2
    selected = [r for r in rules
                if (not args.only or r["id"] in args.only)
                and (not wanted or r["classification"].upper() in wanted)]
    enforced = [r for r in selected if r["state"] == "enforced"]

    print("── registry integrity " + "─" * 50)
    integrity_rc, _ = run([sys.executable, "conformance/check_registry.py"], echo=True)

    results: dict[str, tuple[str, str]] = {}
    if enforced:
        print()
        print("── rules " + "─" * 63)
    for rule in enforced:
        rid = rule["id"]
        print(f"  {rid} …", flush=True)
        rc, out = run(rule["command"], echo=False)
        if rc == 0:
            results[rid] = ("pass", "")
        elif rc == EXIT_HARNESS:
            # NOT a violation, and the distinction is the whole reason finding.py defines a third
            # exit code. Collapsed into FAIL, a stopped docker daemon reads as three architecture
            # violations — and somebody goes looking for a decision that was never contradicted.
            # It still fails the run: a rule nobody could evaluate is not a rule that held.
            detail = next((ln for ln in out.splitlines() if "HARNESS:" in ln), "")
            results[rid] = ("NOT RUN", detail.strip())
            print("\n".join(f"    {ln}" for ln in out.rstrip().splitlines()))
        else:
            detail = next((ln for ln in out.splitlines() if rid in ln and "FAIL" in ln), "")
            results[rid] = ("FAIL", detail.strip())
            print("\n".join(f"    {ln}" for ln in out.rstrip().splitlines()))

    probes_rc = 0
    probes_failed: set[str] = set()
    if enforced:
        print()
        print("── probes " + "─" * 62)
        probes_failed: set[str] = set()
        # ONLY rules whose checker found the corpus clean. A probe estate proves that a rule can
        # still fail; it cannot prove anything about a corpus that is ALREADY violated, because
        # every probe's clean baseline fails before the violation is even applied. Running them
        # anyway turned a genuine finding into "UNPROVEN — checker clean" with the summary reading
        # "0 failing", which is the report naming the harness instead of the decision. It misled
        # the author of that message inside a day.
        probe_targets = [r for r in enforced if results.get(r["id"], ("", ""))[0] == "pass"]
        withheld = [r["id"] for r in enforced if r not in probe_targets]
        for rid in withheld:
            state = results.get(rid, ("", ""))[0]
            print(paint(DIM, f"    {rid}: probes not run — its checker reported the working tree "
                             f"as {state}, so a probe's clean baseline cannot hold."))
        probes_rc = stream(["conformance/run-probes.sh", *[r["id"] for r in probe_targets]],
                           seen=probes_failed) if probe_targets else 0
        if probes_rc != 0 and not probes_failed:
            # The pass failed and named no rule — it fell over before it could. Every rule with
            # probes is then genuinely unproven, and saying so is right rather than pessimistic.
            probes_failed = {r["id"] for r in enforced}

    # ---------------------------------------------------------------- the report
    print()
    print(f"── every rule in {catalogue} " + "─" * max(4, 44 - len(catalogue)))
    # An empty catalogue is a real intermediate state — the harness is installed and nothing has
    # bound a decision to a rule yet. It must REPORT that, loudly, rather than crash (which reads
    # as a broken tool and gets muted) or print a clean summary over nothing (which is the exact
    # vacuous pass this harness exists to prevent). Non-zero, because a project with no rules is
    # not a project whose architecture has been checked.
    if not rules:
        print("    no rules in the catalogue.")
        print("    The harness is installed and NOTHING IS ENFORCED. This is not a pass:")
        print("    bind each recorded decision to a rule, or to an explicit 'none, because...'.")
        return 1
    width = max(len(r["id"]) for r in rules)
    counts = {"pass": 0, "FAIL": 0, "NOT RUN": 0, "UNPROVEN": 0, "pending": 0, "deferred": 0, "blocked": 0, "skipped": 0}
    # Rules that pass but whose coverage is not uniform across their parts. Reported by name at the
    # end, because a flat `pass` on a partly-held rule overstates what is guaranteed, and the whole
    # thesis here is that a green check reads as a guarantee.
    partial: list[str] = []
    # Every un-enforced rule with the reason it is not enforced, printed as a block at the end. The
    # per-rule line has room for an owner id and nothing more, and the owner is the half of the
    # answer that does not tell a reader whether the catalogue is complete.
    unenforced: list[tuple[str, str, str, str]] = []
    for rule in rules:
        rid, state = rule["id"], rule["state"]
        if state == "enforced":
            if rid not in results:
                verdict, colour, detail = ("skipped", YELLOW,
                                           "not selected by --only/--classification")
            elif results[rid][0] == "FAIL":
                # THE CORPUS IS VIOLATED, AND THAT IS THE FINDING. It takes precedence over every
                # statement about the probe estate: the probes of a violated rule cannot run, and
                # saying so instead would bury the one line a reader needs.
                verdict, colour = "FAIL", RED
                detail = results[rid][1]
            elif results[rid][0] == "NOT RUN":
                verdict, colour = "NOT RUN", RED
                detail = results[rid][1] or "the checker could not do its job"
            elif not rule.get("probes"):
                # An enforced rule with no probes has never been shown to fail. check_registry and
                # run-probes.sh both say so and both turn the run red — and the TABLE used to print
                # `pass`, which is the line people quote.
                verdict, colour = "UNPROVEN", RED
                detail = "enforced but declares no probes, so nothing has shown it can fail"
            elif rid in probes_failed:
                # The rule's own checker said the corpus is clean, and its probes could not be run
                # or were not caught. Rendering that as `pass` is the exact thing this harness
                # refuses everywhere else: a green line reading as a guarantee it does not carry.
                # The overall verdict already fails; the TABLE must not disagree with it, because
                # the table is what people read.
                verdict, colour = "UNPROVEN", RED
                detail = "checker clean, but its probes did not run or were not caught"
            else:
                outcome, detail = results[rid]
                verdict = outcome
                colour = GREEN if outcome == "pass" else RED
        else:
            verdict, colour = state, YELLOW
            detail = f"owed by {rule.get('owner_task', '?')}"
            # Print the REASON, not only the owner. A reader deciding whether this catalogue is
            # complete needs to know whether an un-enforced rule is waiting on a subject that does
            # not exist or on nobody having written it, and a task id answers neither.
            unenforced.append((rid, state, rule.get("owner_task", "?"), rule.get("reason", "")))
        counts[verdict] = counts.get(verdict, 0) + 1
        halves = rule.get("halves") or []
        if verdict == "pass" and halves:
            probed = sum(1 for h in halves if h.get("held_by") == "probe")
            if probed < len(halves):
                partial.append(rid)
                detail = (detail + "; " if detail else "") + \
                    f"PARTLY HELD — {probed} of {len(halves)} halves probe-backed"
        line = (f"  {rid:<{width}}  {paint(colour, verdict.ljust(8))}  "
                f"{rule['classification']}  {rule['arming']}")
        if detail:
            line += paint(DIM, f"  — {detail}")
        print(line)
        if verdict == "pass":
            for half in halves:
                # The probe line names WHICH probe, because a rule declares one per detector and
                # "held by a probe" is only checkable if the reader can find the violation. An
                # UNHELD half names its detectors instead, because "not held" is only actionable
                # if the reader can see which failures nothing has been shown to catch — and that
                # is precisely what a half naming a probe it out-runs used to hide.
                held_by = half.get("held_by", "")
                mark = {"probe": f"probe {half.get('probe', '?')}",
                        "negative control": "negative control",
                        "pending": f"NOT HELD — owed by {half.get('owner_task', '?')}"}
                held = mark.get(held_by, held_by or "?")
                if held_by != "probe" and half.get("detectors"):
                    held += " [" + ", ".join(half["detectors"]) + "]"
                hue = GREEN if held_by == "probe" else YELLOW
                print(f"  {'':<{width}}    {paint(hue, held)}: {half.get('half', '')}")

    print()
    total = len(rules)
    print(f"  {total} rules: {counts['pass']} enforced and passing, "
          f"{counts['FAIL']} failing, {counts['NOT RUN']} not run, "
          f"{counts['UNPROVEN']} unproven, {counts['pending']} pending, "
          f"{counts['deferred']} deferred, {counts['blocked']} blocked"
          + (f", {counts['skipped']} not selected" if counts["skipped"] else ""))
    if counts["pending"] or counts["deferred"] or counts["blocked"]:
        print(paint(DIM, "  Pending and deferred rules are NOT enforced. Each names the task that "
                         "owes it; check_registry.py fails if that task disappears."))
    if unenforced:
        print()
        print(paint(DIM, "  Why each un-enforced rule is not enforced — required in the registry, "
                         "printed here, because"))
        print(paint(DIM, "  a task id says who owes it and not whether it COULD be written today:"))
        for rid, state, owner, reason in unenforced:
            print(paint(YELLOW, f"    {rid}  {state}  (owed by {owner})"))
            for line in textwrap.wrap(reason or "<none>", 92,
                                      initial_indent="      ", subsequent_indent="      "):
                print(paint(DIM, line))
    notice = unchecked_notice(harness)
    if notice:
        print(paint(YELLOW, f"  ! {notice}"))
    if partial:
        print(paint(YELLOW, f"  {len(partial)} passing rule(s) are only PARTLY probe-backed: "
                            f"{', '.join(partial)}."))
        print(paint(DIM, "  Their remaining halves are held by an in-checker negative control or "
                         "are owed by another task — both weaker than a probe. The harness places "
                         "no limit on how many probes a rule may declare, so a negative control "
                         "here means the half genuinely resists a probe. The lines above say "
                         "which is which."))

    failed = (integrity_rc != 0 or counts["FAIL"] or counts["NOT RUN"]
              or counts["UNPROVEN"] or probes_rc != 0)
    print()
    if failed:
        why = []
        if integrity_rc:
            why.append("registry integrity")
        if counts["FAIL"]:
            why.append(f"{counts['FAIL']} rule(s)")
        if probes_rc:
            why.append("probes")
        print(paint(RED, "conformance: FAIL — " + ", ".join(why)))
        return 1
    print(paint(GREEN, "conformance: ok"))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
