#!/usr/bin/env bash
# Prove handset delivery, card progress and CSI acknowledgement policy in real guest firmware.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
if ! BIN_DIR="$work/bin" ./ctl.sh build >"$work/build.log" 2>&1; then
    cat "$work/build.log" >&2
    exit 1
fi

trace="$work/bin/firmwaretrace"
run_trace() {
    local label="$1"
    shift
    if ! "$trace" -interval 1000000 "$@" >"$work/$label.stream" 2>"$work/$label.log"; then
        tail -80 "$work/$label.log" >&2
        printf 'links gate: %s firmware run failed\n' "$label" >&2
        exit 1
    fi
}

fail() {
    printf 'links gate: %s\n' "$1" >&2
    exit 1
}

count_pc() {
    local label="$1" pc="$2"
    sed -n "s/^pc-hit $pc total=\([0-9][0-9]*\).*/\1/p" "$work/$label.log"
}

after_key() {
    local pc="$1"
    sed -n "s/^pc-hit $pc .*after-key=\([0-9][0-9]*\)$/\1/p" "$work/key.log"
}

# The first run starts with an absent NVRAM file. The guest creates the system
# area and persists it through the model's real I2C STOP transactions.
run_trace cold -steps 470000000 -nvram "$work/cold.nvram" \
    -pc-hit 0x8002CCA0 -demod-polls -tasks
[[ -f "$work/cold.nvram" ]] || fail 'the guest did not create an NVRAM image'
[[ "$(stat -c %s "$work/cold.nvram")" == 16384 ]] || fail 'NVRAM image has the wrong size'
grep -Fqx 'tasks found: 42' "$work/cold.log" || fail 'cold guest boot did not create 42 tasks'
card_hits="$(count_pc cold 8002CCA0)"
[[ "$card_hits" =~ ^[0-9]+$ ]] && (( card_hits >= 12 )) || fail 'guest smartcard ISR did not complete the boot exchange'
# The current cold boot reaches the demodulator over the guest's I2C path but
# does not poll the fixed lock answers at registers 75 and 78. Pin the actual
# read phases so a missing observer or dead bus cannot pass as zero traffic.
grep -Fqx 'demod-read total=11' "$work/cold.log" || fail 'guest demodulator read count changed'
for observed in \
    '0 count=2 value=00' '1 count=1 value=00' '2 count=1 value=00' \
    '3 count=1 value=00' '4 count=2 value=00' '5 count=1 value=00' \
    '14 count=1 value=00' '1025 count=2 value=00'; do
    grep -Fqx "demod-read register=$observed" "$work/cold.log" \
        || fail "guest demodulator read register $observed changed"
done
[[ "$(grep -c '^demod-read register=' "$work/cold.log")" == 8 ]] \
    || fail 'guest demodulator read additional registers'
for task in SCTask ECM EMM TASK0; do
    grep -Eq "^task [0-9]+ .*name=\"$task\" .*runs=[1-9][0-9]*" "$work/cold.log" \
        || fail "guest CA task $task was not scheduled after the card exchange"
done

for label in baseline key allack; do
    cp -f "$work/cold.nvram" "$work/$label.nvram"
done
run_trace baseline -steps 230000000 -sky-gates -nvram "$work/baseline.nvram" \
    -pc-hit 0x800297B0 -pc-hit 0x8006EA04 -surface-hash -tasks
run_trace key -steps 230000000 -sky-gates -nvram "$work/key.nvram" \
    -key 0x7D -key-at 200000000 -pc-hit 0x800297B0 -pc-hit 0x8006EA04 -surface-hash -tasks
run_trace allack -steps 230000000 -sky-gates -ack-all -nvram "$work/allack.nvram" \
    -surface-hash -tasks

for label in baseline key allack; do
    grep -Fqx 'tasks found: 42' "$work/$label.log" || fail "$label guest boot did not create 42 tasks"
done
[[ "$(count_pc baseline 8006EA04)" == 0 ]] || fail 'no-key baseline unexpectedly reached the input-event routine'
dispatcher="$(after_key 800297B0)"
events="$(after_key 8006EA04)"
[[ "$dispatcher" =~ ^[0-9]+$ ]] && (( dispatcher > 0 )) || fail 'Sky key did not reach the guest dispatcher'
[[ "$events" =~ ^[0-9]+$ ]] && (( events > 0 )) || fail 'Sky key did not reach the guest input-event routine'
grep -Fq 'surface hash=9825B318 distinct=12 ' "$work/baseline.log" \
    || fail 'baseline warm surface disagrees with the measured oracle'
grep -Fq 'surface hash=F3634409 distinct=37 ' "$work/key.log" \
    || fail 'Sky-key surface disagrees with the measured oracle'
cmp -s "$work/baseline.stream" "$work/allack.stream" \
    && fail 'all-ack policy did not change the guest state stream'
baseline_gate="$(sed -n 's/^declared Sky menu gates applied after \([0-9][0-9]*\) guest instructions$/\1/p' "$work/baseline.log")"
allack_gate="$(sed -n 's/^declared Sky menu gates applied after \([0-9][0-9]*\) guest instructions$/\1/p' "$work/allack.log")"
[[ "$baseline_gate" =~ ^[0-9]+$ && "$allack_gate" =~ ^[0-9]+$ && "$baseline_gate" != "$allack_gate" ]] \
    || fail 'all-ack policy did not change guest boot progression to the menu gate'

printf 'links gate: 42-task boot, %s card ISR hits, key dispatcher +%s, input events +%s\n' \
    "$card_hits" "$dispatcher" "$events"
printf 'links gate: oracle surfaces 9825B318/12 -> F3634409/37; all-ack gate %s -> %s\n' \
    "$baseline_gate" "$allack_gate"
printf 'links gate: 11 guest demod I2C reads across 8 registers; no 75/78 lock poll in this cold boot\n'
