#!/usr/bin/env python3
"""Registry integrity — the checks that keep the harness itself honest.

This is not one of the architecture rules. It is the layer underneath them: it asserts that the
registry and the prose catalogue agree about which rules exist, that every enforced rule really
has a checker on disk and every probe it declares, and that no rule can sit un-enforced without
naming a task that still exists.

It runs first on every conformance run, because every other result is only meaningful if this
passes. A harness that reports green because it silently did not know a rule existed is the exact
failure this machinery was created to prevent.

Everything project-shaped is read from the registry's `[harness]` table — the catalogue path, the
rule-id grammar, the probe root and where task ids come from — so this file is the same file in
every repository that uses it.

Exit 0 on success, 1 on any finding. Every finding names the rule id.
"""

from __future__ import annotations

import ast
import re
import subprocess
import sys
import tomllib
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from task_source import TaskSourceUnavailable, load as load_tasks, unchecked_notice  # noqa: E402

ROOT = Path(subprocess.run(["git", "rev-parse", "--show-toplevel"],
                           capture_output=True, text=True, check=True).stdout.strip())
REGISTRY = ROOT / "conformance" / "rules.toml"

# `blocked` is the fourth state and it exists because the other three mislabelled a real case.
#
# `deferred` means THE SUBJECT DOES NOT EXIST YET — the route, the table, the worker has not been
# built, so the rule cannot be written. `pending` means it could be written today and nobody has.
# Neither describes a rule whose subject exists and is observable, where what is missing is a
# HARNESS CAPABILITY: no runtime the probe estate can isolate, no interpreter that can import the
# app, no container the checker may build.
#
# The distinction is not pedantry, it is routing. `deferred` tells a reader to wait for a feature;
# `blocked` tells them to fix the harness, and those are different people doing different work.
# Filed under `deferred`, a harness limitation looks like somebody else's roadmap and is never
# actioned.
#
# Observed: a project whose checkers ran on a host interpreter that could neither import the
# application nor parse it, against a container bind-mounted to the main tree — so a probe worktree
# could not affect the running app at all, and EVERY runtime rule was unarmable for one reason that
# had nothing to do with any of them.
STATES = {"enforced", "pending", "deferred", "blocked"}
REQUIRED = ("id", "invariant", "source", "classification", "arming", "state")

# A probe name is a single lowercase-kebab path component, and the grammar is enforced rather than
# assumed. The name comes out of the registry and is concatenated into a path that run-probes.sh
# then EXECUTES, so `..`, an absolute path or an embedded slash would reach outside
# conformance/probes/<rule-id>/ — the directory the protocol calls the security boundary. Kebab-case
# also keeps the run's own output greppable against the directory tree.
#
# A detector id shares the grammar for the second reason only: it is never joined onto a path, but
# it is matched out of checker output by run-probes.sh and read back against this file, and a
# vocabulary shared between a registry, a checker and a shell script has to be spellable in one way.
KEBAB = re.compile(r"[a-z0-9]+(?:-[a-z0-9]+)*\Z")


def load_registry() -> tuple[dict, list[dict]]:
    with REGISTRY.open("rb") as fh:
        doc = tomllib.load(fh)
    return doc.get("harness", {}), doc.get("rule", [])


