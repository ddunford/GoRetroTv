#!/usr/bin/env python3
"""A disassembler for the OpenTV o-code the Pace 2500N's resident EPG is written in.

WHY IT IS BUILT THIS WAY
------------------------
The bytecode is undocumented. OpenTV's public GPL SDK is version 3.2 and this runtime is
1.2S4Bj, so those headers are indicative at best. The interpreter in the image IS the
specification -- its dispatch is 209 signed 16-bit base-relative offsets at 0x800692E0 -- and
its BEHAVIOUR is the ground truth for operand lengths.

So nothing here is guessed from reading handlers. The operand table is MEASURED from a trace of
the running machine (scripts/digibox-probes/bytecode-trace.js): the interpreter's main fetch
site reads opcode bytes and every other site reads operands, so the bytes consumed between two
main-site reads are that opcode's operands. A sample is only trusted when the next opcode lands
exactly at op + 1 + operands; anything else is recorded as control flow, never as a length.

The control-flow forms in FLOW below were then each fitted to EVERY observed occurrence of that
opcode, and a form that fitted some occurrences but not all was rejected. The returns are the
strongest evidence in the set: their targets are exactly the return addresses of the calls that
preceded them, which no coincidence produces.

WHY THE VALIDATION IS SOUND
---------------------------
A decoder run over bytes does not fail -- it produces plausible output, and the denser the
encoding the more convincing the nonsense. So this tool does not ask you to believe its listing.
`--check` walks the bytecode and requires every instruction boundary it computes to be an
address the machine actually fetched. A decoder with one operand length wrong drifts within a
few instructions and lands between real instructions, and the check says so. Boundaries that are
absent because the trace never took that branch are reported separately from boundaries that are
absent because the decoder drifted -- they are different answers and collapsing them loses the
one you can act on.

USAGE
    ./ctl.sh digibox:probe scripts/digibox-probes/bytecode-trace.js > trace.json
    python3 tools/ocode-disasm.py --trace trace.csv --check 0x9FC4A538 0x9FC4A60B
    python3 tools/ocode-disasm.py --trace trace.csv 0x9FC4A538 0x9FC4A60B
"""
import argparse
import collections
import struct
import sys

FLASH_DEFAULT = 'firmware/FLASH_U202.bin'
FLASH_BASE = 0x9FC00000
MAIN_FETCH_SITE = '0x80069298'      # the interpreter's opcode fetch; everything else is operands
DISPATCH = 0x800692E0               # 209 signed halfword offsets, target = DISPATCH + offset
N_OPCODES = 209
RAM_BASE = 0x800009F4               # where the unpacked runtime image starts, for --handlers

# ---------------------------------------------------------------------------
# The control-flow model, solved against the trace rather than assumed.
#
# A conditional branch's operand length CANNOT be measured from a not-taken sample: the
# interpreter skips the read entirely when it does not branch, so the instruction is two bytes
# long while zero operand bytes are read. That is why these lengths are declared here and the
# measured table is overridden by them.
# CORRECTED AGAINST THE VENDOR'S OWN TABLE. Three of these were wrong, and the listings this
# tool produced were wrong with them: 0x16 and 0x17 are CALLR_NNNN and CALLR_NN -- CALLS, not
# jumps -- and 0xBB is RET_4_NN, a RETURN, not a one-byte call. They were fitted to traces that
# could not tell a call from a jump, because both transfer control and the return only shows up
# later. See SDK_OPS below for where the names come from.
CALL_REL4 = {0x15}                              # CALLR_NNNNNNNN, target = next + int32
CALL_REL2 = {0x16}                              # CALLR_NNNN
CALL_REL1 = {0x17}                              # CALLR_NN
JUMP_REL1 = {0x41}                              # JMPR_NN
JUMP_REL2 = {0x42}                              # JMPR_NNNN
# JLT JGT JGTU JLE JEQ JNE JZ JNZ, all _NN
BRANCH_REL1 = {0x26, 0x29, 0x2A, 0x2C, 0x32, 0x33, 0x34, 0x36}
RETURN = {0xB4, 0xB5, 0xB6, 0xB8, 0xBB}         # RET_0_0/0_4/0_8/4_0 and RET_4_NN
# An indirect call: no operand at all, the target comes off the stack. It cannot be measured the
# way the rest of the table is, because the interpreter never reads an operand for it and the
# trace therefore shows only a transfer -- which is indistinguishable from a jump until you look
# at where control comes BACK to. The RETURN ADDRESS is what settles it: at 0x9FC4DCE3 the
# machine fetched this opcode, transferred to 0x9FC4DF3C, and later resumed at 0x9FC4DCE4. One
# byte on, so the instruction is one byte long, and it is a call rather than a jump because
# execution returned to the instruction after it at all.
CALL_INDIRECT = {0x14}

