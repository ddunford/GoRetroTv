#!/usr/bin/env bash
# The rule guard's own probe estate — the thing it shipped without.
#
# Every rule in this project has to prove it can fail before it is believed. The guard that protects
# those rules had no such proof, and it was wrong in three of the four ways that matter: it refused
# a plain edit and ALLOWED a `git mv` of a checker out of the governed set, a `git mv` of the CI
# workflow away, and the replacement of run.py with a symlink. `--name-only` prints only the new
# path of a rename, and a type change was not in the filter at all.
#
# Run it after touching rule-guard.sh. It works in a throwaway clone and never touches this tree.
set -uo pipefail
cd "$(git rev-parse --show-toplevel)"

CLONE=$(mktemp -d)
trap 'rm -rf "$CLONE"' EXIT
rsync -a --exclude node_modules --exclude .git --exclude .venv --exclude dist \
      --exclude __pycache__ ./ "$CLONE/" >/dev/null
git -C "$CLONE" init -q .
git -C "$CLONE" add -A
git -C "$CLONE" -c user.name=t -c user.email=t@t commit -q --no-verify -m base

fail=0
probe() {                       # probe <expected: refuse|allow> <label> <command...>
  local expected="$1" label="$2"; shift 2
  git -C "$CLONE" reset -q --hard HEAD
  ( cd "$CLONE" && "$@" >/dev/null 2>&1 )
  ( cd "$CLONE" && git add -A >/dev/null 2>&1 )
  local rc=0
  ( cd "$CLONE" && scripts/conformance/rule-guard.sh >/dev/null 2>&1 ) || rc=$?
  local got; [ "$rc" -ne 0 ] && got=refuse || got=allow
  if [ "$got" = "$expected" ]; then
    printf '  ok       %-52s (%s)\n' "$label" "$got"
  else
    printf '  BROKEN   %-52s expected %s, got %s\n' "$label" "$expected" "$got"
    fail=1
  fi
}

echo "rule-guard self-test"
probe refuse "a plain edit to a checker"            sh -c "echo '# probe' >> conformance/checkers/api-rules.py"
probe refuse "git mv a checker out of the set"      git mv conformance/checkers/api-rules.py api-rules-moved.py
probe refuse "git mv the CI workflow away"          git mv .github/workflows/ci.yml ci-old.yml
probe refuse "replace run.py with a symlink"        sh -c "rm conformance/run.py && ln -s /dev/null conformance/run.py"
probe refuse "delete the catalogue"                 rm plan/architecture-rules.md
probe refuse "edit ctl.sh, which carries the verb"  sh -c "echo '# probe' >> ctl.sh"
probe allow  "an ungoverned file (negative control)" sh -c "echo '// probe' >> frontend/src/App.tsx"

[ "$fail" -eq 0 ] && echo "rule-guard: every case behaves" || echo "rule-guard: SELF-TEST FAILED" >&2
exit "$fail"
