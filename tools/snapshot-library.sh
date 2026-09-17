#!/usr/bin/env bash
# Keep named, firmware-containing machine images outside Git.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

usage() {
    printf 'usage: ./ctl.sh snapshot {save|run|inspect|list|seed} [name] [firmwaretrace options]\n' >&2
    exit 2
}

command="${1:-}"
[[ -n "$command" ]] || usage
shift

snapshot_dir="${GORETROTV_SNAPSHOT_DIR:-snapshots}"
binary_dir="${BIN_DIR:-bin}"
if [[ "$command" == seed ]]; then
    (($# == 0)) || usage
    exec ./tools/seed-post-acquisition.sh
fi
if [[ "$command" == list ]]; then
    (($# == 0)) || usage
    [[ -d "$snapshot_dir" ]] || exit 0
    find "$snapshot_dir" -maxdepth 1 -type f -name '*.snapshot' -printf '%f\n' \
        | sed 's/\.snapshot$//' | sort
    exit 0
fi

name="${1:-}"
[[ -n "$name" ]] || usage
shift
[[ "$name" =~ ^[a-z0-9][a-z0-9-]{0,63}$ ]] || {
    printf 'snapshot name must be 1–64 lowercase letters, digits or hyphens\n' >&2
    exit 2
}
path="$snapshot_dir/$name.snapshot"

case "$command" in
    save)
        [[ ! -e "$path" ]] || { printf 'snapshot already exists: %s\n' "$path" >&2; exit 1; }
        mkdir -p "$snapshot_dir"
        chmod 0700 "$snapshot_dir"
        ./ctl.sh build
        "$binary_dir/firmwaretrace" "$@" -snapshot-out "$path"
        printf 'saved snapshot %s\n' "$path" >&2
        ;;
    run)
        [[ -f "$path" ]] || { printf 'snapshot not found: %s\n' "$path" >&2; exit 1; }
        ./ctl.sh build
        exec "$binary_dir/firmwaretrace" -snapshot-in "$path" "$@"
        ;;
    inspect)
        (($# == 0)) || usage
        [[ -f "$path" ]] || { printf 'snapshot not found: %s\n' "$path" >&2; exit 1; }
        ./ctl.sh build
        exec "$binary_dir/firmwaretrace" -snapshot-in "$path" -steps 0 -surface-hash -state-hash
        ;;
    *) usage ;;
esac