FLOW = {}
for _set, _n in ((CALL_REL4, 4), (CALL_REL2, 2), (CALL_REL1, 1), (JUMP_REL1, 1),
                 (JUMP_REL2, 2), (BRANCH_REL1, 1), (CALL_INDIRECT, 0)):
    for _op in _set:
        FLOW[_op] = _n
FLOW[0xB4] = FLOW[0xB5] = FLOW[0xB6] = FLOW[0xB8] = 0
FLOW[0xBB] = 1                                  # RET_4_NN carries a byte

# GENERATED from the OpenTV GNU SDK (optv_sdk32_gnu_cd, GPL) --
# binut-2.51/include/opcode/oplist.h defines the 209 opcodes IN ORDER, HALT first, so the
# index IS the opcode byte. Operand lengths come from the name suffix (NN=1, NNNN=2,
# NNNNNNNN=4, UU=1, UUUU=2) EXCEPT where a length was measured off the running machine, which
# wins -- the two agreed on 95 of the 102 opcodes this project has observed, and all seven
# exceptions are names with single-letter parameters (_N_M, _M_IND_FP_N) the suffix rule
# cannot size. Measured beats derived, and the agreement is the cross-check on both.
SDK_OPS = {
    0x00: ("HALT",                  0),
    0x01: ("BRK",                   0),
    0x02: ("ADD",                   0),
    0x03: ("ADDF",                  0),
    0x04: ("ADD_4",                 0),
    0x05: ("ADD_1",                 0),
    0x06: ("ADD_MINUS1",            0),
    0x07: ("ADD_NN",                1),
    0x08: ("ADD_NNNN",              2),
    0x09: ("ADD_NNNNNNNN",          4),
    0x0A: ("ADDSP_4",               0),
    0x0B: ("ADDSP_NN",              1),
    0x0C: ("ADDSP",                 0),
    0x0D: ("ADDTO_FP_MINUS4_1",     0),
    0x0E: ("ADDTO_FP_MINUS8_1",     0),
    0x0F: ("ADDTO_FP_MINUS12_1",    0),
    0x10: ("ADDTO_FP_N_M",          1),
    0x11: ("ADDTO_DS_UUUU_1",       2),
    0x12: ("ADDTO_DS_UUUU_MINUS1",  2),
    0x13: ("AND",                   0),
    0x14: ("CALL",                  0),
    0x15: ("CALLR_NNNNNNNN",        4),
    0x16: ("CALLR_NNNN",            2),
    0x17: ("CALLR_NN",              1),
    0x18: ("COM",                   0),
    0x19: ("CTOI",                  0),
    0x1A: ("CTOIU",                 0),
    0x1B: ("DIV",                   0),
    0x1C: ("DIVU",                  0),
    0x1D: ("DIVF",                  0),
    0x1E: ("DUP",                   0),
    0x1F: ("FTOI",                  0),
    0x20: ("GET",                   0),
    0x21: ("GETS",                  0),
    0x22: ("GETC",                  0),
    0x23: ("ITOBOOL",               0),
    0x24: ("ITOF",                  0),
    0x25: ("ITOFU",                 0),
    0x26: ("JLT_NN",                1),
    0x27: ("JLTU_NN",               1),
    0x28: ("JLTF_NN",               1),
    0x29: ("JGT_NN",                1),
    0x2A: ("JGTU_NN",               1),
    0x2B: ("JGTF_NN",               1),
    0x2C: ("JLE_NN",                1),
    0x2D: ("JLEU_NN",               1),
    0x2E: ("JLEF_NN",               1),
    0x2F: ("JGE_NN",                1),
    0x30: ("JGEU_NN",               1),
    0x31: ("JGEF_NN",               1),
    0x32: ("JEQ_NN",                1),
    0x33: ("JNE_NN",                1),
    0x34: ("JZ_NN",                 1),
    0x35: ("JZF_NN",                1),
    0x36: ("JNZ_NN",                1),
    0x37: ("JNZF_NN",               1),
    0x38: ("JMP",                   0),
    0x39: ("JMPR_1",                0),
    0x3A: ("JMPR_2",                0),
    0x3B: ("JMPR_3",                0),
    0x3C: ("JMPR_4",                0),
    0x3D: ("JMPR_5",                0),
    0x3E: ("JMPR_6",                0),
    0x3F: ("JMPR_7",                0),
    0x40: ("JMPR_8",                0),
    0x41: ("JMPR_NN",               1),
    0x42: ("JMPR_NNNN",             2),
    0x43: ("JMPR_NNNNNNNN",         4),
    0x44: ("MOD",                   0),
    0x45: ("MODU",                  0),
    0x46: ("MUL",                   0),
    0x47: ("MULF",                  0),
    0x48: ("NEG",                   0),
    0x49: ("NEGF",                  0),
    0x4A: ("NOP",                   0),
    0x4B: ("NOT",                   0),
    0x4C: ("OR",                    0),
    0x4D: ("POP_IND_DS_UUUU",       2),
    0x4E: ("POPS_IND_DS_UUUU",      2),
    0x4F: ("POPC_IND_DS_UUUU",      2),
    0x50: ("POP_IND_FP_8",          0),
    0x51: ("POP_IND_FP_12",         0),
    0x52: ("POP_IND_FP_MINUS4",     0),
    0x53: ("POP_IND_FP_MINUS8",     0),
    0x54: ("POP_IND_FP_MINUS12",    0),
    0x55: ("POP_IND_FP_MINUS16",    0),
    0x56: ("POP_IND_FP_MINUS20",    0),
    0x57: ("POP_IND_FP_NN",         1),
    0x58: ("POPS_IND_FP_NN",        1),
    0x59: ("POPC_IND_FP_NN",        1),
    0x5A: ("POP_MM_IND_FP_8",       0),
    0x5B: ("POP_M_IND_FP_N",        1),
    0x5C: ("POPS_M_IND_FP_N",       0),
    0x5D: ("POPC_M_IND_FP_N",       1),
    0x5E: ("POP_MM_IND_FP_NN",      1),
    0x5F: ("POPS_MM_IND_FP_NN",     1),
    0x60: ("POPC_MM_IND_FP_NN",     1),
    0x61: ("POP_M_IND_SP_U",        0),
    0x62: ("POPS_M_IND_SP_U",       0),
    0x63: ("POPC_M_IND_SP_U",       0),
    0x64: ("POP_DS_UUUU",           2),
    0x65: ("POPS_DS_UUUU",          2),
    0x66: ("POPC_DS_UUUU",          2),
    0x67: ("POP_FP_NN",             1),
    0x68: ("POPS_FP_NN",            1),
    0x69: ("POPC_FP_NN",            1),
    0x6A: ("POP_FP_MINUS20",        0),
    0x6B: ("POP_FP_MINUS16",        0),
    0x6C: ("POP_FP_MINUS12",        0),
    0x6D: ("POP_FP_MINUS8",         0),
    0x6E: ("POP_FP_MINUS4",         0),
    0x6F: ("POP_FP_8",              0),
    0x70: ("POP_FP_12",             0),
    0x71: ("POP_SP_UU",             1),
    0x72: ("PUSH_NNNNNNNN",         4),
    0x73: ("PUSH_NNNN",             2),
    0x74: ("PUSH_NN",               1),
    0x75: ("PUSH_0",                0),
    0x76: ("PUSH_1",                0),
    0x77: ("PUSH_2",                0),
    0x78: ("PUSH_3",                0),
    0x79: ("PUSH_4",                0),
    0x7A: ("PUSH_MINUS1",           0),
    0x7B: ("PUSH_MINUS2",           0),
    0x7C: ("PUSH_MINUS3",           0),
    0x7D: ("PUSH_MINUS4",           0),
    0x7E: ("PUSHEA_PC_NN",          1),
    0x7F: ("PUSHEA_PC_NNNN",        2),
    0x80: ("PUSHEA_FP_NN",          1),
    0x81: ("PUSHEA_DS_UUUU",        2),
    0x82: ("PUSHEA_SP_UU",          1),
    0x83: ("PUSH_IND_DS_UUUU",      2),
    0x84: ("PUSHS_IND_DS_UUUU",     2),
    0x85: ("PUSHC_IND_DS_UUUU",     2),
    0x86: ("PUSH_IND_FP_8",         0),
    0x87: ("PUSH_IND_FP_12",        0),
    0x88: ("PUSH_IND_FP_MINUS4",    0),
    0x89: ("PUSH_IND_FP_MINUS8",    0),
    0x8A: ("PUSH_IND_FP_MINUS12",   0),
    0x8B: ("PUSH_IND_FP_MINUS16",   0),
    0x8C: ("PUSH_IND_FP_MINUS20",   0),
    0x8D: ("PUSH_IND_FP_NN",        1),
    0x8E: ("PUSHS_IND_FP_NN",       1),
    0x8F: ("PUSHC_IND_FP_NN",       1),
    0x90: ("PUSH_MM_IND_FP_8",      0),
    0x91: ("PUSH_M_IND_FP_N",       1),
    0x92: ("PUSHS_M_IND_FP_N",      1),
    0x93: ("PUSHC_M_IND_FP_N",      1),
    0x94: ("PUSH_MM_IND_FP_NN",     1),
    0x95: ("PUSHS_MM_IND_FP_NN",    1),
    0x96: ("PUSHC_MM_IND_FP_NN",    2),
    0x97: ("PUSH_M_IND_SP_U",       0),
    0x98: ("PUSHS_M_IND_SP_U",      0),
    0x99: ("PUSHC_M_IND_SP_U",      0),
    0x9A: ("PUSH_PC_NNNN",          2),
    0x9B: ("PUSH_DS_UUUU",          2),
    0x9C: ("PUSHS_DS_UUUU",         2),
    0x9D: ("PUSHC_DS_UUUU",         2),
    0x9E: ("PUSH_FP_NN",            1),
    0x9F: ("PUSHS_FP_NN",           1),
    0xA0: ("PUSHC_FP_NN",           1),
    0xA1: ("PUSH_FP_MINUS20",       0),
    0xA2: ("PUSH_FP_MINUS16",       0),
    0xA3: ("PUSH_FP_MINUS12",       0),
    0xA4: ("PUSH_FP_MINUS8",        0),
    0xA5: ("PUSH_FP_MINUS4",        0),
    0xA6: ("PUSH_FP_8",             0),
    0xA7: ("PUSH_FP_12",            0),
    0xA8: ("PUSH_FP_16",            0),
    0xA9: ("PUSH_SP_UU",            1),
    0xAA: ("PUSH_DS",               0),
    0xAB: ("PUSH_SP",               0),
    0xAC: ("PUSH_PC",               0),
    0xAD: ("PUSH_FP",               0),
    0xAE: ("POP_DS",                0),
    0xAF: ("POP_SP",                0),
    0xB0: ("POP_FP",                0),
    0xB1: ("PUT",                   0),
    0xB2: ("PUTS",                  0),
    0xB3: ("PUTC",                  0),
    0xB4: ("RET_0_0",               0),
    0xB5: ("RET_0_4",               0),
    0xB6: ("RET_0_8",               0),
    0xB7: ("RET_0_NN",              1),
    0xB8: ("RET_4_0",               0),
    0xB9: ("RET_4_4",               0),
    0xBA: ("RET_4_8",               0),
    0xBB: ("RET_4_NN",              1),
    0xBC: ("SHIFT",                 0),
    0xBD: ("SHIFTU",                0),
    0xBE: ("SHIFT_NN",              1),
    0xBF: ("SHIFTU_NN",             1),
    0xC0: ("SHIFT_1",               0),
    0xC1: ("SHIFT_2",               0),
    0xC2: ("STOI",                  0),
    0xC3: ("STOIU",                 0),
    0xC4: ("SUB",                   0),
    0xC5: ("SUBF",                  0),
    0xC6: ("SWAP",                  0),
    0xC7: ("SWITCH",                0),
    0xC8: ("SCALL_0_UU",            1),
    0xC9: ("SCALL_0_UUUU",          2),
    0xCA: ("SCALL_UU_UU",           2),
    0xCB: ("SCALL",                 0),
    0xCC: ("VCALL_NN",              1),
    0xCD: ("VCALLO_NN",             1),
    0xCE: ("VCALLX_NN",             1),
    0xCF: ("VERSION",               0),
    0xD0: ("XOR",                   0),
}


