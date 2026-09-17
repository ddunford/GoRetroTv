#!/usr/bin/env bash
# Prove one real guest input recording has two byte-identical replays.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

snapshot_dir="${GORETROTV_SNAPSHOT_DIR:-snapshots}"
snapshot="$snapshot_dir/post-acquisition.snapshot"
[[ -f "$snapshot" ]] || { printf 'post-acquisition snapshot missing; run ./ctl.sh snapshot seed\n' >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
BIN_DIR="$work/bin" ./ctl.sh build >"$work/build.log" 2>&1 || { cat "$work/build.log" >&2; exit 1; }
binary="$work/bin/firmwaretrace"
"$binary" -snapshot-in "$snapshot" -steps 1120000000 -interval 2000000000 \
    -key 0x7D -key-at 1100000000 -record-out "$work/inputs.json" \
    -surface-hash -state-hash >"$work/record.out" 2>"$work/record.log"
for run in a b; do
    "$binary" -snapshot-in "$snapshot" -replay-in "$work/inputs.json" \
        -interval 2000000000 -surface-hash -state-hash \
        >"$work/replay-$run.out" 2>"$work/replay-$run.log"
done

record_result="$(rg -m1 '^surface hash=' "$work/record.log")"
replay_a="$(rg -m1 '^surface hash=' "$work/replay-a.log")"
replay_b="$(rg -m1 '^surface hash=' "$work/replay-b.log")"
record_state="$(rg -m1 '^state-hash retired=' "$work/record.log")"
state_a="$(rg -m1 '^state-hash retired=' "$work/replay-a.log")"
state_b="$(rg -m1 '^state-hash retired=' "$work/replay-b.log")"
[[ "$record_result" == "$replay_a" && "$replay_a" == "$replay_b" ]] || { printf 'replayed framebuffer differs\n' >&2; exit 1; }
[[ "$record_state" == "$state_a" && "$state_a" == "$state_b" ]] || { printf 'replayed instruction count or machine state differs\n' >&2; exit 1; }
[[ "$record_result" == *'hash=F3634409 distinct=37'* ]] || { printf 'Sky key did not draw measured Box Office surface\n' >&2; exit 1; }
printf 'input replay gate: two real-firmware replays matched recording: %s; %s\n' "$record_result" "$record_state"
