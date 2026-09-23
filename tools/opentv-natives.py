#!/usr/bin/env python3
"""Resolve the OpenTV native API out of the Digibox flash image.

WHY THIS EXISTS. Every native call the application makes is `scall (module, function)`, and for
months those were named only by their numbers -- so a census could say "(1,0x2D) ran 49 times" and
nothing more. The table that turns a number into firmware is readable statically, and once it is,
each native can be decompiled (`./ctl.sh decompile <fn>`) and NAMED.

THE LAYOUT, and the one reading that has already gone wrong. Module 1's function array at
0x9FC29F04 is 236 FOUR-BYTE POINTERS, each to an 8-byte {implementation, argument descriptor}
record -- it is NOT an array of 8-byte records. Reading it that way is self-consistent enough to
look right (the records sit immediately after the array, so entry n's "implementation" is the
pointer to record n and its "descriptor" is the pointer to record n+1, i.e. implementation + 8)
and every descriptor then reads as impl+8, which is the tell.

The implementation is a MIPS16 address (bit 0 set) of a 16-byte thunk:

    addiu sp,-8 ; sw ra,4(sp) ; lw rx,off(pc) ; jalr rx ; ... ; jr a0 ; addiu sp,8

so the real function is the word at ((lw address) & ~3) + off. 210 of the 236 have that shape; the
other 26 do their work inline and ARE the function.

The descriptor is a byte string: [return type][arg type]...[0x00]. Type 1 is an integer, 2 a
pointer, 3 a long/pair -- read off the call sites, so the ARG COUNT is exact and the types are
indicative. (1,0xD5) declares five arguments and its one known call site pushes five.

VERIFIED AGAINST THE RUNNING MACHINE. The dispatcher indexes a table of {function array, count}
pairs held at the word in 0x8006E71C and built at boot in DRAM; scripts/digibox-probes/
native-table.js reads it live and it holds the same record pointers as the flash copy. Walking the
flash and reporting on the running box would otherwise be exactly the two-record trap.

Usage:
    tools/opentv-natives.py                    # the whole module-1 table
    tools/opentv-natives.py 0x2D 0xE4          # just these
    tools/opentv-natives.py --shims            # shim addresses only, for a pcHits census
    tools/opentv-natives.py --modules          # every module and its size
    tools/opentv-natives.py --module 12 0x08   # a native in another module
"""
import argparse
import struct
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
# The flash lives in firmware/ here. It was frontend/public/ in the predecessor and the
# path did not move with the tool, so every invocation died in the import.
FLASH = ROOT / "firmware/FLASH_U202.bin"
RAMIMG = ROOT / "firmware/application-ram-image.bin"
FLASH_K0 = 0x9FC00000
RAM_BASE = 0x800009F4          # where the bootloader decompresses the application image
MODULE1_ARRAY = 0x9FC29F04
MODULE1_COUNT = 236

# EVERY MODULE, NOT JUST MODULE 1. For a long time this tool resolved 236 natives and all of them
# were module 1, because that was the only function array whose address anyone had written down --
# so a census could name (1,0x5B) and a call to any other module was a number and nothing else.
#
# The dispatcher indexes {function array, count} pairs, eight bytes each, at the address held in the
# word at 0x8006E71C. That table is BUILT AT BOOT IN DRAM, so it cannot be read from the flash image
# and is not hardcoded here out of laziness: the arrays it points at ARE in flash, and these are the
# values a running box produced.
#
# internal/multiplex/firmwaretests/nativemodules_firmware_test.go re-derives this from a live box
# and checks module 1 against MODULE1_ARRAY/MODULE1_COUNT above as its control -- so if the table
# ever moves, that test fails rather than this tool quietly resolving the wrong addresses.
MODULES = {
    0: (0x9FC29A34, 90),
    1: (MODULE1_ARRAY, MODULE1_COUNT),
    2: (0x9FC2AB00, 150),
    3: (0x9FC2B2B4, 99),
    4: (0x9FC2B7C4, 34),
    5: (0x9FC2B9A8, 1),
    7: (0x9FC2B9BC, 61),
    8: (0x9FC24438, 52),
    9: (0x9FC24554, 5),
    12: (0x9FC2BEA4, 51),
    14: (0x9FC2BDF8, 11),
    32: (0x9FC2BD50, 10),
}

_flash = FLASH.read_bytes()
_ram = RAMIMG.read_bytes()


