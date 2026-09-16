#!/usr/bin/env bash
# Refuse to commit a change to the architecture rules without recorded authorisation.
#
# Installed as part of the repository's pre-commit hook.
#
# WHAT THIS IS, AND IS NOT. This is a speed bump and an audit trail, not a boundary.
# Anyone who can edit a rule can also run the authorise command or edit the ledger,
# and `--no-verify` skips the hook entirely. It cannot stop a determined author, and
# a control that claims more than it does is exactly what this harness exists to
# distrust.
#
# What it DOES buy: a rule change can no longer happen quietly as a line inside an
# unrelated commit. It has to be a separate, deliberate, attributable act that
# leaves a dated row in the ledger. That converts "the rules drifted and nobody
# noticed" into "somebody signed for it".
#
# The real boundary is CODEOWNERS plus branch protection requiring a second person's
# review on these paths — an author cannot approve their own pull request, and that
# check lives on the forge rather than in a file the author can edit.
set -uo pipefail

# THE GOVERNED SET — edit this for the repository. It is every path where a change
# would alter what the rules MEAN or whether they RUN: the catalogue, the harness,
# the checkers, the configuration files the checkers read (linter configs, layering
# configs, arch-test suites), and the hook directory itself.
#
# The hook directory is in the set deliberately: disarming the guard is the obvious
# way to change a rule unobserved, so the guard guards itself.
PROTECTED=(
  'plan/architecture-rules.md'
  'conformance/'
  'scripts/conformance/'
  '.githooks/'
  # AND THE FILES THAT DECIDE WHETHER THE RULES RUN AT ALL — the control script carrying the
  # conformance verb, and the CI workflow carrying the job that invokes it. Either can disable
  # every rule in the catalogue without touching a rule. Governing the rules but not their
  # execution guards the lock and leaves the door off its hinges.
  'ctl.sh'
  '.github/workflows/ci.yml'
  'Dockerfile'
  '.dockerignore'
  'docker-compose.yml'
  # Add every config a rule is expressed in, e.g.:
  #   '.dependency-cruiser.cjs'
  #   'stylelint.config.mjs'
  #   'tests/Conformance/'
  #   '.importlinter'
)
LEDGER='.conformance-authorised'

is_protected() {
  for p in "${PROTECTED[@]}"; do
    case "$p" in
      */) [ "${1##"$p"}" != "$1" ] && return 0 ;;
      *)  [ "$1" = "$p" ] && return 0 ;;
    esac
  done
  return 1
}

# Digest the STAGED content, not the working tree — the check must describe what is
# actually being committed. A deleted file contributes its path with no blob, so
# removing a rule changes the digest exactly as editing one does.
# --no-renames AND T IN THE FILTER, both learned the hard way. `--name-only` prints only the NEW
# path of a rename, and rename detection is on by default — so `git mv conformance/checkers/x.py
# x.py` showed one addition, the OLD governed path never appeared, no deletion was recorded, and the
# guard allowed it. Moving the CI workflow away, and replacing the runner with a symlink (status T,
# absent from the filter), were allowed the same way. Plain edits were refused correctly, which is
# why it never showed. The one act this guard exists to make deliberate could be performed by moving
# a file. `rule-guard-selftest.sh` holds those cases; run it after touching this script.
staged_digest() {
  git diff --cached --name-only --no-renames --diff-filter=ACMRDT -z \
    | tr '\0' '\n' | grep -v '^$' | sort | while read -r f; do
        is_protected "$f" || continue
        if git cat-file -e ":$f" 2>/dev/null; then
          printf '%s %s\n' "$f" "$(git show ":$f" | sha256sum | cut -d' ' -f1)"
        else
          printf '%s DELETED\n' "$f"
        fi
      done | sha256sum | cut -d' ' -f1
}

# `--digest` prints the staged digest and exits. The authorise command uses it rather
# than recomputing the digest from its own copy of the governed set: two lists that must
# agree are two lists that will not, and the one that drifts silently stops covering a
# path the other still guards.
if [ "${1:-}" = "--digest" ]; then
  staged_digest
  exit 0
fi

changed=$(git diff --cached --name-only --no-renames --diff-filter=ACMRDT -z 2>/dev/null | tr '\0' '\n' | while read -r f; do
  [ -n "$f" ] && is_protected "$f" && echo "$f"
done)
[ -z "$changed" ] && exit 0

now=$(staged_digest)
recorded=$(grep -E '^digest:' "$LEDGER" 2>/dev/null | tail -1 | awk '{print $2}')

if [ "$now" = "$recorded" ]; then
  exit 0
fi

cat >&2 <<MSG

BLOCKED: this commit changes the architecture rules, which need recorded authorisation.

Changed under the governed set:
$(echo "$changed" | sed 's/^/    /')

    recorded: ${recorded:-<none — ledger missing or unstamped>}
    staged:   $now

The rules are the record of decisions this project agreed not to drift from, so a
change to them is a change to an agreement, not a refactor. Two ways forward:

  Authorised   run the repository's authorise command with a reason, then re-commit.
               It writes a dated row to $LEDGER naming the reason, and that row is
               the audit trail. The reason is the artefact — "update rules" teaches
               a later reader nothing, and the ledger is read by people trying to
               understand why an agreement moved.

  Not yours    git restore --staged <path>  and raise it with the owner first.

This is a speed bump, not a boundary: it can be bypassed, and it is meant to make a
rule change deliberate and attributable rather than impossible. The boundary is the
CODEOWNERS review on these paths.

MSG
exit 1
