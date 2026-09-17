#!/usr/bin/env bash
# Prove that the declared host handoff leads to guest loader and application code.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

if ! BIN_DIR="$work/bin" ./ctl.sh build >"$work/build.log" 2>&1; then
    cat "$work/build.log" >&2
    printf 'handoff gate: build failed\n' >&2
    exit 1
fi
if ! "$work/bin/firmwaretrace" -steps 20000000 -interval 1000000 \
    -stop-pc 0xBFC20618 >"$work/loader-stream" 2>"$work/loader-log"; then
    cat "$work/loader-log" >&2
    printf 'handoff gate: firmware runner failed before guest loader\n' >&2
    exit 1
fi
if ! grep -Eq '^declared host application handoff after [0-9]+ guest instructions: PC=BFC2048C$' "$work/loader-log" \
    || ! grep -Eq '^reached PC=BFC20618 after [0-9]+ guest instructions;' "$work/loader-log"; then
    cat "$work/loader-log" >&2
    printf 'handoff gate: declared handoff did not reach guest loader\n' >&2
    exit 1
fi

if ! "$work/bin/firmwaretrace" -steps 20000000 -interval 1000000 \
    -stop-pc 0x800009F4 -probe 0x800009F4 >"$work/app-stream" 2>"$work/app-log"; then
    cat "$work/app-log" >&2
    printf 'handoff gate: firmware runner failed before application entry\n' >&2
    exit 1
fi
if ! grep -Eq '^reached PC=800009F4 after [0-9]+ guest instructions;' "$work/app-log" \
    || ! grep -Fqx 'probe 800009F4=63FF6201' "$work/app-log"; then
    cat "$work/app-log" >&2
    printf 'handoff gate: guest did not enter a decompressed application image\n' >&2
    exit 1
fi

printf 'handoff gate: declared host handoff, guest loader and application entry verified\n'
