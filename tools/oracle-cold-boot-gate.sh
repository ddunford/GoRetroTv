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
# THE CARD IS HELD TO WHAT THE RECORDED ORACLE ANSWERS (-ack-oracle).
#
# The browser oracle's card answers card status and the heartbeat and stays silent on everything
# else. This port used to do the same and no longer does: leaving 0x41 and 0x42 unanswered starves
# the one task that drains the guest's event queue, and the box stops responding to the handset
# after a few presses (gort-slq, gort-b9n). The fix is right and the oracle does not have it, so
# some twenty million instructions into a cold boot the two machines are legitimately doing
# different things.
#
# A checkpoint comparison needs both sides in the same declared condition, which is why this
# comparison ALREADY runs without -sky-gates. This is the same move on the other axis. The oracle
# file is untouched and still the independent check on the CPU, the devices and the boot -- editing
# it to agree with this port is the one move that would destroy its value.
if ! "$work/bin/firmwaretrace" -steps 470000000 -interval 1000 -tasks -ack-oracle \
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
