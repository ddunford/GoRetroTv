#!/usr/bin/env bash
# Prove every enforced rule still fails on its own deliberate violation.
#
# A rule that cannot fail is worse than no rule, because a green check reads as a guarantee.
# Repositories ship those routinely — a container healthcheck resolving to an address the service
# never bound, a browser-automation option the running version silently ignores, a web-server
# block that is dead code behind an internal redirect — so no rule here is trusted on the strength
# of having been written.
#
# A RULE DECLARES ONE PROBE PER DETECTOR IT WANTS HELD, and every one of them must be caught on
# its own. That is the whole reason the registry's field is a list. Where a rule's detectors sit
# in SERIES — weaken either and the violation slips through — one violation honestly holds both,
# and one probe is the right number. Where they sit in PARALLEL — either alone catches it — a
# single probe is worth nothing for the second detector: delete that detector and the probe is
# still caught by the first, so the rot is invisible. Each probe therefore gets its OWN worktree
# and its own verdict, and a rule with three probes of which two are caught FAILS, naming the one
# that was not.
#
# Per probe, both directions:
#
#   1. build an isolated worktree from the corpus snapshot;
#   2. run ONLY that rule's checker there and require it to PASS. A rule that rejects
#      everything is as useless as one that rejects nothing, and without this step a checker
#      hardcoded to `exit 1` would look like a perfectly caught probe;
#   3. apply the probe's deliberate violation;
#   4. run the same checker again and require it to attribute the violation to the DETECTOR the
#      probe declares it expects to be caught by;
#   5. destroy the worktree.
#
# WHAT "CAUGHT" MEANS, AND WHY IT IS A POSITIVE ASSERTION. Step 4 used to read two facts: the
# checker exited non-zero, and its output mentioned the rule id. A checker that FELL OVER satisfies
# both. That is not hypothetical — while a set of schema rules was being armed, a container lookup
# could not resolve inside a probe worktree, and one rule's three probes reported 3 of 3 CAUGHT with
# none of them having exercised the rule at all: their clean baselines passed from a cache that
# never made the lookup, and only the violated runs recomputed and died. Step 2 cannot catch that.
# It catches a checker broken for EVERY tree; this one was broken only for the VIOLATED tree, which
# is exactly what an expensive checker with a content-keyed cache produces.
#
# So a probe is caught only when the checker EMITS the line that says so:
#
#     <RULE-ID>: FAIL [<detector>] — <what broke>
#
# and the exit status is exactly 1, the only code that means "the rule is violated". Anything else
# — a traceback, a `HARNESS:` line, a silent non-zero, a signal — attributes the failure to no
# detector and is a PROBE FAILURE. Grepping for a `HARNESS:` marker instead would have made the
# guarantee depend on every checker author remembering to print that word forever: forget it and
# the run goes green, which is the same class of thing as a rule that cannot fail. Inverted, the
# default case is safe, and an author who forgets the marker finds out because every probe their
# rule declares turns red. conformance/checkers/finding.py is the vocabulary.
#
# AND IT MUST BE THE RIGHT DETECTOR. Each probe's registry row names the detector (or detectors)
# it exists to hold, and every one of them must fire. Without that, a probe caught by a DIFFERENT
# detector of the same rule counts as caught — so a probe can silently stop testing what it was
# written for, and the only thing between the catalogue and that is an author replicating each
# worktree by hand and reading the output. conformance/README.md's "trip one separable detector"
# is now a check rather than a request.
#
# THE WORKTREE IS THE CORPUS. It is built from the tracked snapshot — `git stash create` when
# the tree has uncommitted changes to tracked files, HEAD when it is clean — which is the same
# definition conformance/corpus.sh gives. So probes see exactly what the rules see: uncommitted
# work in progress is included, untracked scratch files and gitignored config are not.
#
# ISOLATION IS THE SECURITY BOUNDARY. Probe scripts run arbitrary shell, by design: a probe has
# to be able to commit any violation a developer could. They run with the throwaway worktree as
# their working directory, they are only ever loaded from conformance/probes/<rule-id>/<name>/
# where BOTH components come from the registry and the name is checked against a lowercase-kebab
# grammar before it is joined on, and the worktree is removed by git rather than by an rm of a
# computed path.
#
# WHY THIS RUNS PROBES IN PARALLEL, AND WHAT PARALLELISM IS NOT ALLOWED TO CHANGE.
#
# Dozens of probes, each in its own worktree, and any probe that touches a database or a built
# image spends most of its life WAITING — on an image build, on a container coming up, on one
# command inside another container. Serialised, that is minutes, and it gets worse with every rule
# armed rather than better. A gate people stop running is a gate that is not running, so on a
# harness like this the wall clock is a property of the guarantee and not a convenience.
#
# What the fan-out does NOT get to touch:
#
#   the isolation   every probe still gets its OWN worktree, and every schema probe still stands
#                   up its OWN database and its own network, named per worker and removed by name.
#                   Nothing warm is shared between probes and nothing ever will be: a probe
#                   worktree's migrations are a schema nobody agreed to, and they must never meet
#                   another probe's.
#   the reading     each probe's whole report is buffered and replayed in REGISTRY order, so the
#                   output is what a serial run would have printed and a rule's probes stay
#                   together. Verdicts interleaved by whichever worker finished first are a gate
#                   nobody can read, which fails for the same reason as a gate nobody runs.
#   the verdict     a probe whose worker leaves no status behind is a FAILURE, not a skip. Silence
#                   is the one thing this harness may never read as a pass.
#
# Width is CONFORMANCE_JOBS, defaulting to half the host's cores — see JOBS below for why
# half rather than all.
#
# Usage: conformance/run-probes.sh [RULE-ID...]     (default: every enforced rule)
set -uo pipefail

