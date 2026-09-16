#!/usr/bin/env bash
# Record authorisation for a change to the architecture rules.
#
# Wire this into the project's control script (e.g. `./ctl.sh authorise-rules "<why>"`).
# It stamps the digest of the STAGED governed content into the ledger, so the row
# describes exactly what was committed and nothing else.
#
# The digest comes from `rule-guard.sh --digest` rather than from a second copy of the
# governed set. Two lists that must agree are two lists that will not, and the one that
# drifts silently stops covering a path the other still guards.
#
# The reason is not optional and not a formality: the ledger is read by somebody trying
# to understand why an agreement moved, and a row reading "update rules" leaves them
# exactly where they started.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

reason="${1:-}"
[ -n "$reason" ] || { echo 'usage: authorise-rules "why this rule changed"' >&2; exit 2; }

GUARD='scripts/conformance/rule-guard.sh'
LEDGER='.conformance-authorised'
[ -x "$GUARD" ] || { echo "$GUARD is missing or not executable" >&2; exit 2; }

digest="$("$GUARD" --digest)"
[ -n "$digest" ] || { echo "the guard produced no digest" >&2; exit 2; }

[ -s "$LEDGER" ] || printf '%s\n' \
  "# Ledger of authorised changes to the architecture rules." \
  "# Each row is somebody signing for a change to an agreement, not to code." \
  "# Never edit by hand - the whole value is that a row means an act happened." "" > "$LEDGER"

printf 'digest: %s\n  when: %s\n   why: %s\n\n' \
  "$digest" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$reason" >> "$LEDGER"
git add "$LEDGER"
echo "authorised: $digest"
