#!/usr/bin/env python3
"""Find every MIPS16 `lw rx,off(pc)` in the decompressed application image whose pool word
holds a given value -- i.e. the sound way to find callers of a firmware function.

WHY NOT PROXIMITY. An earlier version of the plane-family derivation attributed each pool
word to the nearest preceding function within 0x400 bytes and wrongly put (1,0x3A) in the
family. A pool word belongs to whatever INSTRUCTION resolves to it, and nothing else; this
computes that resolution rather than guessing it.

THE PC BASE RULE. MIPS16 LWPC computes its target from the instruction's own address with
the low two bits cleared -- EXCEPT in a jump delay slot, where the base is the JUMP's
address. Both are emitted here, and a hit under either rule is reported with which rule it
used, because the 0x39/0x3A/0x3B shims only make sense under the delay-slot rule.

Usage:  scripts/mips16-xrefs.py 0x800858f5 [0x800858b1 ...]
        scripts/mips16-xrefs.py --pool 0x80085ef4       (what loads THIS pool word)
"""
import sys, struct

BASE = 0x800009F4
IMG = __file__.rsplit('/', 2)[0] + '/ghidra/unpacked_mine.bin'


def load():
    with open(IMG, 'rb') as fh:
        return fh.read()


def word(img, addr):
    o = addr - BASE
    if o < 0 or o + 4 > len(img):
        return None
    return struct.unpack('>I', img[o:o + 4])[0]


def half(img, addr):
    o = addr - BASE
    if o < 0 or o + 2 > len(img):
        return None
    return struct.unpack('>H', img[o:o + 2])[0]


def is_jump(h):
    """MIPS16 instructions that arm a delay slot: JAL/JALX (0x18 ext) and RR jr/jalr."""
    if (h >> 11) == 0b00011:            # JAL/JALX extend word
        return True
    if (h >> 11) == 0b11101:            # RR
        fn = h & 0x1F
        if fn == 0x00:                  # jr/jalr/jrra family
            return True
    return False


def scan(img, pools):
    """Yield (insn_addr, rx, pool_addr, rule) for every lwpc resolving into `pools`."""
    n = len(img)
    for o in range(0, n - 1, 2):
        addr = BASE + o
        h = struct.unpack('>H', img[o:o + 2])[0]
        if (h >> 11) != 0b10110:        # LWPC: [10110][rx:3][offset:8]
            continue
        rx = (h >> 8) & 7
        off = (h & 0xFF) << 2
        for rule, pcbase in (('normal', addr & ~3),
                             ('delay', (addr - 2) & ~3)):
            if rule == 'delay':
                prev = half(img, addr - 2)
                if prev is None or not is_jump(prev):
                    continue
                pcbase = (addr - 2) & ~3
            target = pcbase + off
            if target in pools:
                yield (addr, rx, target, rule)


# MIPS16 loads and stores with a 5-bit scaled offset. Opcode is the top five bits; the operand
# fields are the standard RRI layout, rx at 10:8, ry at 7:5, immediate at 4:0. `sb ry, off(rx)` --
# the BASE is rx, which is the register an lwpc wrote, and ry is the value. Taken from the
# executor this project runs (frontend/public/mips16.js, MEM_OPS), not from a manual.
MEM_OPS = {0x18: ('sb', 0), 0x19: ('sh', 1), 0x1A: ('sw', 2),
           0x10: ('lb', 0), 0x11: ('lh', 1), 0x12: ('lwsp', 2), 0x13: ('lw', 2),
           0x14: ('lbu', 0), 0x15: ('lhu', 1)}
STORES = {'sb', 'sh', 'sw'}
EXTEND = 0b11110