cd "$(git rev-parse --show-toplevel)"
ROOT="$(pwd -P)"
REGISTRY="conformance/rules.toml"

c_red(){ printf '\033[31m%s\033[0m\n' "$1"; }
c_grn(){ printf '\033[32m%s\033[0m\n' "$1"; }

# One row per PROBE, not per rule, as: id<TAB>probe-name<TAB>detector,detector<TAB>command...
mapfile -t ROWS < <(python3 - "$REGISTRY" "$@" <<'PY'
import re, sys, tomllib
# The same grammar check_registry.py applies. Repeated here rather than imported because this
# script is runnable on its own, and the name is about to be joined onto a path that gets
# EXECUTED: `..`, a slash or an absolute path would escape conformance/probes/<rule-id>/.
KEBAB = re.compile(r"[a-z0-9]+(?:-[a-z0-9]+)*\Z")
registry, wanted = sys.argv[1], set(sys.argv[2:])
with open(registry, "rb") as fh:
    rules = tomllib.load(fh).get("rule", [])
enforced = [r for r in rules if r.get("state") == "enforced"]
if wanted:
    known = {r["id"] for r in rules}
    for w in sorted(wanted - known):
        print(f"!unknown\t{w}")
    for w in sorted(wanted & known - {r["id"] for r in enforced}):
        print(f"!not-enforced\t{w}")
    enforced = [r for r in enforced if r["id"] in wanted]
for r in sorted(enforced, key=lambda r: r["id"]):
    probes = r.get("probes") or []
    if not probes:
        print(f"!no-probes\t{r['id']}")
        continue
    for probe in probes:
        # A bare string is the pre-detector shape. Refused rather than defaulted: a probe whose
        # expected detector defaults to "whatever fires" is the hole this field exists to close.
        if not isinstance(probe, dict):
            print(f"!bad-probe-row\t{r['id']}: {probe!r}")
            continue
        name = probe.get("name")
        if not isinstance(name, str) or not KEBAB.fullmatch(name):
            print(f"!bad-probe-name\t{r['id']}: {name!r}")
            continue
        detectors = probe.get("detectors")
        if not isinstance(detectors, list) or not detectors or not all(
                isinstance(d, str) and KEBAB.fullmatch(d) for d in detectors):
            print(f"!bad-detectors\t{r['id']}/{name}: {detectors!r}")
            continue
        print("\t".join([r["id"], name, ",".join(detectors), *r["command"]]))
PY
)