def fw(addr):
    o = addr - FLASH_K0
    if not 0 <= o <= len(_flash) - 4:
        raise ValueError("flash address out of range: %08x" % addr)
    return struct.unpack_from(">I", _flash, o)[0]


def fb(addr, n):
    o = addr - FLASH_K0
    return _flash[o:o + n]


def rw(addr):
    o = addr - RAM_BASE
    if not 0 <= o <= len(_ram) - 4:
        raise ValueError("ram address out of range: %08x" % addr)
    return struct.unpack_from(">I", _ram, o)[0]


def rh(addr):
    o = addr - RAM_BASE
    return struct.unpack_from(">H", _ram, o)[0]


def shim_target(impl):
    """Follow the 16-byte thunk. Returns (function, pool_address) or (None, None)."""
    a = impl & ~1
    for i in range(0, 10, 2):
        try:
            ins = rh(a + i)
        except ValueError:
            return None, None
        if (ins & 0xF800) != 0xB000:          # lw rx, off(pc)
            continue
        rx = (ins >> 8) & 7
        pool = ((a + i) & ~3) + (ins & 0xFF) * 4
        nxt = rh(a + i + 2)
        if (nxt & 0xF8FF) != 0xE840 or ((nxt >> 8) & 7) != rx:   # jalr rx
            return None, None
        return rw(pool), pool
    return None, None


def descriptor(ptr):
    out = []
    while len(out) < 16:
        c = fb(ptr + len(out), 1)[0]
        out.append(c)
        if len(out) > 1 and c == 0:
            break
    return out


def resolve(n, module=1):
    array, _ = MODULES[module]
    rec = fw(array + 4 * n)
    impl = fw(rec)
    desc = descriptor(fw(rec + 4))
    fn, pool = shim_target(impl)
    inline = fn is None
    return {
        "n": n, "module": module, "rec": rec, "shim": impl & ~1, "pool": pool,
        "fn": (fn & ~1) if fn else (impl & ~1),
        "inline": inline,
        "ret": desc[0], "args": desc[1:-1],
    }


def table(module=1):
    return [resolve(n, module) for n in range(MODULES[module][1])]


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("ids", nargs="*", help="native ids, e.g. 0x2D (default: all)")
    ap.add_argument("--shims", action="store_true", help="print shim addresses only")
    ap.add_argument("--module", type=lambda x: int(x, 0), default=1,
                    help="which native module (default 1); --modules lists them")
    ap.add_argument("--modules", action="store_true", help="list every module and its size")
    args = ap.parse_args()

    if args.modules:
        print("module  array       functions")
        for m in sorted(MODULES):
            print("  %2d    0x%08X  %4d" % (m, MODULES[m][0], MODULES[m][1]))
        print("\n%d modules, %d natives. Re-derived from a live box by "
              "TestEveryNativeModuleTheDispatcherKnows." %
              (len(MODULES), sum(c for _, c in MODULES.values())))
        return
    if args.module not in MODULES:
        sys.exit("no module %d; known: %s" % (args.module, ", ".join(str(m) for m in sorted(MODULES))))

    rows = table(args.module)
    if args.ids:
        want = {int(x, 0) for x in args.ids}
        rows = [r for r in rows if r["n"] in want]
        if len(rows) != len(want):
            sys.exit("no such native: %s" % sorted(want - {r["n"] for r in rows}))

    if args.shims:
        print(" ".join("0x%08X" % r["shim"] for r in rows))
        return

    # A table nobody can check is testimony. Say which entries are inline so a reader knows the
    # "fn" for those is the implementation itself and not the target of a thunk.
    shared = {}
    for r in table(args.module):
        shared.setdefault(r["fn"], []).append(r["n"])
    print("native     shim        fn          ret args          shares fn with")
    for r in rows:
        peers = [x for x in shared[r["fn"]] if x != r["n"]]
        print("(%d,0x%02X)  0x%08X  0x%08X%s  %d   %-12s  %s" % (
            args.module, r["n"], r["shim"], r["fn"], "*" if r["inline"] else " ", r["ret"],
            ",".join(str(a) for a in r["args"]) or "-",
            " ".join("0x%02X" % p for p in peers) or ""))
    if not args.ids:
        print("\n* = no thunk; the implementation does the work inline (%d of %d)"
              % (sum(1 for r in table(args.module) if r["inline"]), MODULES[args.module][1]))


if __name__ == "__main__":
    main()
