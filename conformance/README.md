# Conformance — the architecture rules, and the machinery that keeps them honest

`plan/architecture-rules.md` is the catalogue: what each rule forbids, which recorded decision it comes from, and
why. **This directory is the part that runs.**

```
./ctl.sh conformance                         every rule, every probe, one verdict
./ctl.sh conformance --only ARCH-DEV-1       one rule — its checker AND every probe it declares
```

Everything here exists to make one sentence true: **a rule that cannot fail is worse than no rule.**
A green check reads as a guarantee, and repositories ship ones that are not — a healthcheck probing
an address the service never bound, a test option the running version silently ignores, a
web-server block that is dead code behind an internal redirect. All of those sit green for weeks.

The reasoning behind each mechanism is in the file that implements it. This document is the
protocol: what an author has to do, and what the harness will refuse.

---

## What runs, in what order

| | |
|---|---|
| `run.py` | the entry point. Runs the three stages below and prints a line for **every** rule id in the catalogue, enforced or not. |
| `check_registry.py` | registry integrity. Runs first, and its failure is fatal, because nothing after it means anything if the harness does not know which rules exist. |
| `<each rule's command>` | the checkers, against the working tree. |
| `task_source.py` | where "does the owner task still exist?" is answered. The one seam that knows how this project tracks work. |
| `checkers/finding.py` `.sh` | the vocabulary a checker reports in: one line per violation, naming the detector that produced it. |
| `run-probes.sh` | every probe every enforced rule declares, each in its own isolated worktree. Every violation must still be caught, independently. |

The report distinguishes **green** from **complete**. Un-enforced rules are printed next to the
task that owes them, and none of them is enforcing anything. Conflating "the run passed" with "the
architecture is held" is the failure this directory exists to prevent, so the summary line always
says both numbers. It carries the live counts rather than any number written into this file — a
count written down here is a count that is wrong a week later.

---

## The corpus — tracked content only

`corpus.sh` is the single definition, and it is `git ls-files`: **the corpus is what the repository
asserts, not what happens to be sitting in a working directory.** The probe runner builds its
worktree from the same tracked snapshot, so a rule and its probe can never disagree about what they
are looking at. A checker or probe you have written but never `git add`ed is not in the corpus, and
the probe worktree will not contain it.

---

## The registry

`rules.toml` maps every rule id to its classification, arming, state, checker and probes. TOML
rather than YAML because `tomllib` is in the Python standard library: the gate that reports whether
the architecture is intact should not have a dependency of its own to be missing.

`state` is the field the runner acts on, and it is deliberately not the same thing as the
catalogue's `arming`:

| state | meaning |
|---|---|
| `enforced` | a checker exists. `command`, `probes`, `halves` and `tooling` are mandatory; the checker and **every** probe are exercised on every run. |
| `pending` | the checker could be written today but has not been. `owner_task` names who will. |
| `deferred` | the checker cannot be written yet because its subject does not exist. `owner_task` names the task that creates the subject. |

**`state` cannot be used as a mute button.** `pending` and `deferred` both require an `owner_task`
the configured task source still knows about; delete or renumber it and the run goes red instead of
silently orphaning the rule. A `pending` row may not carry a `command` or `probes` — a checker the
harness does not run is exactly the decoration this catalogue is against.

---

## Probes — the protocol

Each enforced rule owns `probes/<RULE-ID>/`, and inside it **one directory per probe**, named in
the registry row. Each probe declares **what it expects to be caught by**:

```toml
probes = [
  { name = "ticket-id-in-constant", detectors = ["plan-metadata-in-code"] },
  { name = "ticket-id-in-path",     detectors = ["plan-metadata-in-path"] },
]
```

`run-probes.sh` does both directions, per probe:

1. build an isolated worktree from the corpus snapshot;
2. run **only that rule's checker** there, and require it to **pass** — a rule that rejects
   everything is as useless as one that rejects nothing;
3. run `apply.sh`, and require it to actually change something (a before/after delta taken *inside*
   the worktree, so a new-file violation counts and a dirty main tree cannot satisfy it);
4. run the same checker again and require it to **attribute** the violation to every detector this
   probe declares — exit status exactly 1, and a `<RULE-ID>: FAIL [<detector>] — …` line for each.

**"Caught" is a positive assertion.** A checker that fell over exits non-zero and names the rule in
its traceback; that is not a catch. Only an emitted, well-formed, attributed violation line is.
Exit codes are the second, independent reading and they are a closed vocabulary: `0` the rule
holds, `1` the rule is **violated** (the only status that means that), `2` the checker could not do
its job.

### Rules for writing one

- **One probe per detector.** In *series* — weaken either and the violation slips through — one
  violation honestly holds both. In *parallel* — either alone catches it — one probe is worth
  nothing for the second detector, so each gets its own violation, written so the siblings stay
  quiet.
- **A probe that ADDS a file must `git add` it**, or the corpus cannot see it. Stage, do not commit:
  step 3 reads `git status`, which a commit empties.
- **Trip one separable detector, and name what you trip.** Detectors that are inseparable by
  construction are named together; leaving one out because another happened to fire is the drift
  the field exists to stop.
- **Name the directory after the violation, not the rule.** The rule id is already the parent.
- **Commit the violation the way a developer would**, not the way that is easiest to detect —
  leaving the explanatory comment beside a control that no longer runs, for instance, because that
  asymmetry is what let the original incident survive review.

### `halves`

**Every enforced rule declares `halves` — one entry per part — and the field is required.** A half
says how it is held and **which detectors** hold it, and a half may only name detectors the probe
it claims is required to trip. A half can therefore never be written more ambitiously than the
violation behind it. `held_by = "negative control"` means the part *genuinely resists a probe*, not
that nobody wrote one; an in-checker grep a probe could plainly defeat is `pending` with an owner.

### Isolation is the security boundary

Probe scripts run arbitrary shell, by design. They are constrained by where they run: a throwaway
git worktree, loaded only from `probes/<rule-id>/<name>/` with both components validated against a
kebab grammar before being joined onto a path, and removed by `git worktree remove` rather than by
an `rm -rf` of a computed path.

---

## Adding a rule

1. Write its `###` section and summary-table row in the catalogue.
2. Add a row to `rules.toml`. `pending` with an `owner_task` is a legitimate first commit.
3. To enforce it: write the checker so every finding names its detector, write one probe per
   detector, list each in `probes`, declare `halves` and `tooling`, and flip `state` to `enforced`.

**Prefer tooling already in the stack**, and record which tool and why in the row's `tooling` field.
Reach for a new dependency only where nothing installed can express the rule.

**Before you call a rule armed, neutralise each detector in turn and confirm only its own probe
goes uncaught.** A probe caught with the detector disabled is not testing what it claims.

## The probe worktree is built from the INDEX, not your working tree

Neutralising a detector for the stop-gate means editing the checker — and an UNSTAGED edit does not
reach the probe worktree. The harness then runs the OLD checker, every probe is still caught, and
the stop-gate reads as passed for a detector you believed you had disabled. That is the stop-gate's
own failure mode: the step that exists to catch a rule which cannot fail, silently not running.

Stage the neutralisation before running the probes, and restore afterwards. If a stop-gate reports
every detector passing on the first attempt, suspect this before believing it.