FAILED=0
RAN=0
RULES_SEEN=""

# HOW WIDE THE FAN-OUT GOES, AND WHY IT IS NOT `nproc`.
#
# A probe of any rule that needs a running system is not a CPU workload. It builds an image, stands
# up a database container and waits for it, then runs one command inside another container — so
# what it mostly does is WAIT on the container daemon. That is why the fan-out pays at all. It is
# also why the width is half the core count rather than all of it: each such probe holds a database
# and an application container open at its peak, and a width that outruns the host's memory turns a
# slow gate into an OOM-killed one, which is a gate people stop running for a worse reason.
#
# Half the cores, floored at 2 and capped at 8.
JOBS="${CONFORMANCE_JOBS:-}"
if [ -z "$JOBS" ]; then
  CPUS="$(nproc 2>/dev/null || echo 4)"
  JOBS=$(( CPUS / 2 ))
  [ "$JOBS" -lt 2 ] && JOBS=2
  [ "$JOBS" -gt 8 ] && JOBS=8
fi
case "$JOBS" in ''|*[!0-9]*|0) JOBS=1 ;; esac

# Every worker's scratch: one `<index>.out` holding that probe's entire report, one `<index>.rc`
# written LAST and by rename, so its existence means the report beside it is complete, and one
# `<index>.wt` naming the worktree so the EXIT trap can remove a worktree its own shell never saw.
RUNDIR="$(mktemp -d "${TMPDIR:-/tmp}/conformance-probes-XXXXXXXX")"
# `git worktree add` and `remove` mutate .git/worktrees for the whole repository, so they are the
# one part of a probe that is not private to it. Serialised through a lock file rather than left
# to chance: the operation is under a second, and a lost race here fails a probe for a reason that
# has nothing to do with the rule it holds.
GIT_LOCK="$RUNDIR/git.lock"
: >"$GIT_LOCK"

