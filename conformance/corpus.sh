#!/usr/bin/env bash
# THE definition of the conformance corpus. Every rule that walks files walks this list — via
# `checkers/corpus.py`, which is the Python side of this file and is what the checkers must import.
#
# THAT IS AN INSTRUCTION, NOT A DESCRIPTION, and it is worth checking rather than assuming. In the
# repository this was written for, it was a claim for weeks and false the whole time: the checkers
# enumerated the filesystem with glob and rglob, so an untracked scratch file was inside every
# rule's reach and the failure argued against below was live rather than hypothetical.
#
# The corpus is TRACKED CONTENT ONLY — what the repository asserts, not what happens to be
# sitting in somebody's working directory. `git ls-files` is the definition, and the probe
# runner builds its worktree from the same tracked snapshot, so the two cannot disagree.
#
# WHY THIS IS WRITTEN DOWN RATHER THAN LEFT TO WHATEVER GLOB A CHECKER HAPPENS TO USE.
# A conformance run whose corpus is undefined is exactly the green check this harness exists
# to stop: it reports on whichever files the author's `find` happened to reach, and that set
# changes when somebody leaves a scratch file in the tree.
#
# The concrete case that settles it recurs in every repository. A gitignored local `.env`
# carries a line that some naming rule forbids — a ticket id, a placeholder, a scratch note.
# A corpus defined by globbing the working tree turns that rule red on a file that is
# deliberately not in version control, that differs on every machine, and that no rule can
# legitimately govern. The fix everyone reaches for is a standing exception, which is how an
# exception list quietly becomes the rule.
#
# Defining the corpus as tracked content excludes that file FOR A REASON rather than by name,
# and excludes vendored dependencies, build output and every future scratch file with it.
#
# ONE CONSEQUENCE WORTH KNOWING: a checker or a probe you have written but never `git add`ed is
# not in the corpus, so the probe worktree does not contain it. run-probes.sh says so in those
# words rather than letting the shell report a bare "No such file".
#
# Usage:  conformance/corpus.sh [pathspec...]      newline-separated
#         conformance/corpus.sh -z [pathspec...]   NUL-separated, for paths with spaces
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
if [ "${1:-}" = "-z" ]; then shift; exec git ls-files -z -- "$@"; fi
exec git ls-files -- "$@"