def check(harness: dict, rules: list[dict]) -> list[str]:
    findings: list[str] = []

    catalogue = ROOT / harness.get("catalogue", "plan/architecture-rules.md")
    probe_root = ROOT / harness.get("probes", "conformance/probes")
    rule_id = re.compile(harness.get("rule_id", r"[A-Z][A-Z0-9]*-[A-Z]+-\d+"))
    definition = re.compile(r"^###\s+`(" + rule_id.pattern + r")`", re.MULTILINE)

    if not catalogue.is_file():
        return [f"the catalogue {harness.get('catalogue')} does not exist. The registry is the "
                f"machine-readable half of a document a person reads; without the document, a rule "
                f"cannot be reviewed."]

    text = catalogue.read_text()
    defined = set(definition.findall(text))
    mentioned = set(rule_id.findall(text))
    by_id: dict[str, dict] = {}

    for rule in rules:
        rid = rule.get("id", "<missing id>")
        if rid in by_id:
            findings.append(f"{rid}: appears twice in the registry. Ids are stable and unique.")
            continue
        by_id[rid] = rule
        for key in REQUIRED:
            if not rule.get(key):
                findings.append(f"{rid}: registry row has no `{key}`.")
        state = rule.get("state")
        if state not in STATES:
            findings.append(f"{rid}: state {state!r} is not one of {sorted(STATES)}.")
        if "probe" in rule:
            findings.append(
                f"{rid}: declares `probe`, which is not a field. A rule declares "
                f"`probes = [{{ name = \"<name>\", detectors = [\"<detector>\", ...] }}, ...]` — "
                f"one entry per deliberate violation, each resolved under "
                f"{probe_root.relative_to(ROOT)}/{rid}/<name>/, each required to be caught on its "
                f"own, and each naming the detector it exists to be caught BY. Left as `probe` the "
                f"row would be silently unprobed, which is the exact shape of failure this file "
                f"exists to refuse."
            )

    # --- the two directions of catalogue <-> registry symmetry -------------------------
    for rid in sorted(mentioned - set(by_id)):
        findings.append(
            f"{rid}: named in {catalogue.name} but absent from the registry, so the harness "
            f"would never report on it. Add a row — `pending` with an owner_task is fine."
        )
    for rid in sorted(set(by_id) - mentioned):
        findings.append(
            f"{rid}: has a registry row but is named nowhere in {catalogue.name}. A rule the "
            f"catalogue does not describe cannot be reviewed."
        )

    # --- per-state obligations ---------------------------------------------------------
    try:
        tasks_known = load_tasks(ROOT, harness)
    except TaskSourceUnavailable as exc:
        return findings + [f"task source: {exc}"]

    for rid, rule in sorted(by_id.items()):
        state = rule.get("state")

        if state in ("pending", "deferred", "blocked"):
            # The phase STOP-GATE, mechanised: no product phase starts until every rule is either
            # ARMED or explicitly un-enforced WITH A REASON. `owner_task` says who owes it;
            # `reason` says why it is not written, and they answer different questions. A task id
            # alone lets "nobody got round to it" and "this cannot be written until the subject
            # exists" look identical on the report — and only one of those is a plan.
            #
            # It earns its keep on the way in rather than later: writing these out for the first
            # time immediately exposed a RUNTIME rule filed `pending` against a task that writes
            # source scanners, which could never have armed it.
            if not rule.get("reason"):
                findings.append(
                    f"{rid}: state is {state} but no `reason`. An un-enforced rule has to say why "
                    f"in the registry, where the report can print it — not only in the catalogue, "
                    f"which nobody opens before trusting a green run."
                )
            owner = rule.get("owner_task")
            if not owner:
                findings.append(
                    f"{rid}: state is {state} but no owner_task. An un-enforced rule with no "
                    f"owner is a rule nobody has agreed to write."
                )
            elif tasks_known is not None and owner not in tasks_known:
                findings.append(
                    f"{rid}: owner_task {owner} is not live in the task source. Renumbering, "
                    f"closing or deleting a task must not silently orphan a rule."
                )
            for forbidden in ("command", "probes"):
                if rule.get(forbidden):
                    findings.append(
                        f"{rid}: state is {state} but it declares `{forbidden}`. Flip state to "
                        f"`enforced` — a rule with a checker that the harness does not run is "
                        f"the decoration this catalogue exists to prevent."
                    )

        if state == "enforced":
            if rid not in defined:
                findings.append(
                    f"{rid}: enforced, but {catalogue.name} has no `### {rid}` section. An "
                    f"enforced rule must say what it forbids and why."
                )
            command = rule.get("command")
            if not command or not isinstance(command, list):
                findings.append(f"{rid}: enforced but has no `command` list.")
            else:
                exe = ROOT / command[0]
                if not exe.exists():
                    findings.append(f"{rid}: command[0] {command[0]} does not exist.")
                elif exe.is_file() and not exe.stat().st_mode & 0o111:
                    findings.append(f"{rid}: command[0] {command[0]} is not executable.")
            if not rule.get("subjects"):
                findings.append( f"{rid}: enforced but declares no `subjects`. A subject is a place the checker READS, and `conformance/ablate.py` moves each aside in turn to prove the coverage detector fires — the one question the stop-gate cannot ask. Without them, nothing establishes that this rule looks at anything at all, and a rule inspecting an empty set satisfies every other check here."
                )
            findings += check_probes(rid, rule, probe_root)
            if not rule.get("tooling"):
                findings.append(
                    f"{rid}: enforced but records no `tooling` rationale. Which tool expresses "
                    f"this rule, and why that one, is what stops the next rule reaching for a "
                    f"new dependency by reflex."
                )
            findings += check_halves(rid, rule, tasks_known)

        elif rule.get("halves"):
            findings.append(
                f"{rid}: declares `halves` but is not enforced. Halves describe how each part of a "
                f"RUNNING rule is held; on an un-enforced rule they describe nothing."
            )

    findings += check_orphan_probes(by_id, probe_root)
    findings += check_emitted_detectors(by_id)
    findings += check_mirror(harness)
    findings += check_coverage(harness, by_id)
    return findings