cleanup() {
  for marker in "$RUNDIR"/*.wt; do
    [ -f "$marker" ] || continue
    wt="$(cat "$marker" 2>/dev/null || true)"
    [ -n "$wt" ] || continue
    flock "$GIT_LOCK" git worktree remove --force "$wt" >/dev/null 2>&1 || true
  done
  git worktree prune >/dev/null 2>&1 || true
  rm -rf "$RUNDIR"
}
trap cleanup EXIT

# The tracked snapshot: every committed file plus every uncommitted change to a tracked file.
snapshot_ref() {
  local ref
  ref="$(git stash create 2>/dev/null || true)"
  [ -n "$ref" ] || ref="$(git rev-parse HEAD)"
  printf '%s' "$ref"
}
SNAPSHOT="$(snapshot_ref)"

# ONE PROBE, START TO FINISH. Everything it prints goes to its own buffer and is replayed in
# registry order by the collector below, so a run that is executed out of order is still READ in
# order and grouped by rule. Interleaved verdicts would be a gate nobody can read, which is the
# same failure as a gate nobody runs.
#
# Returns 0 if the probe was caught by the detectors it declares, 1 otherwise. It never sets a
# variable the caller reads: it is a subshell, and the collector reads the exit status from disk.
probe_one() {
  local row="$1" slot="$2"
  local id name dets cmd0 rest
  IFS=$'\t' read -r id name dets cmd0 rest <<<"$row"

  if [ "$id" = "!unknown" ]; then
    c_red "$name: no such rule in $REGISTRY"; return 1
  fi
  if [ "$id" = "!not-enforced" ]; then
    c_red "$name: not enforced, so it has no probes to run. Enforce it first."; return 1
  fi
  if [ "$id" = "!no-probes" ]; then
    c_red "$name: enforced but declares no \`probes\`. A rule with no probe has never been shown"
    c_red "    to fail, and a rule that cannot fail is worse than no rule."
    return 1
  fi
  if [ "$id" = "!bad-probe-name" ]; then
    c_red "$name: probe name is not a lowercase-kebab path component. The name is joined onto"
    c_red "    conformance/probes/<rule-id>/ and the result is EXECUTED."
    return 1
  fi
  if [ "$id" = "!bad-probe-row" ]; then
    c_red "$name: a \`probes\` entry is not a table. A probe is declared as"
    c_red "    { name = \"<dir>\", detectors = [\"<detector>\", ...] } — the detectors being what"
    c_red "    the probe exists to be caught BY. A bare name would let any detector of the rule"
    c_red "    count as a catch, so the probe could silently stop testing what it was written for."
    return 1
  fi
  if [ "$id" = "!bad-detectors" ]; then
    c_red "$name: \`detectors\` is missing, empty, or not a list of lowercase-kebab ids. It is"
    c_red "    required and it is not defaulted: a probe whose expected detector is \"whatever"
    c_red "    fires\" is the hole this field exists to close."
    return 1
  fi

  local COMMAND
  IFS=$'\t' read -r -a COMMAND <<<"$(printf '%s\t%s' "$cmd0" "${rest:-}")"
  # A trailing empty field appears when the command is a single element.
  [ -n "${COMMAND[-1]}" ] || unset 'COMMAND[-1]'

  # Both components come from the registry and the name has already been matched against the
  # kebab grammar, so this cannot reach outside the probe root.
  local PROBE="conformance/probes/$id/$name"

  echo "── $id ─ probe: $name ─ expects: ${dets//,/, }"

  local WT
  WT="$(mktemp -d "${TMPDIR:-/tmp}/conformance-probe-XXXXXXXX")"
  rmdir "$WT"
  # Recorded BEFORE the checkout, so a worker killed mid-`worktree add` still leaves the EXIT
  # trap something to remove.
  printf '%s' "$WT" >"$slot.wt"
  # core.hooksPath is pointed at nothing for the checkout: the post-checkout hooks belong to the
  # developer's repository (tracker export, dependency notices) and have no business firing for a
  # throwaway tree. The credential guard is a pre-commit hook and nothing here ever commits.
  if ! flock "$GIT_LOCK" git -c core.hooksPath=/dev/null worktree add --detach --quiet "$WT" "$SNAPSHOT"; then
    c_red "$id: could not create the probe worktree"; return 1
  fi

  # The corpus is tracked content, so a checker that has never been `git add`ed is not in the
  # worktree at all. Say that, rather than letting the shell report a bare "No such file".
  if [ ! -e "$WT/${COMMAND[0]}" ] && [ ! -e "${COMMAND[0]}" ]; then
    c_red "$id: ${COMMAND[0]} is not in the corpus snapshot, so the probe worktree has no checker"
    c_red "    to run. The corpus is TRACKED content only — \`git add\` the checker and re-run."
    flock "$GIT_LOCK" git worktree remove --force "$WT" >/dev/null 2>&1 || true
    return 1
  fi

  # (2) the clean baseline must pass, in the worktree, with the same checker.
  #
  # This catches a checker broken for EVERY tree. It cannot catch one broken only for the VIOLATED
  # tree — an expensive checker with a content-keyed cache hits the cache here and recomputes
  # there, so the two runs execute different code. That case is step 4's, and it is the whole
  # reason step 4 requires a positive attribution rather than merely a non-zero exit.
  (cd "$WT" && "${COMMAND[@]}") >"$WT.clean.log" 2>&1
  local CLEAN_RC=$?
  if [ "$CLEAN_RC" -ne 0 ]; then
    if [ "$CLEAN_RC" -eq 1 ]; then
      c_red "$id: the checker reports the CLEAN corpus as violating the rule, so a caught probe"
      c_red "    would prove nothing — a rule that rejects everything is as useless as one that"
      c_red "    rejects nothing."
    else
      c_red "$id: the checker COULD NOT RUN against the clean corpus (exit $CLEAN_RC). Nothing"
      c_red "    below would be a statement about the architecture."
    fi
    sed 's/^/    /' "$WT.clean.log" | tail -20
    rm -f "$WT.clean.log"
    flock "$GIT_LOCK" git worktree remove --force "$WT" >/dev/null 2>&1 || true
    return 1
  fi
  rm -f "$WT.clean.log"

  # The worktree's state AFTER the clean run and BEFORE the violation. The guard below compares
  # against this rather than against "empty", so a checker that leaves an artefact behind cannot
  # satisfy it on the probe's behalf.
  local BEFORE
  BEFORE="$(git -C "$WT" status --porcelain)"

  # (3) apply the deliberate violation.
  if ! (cd "$WT" && "$ROOT/$PROBE/apply.sh") >"$WT.apply.log" 2>&1; then
    c_red "$id: $PROBE/apply.sh failed, so the violation was never committed."
    sed 's/^/    /' "$WT.apply.log" | tail -20
    rm -f "$WT.apply.log"
    flock "$GIT_LOCK" git worktree remove --force "$WT" >/dev/null 2>&1 || true
    return 1
  fi
  rm -f "$WT.apply.log"

  # DID THE PROBE ACTUALLY CHANGE THE WORKTREE.
  #
  # This was `git diff --quiet`, which compares the worktree to the INDEX and is therefore blind to
  # a violation that is a NEW FILE -- and many of them are: a queued job class, a migration, a spec
  # file whose NAME is the violation. Every add-only probe was refused as empty, the exact opposite
  # of what this guard is for.
  #
  # Two things make the replacement sound, and both are load-bearing:
  #
  #   -C "$WT"          the reading is taken INSIDE the probe worktree. Run in the main tree it
  #                     would be non-empty whenever anybody has uncommitted work, which turns a
  #                     guard that was wrongly failing into one that can never fail -- the same
  #                     defect pointed the other way. Proven with a no-op apply.sh against a dirty
  #                     main tree: still refused.
  #   BEFORE vs AFTER   a DELTA, not "non-empty". A checker that writes an artefact into the
  #                     worktree during the clean baseline run would otherwise satisfy this guard
  #                     on the probe's behalf.
  #
  # `status --porcelain` reports added, modified and deleted alike, and still ignores gitignored
  # paths -- correctly, since a file the corpus excludes is not a violation any rule can see.
  if [ "$BEFORE" = "$(git -C "$WT" status --porcelain)" ]; then
    c_red "$id: $PROBE/apply.sh changed nothing. An empty probe is caught by nothing."
    flock "$GIT_LOCK" git worktree remove --force "$WT" >/dev/null 2>&1 || true
    return 1
  fi

  # (4) and now the checker must ATTRIBUTE the violation to the detector this probe holds.
  #
  # Three readings, and each rules out a different way of being wrong. The exit status must be
  # exactly 1 — the only code that means "the rule is violated"; 0 is uncaught, and 2, 127 or 139
  # are a checker that could not run, was not found, or died. The output must carry at least one
  # well-formed violation line, which is what a traceback, a HARNESS: line or a silent non-zero
  # does NOT have. And the detectors the registry says this probe expects must be among the ones
  # that fired, because a probe caught by a sibling detector has stopped testing what it holds.
  local OUT RC FIRED MISSING want WANT
  OUT="$( (cd "$WT" && "${COMMAND[@]}") 2>&1 )"
  RC=$?
  # Every detector this run attributed a finding to, deduplicated. `sed` rather than `grep -o` so
  # the rule id anchors the line: a detector id quoted inside somebody's prose is not a finding.
  FIRED="$(printf '%s\n' "$OUT" \
             | sed -n "s/^$id: FAIL \[\([a-z0-9-]\{1,\}\)\].*/\1/p" | sort -u | tr '\n' ' ')"
  MISSING=""
  IFS=',' read -r -a WANT <<<"$dets"
  for want in "${WANT[@]}"; do
    # An empty element would match anything inside " $FIRED " and quietly satisfy the requirement.
    # The registry reader already refuses an empty `detectors`, so this can only be reached by a
    # future change to that reader — which is exactly when a silent pass would be worst.
    [ -n "$want" ] || { MISSING="$MISSING <empty>"; continue; }
    case " $FIRED " in *" $want "*) ;; *) MISSING="$MISSING $want" ;; esac
  done

  local VERDICT=0
  if [ "$RC" -eq 0 ]; then
    c_red "$id: PROBE $name IS NO LONGER CAUGHT. The violation in $PROBE was applied and the"
    c_red "    checker still passed. Every probe a rule declares must be caught on its OWN — the"
    c_red "    others passing says nothing about the detector this one exists to hold."
    printf '%s\n' "$OUT" | sed 's/^/    /' | tail -20
    VERDICT=1
  elif [ "$RC" -ne 1 ]; then
    c_red "$id: THE CHECKER COULD NOT RUN. It exited $RC on the violated tree, and 1 is the only"
    c_red "    status that means the rule was violated. This is NOT a caught probe: the clean"
    c_red "    baseline at step 2 can pass from a cache while the violated run recomputes and"
    c_red "    dies, which is how three probes once reported CAUGHT having exercised nothing."
    printf '%s\n' "$OUT" | sed 's/^/    /' | tail -20
    VERDICT=1
  elif [ -z "${FIRED// /}" ]; then
    c_red "$id: THE CHECKER ATTRIBUTED NOTHING. It exited 1 but printed no"
    c_red "    \`$id: FAIL [<detector>] — …\` line, so nothing says which detector found the"
    c_red "    violation — or whether one did at all. A checker that fell over exits non-zero and"
    c_red "    names the rule in its traceback, which is exactly what this refuses to read as a"
    c_red "    catch. See conformance/checkers/finding.py."
    printf '%s\n' "$OUT" | sed 's/^/    /' | tail -20
    VERDICT=1
  elif [ -n "$MISSING" ]; then
    c_red "$id: PROBE $name WAS CAUGHT BY THE WRONG DETECTOR. It declares that it exists to be"
    c_red "    caught by: ${dets//,/, } — and${MISSING} never fired."
    c_red "    What fired instead: ${FIRED% }."
    c_red "    A probe caught by a sibling detector has stopped testing what it was written for:"
    c_red "    delete the detector it holds and the run stays green. Either the probe's violation"
    c_red "    has drifted, or the registry row names the wrong detector — and which of those it"
    c_red "    is has to be decided by a person, not defaulted to 'close enough'."
    printf '%s\n' "$OUT" | sed 's/^/    /' | tail -20
    VERDICT=1
  else
    c_grn "$id/$name: clean corpus passes; caught by ${FIRED% }."
  fi

  flock "$GIT_LOCK" git worktree remove --force "$WT" >/dev/null 2>&1 || true
  return "$VERDICT"
}

