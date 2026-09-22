#!/usr/bin/env bash
# Compare a real firmware run with one restored ten million instructions earlier.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
if ! BIN_DIR="$work/bin" ./ctl.sh build >"$work/build.log" 2>&1; then
    cat "$work/build.log" >&2
    exit 1
fi

start=1000000
end=11000000
"$work/bin/firmwaretrace" -steps "$start" -interval "$end" \
    -snapshot-out "$work/paused.snapshot" >"$work/save.stream" 2>"$work/save.log"
"$work/bin/firmwaretrace" -steps "$end" -interval "$end" -state-hash \
    >"$work/cold.stream" 2>"$work/cold.log"
"$work/bin/firmwaretrace" -steps "$end" -interval "$end" -state-hash \
    -snapshot-in "$work/paused.snapshot" >"$work/restored.stream" 2>"$work/restored.log"

# grep, NOT rg. On at least one machine here `rg` is a shell FUNCTION from an interactive profile
# rather than a binary, so a non-interactive gate script gets "rg: command not found" and the whole
# gate never runs -- silently, because a gate nobody can start looks exactly like a gate nobody
# broke. grep -E is in POSIX and does the same job for these patterns.
cold="$(grep -m1 -E '^state-hash retired=11000000 hash=[0-9A-F]{8}$' "$work/cold.log")"
restored="$(grep -m1 -E '^state-hash retired=11000000 hash=[0-9A-F]{8}$' "$work/restored.log")"
if [[ "$cold" != "$restored" ]]; then
    printf 'snapshot run-on mismatch: cold %s, restored %s\n' "$cold" "$restored" >&2
    exit 1
fi
printf 'snapshot gate: real firmware matched after restore and 10,000,000 more instructions: %s\n' "$cold"