# --- what the CHECKERS can actually emit -------------------------------------------------
#
# Everything above this point reads the registry against itself and against the tree. None of it
# opens a checker, and that leaves the hole this section closes: a detector that EXISTS IN THE
# CODE but is named nowhere in the registry has no probe, no half, and nothing that notices. The
# report then prints every declared half held and a flat `pass`, over a rule with an assertion
# nobody has ever seen fail.
#
# Verified rather than assumed: a `violation(RULE, "silently-added-detector", ...)` added to a
# live checker left `registry integrity: ok` and the rule green. It is the cheapest possible
# mistake to make — a checker grows a branch, its author adds no probe — and it is invisible from
# every direction except this one.
#
# THE ID MUST BE WRITTEN AT THE POINT THE FINDING IS RAISED. A detector id assembled from a
# variable, a lookup table or an f-string cannot be enumerated here, and more importantly it drifts
# away from the assertion it names — the id stops moving when the code moves. The one indirection
# allowed is a module-level constant bound to a string literal, because a checker serving a single
# rule naming that rule once at the top is clearer, not less traceable.

_SH_VIOLATION = re.compile(r"(?:^|[;&|(]|\s)violation\s+(\S+)\s+(\S+)")
_SH_ASSIGN = re.compile(r"^\s*(?:local\s+|readonly\s+|export\s+)?([A-Za-z_][A-Za-z0-9_]*)="
                        r"\"?'?([A-Za-z0-9][A-Za-z0-9_.-]*)'?\"?\s*$")


def _py_literal(node: ast.AST, consts: dict[str, str]) -> str | None:
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return node.value
    if isinstance(node, ast.Name):
        return consts.get(node.id)
    return None


def emitted_detectors(path: Path) -> tuple[dict[str, set[str]], list[str]]:
    """(rule id -> the detectors this checker can attribute a finding to, problems reading it)."""
    text = path.read_text()
    emitted: dict[str, set[str]] = {}
    problems: list[str] = []

    if path.suffix == ".py":
        try:
            tree = ast.parse(text, filename=str(path))
        except SyntaxError as exc:
            return {}, [f"{path.name} does not parse ({exc}), so its detectors cannot be read."]
        consts = {
            target.id: node.value.value
            for node in tree.body
            if isinstance(node, ast.Assign)
            and isinstance(node.value, ast.Constant)
            and isinstance(node.value.value, str)
            for target in node.targets
            if isinstance(target, ast.Name)
        }
        for node in ast.walk(tree):
            if not isinstance(node, ast.Call):
                continue
            func = node.func
            called = (func.id if isinstance(func, ast.Name)
                      else func.attr if isinstance(func, ast.Attribute) else None)
            # `coverage()` is the other emitter in finding.py: same (rule, detector, …)
            # shape, used for the per-subject guards. A census that knows only one of
            # them reports the other as a detector that has drifted out of the code.
            if called not in {"violation", "coverage"}:
                continue
            kw = {k.arg: k.value for k in node.keywords}
            rule_node = node.args[0] if node.args else kw.get("rule")
            det_node = (node.args[1] if len(node.args) > 1 else kw.get("detector"))
            rule_id = _py_literal(rule_node, consts) if rule_node is not None else None
            detector = _py_literal(det_node, consts) if det_node is not None else None
            if rule_id is None or detector is None:
                problems.append(
                    f"{path.name}:{node.lineno}: the violation's rule id or detector id is not a "
                    f"string literal (nor a module-level constant bound to one), so no probe can "
                    f"be required to trip it. Write the id where the finding is raised."
                )
                continue
            emitted.setdefault(rule_id, set()).add(detector)
        return emitted, problems

    if path.suffix == ".sh" or text.startswith("#!") and "sh" in text.splitlines()[0]:
        assigns = {}
        for line in text.splitlines():
            m = _SH_ASSIGN.match(line)
            if m:
                assigns[m.group(1)] = m.group(2)

        def unshell(token: str) -> str | None:
            token = token.strip("\"'")
            if token.startswith("$"):
                return assigns.get(token.lstrip("${").rstrip("}"))
            return token or None

        for lineno, line in enumerate(text.splitlines(), 1):
            if line.lstrip().startswith("#"):
                continue
            for m in _SH_VIOLATION.finditer(line):
                rule_id, detector = unshell(m.group(1)), unshell(m.group(2))
                if not rule_id or not detector or not KEBAB.fullmatch(detector):
                    problems.append(
                        f"{path.name}:{lineno}: this violation's rule id or detector id could not "
                        f"be resolved to a literal, so no probe can be required to trip it."
                    )
                    continue
                emitted.setdefault(rule_id, set()).add(detector)
        return emitted, problems

    return {}, [
        f"{path.name}: the harness can enumerate detectors in a Python or shell checker and this "
        f"is neither, so nothing can confirm every detector it emits has a probe. Add a reading "
        f"for this kind of checker rather than leaving its coverage unverified."
    ]


