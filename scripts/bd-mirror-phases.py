#!/usr/bin/env python3
"""Mirror bd issue states onto plan/phase-*.md task checkboxes.

bd is the SOURCE OF TRUTH; phase-file checkboxes are a generated,
human-readable projection. Run at close-out boundaries (phase close,
wave close, reconcile) — never hand-tick a box on a bd project.

Mapping (by the [TASK-N.x] convention in issue titles):
  closed (built)          -> - [x]
  closed (DESCOPED/SUPERSEDED close reason) -> - [?]  (parked, NOT done — rule 4: [x] means fully done)
  blocked                 -> - [?]
  deferred                -> - [?]  (parked out of the phase; NOT pending work)
  open/in_progress        -> - [ ]
Lines whose TASK id has no bd issue are left untouched (pre-bd history).
Test-plan files are never touched (TC boxes are QA-owned, not bd state).

Usage: bd-mirror-phases.py [--dry-run|--check] [project_root]

  --dry-run  report the drift, write nothing, exit 0
  --check    report the drift, write nothing, exit 1 IF ANY — for a pre-commit
             hook, so a hand-ticked box or a stale mirror cannot commit quietly.
             `never hand-tick` is a rule with no teeth until something bites.
"""
import json, re, subprocess, sys, os, tempfile

check = "--check" in sys.argv
strict = "--strict" in sys.argv  # ALSO fail on an unwalked test plan, not only on mirror drift
dry = check or "--dry-run" in sys.argv
args = [a for a in sys.argv[1:] if a not in ("--dry-run", "--check", "--strict")]
root = os.path.abspath(args[0]) if args else os.getcwd()

if not os.path.isdir(os.path.join(root, ".beads")):
    sys.exit(f"not a bd project (no .beads/): {root}")

# bd export carries close_reason (bd list --json does not)
tmp = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False)
tmp.close()
try:
    out = subprocess.run(["bd", "export", "-o", tmp.name], cwd=root,
                         capture_output=True, text=True, timeout=120)
    if out.returncode != 0:
        sys.exit(f"bd export failed: {out.stderr.strip()[:200]}")
    issues = [json.loads(l) for l in open(tmp.name, encoding="utf-8") if l.strip()]
finally:
    os.unlink(tmp.name)

PARKED = re.compile(r"^\s*(DESCOPED|SUPERSEDED)", re.I)

# TASK id -> effective state, from issue titles like "[TASK-2c.1] ..." (first tag wins)
state = {}
for i in issues:
    m = re.match(r"\[(TASK-[A-Za-z0-9._\-]+)\]", i.get("title", ""))
    if not m:
        continue
    st = i["status"]
    if st == "closed" and PARKED.match(i.get("close_reason") or ""):
        st = "blocked"  # parked reads as [?], never [x]
    state.setdefault(m.group(1), st)

# `deferred` is here because a deferred task rendered as `[ ]` reads as PENDING WORK,
# and the phase-progress line the session hook builds from these markers then names it
# as the next thing to do. That happened on 2026-09-07: MCP was deliberately deferred
# out of v1 and the hook went on announcing it as next, which would have had the next
# session start by building the thing the operator had just decided not to build.
MARK = {"closed": "x", "blocked": "?", "deferred": "?"}  # everything else -> " "
line_re = re.compile(r"^(\s*- )\[( |x|!|\?)\](\s+`?)(TASK-[A-Za-z0-9._\-]+)(`?)")

changed_total = 0
mirrored = set()  # every TASK id that appears as a task line in some phase file
plan = os.path.join(root, "plan")
for fn in sorted(os.listdir(plan)) if os.path.isdir(plan) else []:
    if not (fn.startswith("phase-") and fn.endswith(".md")) or fn.startswith("test-plan"):
        continue
    path = os.path.join(plan, fn)
    lines = open(path, encoding="utf-8").read().splitlines(keepends=True)
    changed = []
    for n, line in enumerate(lines):
        m = line_re.match(line)
        if not m:
            continue
        mirrored.add(m.group(4))
        if m.group(4) not in state:
            continue
        want = MARK.get(state[m.group(4)], " ")
        if m.group(2) != want:
            lines[n] = line_re.sub(lambda mm: f"{mm.group(1)}[{want}]{mm.group(3)}{mm.group(4)}{mm.group(5)}", line, count=1)
            changed.append(f"{m.group(4)}: [{m.group(2)}] -> [{want}]")
    if changed:
        changed_total += len(changed)
        print(f"{fn}: {len(changed)} marker(s)" + ("" if dry else " updated"))
        for c in changed:
            print(f"  {c}")
        if not dry:
            open(path, "w", encoding="utf-8").write("".join(lines))

# A bd TASK id with no task line in any phase file is NOT mirrored, and the loop above
# skips it in silence — it walks plan lines and asks bd about each, never the reverse.
# Reporting `from {len(state)} bd-tracked ids` while reconciling fewer is the vacuous
# shape this whole mirror exists to prevent: a count that includes what was not checked.
# Found 2026-09-12, when a phase closed with six built-and-closed tasks (follow-ups
# numbered TASK-3.9a, 3.10a/b, 3.11a/b, 1a.3b) recorded nowhere in the phase file, while
# the pre-commit check printed "drifted on 0 marker(s) from 156 bd-tracked TASK ids" —
# having reconciled 150. The phase file is the spec a later reader trusts for what the
# phase delivered, and it was missing a third of phase 3's actual task count.
unmirrored = sorted(state.keys() - mirrored)
if unmirrored:
    print(f"{len(unmirrored)} bd TASK id(s) have no task line in any phase file "
          "(built but unrecorded in the spec):")
    for t in unmirrored:
        print(f"  {t}  [{state[t]}]")