MNEMONIC = dict((op, name.lower()) for op, (name, _len) in SDK_OPS.items())


class Image:
    def __init__(self, path):
        with open(path, 'rb') as fh:
            self.data = fh.read()

    def byte(self, va):
        return self.data[va - FLASH_BASE]

    def slice(self, va, n):
        return self.data[va - FLASH_BASE:va - FLASH_BASE + n]


def read_trace(path):
    """The trace as (icount, pc, address, size) in execution order."""
    rows = []
    with open(path) as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            pc, at, size, icount = line.split(',')
            rows.append((int(icount), pc, int(at, 16), int(size)))
    if not rows:
        sys.exit('%s holds no trace rows -- nothing can be concluded from it' % path)
    rows.sort()
    return rows


def steps(rows):
    """One entry per executed instruction: (address, operand bytes read, next address)."""
    out, i = [], 0
    while i < len(rows):
        if rows[i][1] != MAIN_FETCH_SITE:
            i += 1
            continue
        addr, j, nbytes = rows[i][2], i + 1, 0
        while j < len(rows) and rows[j][1] != MAIN_FETCH_SITE:
            nbytes += rows[j][3]
            j += 1
        if j < len(rows):
            out.append((addr, nbytes, rows[j][2]))
        i = j
    return out


def operand_table(img, traces):
    """Measured operand lengths, merged across traces, overridden by the solved flow forms.

    EACH TRACE IS LEARNED SEPARATELY AND THE RESULTS MERGED, never concatenated. steps() reads
    adjacency -- which opcode followed which -- and two runs whose instruction counts overlap
    would manufacture adjacencies that never happened, an opcode "followed by" a read from the
    other run. Learning per trace makes that impossible, and a disagreement BETWEEN traces is
    then a real finding rather than an artefact, so it is reported rather than averaged away."""
    if not isinstance(traces, list):
        traces = [traces]
    lengths = collections.defaultdict(collections.Counter)
    for rows in traces:
        for addr, nbytes, nxt in steps(rows):
            if nxt == addr + 1 + nbytes:        # sequential: the byte count IS the operand count
                lengths[img.byte(addr)][nbytes] += 1
    # The SDK's table is the FLOOR and measurement is the ceiling: every opcode gets a length
    # from the vendor's own definitions, and any opcode this machine has actually been watched
    # executing overrides it. That ordering matters -- the SDK is version 3.2 and this runtime is
    # 1.2S4Bj, so where they differ the running machine is the authority, and where they agree
    # (95 of the 102 opcodes observed) each is a check on the other.
    table = {op: n for op, (_name, n) in SDK_OPS.items()}
    table.update({op: c.most_common(1)[0][0] for op, c in lengths.items()})
    ambiguous = {op: dict(c) for op, c in lengths.items() if len(c) > 1 and op not in FLOW}
    table.update(FLOW)
    return table, ambiguous


