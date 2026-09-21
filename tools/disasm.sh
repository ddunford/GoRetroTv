#!/usr/bin/env bash
# Disassemble the decompressed application image at a guest address.
#
# WHY THIS EXISTS. Every address this project has found by census -- the grid's row loop, the 544
# a 0xC1 section wakes, the listings ladder -- was named by BEHAVIOUR, because nothing here could
# read the code. tools/ghidra/ was meant to fill that gap and cannot: its ROOT resolves one
# directory short, so it looks for the image and its own scripts under tools/ instead of the repo
# root, and DigiboxDump.java / DigiboxSetup.java are not in the tree at all. Until that is fixed
# this is the way to read MIPS16.
#
# THE IMAGE IS THE SAME BYTES THE CPU RUNS, checked rather than assumed: the words at 0x800CCECC,
# 0x800A8176 and 0x800C4C34 in firmware/application-ram-image.bin are identical to those read out
# of a restored box's DRAM (internal/multiplex/firmwaretests/ramdump_firmware_test.go). The base
# is 0x800009F4, which is also what tools/mips16-xrefs.py uses.
#
# MIPS16 IS THE DEFAULT because that is what the application is compiled to, and a MIPS32 reading
# of it decodes a plausible-looking page of the wrong instructions. Pass -m mips:isa32 for the
# MIPS32 parts reached through JALX.
#
# Usage:  tools/disasm.sh 0x800C4C34 [bytes]        default 256
#         tools/disasm.sh 0x800C4C34 512 -m mips:isa32
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="$ROOT/firmware/application-ram-image.bin"
BASE=$((0x800009F4))
OBJDUMP="${MIPS_OBJDUMP:-mips-linux-gnu-objdump}"

if ! command -v "$OBJDUMP" >/dev/null; then
  echo "$OBJDUMP not found — install binutils-mips-linux-gnu, or set MIPS_OBJDUMP." >&2
  exit 2
fi
if [ ! -f "$IMAGE" ]; then
  echo "missing $IMAGE — the firmware is not redistributable and is not in the tree by default." >&2
  exit 2
fi
if [ $# -eq 0 ]; then
  echo "usage: $0 0x800C4C34 [bytes] [extra objdump args…]" >&2
  exit 2
fi

addr=$(($1)); shift
len=256
if [ $# -gt 0 ] && [[ ${1:-} =~ ^[0-9]+$ ]]; then len=$1; shift; fi

off=$((addr - BASE))
if [ "$off" -lt 0 ]; then
  echo "address $(printf '%#x' "$addr") is below the image base $(printf '%#x' "$BASE")" >&2
  exit 2
fi

tmp="$(mktemp)"; trap 'rm -f "$tmp"' EXIT
dd if="$IMAGE" of="$tmp" bs=1 skip="$off" count="$len" status=none
"$OBJDUMP" -D -b binary -m mips:16 -EB --adjust-vma="$addr" "$@" "$tmp" \
  | sed -n '/^ /p'
