"""How a checker says WHAT it found, in a form the harness can act on.

THE PROBLEM THIS SOLVES. `run-probes.sh` used to decide a probe was caught from two facts: the
checker exited non-zero, and its output mentioned the rule id. A checker that FELL OVER satisfies
both, so a harness failure inside a probe worktree was reported as a caught probe -- a green line
meaning the exact opposite of what happened.

That is not hypothetical. It is what a content-keyed cache does to an expensive checker: while a
set of schema rules was being armed, a container lookup could not resolve inside the probe
worktree, and one rule's three probes reported 3 of 3 CAUGHT with none of them having exercised
the rule at all. Their clean baselines passed from a cache that never made the lookup; only the
violated runs recomputed and died. The baseline and the violated run were executing different
code, which is precisely the case a clean-baseline check cannot see.

THE FIX IS A POSITIVE ASSERTION, NOT THE ABSENCE OF A NEGATIVE. The obvious repair -- grep the
output for a `HARNESS:` marker and treat it as a failure -- makes the guarantee depend on every
checker author remembering to print that word, forever. Forget it and the run goes GREEN, which is
the same class of thing as a rule that cannot fail. So the polarity is inverted: a probe is caught
only when the checker EMITS a well-formed violation line naming the detector that fired.

    ARCH-DATA-2: FAIL [undeclared-table] — the migrated database contains `sessions`, which ...

Anything else -- a traceback, a `HARNESS:` line, a silent non-zero exit, a checker killed by a
signal -- attributes no violation to any detector and is therefore a PROBE FAILURE. The default
case is the safe one, and a checker author who forgets the marker discovers it because every probe
their rule declares turns red, not because somebody notices a green line was lying.

EXIT CODES ARE THE SECOND, INDEPENDENT READING.

    0   the rule holds
    1   the rule is VIOLATED -- and this is the only code that means that
    2   the checker could not do its job: no container runtime, no database, an unparseable manifest

A Python traceback exits 1, which is why the exit code alone would not be enough and why the
marker line carries the weight. But an environment failure that has been recognised as one should
say so structurally as well as in prose, and `EXIT_HARNESS` is how.

WHY THE DETECTOR ID AND NOT JUST "SOMETHING FAILED". A rule holds several detectors, and a probe
exists to hold exactly one of them (or, where a violation honestly trips several, exactly those).
Without the id, a probe caught by a DIFFERENT detector of the same rule counts as caught -- so a
probe can silently stop testing what it was written for, and the only thing standing between the
catalogue and that is an author replicating each worktree by hand and reading the output. The
registry declares what each probe expects to be caught by; this module is how a checker answers.
"""

from __future__ import annotations

import re

EXIT_OK = 0
EXIT_VIOLATION = 1
EXIT_HARNESS = 2

# The same grammar the registry and run-probes.sh apply to a detector id. Kebab-case so the run's
# output greps against the registry rows, and so a detector reads as a name rather than a sentence.
DETECTOR = re.compile(r"[a-z0-9]+(?:-[a-z0-9]+)*\Z")


class UndeclarableDetector(Exception):
    """A checker tried to attribute a finding to something that is not a detector id.

    Raised rather than printed. A malformed id would produce a line the harness cannot read, which
    is indistinguishable from the checker having said nothing -- so it fails loudly here, at the
    call site, instead of quietly turning every probe for the rule red for a reason nobody can see.
    """


def violation(rule: str, detector: str, message: str) -> str:
    """One rule violation, attributed to the detector that found it."""
    if not DETECTOR.fullmatch(detector):
        raise UndeclarableDetector(
            f"{detector!r} is not a lowercase-kebab detector id, so {rule} would emit a finding "
            f"the harness cannot attribute."
        )
    return f"{rule}: FAIL [{detector}] — {message}"


def coverage(rule: str, detector: str, subjects: dict[str, int]) -> str | None:
    """A finding if ANY subject of the rule inspected nothing, naming which ones.

    THE WHOLE POINT IS THE WORD *ANY*. A rule that reads several places had been counting them
    together — `len(models) + len(migrations)` — so removing one left that half inspecting nothing
    while the other kept the total above zero and the guard silent. Eight subjects across three
    rules could vanish that way, and every probe missed it because each "the subject vanishes" probe
    removed ALL of them at once.

    The count is per subject, the finding names the empty one, and `conformance/ablate.py` removes
    each declared subject in turn to prove this fires for every one of them. A coverage guard is the
    only thing standing between a self-arming rule and a green tick over nothing, so it is the last
    place to accept an aggregate.
    """
    empty = sorted(name for name, count in subjects.items() if not count)
    if not empty:
        return None
    seen = ", ".join(f"{name}={count}" for name, count in sorted(subjects.items()))
    return violation(
        rule, detector,
        f"inspected NOTHING for: {', '.join(empty)} (counts: {seen}). A rule that compares an "
        f"empty set reports success having examined nothing — and it is the subject moving in a "
        f"refactor, not a decision, that gets it there."
    )


def harness(rule: str, message: str) -> str:
    """The checker could not do its job. Deliberately carries NO detector.

    The word HARNESS is here for the human reading a terminal. The machine-readable half is the
    absence of a detector attribution and the `EXIT_HARNESS` status -- neither of which depends on
    anybody remembering to write this word.
    """
    return f"{rule}: FAIL — HARNESS: {message}"