def check_emitted_detectors(by_id: dict[str, dict]) -> list[str]:
    """Every detector a checker can emit is declared, and every declared detector can be emitted.

    Both directions, because they rot differently. An UNDECLARED detector is an assertion nobody
    has ever watched fail, wearing the rule's green tick. A DECLARED detector the checker cannot
    emit is an id that drifted — the probe requiring it turns red for a reason that has nothing to
    do with the architecture, and a half claiming it credits coverage to a name no longer in the
    code.

    A detector may be declared by a PROBE or by a `halves` entry that is honest about not having
    one (`negative control`, or `pending` with an owner_task). That is the escape hatch and it is
    deliberately a written, reviewable one: silence is what this refuses, not the absence of a
    probe.
    """
    findings: list[str] = []
    by_checker: dict[Path, list[dict]] = {}
    for rule in by_id.values():
        if rule.get("state") != "enforced":
            continue
        command = rule.get("command")
        if not isinstance(command, list) or not command:
            continue  # already reported by the enforced-state checks
        by_checker.setdefault(ROOT / command[0], []).append(rule)

    for path, rules in sorted(by_checker.items()):
        rids = sorted(r["id"] for r in rules)
        if not path.is_file():
            continue  # already reported
        emitted, problems = emitted_detectors(path)
        findings += [f"{rids[0]}: {p}" for p in problems]

        for rule_id in sorted(set(emitted) - set(rids)):
            findings.append(
                f"{rule_id}: {path.name} emits findings attributed to it, but no enforced rule "
                f"runs that checker for {rule_id}. The findings would be printed by whichever "
                f"rule happened to run and counted against the wrong decision."
            )

        for rule in rules:
            rid = rule["id"]
            can_emit = emitted.get(rid, set())
            declared = {
                d
                for probe in (rule.get("probes") or [])
                if isinstance(probe, dict)
                for d in (probe.get("detectors") or [])
                if isinstance(d, str)
            } | {
                d
                for half in (rule.get("halves") or [])
                if isinstance(half, dict)
                for d in (half.get("detectors") or [])
                if isinstance(d, str)
            }
            if not can_emit:
                findings.append(
                    f"{rid}: {path.name} contains no violation this rule could emit, so the rule "
                    f"cannot fail whatever the corpus contains. A rule that cannot fail is worse "
                    f"than no rule."
                )
                continue
            for detector in sorted(can_emit - declared):
                findings.append(
                    f"{rid}: {path.name} can emit detector {detector!r}, which no probe and no "
                    f"half declares. Nothing has ever shown it fail, and the report counts the "
                    f"rule as held anyway. Give it a probe, or declare the half it belongs to and "
                    f"say honestly how that half is held."
                )
            for detector in sorted(declared - can_emit):
                findings.append(
                    f"{rid}: a probe or half names detector {detector!r}, but {path.name} can "
                    f"never emit it. The id has drifted from the assertion it was written for."
                )
    return findings


