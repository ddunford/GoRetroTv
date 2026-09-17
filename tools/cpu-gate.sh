#!/usr/bin/env bash
# Run the built CPU through the Phase 2 peripheral wall on the operator's verified firmware.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

anchors="${CPU_GATE_ANCHORS:-tests/fixtures/cpu-oracle-anchors.txt}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

./ctl.sh build >"$work/build.log"
if ! ./bin/firmwaretrace -steps 3204501 -interval 100 -trace -trace-from 3204423 \
    -trace-to 3204424 -unmapped >"$work/checkpoints" 2>"$work/diagnostics"; then
    cat "$work/diagnostics" >&2
    printf 'cpu gate: firmware runner failed\n' >&2
    exit 1
fi

checked=0
while read -r icount hash; do
    [[ -n "${icount:-}" && "${icount:0:1}" != '#' ]] || continue
    if ! grep -Fqx "$icount $hash" "$work/checkpoints"; then
        printf 'cpu gate: oracle checkpoint %s %s was not reproduced\n' "$icount" "$hash" >&2
        exit 1
    fi
    ((checked+=1))
done < "$anchors"
((checked == 8)) || { printf 'cpu gate: expected 8 independent oracle anchors, checked %d\n' "$checked" >&2; exit 1; }

# The Go bus does not model the video RAM port yet. The last anchor must match, then the
# immediately following checkpoint must differ for the documented reason. This expectation is
# replaced by farther oracle agreement when the Phase 3b video RAM device is attached.
grep -Fqx '3204500 0x97464295' "$work/checkpoints" || {
    printf 'cpu gate: the measured first divergent checkpoint moved\n' >&2; exit 1;
}
grep -Eq '^3204423 PC=BFC0AF5A ISA=true ' "$work/diagnostics" || {
    printf 'cpu gate: the expected MIPS16 video RAM load was not executed\n' >&2; exit 1;
}
grep -Eq '^unmapped \{Virtual:2952798384 Physical:268443824 Kind:no device Reads:1 ' "$work/diagnostics" || {
    printf 'cpu gate: video RAM data port 0xB00020B0 was not identified as the missing device\n' >&2; exit 1;
}

printf 'cpu gate: %d oracle anchors matched through instruction 3,204,400\n' "$checked"
printf 'cpu gate: first divergent checkpoint 3,204,500 follows video RAM read at 0xB00020B0\n'
