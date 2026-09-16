#!/usr/bin/env python3
"""Where the harness learns which owner tasks still exist.

WHY THIS IS A SEAM AND NOT A HARDCODED PATH. A rule that is not enforced yet must name the task
that owes it, and the registry check must be able to tell that the task still exists -- otherwise
`state = "pending"` becomes a mute button: rename or delete the task and the rule waits forever
for something nobody will ever do. That check is the only part of the harness that has to know
anything about how THIS project tracks work, so it is the only part that is configured.

Declared in the registry's `[harness]` table:

    [harness]
    task_source = "beads"          # or "python:<path>", "lines:<path>", "none"
    task_liveness = "exists"       # or "open"

SOURCES

  beads:[<path>]      Read a beads JSONL export (default `.beads/issues.jsonl`). The export is
                      tracked content and needs no `bd` binary on PATH, which matters: the gate
                      that reports whether the architecture is intact should not have a
                      dependency of its own to be missing.

  python:<path>       Import a plan module and call its `task_ids()`, which must return an
                      iterable of ids. IMPORTED RATHER THAN GREPPED, and that is the point: the
                      module is the generator's own source of truth, and a regex over it keeps
                      passing after a task id moves into a variable. The `task_ids()` contract is
                      required because guessing at a module's internal shape is the same class of
                      fragility one level up.

  lines:<path>        Extract ids from a plain file with `task_id` as the pattern. The weakest
                      source and the honest one for a markdown-tracked plan: it is a regex over
                      prose, so it sees an id that has moved into a variable exactly as well as
                      any other regex does, which is to say not at all.

  none                No source. Owner-task liveness is NOT CHECKED, and the run says so on every
                      run -- see `unchecked_notice()`. Declaring `none` is allowed because some
                      repositories genuinely have no machine-readable task list; declaring it
                      SILENTLY is not, because an unchecked obligation that reads as a checked one
                      is the shape of failure this whole harness is against.

LIVENESS: `exists` VERSUS `open`

`exists` asks only whether the id is still in the tracker. It catches the deletion and the
renumbering, which is what the check was built for.

`open` additionally requires the task to be unfinished, and it catches a sharper failure: a rule
parked on a task that has since CLOSED without arming it. The report then prints "owed by <id>"
for work that id will never do -- the mute-button shape, arrived at by a different route.

`open` is not the default, deliberately. It couples the conformance gate to tracker state, so
closing an issue turns the run red until somebody restates the obligation. That is arguably
correct and it is definitely a new failure mode, so it is a decision a project makes in writing
rather than one this file makes for it.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

LIVENESS = ("exists", "open")
DEFAULT_BEADS_EXPORT = ".beads/issues.jsonl"
# Anything a tracker calls "finished". Compared case-folded.
CLOSED_STATES = {"closed", "done", "completed", "resolved", "cancelled", "canceled"}


class TaskSourceUnavailable(RuntimeError):
    """The configured source could not be read, so nothing may be concluded about liveness.

    Raised rather than returning an empty set. An empty set would fail every un-enforced rule at
    once with a message about the rules, when the truth is that the harness cannot see the
    tracker -- and a set assembled from a source that did not load is the vacuous-input failure
    this harness spends most of its guards refusing.
    """


def _beads(root: Path, arg: str, liveness: str) -> set[str]:
    export = root / (arg or DEFAULT_BEADS_EXPORT)
    if not export.is_file():
        raise TaskSourceUnavailable(
            f"task_source is beads but {export.relative_to(root)} does not exist. The export is "
            f"tracked content; run the tracker's export/sync before the conformance run, or point "
            f"`task_source` at the real path."
        )
    ids: set[str] = set()
    for lineno, line in enumerate(export.read_text().splitlines(), 1):
        line = line.strip()
        if not line:
            continue
        try:
            row = json.loads(line)
        except json.JSONDecodeError as exc:
            raise TaskSourceUnavailable(
                f"{export.name}:{lineno} is not JSON ({exc}). A partially-written export would "
                f"otherwise read as a tracker that has lost half its issues."
            ) from exc
        if row.get("_type", "issue") != "issue":
            continue
        rid = row.get("id")
        if not isinstance(rid, str):
            continue
        if liveness == "open" and str(row.get("status", "")).lower() in CLOSED_STATES:
            continue
        ids.add(rid)
    if not ids:
        raise TaskSourceUnavailable(
            f"{export.name} yielded no task ids. An empty task list makes every un-enforced rule "
            f"fail for a reason that has nothing to do with the rules."
        )
    return ids


def _python(root: Path, arg: str, liveness: str) -> set[str]:
    if liveness == "open":
        raise TaskSourceUnavailable(
            "task_liveness = \"open\" is not available for a `python:` source: a plan module lists "
            "the tasks that EXIST and says nothing about which are finished. Use `beads:` for "
            "openness, or declare `exists`."
        )
    module_path = (root / arg).resolve()
    if not module_path.is_file():
        raise TaskSourceUnavailable(f"task_source module {arg} does not exist.")
    sys.path.insert(0, str(module_path.parent))
    try:
        module = __import__(module_path.stem)
    except Exception as exc:  # noqa: BLE001 - any import failure is the same finding
        raise TaskSourceUnavailable(f"could not import {arg}: {exc}") from exc
    finally:
        sys.path.pop(0)
    getter = getattr(module, "task_ids", None)
    if not callable(getter):
        raise TaskSourceUnavailable(
            f"{arg} does not expose `task_ids()`. The harness imports the plan module rather than "
            f"grepping it, so the module has to publish the list; add a `task_ids()` returning "
            f"every task id it knows about."
        )
    ids = {str(t) for t in getter()}
    if not ids:
        raise TaskSourceUnavailable(f"{arg}.task_ids() returned nothing.")
    return ids


def _lines(root: Path, arg: str, liveness: str, pattern: str) -> set[str]:
    if liveness == "open":
        raise TaskSourceUnavailable(
            "task_liveness = \"open\" is not available for a `lines:` source: a flat file records "
            "which ids are written down, not which are finished."
        )
    path = root / arg
    if not path.is_file():
        raise TaskSourceUnavailable(f"task_source file {arg} does not exist.")
    ids = set(re.findall(pattern, path.read_text()))
    if not ids:
        raise TaskSourceUnavailable(
            f"{arg} contains no id matching {pattern!r}. A pattern that matches nothing passes "
            f"every liveness check forever, which is the vacuous green this harness refuses."
        )
    return ids


def load(root: Path, harness: dict) -> set[str] | None:
    """Every task id the project's tracker knows about, or None when liveness is not checked.

    `None` is distinct from the empty set on purpose: empty means the tracker answered and had
    nothing, which is a finding; None means nobody asked, which the report has to disclose.
    """
    spec = str(harness.get("task_source", "none")).strip()
    liveness = str(harness.get("task_liveness", "exists")).strip()
    if liveness not in LIVENESS:
        raise TaskSourceUnavailable(
            f"task_liveness {liveness!r} is not one of {list(LIVENESS)}."
        )
    kind, _, arg = spec.partition(":")
    if kind == "none":
        return None
    if kind == "beads":
        return _beads(root, arg, liveness)
    if kind == "python":
        return _python(root, arg, liveness)
    if kind == "lines":
        return _lines(root, arg, liveness, str(harness.get("task_id", r"TASK-[0-9a-z.]+")))
    raise TaskSourceUnavailable(
        f"task_source {spec!r} is not one of beads:, python:, lines:, none."
    )


def unchecked_notice(harness: dict) -> str | None:
    """What the report must say when nothing verifies that owner tasks still exist."""
    if str(harness.get("task_source", "none")).strip().split(":", 1)[0] != "none":
        return None
    return ("owner-task liveness is NOT CHECKED (task_source = \"none\"). A pending or deferred "
            "rule can name a task that has been renamed, descoped or deleted, and nothing will "
            "say so — `state` is a mute button until a task source is configured.")