def check_coverage(harness: dict, by_id: dict[str, dict]) -> list[str]:
    """No enforced rule may have a classification that nothing is declared to run.

    A gate is often split across environments — a fast job that needs nothing, and a slower one
    with a built image and a running stack. The split is correct and it has a failure mode: add a
    rule of a THIRD classification and it runs in neither job, silently, while the catalogue counts
    it as enforced. Declaring the covered set makes adding one a deliberate act that turns the run
    red until somebody wires a job for it.

    Optional: a project that runs everything in one place declares nothing and this check is inert
    by design rather than by accident — the `unchecked_notice` convention applies.
    """
    covered = harness.get("covered_classifications")
    if not covered:
        return []
    allowed = {str(c).upper() for c in covered}
    findings = []
    for rid, rule in sorted(by_id.items()):
        if rule.get("state") != "enforced":
            continue
        actual = str(rule.get("classification", "")).upper()
        if actual and actual not in allowed:
            findings.append(
                f"{rid}: is classified {actual}, which no CI job is declared to run "
                f"(covered: {sorted(allowed)}). An enforced rule that runs nowhere is decoration "
                f"arriving by omission. Wire a job for it and add it to "
                f"`[harness] covered_classifications`, or reclassify the rule."
            )
    return findings


def check_mirror(harness: dict) -> list[str]:
    """If the catalogue carries a generated index of the registry, it must be current.

    The id-symmetry check above stops a rule existing in one document and not the other. It says
    nothing about whether the row DESCRIBES the rule correctly — a rule can be flipped from
    `pending` to `enforced`, or from ARMED to DEFERRED, with its summary row still stating the old
    answer. That row is what gets quoted into a phase close-out, so a stale one is a false report
    about the state of the architecture rather than a formatting nit.

    Optional: a project without a generated index simply declares no `mirror`.
    """
    mirror = harness.get("mirror")
    if not mirror:
        return []
    path = ROOT / mirror
    if not path.is_file():
        return [f"harness: `mirror` names {mirror}, which does not exist."]
    r = subprocess.run([sys.executable, str(path), "--check"], cwd=ROOT,
                       capture_output=True, text=True)
    if r.returncode == 0:
        return []
    detail = (r.stdout + r.stderr).strip().splitlines() or ["no output"]
    return [f"catalogue index: {line}" for line in detail]


def check_orphan_probes(by_id: dict[str, dict], probe_root: Path) -> list[str]:
    """No probe directory that nothing declares, at either level of the tree.

    Both directions matter and they fail differently. A DECLARED probe with no directory is caught
    by check_probes: the harness would try to run it and could not. An UNDECLARED directory is
    caught here: nothing would ever run it, so it sits in the tree looking like coverage while
    proving nothing.
    """
    findings: list[str] = []
    declared: set[tuple[str, str]] = {
        (rid, probe["name"])
        for rid, r in by_id.items()
        if r.get("state") == "enforced"
        for probe in (r.get("probes") or [])
        if isinstance(probe, dict) and isinstance(probe.get("name"), str)
    }
    enforced_ids = {rid for rid, r in by_id.items() if r.get("state") == "enforced"}
    if not probe_root.is_dir():
        return findings
    for child in sorted(probe_root.iterdir()):
        if not child.is_dir():
            findings.append(
                f"{child.name}: {child.relative_to(ROOT)} is a loose file at the probe root. "
                f"Everything under the probe root is a rule directory."
            )
            continue
        if child.name not in enforced_ids:
            findings.append(
                f"{child.name}: {child.relative_to(ROOT)} exists but no enforced rule claims "
                f"it. Either the rule was un-enforced and its probes were left behind, or the "
                f"directory is misnamed — a probe nothing runs proves nothing."
            )
            continue
        for entry in sorted(child.iterdir()):
            if not entry.is_dir():
                findings.append(
                    f"{child.name}: {entry.relative_to(ROOT)} is a file directly inside the "
                    f"rule's probe directory. Every probe is a DIRECTORY named by the rule's "
                    f"`probes` list; a loose apply.sh here is a probe the harness never runs."
                )
                continue
            if (child.name, entry.name) not in declared:
                findings.append(
                    f"{child.name}: {entry.relative_to(ROOT)} exists but {child.name} does not "
                    f"declare a probe named {entry.name!r}. A probe nothing runs proves "
                    f"nothing, and one sitting beside probes that do run reads as coverage."
                )
    return findings


HELD_BY = {"probe", "negative control", "pending"}