def target(img, va, op, n):
    """Branch or call target for a control-flow opcode, else None."""
    nxt = va + 1 + n
    if op in CALL_REL4:
        return (nxt + struct.unpack('>i', img.slice(va + 1, 4))[0]) & 0xFFFFFFFF
    if op in JUMP_REL2:
        return nxt + struct.unpack('>h', img.slice(va + 1, 2))[0]
    if op in CALL_REL2:
        return nxt + struct.unpack('>h', img.slice(va + 1, 2))[0]
    if op in CALL_REL1 | JUMP_REL1 | BRANCH_REL1:
        b = img.byte(va + 1)
        return nxt + (b - 256 if b > 127 else b)
    return None


def disassemble(img, lo, hi, table, executed):
    lines, va = [], lo
    while va < hi:
        op = img.byte(va)
        n = table.get(op)
        if n is None:
            lines.append('  %08x  %02x  ??? operand length for this opcode was never observed' % (va, op))
            break
        raw = ' '.join('%02x' % b for b in img.slice(va + 1, n))
        tgt = target(img, va, op, n)
        hits = executed.get(va, 0)
        lines.append('%5s  %08x  %02x %-11s %-5s %s' % (
            ('x%d' % hits) if hits else '.',
            va, op, raw,
            MNEMONIC.get(op, 'op_%02x' % op),
            '0x%08x' % tgt if tgt is not None else ''))
        va += 1 + n
    return lines


