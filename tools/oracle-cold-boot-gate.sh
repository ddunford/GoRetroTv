#!/usr/bin/env bash
# Compare a real cold boot with the independent browser trace before presentation patches.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

oracle="internal/platform/statehash/testdata/oracle-cold-boot-nogates.stream"
[[ -f "$oracle" ]] || { printf 'oracle gate: missing browser trace %s\n' "$oracle" >&2; exit 1; }
grep -Fqx 'GRTV-CHECKPOINTS 1 interval=1000' "$oracle" || {
    printf 'oracle gate: browser trace has the wrong cadence\n' >&2
    exit 1
}
grep -Fqx 'END 470000 469999000' "$oracle" || {
    printf 'oracle gate: browser trace is incomplete\n' >&2
    exit 1
}

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
if ! BIN_DIR="$work/bin" ./ctl.sh build >"$work/build.log" 2>&1; then
    cat "$work/build.log" >&2
    printf 'oracle gate: build failed\n' >&2
    exit 1
fi
if ! "$work/bin/firmwaretrace" -steps 470000000 -interval 1000 -tasks \
    >"$work/go.stream" 2>"$work/boot.log"; then
    tail -80 "$work/boot.log" >&2
    printf 'oracle gate: firmware run failed\n' >&2
    exit 1
fi
if ! grep -Fqx 'tasks found: 42' "$work/boot.log"; then
    tail -80 "$work/boot.log" >&2
    printf 'oracle gate: firmware did not create 42 Nucleus tasks\n' >&2
    exit 1
fi
if ! "$work/bin/oraclecmp" "$oracle" "$work/go.stream"; then
    printf 'oracle gate: full cold-boot checkpoints differ\n' >&2
    exit 1
fi
printf 'oracle gate: 42 guest tasks and 470,000 matching browser checkpoints through 469,999,000 instructions\n'