def check_halves(rid: str, rule: dict, tasks_known: set[str] | None) -> list[str]:
    """Every part of a rule must say HOW it is held, WHICH probe holds it, and WHICH DETECTORS
    that probe actually reaches.

    A rule whose detectors sit in PARALLEL — either one alone catches the violation — cannot hold
    them both with a single probe: weaken one and the probe is still caught by the other, so the rot
    is invisible. That is why a rule declares one probe per detector it wants held, and why each
    half that claims a probe has to name it. The link is checked in both directions: a half may not
    name a probe the rule does not declare, and a declared probe that no half claims means the run
    is doing work its own report does not account for.

    Naming the probe is not enough on its own, and that gap is what this function was widened to
    close. A half's SENTENCE can promise more than the probe it names demonstrates — "every pinned
    column is present WITH ITS DECLARED TYPE AND NULLABILITY" claimed a probe that only ever drops
    the column, so the type and nullability detectors read as probe-backed while nothing had shown
    either of them fail. Checking that the probe merely EXISTS cannot see that. So a half now
    declares the `detectors` its sentence is held by, and those must be a subset of the detectors
    the probe it names is required to trip. A half can no longer out-run its probe by being written
    more ambitiously than the violation behind it, and when a probe gains or loses a detector the
    halves have to be restated in the same edit.

    The check runs in both directions here too: every detector a probe declares must be claimed by
    the half that probe holds, or the run is proving something its report never counts. And no
    detector may be claimed by a probe half AND by an unheld one — one line saying it is held while
    the next says nothing does is exactly the mixed message the report exists to remove. Two probe
    halves sharing a detector is fine and deliberate; the comment below the signature says why.

    What is left after that is honest rather than incidental. `negative control` means "this
    genuinely cannot be probed", not "the harness only allowed one" — and the run says which is
    which on every line, because nobody opens a probe README before trusting a green check.

    A `halves` list is REQUIRED on an enforced rule, and that is the point rather than a formality.
    The report prints per-half coverage and its PARTLY HELD warning only for a rule that declares
    halves, so while the field was optional the incentive ran backwards: an author who decomposed a
    rule honestly earned a yellow warning, and one who left the field out got a clean green on a
    rule with eight detectors and one probe. Requiring it means a flat `pass` is a claim someone had
    to make in writing.
    """
    halves = rule.get("halves")
    if not halves:
        return [
            f"{rid}: enforced but declares no `halves`. Every enforced rule states what its parts "
            f"are and how each one is held, because the report prints per-half coverage and its "
            f"PARTLY HELD warning ONLY for a rule that declares them — so omitting the field is "
            f"the one way to make a partly-held rule print a flat `pass`. Declare one half per "
            f"part, each with its `held_by` and the `detectors` it is held by."
        ]
    findings = []
    declared_probes = {p["name"]: list(p.get("detectors") or [])
                       for p in (rule.get("probes") or [])
                       if isinstance(p, dict) and isinstance(p.get("name"), str)}
    # detector -> the half already claiming it, split by whether that half is probe-backed. Two
    # PROBE halves may name one detector: a rule can have two independent routes to the same
    # finding — a queue set can be widened by a whole new supervisor or by a queue merged into an
    # existing one, both surfacing as the same detector — and each route needs its own violation.
    # What is forbidden is a detector claimed as probe-backed by one half and declared unheld by
    # another, because the report would then print it both ways in consecutive lines.
    claimant: dict[str, str] = {}
    unheld_claimant: dict[str, str] = {}
    # probe name -> the detectors the halves claiming that probe say it holds.
    accounted: dict[str, set[str]] = {name: set() for name in declared_probes}
    for half in halves:
        name, held_by = half.get("half"), half.get("held_by")
        if not name or not held_by:
            findings.append(f"{rid}: a `halves` entry is missing `half` or `held_by`: {half!r}.")
            continue
        if held_by not in HELD_BY:
            findings.append(
                f"{rid}: half {name!r} is held_by {held_by!r}, which is not one of "
                f"{sorted(HELD_BY)}."
            )
        detectors = half.get("detectors")
        if detectors is not None and not isinstance(detectors, list):
            findings.append(
                f"{rid}: half {name!r} has `detectors` = {type(detectors).__name__}, not a list."
            )
            detectors = None
        for detector in detectors or []:
            if not isinstance(detector, str) or not KEBAB.fullmatch(detector):
                findings.append(
                    f"{rid}: half {name!r} names detector {detector!r}, which is not a "
                    f"lowercase-kebab detector id."
                )
                continue
            here, there = ((claimant, unheld_claimant) if held_by == "probe"
                           else (unheld_claimant, claimant))
            if detector in there:
                findings.append(
                    f"{rid}: detector {detector!r} is claimed by two halves that disagree about "
                    f"it — {there[detector]!r} and {name!r}. One says a probe demonstrates it and "
                    f"the other says nothing does, so the report prints it held on one line and "
                    f"unheld on the next."
                )
                continue
            here[detector] = name
        if held_by == "probe":
            named = half.get("probe")
            if not named:
                findings.append(
                    f"{rid}: half {name!r} is held_by a probe but does not say which one. Add "
                    f"`probe = \"<name>\"` from this rule's `probes`, or the claim cannot be "
                    f"checked against anything that actually runs."
                )
            elif named not in declared_probes:
                findings.append(
                    f"{rid}: half {name!r} claims probe {named!r}, which is not in this rule's "
                    f"`probes` list. The half is therefore held by nothing that runs."
                )
            if not detectors:
                findings.append(
                    f"{rid}: half {name!r} is held_by a probe but declares no `detectors`. Naming "
                    f"the probe says a violation exists; naming the detectors says WHICH failures "
                    f"that violation demonstrates, and without them the half's sentence can promise "
                    f"more than the probe ever trips."
                )
            elif named in declared_probes:
                accounted[named] |= set(detectors)
                for detector in sorted(set(detectors) - set(declared_probes[named])):
                    findings.append(
                        f"{rid}: half {name!r} claims detector {detector!r}, but probe {named!r} "
                        f"is not required to trip it — it declares "
                        f"{sorted(declared_probes[named])}. The half therefore claims coverage no "
                        f"probe has demonstrated. Narrow the half to what the probe reaches and "
                        f"declare the rest as a separate half, or give {detector!r} a probe."
                    )
        elif half.get("probe"):
            findings.append(
                f"{rid}: half {name!r} is held_by {held_by!r} but still names a probe. Only a half "
                f"held_by a probe names one; anything else reads as coverage it does not have."
            )
        if held_by == "pending":
            owner = half.get("owner_task")
            if not owner:
                findings.append(
                    f"{rid}: half {name!r} is pending but names no owner_task. An unheld half with "
                    f"no owner is one nobody has agreed to write."
                )
            elif tasks_known is not None and owner not in tasks_known:
                findings.append(
                    f"{rid}: half {name!r} names owner_task {owner}, which is not live in the "
                    f"task source."
                )
    if not any(h.get("held_by") == "probe" for h in halves):
        findings.append(
            f"{rid}: no half is held_by a probe, so nothing about this rule has been shown to "
            f"fail. A rule that cannot fail is worse than no rule."
        )
    claimed = {h.get("probe") for h in halves if h.get("held_by") == "probe"}
    for name, detectors in declared_probes.items():
        if name not in claimed:
            findings.append(
                f"{rid}: probe {name!r} runs on every conformance run but no half claims it, so "
                f"the report counts it as holding nothing. Either name it from the half it holds, "
                f"or delete it — an unaccounted probe makes the PARTLY HELD count wrong."
            )
            continue
        for detector in sorted(set(detectors) - accounted[name]):
            findings.append(
                f"{rid}: probe {name!r} is required to trip detector {detector!r}, but no half "
                f"claims it. The run proves that detector still fires and the report credits "
                f"nothing for it — name it from the half it holds, or drop it from the probe."
            )
    return findings