def check(img, lo, hi, table, executed):
    """Validate the decoder against the machine.

    THE SOUND TEST IS "DID THE WALK HIT EVERY ADDRESS THE MACHINE FETCHED", NOT "WAS EVERY
    BOUNDARY PLAUSIBLE". The first version of this function asked the second question -- every
    decoded boundary had to be either fetched or reachable from a decoded instruction -- and it
    stayed GREEN when an operand length was deliberately corrupted, because `reachable` was
    computed from the drifted walk itself and each bad boundary whitewashed the next. Almost
    every opcode falls through, so almost every address was reachable, and the check could not
    fail. Kept as a comment because the broken version looked more thorough than this one.

    An operand length that is too long makes the walk STEP OVER an address the machine really
    fetched, and no amount of self-consistency can hide that: the trace is independent evidence.
    So the three answers are:

      missed    -- the machine fetched an opcode here and the walk stepped over it. The decode
                   is WRONG, and every line after the first one is fiction.
      unvisited -- decoded boundaries the trace never fetched. Expected: real code on a branch
                   this run did not take. Reported, never failed on.
      unknown   -- an opcode whose operand length was never observed. A coverage gap, not a
                   defect; it stops the walk without condemning what came before."""
    boundaries, va, unknown = [], lo, None
    while va < hi:
        op = img.byte(va)
        n = table.get(op)
        if n is None:
            unknown = (va, op)
            break
        boundaries.append(va)
        va += 1 + n
    stopped = va                                 # nothing beyond here was decoded at all
    seen = set(boundaries)
    missed = sorted(a for a in executed if lo <= a < stopped and a not in seen)
    unvisited = [b for b in boundaries if b not in executed]
    return boundaries, missed, unvisited, unknown