# THE ORDER PROBES ARE STARTED IN IS NOT THE ORDER THEY ARE READ IN, AND THAT IS THE POINT.
#
# The registry lists a rule's probes together, so starting in registry order runs all of one rule's
# probes at once. That is exactly wrong for the expensive ones. A rule whose checker runs a
# deliberate CONCURRENCY race — several barriered processes whose overlap assertion is the point —
# would have every one of its probes running that race simultaneously, which is a way to make a
# deliberately load-insensitive test load-sensitive again. A flaky probe is a worse outcome than a
# slow one.
#
# So the launch order STRIPES each rule's probes evenly across the whole run: a rule's nth probe
# of m is placed at (n - 0.5)/m of the way through, so six probes of one rule sit at roughly a
# sixth, a half, five sixths and so on rather than consecutively. Across a few dozen probes they
# then land many slots apart, which at any width this runs at means one race at a time.
# Interleaving by position rather than by round matters: a plain round-robin puts every rule's
# LAST probe at the end together, which is where the rule with the most probes bunches again.
#
# The COLLECTOR below still reads in registry order, so the report is unchanged. Decoupling the
# order probes RUN in from the order they are READ in is what buys this for nothing.
mapfile -t LAUNCH < <(
  slot=0
  for row in "${ROWS[@]:-}"; do
    printf '%s\t%s\n' "${row%%$'\t'*}" "$slot"
    slot=$((slot + 1))
  done | awk -F'\t' '
      { rule[NR] = $1; slot[NR] = $2; total[$1]++ }
      END { for (i = 1; i <= NR; i++) {
              nth[rule[i]]++
              printf "%.6f\t%s\n", (nth[rule[i]] - 0.5) / total[rule[i]], slot[i]
            } }' | sort -k1,1n -k2,2n | cut -f2
)