def check_probes(rid: str, rule: dict, probe_root: Path) -> list[str]:
    """Every probe a rule declares exists on disk, is uniquely named, runnable — and says what it
    expects to be caught BY.

    A rule declares a LIST because its detectors can sit in parallel, and each parallel detector
    needs a violation only it catches. One probe per detector, each required independently — so
    weakening any one detector turns the run red on the probe that names it.

    `detectors` is what makes that claim checkable instead of asserted. Without it a probe caught
    by a DIFFERENT detector of the same rule counts as caught, so a probe can silently stop testing
    what it was written for and the only thing in the way is an author replicating each worktree by
    hand. It is a list because a violation can honestly trip more than one — deleting a runtime
    config directive is missing at every SAPI that reads it, and a banned import trips both linters
    that ban it — and requiring ALL of them is strictly stronger than requiring one, never weaker.
    """
    probes = rule.get("probes")
    if not probes:
        return [
            f"{rid}: enforced but declares no `probes`. A rule with no probe has never been shown "
            f"to fail, and a rule that cannot fail is worse than no rule."
        ]
    if not isinstance(probes, list):
        return [f"{rid}: `probes` is {type(probes).__name__}, not a list of probe declarations."]
    findings: list[str] = []
    seen: set[str] = set()
    for probe in probes:
        if not isinstance(probe, dict):
            findings.append(
                f"{rid}: probe {probe!r} is declared as a bare name. A probe is declared as "
                f'{{ name = "<dir>", detectors = ["<detector>", ...] }} — the detectors being what '
                f"the probe exists to be caught BY. Left as a bare name, any detector of the rule "
                f"would count as a catch, so the probe could stop testing what it holds without "
                f"anything going red."
            )
            continue
        name = probe.get("name")
        if not isinstance(name, str) or not KEBAB.fullmatch(name):
            findings.append(
                f"{rid}: probe name {name!r} is not a lowercase-kebab path component. The name is "
                f"joined onto {probe_root.relative_to(ROOT)}/{rid}/ and the result is executed, so "
                f"it may not contain a slash, a dot or an upper-case letter."
            )
            continue
        if name in seen:
            findings.append(
                f"{rid}: declares probe {name!r} twice. Running the same violation twice is not "
                f"two probes, but it counts as two in the report."
            )
            continue
        seen.add(name)
        findings += check_detectors(rid, name, probe)
        findings += check_probe(rid, name, probe_root)
    return findings


