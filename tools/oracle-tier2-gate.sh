#!/usr/bin/env bash
# Prove exact-instruction localisation against a deliberate mutation of a real browser boot.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
node tools/oracle-cold-boot-capture.mjs --stop-at 102000 --interval 1000 \
    --inject-at 100123 --trace-from 100000 --trace-to 101001 \
    --out "$work/oracle.stream" --trace-out "$work/oracle.trace" >"$work/capture.json"

if ! BIN_DIR="$work/bin" ./ctl.sh build >"$work/build.log" 2>&1; then
    cat "$work/build.log" >&2
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
"$work/bin/firmwaretrace" -steps 102000 -interval 1000 -ack-oracle >"$work/go-1k.stream" 2>"$work/go-1k.log"
set +e
"$work/bin/oraclecmp" "$work/oracle.stream" "$work/go-1k.stream" >"$work/tier1.report"
cmp_status=$?
set -e
if [[ "$cmp_status" -ne 1 ]] || ! grep -Fq 'next: re-run instructions 100001..101000' "$work/tier1.report"; then
    cat "$work/tier1.report" >&2
    printf 'tier 2 gate: tier-1 window did not identify the causal range\n' >&2
    exit 1
fi

"$work/bin/firmwaretrace" -steps 101001 -interval 1 -trace -ack-oracle \
    -trace-from 100000 -trace-to 101001 >"$work/go-cp1.stream" 2>"$work/go.trace"
python3 tools/oracle-tier2-compare.py "$work/oracle.trace" "$work/go-cp1.stream" \
    "$work/go.trace" 100123
cat "$work/tier1.report"
