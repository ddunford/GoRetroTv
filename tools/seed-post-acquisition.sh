#!/usr/bin/env bash
# Rebuild the private, pressable SI checkpoint from verified local firmware.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

snapshot_dir="${GORETROTV_SNAPSHOT_DIR:-snapshots}"
binary_dir="${BIN_DIR:-bin}"
target="$snapshot_dir/post-acquisition.snapshot"
[[ ! -e "$target" ]] || { printf 'snapshot already exists: %s\n' "$target" >&2; exit 1; }
mkdir -p "$snapshot_dir"
chmod 0700 "$snapshot_dir"
work="$(mktemp -d "$snapshot_dir/.seed.XXXXXX")"
cleanup() {
    if [[ "${GORETROTV_KEEP_SEED_WORK:-0}" == 1 ]]; then
        printf 'kept seed evidence in %s\n' "$work" >&2
    else
        rm -rf "$work"
    fi
}
trap cleanup EXIT

./ctl.sh build
"$binary_dir/firmwaretrace" -steps 470000000 -interval 470000000 \
    -nvram "$work/cold.nvram" -tasks > "$work/cold.log" 2>&1
grep -Fqx 'tasks found: 42' "$work/cold.log" || {
    printf 'cold firmware did not reach 42 guest tasks\n' >&2; exit 1;
}
nvram_digest="$(sha256sum "$work/cold.nvram")"
[[ "${nvram_digest%% *}" == 41f13bfe6882f1ad4b82de1e0ff5db134b79a676482f1e0c008b063bac40473b ]] || {
    printf 'cold EEPROM differs from the measured acquisition seed\n' >&2; exit 1;
}
cp -f "$work/cold.nvram" "$work/warm.nvram"

# The guest's own filters request these clock, network, bouquet and service tables.
# Their byte fixtures and the independent browser comparison are recorded in TC-3a.7.
sections=(
    -section 650000000:20:707005c67e120000
    -section 650000000:20:73701ac67e120000f00f580d474252020000c67e12000000005b445d38
    -section 654000000:20:707005c67e120002
    -section 654000000:20:73701ac67e120002f00f580d474252020000c67e12000200005efbb56b
    -section 658000000:20:707005c67e120004
    -section 658000000:20:73701ac67e120004f00f580d474252020000c67e1200040000503b8d9e
    -section 662000000:16:40b0320020c10000f00d400b536b79204469676974616cf01800000020f012430b0117780002828100275002410300640170d6cb6a
    -section 663000000:17:4ab01d1000c10000f0054703536b79f00b00000020f00541030064014021fe68
    -section 664000000:17:42b0220000c100000020ff0064fd8011480f010542536b794207536b79204f6e65ea67d4e9
)
"$binary_dir/firmwaretrace" -steps 1100000000 -interval 1100000000 \
    -sky-gates -nvram "$work/warm.nvram" "${sections[@]}" \
    -tasks -section-state -surface-hash -state-hash \
    -snapshot-out "$work/acquired.snapshot" > "$work/acquired.log" 2>&1
for expected in \
    'tasks found: 42' \
    'section-state enable=FFE00000 status=00000000 armed-pids=[82 20 17 16]' \
    'section-match unit=1 table=40/FE extension=0020/FFFF' \
    'section-match unit=2 table=42/FB extension=0000/FFFF' \
    'section-match unit=3 table=4A/FF extension=1000/FFFF' \
    'section-match unit=5 table=73/FF extension=0000/0000' \
    'state-hash retired=1100000000 hash=8B2A7E0B'; do
    grep -Fqx "$expected" "$work/acquired.log" || {
        printf 'acquired firmware missed expected state: %s\n' "$expected" >&2; exit 1;
    }
done
grep -Fq 'surface hash=9825B318 distinct=12 ' "$work/acquired.log" || {
    printf 'acquired firmware baseline surface changed\n' >&2; exit 1;
}

"$binary_dir/firmwaretrace" -snapshot-in "$work/acquired.snapshot" \
    -steps 1120000000 -key 0x7D -key-at 1100000000 \
    -pc-hit 0x8006EA04 -surface-hash -state-hash > "$work/key.log" 2>&1
grep -Fqx 'pc-hit 8006EA04 total=2 before-key=0 after-key=2' "$work/key.log" || {
    printf 'Sky key did not reach the guest input routine\n' >&2; exit 1;
}
grep -Fq 'surface hash=F3634409 distinct=37 ' "$work/key.log" || {
    printf 'Sky key did not draw the measured Box Office surface\n' >&2; exit 1;
}
grep -Fqx 'state-hash retired=1120000000 hash=847B9151' "$work/key.log" || {
    printf 'Sky key run-on state differs from the measured checkpoint\n' >&2; exit 1;
}

# The temporary image and final name are on one filesystem. Hard-linking refuses
# a concurrent creator without replacing their image; the trap removes our source.
ln "$work/acquired.snapshot" "$target"
printf 'saved verified post-acquisition snapshot %s\n' "$target"