def check_detectors(rid: str, name: str, probe: dict) -> list[str]:
    """What this probe expects to be caught by, stated rather than inferred."""
    detectors = probe.get("detectors")
    if not detectors:
        return [
            f"{rid}: probe {name!r} declares no `detectors`. It is required and it is not "
            f"defaulted: a probe whose expected detector is 'whatever fires' is exactly the hole "
            f"this field closes, because a sibling detector catching the violation would keep the "
            f"run green while the detector the probe exists to hold had gone."
        ]
    if not isinstance(detectors, list):
        return [f"{rid}: probe {name!r} has `detectors` = {type(detectors).__name__}, not a list."]
    findings = []
    for detector in detectors:
        if not isinstance(detector, str) or not KEBAB.fullmatch(detector):
            findings.append(
                f"{rid}: probe {name!r} names detector {detector!r}, which is not a "
                f"lowercase-kebab id. run-probes.sh matches it against the "
                f"`{rid}: FAIL [<detector>] — …` lines the checker emits, so an id it cannot "
                f"match would fail the probe for a reason that has nothing to do with the rule."
            )
    if len(set(detectors)) != len(detectors):
        findings.append(
            f"{rid}: probe {name!r} names the same detector twice. Requiring it twice is not two "
            f"requirements."
        )
    return findings


def check_probe(rid: str, name: str, probe_root: Path) -> list[str]:
    findings = []
    probe = probe_root / rid / name
    rel = probe.relative_to(ROOT)
    if not probe.is_dir():
        findings.append(
            f"{rid}: probe directory {rel} is missing. The deliberate violation this rule must "
            f"catch no longer exists, so nothing proves the rule still fails."
        )
        return findings
    readme = probe / "README.md"
    if not readme.is_file() or not readme.read_text().strip():
        findings.append(
            f"{rid}: probe {rel} has no README.md. A probe with no stated violation is a file "
            f"the next reader has to reverse-engineer before they can trust it."
        )
    apply = probe / "apply.sh"
    if not apply.is_file():
        findings.append(f"{rid}: probe {rel} has no apply.sh.")
    elif not apply.stat().st_mode & 0o111:
        findings.append(f"{rid}: probe {rel}/apply.sh is not executable.")
    return findings


def main() -> int:
    harness, rules = load_registry()
    findings = check(harness, rules)
    notice = unchecked_notice(harness)
    if findings:
        print("registry integrity: FAIL")
        for f in findings:
            print(f"  - {f}")
        if notice:
            print(f"  ! {notice}")
        return 1
    print("registry integrity: ok")
    if notice:
        print(f"  ! {notice}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
