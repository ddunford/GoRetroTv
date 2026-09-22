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

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# ROOT is the REPO root: this script lives in tools/ghidra/, so it is two levels up, not one. It
# was one for a while and every path below silently resolved inside tools/ -- the scripts looked
# for tools/ghidra/unpacked_mine.bin and tools/ghidra/scripts, neither of which has ever existed,
# while tools/mips16-xrefs.py resolved the same image to the repo root. The two tools disagreed
# about where the firmware lived and neither said so.
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

  # THE ALL CHANNELS GRID's own path, every address measured on the running machine.
  #
  # The four PRESENTATION CALLBACKS registered in the six-entry table at 0x80164978: each
  # descriptor is 44 bytes with a mode at +0x28 and a function pointer at +0x24, and the grid
  # selects entry 1. Entries 4 and 5 hold the "not registered" sentinel and a null pointer.
  0x8009FE6C   # [0] mode 1
  0x800CB7B8   # [1] mode 1 -- THE GRID'S ROW CALLBACK
  0x800CBB60   # [2] mode 0
  0x800CBB80   # [3] mode 2
  # The enumeration and the chain its exit runs through.
  0x800A4A90   # the enumeration containing the row loop at 0x800A4B60
  0x800A45C4   # "fill in this channel's details, by index"
  0x800ADD08   # the handle resolver: pool index (id-1)>>12, element (id-1)&0xFFF
  0x800AC534   # the state -> code jump table (4,5 -> 2; 6 -> 3; 7 -> 4)
  0x800AA8F8   # writes the transport state the grid gates on
  # The listings module the banner enters and the grid never does.
  0x800A8520 0x800A86DC 0x800A9046 0x800AA968
)

# EXTRA SEEDS WITHOUT EDITING THIS FILE. A full re-import is about twenty-five minutes, and the
# reason to run one is nearly always a single new MIPS16 address that the analyser's flow never
# reached -- which DigiboxDump reports as "no function here". Passing it here beats a code edit:
#   GHIDRA_SEEDS="0x800CB7B8 0x8009FE6C" ./ctl.sh ghidra:import
read -r -a EXTRA_SEEDS <<< "${GHIDRA_SEEDS:-}"
SEEDS+=("${EXTRA_SEEDS[@]}")

rm -rf "$ROOT/ghidra/proj"
mkdir -p "$ROOT/ghidra/proj"
"$GHIDRA/support/analyzeHeadless" "$ROOT/ghidra/proj" digibox \
  -import "$IMAGE" \
  -processor MIPS:BE:32:16e -loader BinaryLoader -loader-baseAddr 0x800009F4 \
  -scriptPath "$ROOT/tools/ghidra/scripts" -preScript DigiboxSetup.java "$ROOT" "${SEEDS[@]}" \
  "$@"
