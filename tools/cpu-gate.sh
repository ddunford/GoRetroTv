#!/usr/bin/env bash
# Run the built CPU through the first video RAM read on the operator's verified firmware.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

anchors="${CPU_GATE_ANCHORS:-tests/fixtures/cpu-oracle-anchors.txt}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

./ctl.sh build >"$work/build.log"
if ! ./bin/firmwaretrace -steps 3204501 -interval 100 -trace -trace-from 3204423 \
    -trace-to 3204425 -unmapped >"$work/checkpoints" 2>"$work/diagnostics"; then
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
((checked == 9)) || { printf 'cpu gate: expected 9 independent oracle anchors, checked %d\n' "$checked" >&2; exit 1; }

grep -Eq '^3204423 PC=BFC0AF5A ISA=true ' "$work/diagnostics" || {
    printf 'cpu gate: the expected MIPS16 video RAM load was not executed\n' >&2; exit 1;
}
grep -Eq '^3204424 .*GPR=.*AAAAAAAA AAAAAAAA ' "$work/diagnostics" || {
    printf 'cpu gate: video RAM read did not return the pattern written by firmware\n' >&2; exit 1;
}
! grep -q 'Virtual:2952798384 ' "$work/diagnostics" || {
    printf 'cpu gate: video RAM data port remains unmapped\n' >&2; exit 1;
}

printf 'cpu gate: %d oracle anchors matched through video RAM read at instruction 3,204,424\n' "$checked"