def handlers(ram_path):
    """opcode -> handler address, read from the interpreter's own dispatch table."""
    with open(ram_path, 'rb') as fh:
        ram = fh.read()
    out = {}
    for op in range(N_OPCODES):
        off = DISPATCH + op * 2 - RAM_BASE
        out[op] = (DISPATCH + struct.unpack('>h', ram[off:off + 2])[0]) & 0xFFFFFFFF
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('start', nargs='?', help='first address, e.g. 0x9FC4A538')
    ap.add_argument('end', nargs='?', help='one past the last address')
    ap.add_argument('--trace', required=True, action='append',
                    help='CSV of pc,address,size,icount from bytecode-trace.js; repeatable, and '
                         'more traces mean more opcodes covered')
    ap.add_argument('--flash', default=FLASH_DEFAULT)
    ap.add_argument('--check', action='store_true', help='validate the decoder against the trace and exit')
    ap.add_argument('--table', action='store_true', help='print the measured operand table')
    ap.add_argument('--handlers', metavar='RAM_IMAGE', help='also print each opcode\'s handler address')
    args = ap.parse_args()

    img = Image(args.flash)
    traces = [read_trace(t) for t in args.trace]
    table, ambiguous = operand_table(img, traces)
    # steps() drops the final fetch (it has no successor to measure against), so add it back --
    # from the last MAIN-SITE row, not the last row of the trace, which is an operand byte.
    executed = collections.Counter()
    for rows in traces:
        executed.update(a for a, _n, _x in steps(rows))
        last = [r[2] for r in rows if r[1] == MAIN_FETCH_SITE]
        if last:
            executed[last[-1]] += 1

    if ambiguous:
        print('AMBIGUOUS -- these opcodes were seen with more than one operand length, so the')
        print('table cannot be trusted for them and the listing may drift:')
        for op, counts in sorted(ambiguous.items()):
            print('  0x%02x  %s' % (op, counts))
        print()

    if args.table:
        hmap = handlers(args.handlers) if args.handlers else {}
        print('%-6s %-8s %-9s %s' % ('opcode', 'operands', 'kind', 'handler' if hmap else ''))
        for op in sorted(table):
            print('  0x%02x   %-8d %-9s %s' % (
                op, table[op], MNEMONIC.get(op, ''),
                '0x%08x' % hmap[op] if op in hmap else ''))
        print('\n%d of %d opcodes observed; %d distinct addresses executed across %d trace(s)'
              % (len(table), N_OPCODES, len(executed), len(traces)))
        return 0

    if not args.start or not args.end:
        ap.error('start and end are required unless --table is given')
    lo, hi = int(args.start, 0), int(args.end, 0)

    boundaries, missed, unvisited, unknown = check(img, lo, hi, table, executed)
    if args.check:
        print('%d instruction boundaries decoded over 0x%08X-0x%08X' % (len(boundaries), lo, hi - 1))
        print('  %d were addresses the machine actually fetched' % (len(boundaries) - len(unvisited)))
        print('  %d were not fetched -- code on a branch this trace never took' % len(unvisited))
        if missed:
            print('  WRONG -- the walk stepped over %d address(es) the machine DID fetch:' % len(missed))
            for a in missed[:12]:
                print('    %08x' % a)
            print('  An operand length is wrong. The listing is fiction from the first one onward.')
        else:
            print('  correct: the walk landed on every address the machine fetched in this range')
        if unknown:
            print('  COVERAGE GAP -- stopped at %08x, opcode 0x%02x, whose operand length this'
                  % (unknown[0], unknown[1]))
            print('  trace never showed. Capture a trace that executes it, or read its handler.')
        return 1 if (missed or unknown) else 0

    for line in disassemble(img, lo, hi, table, executed):
        print(line)
    if missed:
        print('\nWARNING: the walk stepped over %d fetched address(es) -- this listing is WRONG.'
              % len(missed))
    return 0


if __name__ == '__main__':
    sys.exit(main())