def follow(img, site, base_field, span=48):
    """From an lwpc at `site` that wrote `base_field`, report accesses using it as a base.

    NAIVE ON PURPOSE, AND IT SAYS SO. This walks straight forward for `span` halfwords without
    following branches and stops when something else writes the register, which is enough to find
    the store beside a pool load and is NOT enough to prove one is absent. An empty result here
    means "not in the next `span` halfwords on the fall-through path", never "this site does not
    store". Read the function when it matters.
    """
    out = []
    addr = site + 2
    end = site + 2 * span
    while addr < end:
        h = half(img, addr)
        if h is None:
            break
        if (h >> 11) == EXTEND:          # an EXTEND prefix; the real instruction follows
            nxt = half(img, addr + 2)
            if nxt is not None and (nxt >> 11) in MEM_OPS:
                name, _ = MEM_OPS[nxt >> 11]
                rx = (nxt >> 8) & 7
                if rx == base_field:
                    out.append((addr, name + ' (extended offset)', (nxt >> 5) & 7, None))
            addr += 4
            continue
        op = h >> 11
        if op in MEM_OPS:
            name, shift = MEM_OPS[op]
            rx, ry, imm = (h >> 8) & 7, (h >> 5) & 7, (h & 0x1F) << MEM_OPS[op][1]
            if rx == base_field:
                out.append((addr, name, ry, imm))
        # another lwpc into the same field retires our tracking
        if op == 0b10110 and ((h >> 8) & 7) == base_field:
            break
        addr += 2
    return out


def main():
    argv = [a for a in sys.argv[1:] if a != '--stores']
    by_pool = False
    if argv and argv[0] == '--pool':
        by_pool = True
        argv = argv[1:]
    if not argv:
        print(__doc__)
        return 2
    img = load()
    wanted = [int(a, 16) for a in argv]

    if by_pool:
        pools = set(wanted)
        label = {p: f'pool {p:#010x} = {word(img, p):#010x}' for p in pools}
    else:
        # every 4-aligned word in the image holding one of the wanted values is a candidate pool
        pools = {}
        vals = set(wanted)
        for o in range(0, len(img) - 3, 4):
            v = struct.unpack('>I', img[o:o + 4])[0]
            if v in vals:
                pools[BASE + o] = v
        label = {p: f'pool {p:#010x} -> {v:#010x}' for p, v in pools.items()}
        pools = set(pools)
        if not pools:
            print('no pool word in the image holds any of those values', file=sys.stderr)
            return 1

    hits = list(scan(img, pools))
    if '--stores' in sys.argv:
        # CONTROL FIRST. The decoder must find accesses it is known to be able to see, or every
        # "no store here" below is a statement about the decoder rather than about the firmware.
        ctl = follow(img, 0x80054DDC, 3)      # (3,0x15): lb v0,0(v1) then lw v0,4(v1), field 3
        kinds = {c[1] for c in ctl}
        if not {'lb', 'lw'} <= kinds:
            print('CONTROL FAILED: the known lb/lw at 0x80054DDC were not decoded (%s)' % (ctl,),
                  file=sys.stderr)
            return 2
        print('control ok: 0x80054DDC decodes as %s\n'
              % ', '.join('%s +%s' % (c[1], c[3]) for c in ctl))
        any_store = False
        for addr, rx, pool, rule in sorted(hits):
            acc = follow(img, addr, rx)
            st = [a for a in acc if a[1] in STORES or a[1].startswith('s')]
            if st:
                any_store = True
                for a, name, ry, imm in st:
                    print('%s  STORE  %s r%d, +%s(base from %s)  [pool %s]'
                          % (f'{a:#010x}', name, ry, imm, f'{addr:#010x}', f'{pool:#010x}'))
        if not any_store:
            print('NO STORE found on any site\'s fall-through path within the scan window.',
                  file=sys.stderr)
            print('That is a limit of this scan, not a proof of absence -- read the functions.',
                  file=sys.stderr)
            return 1
        return 0
    if not hits:
        print('NO INSTRUCTION RESOLVES TO ANY OF THOSE POOL WORDS', file=sys.stderr)
        print('(that is a finding, not an empty result: the pool words exist but nothing loads them)',
              file=sys.stderr)
        return 1
    for addr, rx, pool, rule in sorted(hits):
        print(f'{addr:#010x}  lw r{rx},pc  [{rule}]  {label[pool]}')
    print(f'\n{len(hits)} instruction(s) over {len(pools)} pool word(s)')
    return 0


if __name__ == '__main__':
    sys.exit(main())