verb = "drifted on" if check else ("would update" if dry else "updated")
print(f"{verb} {changed_total} marker(s); reconciled {len(mirrored & state.keys())} "
      f"of {len(state)} bd-tracked TASK ids, {len(unmirrored)} unmirrored")

# ---------------------------------------------------------------- test plans
#
# A PHASE WHOSE TASKS ARE ALL BUILT WHILE ITS TEST PLAN IS UNWALKED.
#
# This is deliberately NOT "the test plan has open cases" — mid-phase that is the
# normal and correct state, and a check that complains then is one people learn to
# ignore. It fires only on the combination that means something: every TASK box in
# the phase file is `[x]`, and the matching test plan still has `- [ ]` cases nobody
# executed.
#
# Written because it happened. A phase closed with its architecture rules green, its
# unit and component suites green, browser specs green and live walks done — and its
# test plan at ZERO of eleven cases walked. Every automated gate passed and the
# product could not perform its main function against a real input. "Walk the test
# plan" was already a numbered item in the close-out procedure; it was a bullet in a
# checklist, and a rule nobody reads at the moment they skip it is decoration.
#
# The plan file is the QA-owned half and this script never writes to it. It only
# refuses to let a phase LOOK complete while its plan says otherwise.
unwalked = []
for fn in sorted(os.listdir(os.path.join(root, "plan"))) if os.path.isdir(os.path.join(root, "plan")) else []:
    # Phase ids are numeric ("2", "2c") for feature phases and lettered for review-generated
    # ones ("P1", "R2", and "P" for phase-P-{layer}.md) — match both cases.
    m = re.match(r"^phase-([0-9A-Za-z]+)-.*\.md$", fn)
    if not m:
        continue
    phase_path = os.path.join(root, "plan", fn)
    body = open(phase_path, encoding="utf-8").read()
    boxes = re.findall(r"^- \[([ x?!])\]\s+`?TASK-", body, re.M)
    if not boxes:
        continue
    built = sum(1 for b in boxes if b == "x")
    all_done = all(b == "x" for b in boxes)
    # TWO TRIGGERS, and the second one exists because the first missed the case that
    # prompted this. "All tasks built" catches a finished phase whose plan was skipped.
    # It does NOT catch a phase that ships half its tasks and tests none of them —
    # which is what actually happened: four tasks built, the rest blocked on an
    # external gate, and zero of eleven cases walked. A phase reads as in progress and
    # is therefore exempt, while real work has shipped behind an untouched plan.
    # So: all built, OR anything built with NOTHING walked at all.
    if not all_done and built == 0:
        continue  # nothing shipped yet; an untouched plan is correct
    # Two naming conventions: `test-plan-phase-{id}.md` (feature phases) and
    # `test-plan-{phase-file-name}` (review-generated phase-P-{layer}.md files).
    candidates = [os.path.join(root, "plan", f"test-plan-phase-{m.group(1)}.md"),
                  os.path.join(root, "plan", f"test-plan-{fn}")]
    plan_path = next((p for p in candidates if os.path.exists(p)), None)
    if plan_path is None:
        continue
    plan_body = open(plan_path, encoding="utf-8").read()
    open_cases = len(re.findall(r"^- \[ \]", plan_body, re.M))
    walked = len(re.findall(r"^- \[[x!?]\]", plan_body, re.M))
    if open_cases and (all_done or walked == 0):
        unwalked.append((os.path.basename(plan_path), open_cases, walked, built, len(boxes)))

if unwalked:
    print()
    print("PHASES REPORTING BUILT WITH AN UNWALKED TEST PLAN:")
    for name, o, w, built, total in unwalked:
        shipped = "all" if built == total else f"{built} of {total}"
        note = "" if w else "  <- NOTHING walked at all"
        print(f"  {name}: {o} case(s) never executed ({w} walked); {shipped} task(s) built{note}")
    print()
    print("Work has shipped in these phases and their plans do not reflect it. The plan is the")
    print("half that talks to the PRODUCT; every other gate compares the code against something")
    print("somebody wrote down.")
    print("Walk the cases and mark them [x] pass / [!] fail / [?] blocked — never [x] because the")
    print("code looks right. A case that cannot run yet is [?] with the reason it cannot.")
    if not strict:
        print()
        print("(Reported, not fatal. This becomes fatal on any commit that touches a phase file —")
        print(" the moment somebody records a phase as further along than its plan can support.)")

fatal = bool(changed_total) or (strict and (bool(unwalked) or bool(unmirrored)))
# `unmirrored` is fatal only under --strict, deliberately. It reports a real gap in the
# spec, but making it fatal by default would turn every pre-commit on every project red
# the moment a follow-up task is filed and before anyone has had a chance to record it —
# a check that fires at a time you cannot act on it gets suppressed, and then it protects
# nothing. It prints unconditionally; --strict is where it bites.
if check and fatal:
    if changed_total:
        print()
        print("The phase-file checkboxes disagree with bd, which is the source of truth.")
        print("Either a box was hand-ticked, or a close landed without regenerating the mirror.")
        print(f"Fix it by regenerating, never by editing the box: {sys.argv[0]}")
    sys.exit(1)
