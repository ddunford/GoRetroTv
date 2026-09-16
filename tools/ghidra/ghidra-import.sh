#!/usr/bin/env bash
# (Re)build the Ghidra project for the Digibox application image.
#
# TWO BLOCKS, because the running program is not one file. The application executes from a RAM
# image the bootloader DECOMPRESSES to 0x800009F4; its strings and the pool words it loads with
# `lw $rx,n($pc)` live in the flash at 0x9FC00000. Import one without the other and half of every
# function is unreadable.
#
# THE SEEDS ARE MIPS16 ENTRY POINTS THAT ARE ALREADY PROVED. The image is mixed MIPS32 and
# MIPS16 reached through JALX, so ISA_MODE cannot be blanketed across it -- that would decode
# every MIPS32 function into plausible nonsense, which is the one failure this project cannot
# afford. Each address below was reached by TRACING the running machine, so each is known to be
# MIPS16 and known to be a function entry; Ghidra's flow analysis expands from them.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GHIDRA="${GHIDRA_HOME:-/opt/ghidra}"
IMAGE="$ROOT/ghidra/unpacked_mine.bin"

if [ ! -x "$GHIDRA/support/analyzeHeadless" ]; then
  echo "Ghidra not found at $GHIDRA — set GHIDRA_HOME, or install it there." >&2
  exit 2
fi
if [ ! -f "$IMAGE" ]; then
  echo "missing $IMAGE — the decompressed application image. It is gitignored like the flash" >&2
  echo "dumps; copy it in before importing." >&2
  exit 2
fi

SEEDS=(
  0x8005141C 0x8008C8A8 0x8003A124 0x80036D0C 0x80036B54 0x8007BB3C 0x8007BCC0 0x8008D0F4
  0x8008E7C8 0x8008CDA4 0x8007D06C 0x800992A0 0x80099244 0x8008DC2C 0x8008DB28 0x800511D0
  0x8008C290 0x80051B2C 0x8005041C 0x800E46F0 0x800DCD58 0x800DCD90 0x800E40EC 0x800E8ED0
  0x80050770 0x8008E5EC 0x800B1AF8 0x80081B40
)

rm -rf "$ROOT/ghidra/proj"
mkdir -p "$ROOT/ghidra/proj"
"$GHIDRA/support/analyzeHeadless" "$ROOT/ghidra/proj" digibox \
  -import "$IMAGE" \
  -processor MIPS:BE:32:16e -loader BinaryLoader -loader-baseAddr 0x800009F4 \
  -scriptPath "$ROOT/ghidra/scripts" -preScript DigiboxSetup.java "${SEEDS[@]}" \
  "$@"