# THE DISPATCHER. Launches at most $JOBS probes at once and never blocks on a particular one —
# the slot frees on whichever finishes first.
(
  for slot in "${LAUNCH[@]:-}"; do
    while [ "$(jobs -rp | wc -l)" -ge "$JOBS" ]; do wait -n 2>/dev/null || break; done
    (
      probe_one "${ROWS[$slot]}" "$RUNDIR/$slot" >"$RUNDIR/$slot.out" 2>&1
      # `.rc` is written by RENAME so it appears whole. The collector treats its existence as
      # "the report beside it is finished", and a half-written status would be read as a pass.
      printf '%s' "$?" >"$RUNDIR/$slot.rc.part" && mv "$RUNDIR/$slot.rc.part" "$RUNDIR/$slot.rc"
    ) &
  done
  wait
) &
DISPATCHER=$!

# THE COLLECTOR. Replays each probe's report in registry order as it becomes available, so the
# output is identical to a serial run's and a rule's probes stay together. A probe whose status
# never lands — a worker killed, the dispatcher gone — is a FAILURE, not a skip: silence is the
# one thing a conformance harness must never read as a pass.
slot=0
for row in "${ROWS[@]:-}"; do
  IFS=$'\t' read -r id _ _ _ _ <<<"$row"
  case "$id" in
    !*) ;;
    *)
      RAN=$((RAN + 1))
      case " $RULES_SEEN " in *" $id "*) ;; *) RULES_SEEN="$RULES_SEEN $id" ;; esac
      ;;
  esac

  while [ ! -f "$RUNDIR/$slot.rc" ]; do
    if ! kill -0 "$DISPATCHER" 2>/dev/null; then
      # The dispatcher has exited; anything still missing is never arriving. One more look, to
      # close the race between the last worker's rename and the dispatcher's own exit.
      sleep 0.5
      break
    fi
    sleep 0.2
  done

  echo
  if [ -f "$RUNDIR/$slot.rc" ]; then
    [ -f "$RUNDIR/$slot.out" ] && cat "$RUNDIR/$slot.out"
    [ "$(cat "$RUNDIR/$slot.rc")" = "0" ] || FAILED=1
  else
    c_red "$id: the probe worker produced no verdict at all. It was killed, or the harness that"
    c_red "    runs it died. Nothing was proven about this probe, which is not the same thing as"
    c_red "    the probe passing."
    FAILED=1
  fi
  slot=$((slot + 1))
done
wait "$DISPATCHER" 2>/dev/null || true

echo
if [ "$RAN" -eq 0 ] && [ "$FAILED" -eq 0 ]; then
  echo "no enforced rules to probe."
  exit 0
fi
RULES=$(printf '%s' "$RULES_SEEN" | wc -w | tr -d ' ')
if [ "$FAILED" -ne 0 ]; then
  c_red "probes: FAIL"
  exit 1
fi
c_grn "probes: $RAN/$RAN caught across $RULES rule(s) — $JOBS at a time"
exit 0
