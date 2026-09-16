/*
 * MIPS16 / MIPS16e decoder and executor.  ES5, self-contained, no imports.
 *
 * This is a port of mips16.py, which was verified differentially against
 * binutils 2.42 `mips-linux-gnu-objdump -m mips:16`.  The Python is the
 * oracle: every encoding, every immediate scaling and every operand spelling
 * here is the Python's, not a re-derivation.  Its citations are kept where
 * they explain a field layout; the full provenance is in the Python's
 * docstring and README.
 *
 *   MIPS32(R) Architecture for Programmers Volume IV-a: The MIPS16e(TM)
 *   Application-Specific Extension to the MIPS32(R) Architecture,
 *   Document MD00076, Revision 2.63, July 16 2013.
 *
 * Public surface:
 *
 *   M16.decode(hw, extend, nextHw, addr, pcBase [, isa])
 *       -> {op, operands, length, target, targetIsa, reason, ...}
 *   M16.step(cpu, mem)
 *       -> {ok:true, length, ...} | {ok:false, reason, ...}
 *
 *   cpu = {pc, isa, reg:[32], hi, lo}          (isa is 16 or 32)
 *   mem = {read8,read16,read32,write8,write16,write32}   (big-endian)
 *
 * Also ported from the Python, because they are part of it: decodeStrict()
 * (the raising form), disassemble() (the linear sweep that tracks delay
 * slots), text() (Insn.text), XLAT and REG_NAMES.
 */
var M16 = (function () {
'use strict';

/* ------------------------------------------------------------------ */
/* Registers                                                          */
/* ------------------------------------------------------------------ */

/* [MD00076 Figure 4-1] Xlat: 0b000 -> GPR16, 0b001 -> GPR17, else identity. */
var XLAT = [16, 17, 2, 3, 4, 5, 6, 7];

var REG_NAMES = [
    'zero', 'at', 'v0', 'v1', 'a0', 'a1', 'a2', 'a3',
    't0', 't1', 't2', 't3', 't4', 't5', 't6', 't7',
    's0', 's1', 's2', 's3', 's4', 's5', 's6', 's7',
    't8', 't9', 'k0', 'k1', 'gp', 'sp', 's8', 'ra'
];

var GPR_T = 24;      /* the implicit condition register, "T" */
var GPR_SP = 29;
var GPR_RA = 31;

function xl(f3) { return XLAT[f3 & 7]; }
function rn(f3) { return REG_NAMES[XLAT[f3 & 7]]; }
function rn32(n) { return REG_NAMES[n & 31]; }

/* ------------------------------------------------------------------ */
/* Small helpers                                                      */
/* ------------------------------------------------------------------ */

function hexs(n) {
    /* Python _hex(): "-0x%x" % -n for negatives. */
    if (n < 0) { return '-0x' + (-n).toString(16); }
    return '0x' + n.toString(16);
}

function uhex(n) { return '0x' + (n >>> 0).toString(16); }

function sx(value, bits) {
    /* Python _sx(); bits is always <= 16 here. */
    var sign = 1 << (bits - 1);
    return (value & (sign - 1)) - (value & sign);
}

function isNil(v) { return v === null || v === undefined; }

/* ------------------------------------------------------------------ */
/* Result type                                                        */
/* ------------------------------------------------------------------ */

function Insn(op, operands, length, opts) {
    opts = opts || {};
    this.op = op;
    this.operands = operands || [];
    this.length = length;
    this.target = isNil(opts.target) ? null : opts.target;      /* byte address, no ISA bit */
    this.targetIsa = isNil(opts.targetIsa) ? null : opts.targetIsa;
    this.needsNext = !!opts.needsNext;
    this.reason = isNil(opts.reason) ? null : opts.reason;
    this.isa64 = !!opts.isa64;      /* encoding is legal only on a 64-bit core */
    this.legacy = !!opts.legacy;    /* pre-MIPS16e (ENTRY/EXIT) encoding */
    this.notext = !!opts.notext;    /* UNKNOWN only because EXTEND is illegal here */
    this.x = isNil(opts.x) ? null : opts.x;   /* execution payload, see step() */
}

Insn.prototype.text = function () {
    if (!this.operands.length) { return this.op; }
    var m = this.op;
    while (m.length < 9) { m += ' '; }
    return m + ' ' + this.operands.join(',');
};

function unk(reason, length, notext) {
    return new Insn('UNKNOWN', [], length === undefined ? 2 : length,
                    {reason: reason, notext: !!notext});
}

function notextInsn(what) {
    /* EXTEND on a non-extensible instruction: Reserved Instruction Exception.
       [MD00076 3.11 and the 'not extensible' marker in Tables 3.18/3.22/3.24] */
    return unk(what + ' is not extensible; EXTEND prefix is illegal', 4, true);
}

/* ------------------------------------------------------------------ */
/* Immediate assembly for EXTEND-prefixed instructions                */
/* ------------------------------------------------------------------ */

function extImm16(ext, insn) {
    /* [MD00076 3.14.13, 3.14.15, 3.14.16, 3.14.19]
         bits 26..21 = imm[10:5] -> ext[10:5]
         bits 20..16 = imm[15:11] -> ext[4:0]
         bits  4..0  = imm[4:0]   -> insn[4:0] */
    return (((ext & 0x1F) << 11)
            | (((ext >> 5) & 0x3F) << 5)
            | (insn & 0x1F));
}

function extImm15Rria(ext, insn) {
    /* [MD00076 3.14.17]
         bits 26..20 = imm[10:4] -> ext[10:4]
         bits 19..16 = imm[14:11] -> ext[3:0]
         bits  3..0  = imm[3:0]  -> insn[3:0] */
    return (((ext & 0xF) << 11)
            | (((ext >> 4) & 0x7F) << 4)
            | (insn & 0xF));
}

/* ------------------------------------------------------------------ */
/* Sub-tables                                                         */
/* ------------------------------------------------------------------ */

/* [MD00076 Table 3.24] RR funct, bits 4..0.  kind:
     'a2' two-operand ALU "OP rx, ry" · 'sv' shift-variable "OP ry, rx"
     'r1' one-operand "OP rx" (ry must be 0) · 'd' sub-table
   Entries flagged 64 are the beta ("valid at a higher-order MIPS ISA level")
   encodings: Reserved Instruction on a MIPS32 core. */
var RR_FUNCT = {};
RR_FUNCT[0x00] = ['J(AL)R(C)', 'd', false];
RR_FUNCT[0x01] = ['sdbbp', 'code', false];
RR_FUNCT[0x02] = ['slt', 'a2', false];
RR_FUNCT[0x03] = ['sltu', 'a2', false];
RR_FUNCT[0x04] = ['sllv', 'sv', false];
RR_FUNCT[0x05] = ['break', 'code', false];
RR_FUNCT[0x06] = ['srlv', 'sv', false];
RR_FUNCT[0x07] = ['srav', 'sv', false];
RR_FUNCT[0x08] = ['dsrl', 'sh64', true];
RR_FUNCT[0x09] = [null, null, false];      /* '*' Reserved in MIPS16e (ENTRY/EXIT) */
RR_FUNCT[0x0A] = ['cmp', 'a2', false];
RR_FUNCT[0x0B] = ['neg', 'a2', false];
RR_FUNCT[0x0C] = ['and', 'a2', false];
RR_FUNCT[0x0D] = ['or', 'a2', false];
RR_FUNCT[0x0E] = ['xor', 'a2', false];
RR_FUNCT[0x0F] = ['not', 'a2', false];
RR_FUNCT[0x10] = ['mfhi', 'r1', false];
RR_FUNCT[0x11] = ['CNVT', 'd', false];
RR_FUNCT[0x12] = ['mflo', 'r1', false];
RR_FUNCT[0x13] = ['dsra', 'sh64', true];
RR_FUNCT[0x14] = ['dsllv', 'sv', true];
RR_FUNCT[0x15] = [null, null, false];      /* '*' Reserved */
RR_FUNCT[0x16] = ['dsrlv', 'sv', true];
RR_FUNCT[0x17] = ['dsrav', 'sv', true];
RR_FUNCT[0x18] = ['mult', 'a2', false];
RR_FUNCT[0x19] = ['multu', 'a2', false];
RR_FUNCT[0x1A] = ['div', 'dv', false];
RR_FUNCT[0x1B] = ['divu', 'dv', false];
RR_FUNCT[0x1C] = ['dmult', 'a2', true];
RR_FUNCT[0x1D] = ['dmultu', 'a2', true];
RR_FUNCT[0x1E] = ['ddiv', 'dv', true];
RR_FUNCT[0x1F] = ['ddivu', 'dv', true];

/* [MD00076 Table 3.26] ry field when funct = J(AL)R(C). */
var JALRC = {0: 'JR_RX', 1: 'JR_RA', 2: 'JALR', 4: 'JRC_RX', 5: 'JRC_RA', 6: 'JALRC'};
var JALRC_NAME = {JR_RX: 'jr', JR_RA: 'jr', JALR: 'jalr',
                  JRC_RX: 'jrc', JRC_RA: 'jrc', JALRC: 'jalrc'};

/* [MD00076 Table 3.27] ry field when funct = CNVT.  2 and 6 are beta. */
var CNVT = {0: ['zeb', false], 1: ['zeh', false], 2: ['zew', true],
            4: ['seb', false], 5: ['seh', false], 6: ['sew', true]};

/* [MD00076 Table 3.22] I8 funct, bits 10..8. */
var I8 = {0: 'BTEQZ', 1: 'BTNEZ', 2: 'SWRASP', 3: 'ADJSP',
          4: 'SVRS', 5: 'MOV32R', 7: 'MOVR32'};

/* [MD00076 SAVE page] aregs -> [argument registers saved, static registers]. */
var AREGS = {};
AREGS[0x0] = [0, []];                AREGS[0x1] = [0, ['a3']];
AREGS[0x2] = [0, ['a2', 'a3']];      AREGS[0x3] = [0, ['a1', 'a2', 'a3']];
AREGS[0x4] = [1, []];                AREGS[0x5] = [1, ['a3']];
AREGS[0x6] = [1, ['a2', 'a3']];      AREGS[0x7] = [1, ['a1', 'a2', 'a3']];
AREGS[0x8] = [2, []];                AREGS[0x9] = [2, ['a3']];
AREGS[0xA] = [2, ['a2', 'a3']];      AREGS[0xB] = [0, ['a0', 'a1', 'a2', 'a3']];
AREGS[0xC] = [3, []];                AREGS[0xD] = [3, ['a3']];
AREGS[0xE] = [4, []];
/* 0b1111 is Reserved -> UNPREDICTABLE */

/* xsregs -> highest GPR of the GPR[18-23,30] set saved/restored. */
var XSREGS = [null, 's2', 's3', 's4', 's5', 's6', 's7', 's8'];

var SREG_ORDER = ['s0', 's1', 's2', 's3', 's4', 's5', 's6', 's7', 's8'];

function svrs(ext, insn) {
    /* SAVE / RESTORE.  [MD00076 3.14.11, 3.14.20, Table 3.25] */
    var s = (insn >> 7) & 1;
    var ra = (insn >> 6) & 1;
    var s0 = (insn >> 5) & 1;
    var s1 = (insn >> 4) & 1;
    var fsLo = insn & 0xF;
    var name = s ? 'save' : 'restore';
    var framesize, xsregs, aregs, fsHi;

    if (ext === null) {
        /* 4-bit framesize; a value of 0 means 128 bytes. */
        framesize = fsLo ? (fsLo * 8) : 128;
        xsregs = 0;
        aregs = 0;
    } else {
        /* [MD00076 3.14.20 EXT-I8_SVRS]
             bits 26..24 xsregs, 23..20 framesize[7:4], 19..16 aregs */
        xsregs = (ext >> 8) & 7;
        fsHi = (ext >> 4) & 0xF;
        aregs = ext & 0xF;
        framesize = ((fsHi << 4) | fsLo) * 8;
        if (aregs === 0xF) {
            return unk('SAVE/RESTORE aregs=0b1111 is Reserved', 4);
        }
    }

    var got = AREGS[aregs];
    var args = got[0], statics = got[1];

    function rng(regs) {
        return regs.length === 1 ? regs[0] : (regs[0] + '-' + regs[regs.length - 1]);
    }
    function runs(set, order) {
        var out = [], cur = [], i;
        for (i = 0; i < order.length; i++) {
            if (set[order[i]]) { cur.push(order[i]); }
            else if (cur.length) { out.push(rng(cur)); cur = []; }
        }
        if (cur.length) { out.push(rng(cur)); }
        return out;
    }

    /* Operand order follows binutils' printed form. */
    var ops = [], i, argnames = [];
    if (args) {
        for (i = 0; i < args; i++) { argnames.push('a' + i); }
        ops.push(rng(argnames));
    }
    ops.push(String(framesize));
    if (ra) { ops.push('ra'); }
    var saved = {};
    if (s0) { saved.s0 = true; }
    if (s1) { saved.s1 = true; }
    for (i = 1; i <= xsregs; i++) { saved[XSREGS[i]] = true; }
    ops = ops.concat(runs(saved, SREG_ORDER));
    if (statics.length) { ops.push(rng(statics)); }

    return new Insn(name, ops, ext === null ? 2 : 4, {
        x: {k: name, framesize: framesize, ra: ra, s0: s0, s1: s1,
            xsregs: xsregs, aregs: aregs}
    });
}

/* ------------------------------------------------------------------ */
/* The decoder                                                        */
/* ------------------------------------------------------------------ */

/* Loads / stores with a 5-bit scaled offset: op -> [name, shift, isa64]. */
var MEM_OPS = {};
MEM_OPS[0x10] = ['lb', 0, false];
MEM_OPS[0x11] = ['lh', 1, false];
MEM_OPS[0x13] = ['lw', 2, false];
MEM_OPS[0x14] = ['lbu', 0, false];
MEM_OPS[0x15] = ['lhu', 1, false];
MEM_OPS[0x17] = ['lwu', 2, true];
MEM_OPS[0x18] = ['sb', 0, false];
MEM_OPS[0x19] = ['sh', 1, false];
MEM_OPS[0x1B] = ['sw', 2, false];

var SHIFT_NAMES = {0: 'sll', 1: 'dsll', 2: 'srl', 3: 'sra'};
var RRR_NAMES = {0: 'daddu', 1: 'addu', 2: 'dsubu', 3: 'subu'};

/**
 * Decode one MIPS16e instruction.
 *
 * hw      -- the 16-bit instruction (the caller has done the big-endian fetch)
 * extend  -- the preceding EXTEND halfword (0xF000..0xF7FF), or null
 * nextHw  -- the following halfword; only JAL and JALX need it
 * addr    -- address of this instruction (of the EXTEND halfword, when the
 *            instruction is extended), or null
 * pcBase  -- address the PC-relative forms (ADDIUPC/"la", LWPC) must use as
 *            their base.  [MD00076 ADDIUPC] says the immediate is added to the
 *            address of the ADDIU instruction OR the address of the jump in
 *            whose delay slot it is executed -- so a caller stepping through
 *            a delay slot must pass the JUMP's address here.  null = use addr.
 * isa     -- 32 (default) decodes MD00076 exactly and returns UNKNOWN for the
 *            beta encodings; 64 additionally decodes MIPS16e64.
 */
function decode(hw, extend, nextHw, addr, pcBase, isa) {
    if (isNil(extend)) { extend = null; }
    if (isNil(nextHw)) { nextHw = null; }
    if (isNil(addr)) { addr = null; }
    if (isNil(pcBase)) { pcBase = null; }
    if (isNil(isa)) { isa = 32; }

    var insn = hw & 0xFFFF;
    if (extend !== null) {
        extend &= 0xFFFF;
        if ((extend >> 11) !== 0x1E) {
            throw new Error('extend=0x' + extend.toString(16)
                            + ' is not an EXTEND halfword');
        }
        extend &= 0x07FF;          /* bits 26..16 of the 32-bit word */
    }
    if (isa !== 32 && isa !== 64) { throw new Error('isa must be 32 or 64'); }

    var ext = extend;
    var op = (insn >> 11) & 0x1F;      /* [MD00076 Table 3.18] */
    var rx = (insn >> 8) & 7;
    var ry = (insn >> 5) & 7;
    var n = (ext === null) ? 2 : 4;
    var pcb = (pcBase === null) ? addr : pcBase;

    function gate(ins) {
        /* beta encodings are Reserved Instruction on a MIPS32 core. */
        if (ins.isa64 && isa === 32) {
            return unk(ins.op + ' is a MIPS64-only (beta) encoding; Reserved on '
                       + 'MIPS32', ins.length);
        }
        return ins;
    }

    function zfield(mask, what) {
        /* The EXT-* formats pin some instruction-half bits to zero.  A nonzero
           value there is not a MIPS16e encoding -- it is the space the later
           MIPS16e2 ASE (2019) reuses, which a 2002 image cannot contain. */
        if (ext !== null && (insn & mask)) {
            return unk('extended ' + what + ' with non-zero constant field '
                       + '(MD00076 requires it zero; this is MIPS16e2 space)',
                       4, true);
        }
        return null;
    }

    /* Finish a PC-relative branch: target = PC + n + offset.
       Returns [printedText, targetOrNull]. */
    function br(off) {
        if (addr === null) { return [hexs(off), null]; }
        var t = (addr + n + off) >>> 0;
        return [uhex(t | 1), t];       /* | 1: ISA bit, as objdump prints */
    }

    var bad, imm, off, txt, t, name, f, sa, ops, funct, kind, code, got, sel;

    /* ---- 00000 ADDIUSP : ADDIU rx, sp, immediate --------------------- */
    if (op === 0x00) {
        bad = zfield(0xE0, 'ADDIUSP');
        if (bad) { return bad; }
        imm = (ext === null) ? ((insn & 0xFF) << 2) : sx(extImm16(ext, insn), 16);
        return new Insn('addiu', [rn(rx), 'sp', hexs(imm)], n,
                        {x: {k: 'addiusp', d: xl(rx), imm: imm}});
    }

    /* ---- 00001 ADDIUPC : ADDIU rx, pc, immediate  (printed "la") ------ */
    if (op === 0x01) {
        bad = zfield(0xE0, 'ADDIUPC');
        if (bad) { return bad; }
        imm = (ext === null) ? ((insn & 0xFF) << 2) : sx(extImm16(ext, insn), 16);
        if (pcb === null) {
            return new Insn('addiu', [rn(rx), 'pc', hexs(imm)], n,
                            {x: {k: 'addiupc', d: xl(rx), imm: imm}});
        }
        t = (((pcb & ~3) >>> 0) + imm) >>> 0;
        return new Insn('la', [rn(rx), uhex(t)], n,
                        {target: t, x: {k: 'addiupc', d: xl(rx), imm: imm, val: t}});
    }

    /* ---- 00010 B offset ---------------------------------------------- */
    if (op === 0x02) {
        if (ext === null) {
            off = sx(insn & 0x7FF, 11) << 1;
        } else {
            bad = zfield(0x7E0, 'B');
            if (bad) { return bad; }
            off = sx(extImm16(ext, insn), 16) << 1;
        }
        txt = br(off);
        return new Insn('b', [txt[0]], n,
                        {target: txt[1], targetIsa: 16,
                         x: {k: 'b', off: off}});
    }

    /* ---- 00011 JAL / JALX -------------------------------------------- */
    if (op === 0x03) {
        if (ext !== null) { return notextInsn('JAL/JALX'); }
        var x = (insn >> 10) & 1;                  /* [MD00076 Table 3.19] */
        name = x ? 'jalx' : 'jal';
        if (nextHw === null) {
            return new Insn(name, ['<second halfword required>'], 4,
                            {needsNext: true});
        }
        /* [MD00076 3.14.12] imm[20:16] in bits 9..5, imm[25:21] in bits 4..0 */
        var idx = (((insn & 0x1F) << 21)
                   | (((insn >> 5) & 0x1F) << 16)
                   | (nextHw & 0xFFFF));
        if (addr === null) {
            var s = idx.toString(16);
            while (s.length < 7) { s = '0' + s; }
            return new Insn(name, ['0x' + s], 4, {targetIsa: x ? 32 : 16});
        }
        /* [MD00076 JAL/JALX Operation] the DELAY SLOT's PC supplies bits 31..28 */
        t = ((((addr + 4) & 0xF0000000) | (idx << 2)) >>> 0);
        var shown = x ? t : (t | 1);               /* ISA bit as objdump prints */
        return new Insn(name, [uhex(shown)], 4,
                        {target: t, targetIsa: x ? 32 : 16,
                         x: {k: x ? 'jalx' : 'jal', tgt: t}});
    }

    /* ---- 00100 BEQZ / 00101 BNEZ ------------------------------------- */
    if (op === 0x04 || op === 0x05) {
        name = (op === 0x04) ? 'beqz' : 'bnez';
        if (ext === null) {
            off = sx(insn & 0xFF, 8) << 1;
        } else {
            bad = zfield(0xE0, name.toUpperCase());
            if (bad) { return bad; }
            off = sx(extImm16(ext, insn), 16) << 1;
        }
        txt = br(off);
        return new Insn(name, [rn(rx), txt[0]], n,
                        {target: txt[1], targetIsa: 16,
                         x: {k: name, s: xl(rx), off: off}});
    }

    /* ---- 00110 SHIFT -------------------------------------------------- */
    if (op === 0x06) {
        f = insn & 3;                              /* [MD00076 Table 3.20] */
        name = SHIFT_NAMES[f];
        var sIsa64 = (f === 1);                    /* beta: MIPS64 only */
        if (ext === null) {
            sa = (insn >> 2) & 7;
            if (sa === 0) { sa = 8; }              /* [MD00076 3.14.7 note] */
        } else {
            /* [MD00076 3.14.18 EXT-SHIFT] sa[4:0] = bits 26..22 = ext[10:6],
               s5 = bit 21 = ext[5]; s5 may only be set for DSLL. */
            if ((insn >> 2) & 7) {
                return unk('extended SHIFT with non-zero sa field', 4);
            }
            if (ext & 0x1F) {
                return unk('extended SHIFT with non-zero bits 20..16', 4, true);
            }
            sa = (ext >> 6) & 0x1F;
            if ((ext >> 5) & 1) {
                if (f !== 1) {
                    return unk('extended SHIFT with s5 set on a 32-bit shift', 4);
                }
                sa += 32;
            }
        }
        ops = (rx === ry) ? [rn(rx), String(sa)] : [rn(rx), rn(ry), String(sa)];
        return gate(new Insn(name, ops, n,
                             {isa64: sIsa64,
                              x: {k: name, d: xl(rx), s: xl(ry), sa: sa}}));
    }

    /* ---- 00111 LD (beta, MIPS64) ------------------------------------- */
    if (op === 0x07) {
        off = (ext === null) ? ((insn & 0x1F) << 3) : sx(extImm16(ext, insn), 16);
        return gate(new Insn('ld', [rn(ry), hexs(off) + '(' + rn(rx) + ')'], n,
                             {isa64: true}));
    }

    /* ---- 01000 RRI-A -------------------------------------------------- */
    if (op === 0x08) {
        f = (insn >> 4) & 1;                       /* [MD00076 Table 3.21] */
        name = f ? 'daddiu' : 'addiu';             /* f=1 is beta (MIPS64) */
        imm = (ext === null) ? sx(insn & 0xF, 4)
                             : sx(extImm15Rria(ext, insn), 15);
        return gate(new Insn(name, [rn(ry), rn(rx), hexs(imm)], n,
                             {isa64: !!f,
                              x: {k: 'addiu_rri', d: xl(ry), s: xl(rx), imm: imm}}));
    }

    /* ---- 01001 ADDIU8 : ADDIU rx, immediate --------------------------- */
    if (op === 0x09) {
        bad = zfield(0xE0, 'ADDIU8');
        if (bad) { return bad; }
        imm = (ext === null) ? sx(insn & 0xFF, 8) : sx(extImm16(ext, insn), 16);
        return new Insn('addiu', [rn(rx), hexs(imm)], n,
                        {x: {k: 'addiu8', d: xl(rx), imm: imm}});
    }

    /* ---- 01010 SLTI / 01011 SLTIU ------------------------------------- */
    if (op === 0x0A || op === 0x0B) {
        name = (op === 0x0A) ? 'slti' : 'sltiu';
        bad = zfield(0xE0, name.toUpperCase());
        if (bad) { return bad; }
        imm = (ext === null) ? (insn & 0xFF) : sx(extImm16(ext, insn), 16);
        return new Insn(name, [rn(rx), hexs(imm)], n,
                        {x: {k: name, s: xl(rx), imm: imm}});
    }

    /* ---- 01100 I8 ----------------------------------------------------- */
    if (op === 0x0C) {
        funct = (insn >> 8) & 7;                   /* [MD00076 Table 3.22] */
        kind = I8[funct];
        if (kind === undefined) {
            return unk('I8 funct=' + funct + ' is Reserved', n);
        }

        if (kind === 'BTEQZ' || kind === 'BTNEZ') {
            name = kind.toLowerCase();
            if (ext === null) {
                off = sx(insn & 0xFF, 8) << 1;
            } else {
                bad = zfield(0xE0, kind);
                if (bad) { return bad; }
                off = sx(extImm16(ext, insn), 16) << 1;
            }
            txt = br(off);
            return new Insn(name, [txt[0]], n,
                            {target: txt[1], targetIsa: 16,
                             x: {k: name, off: off}});
        }

        if (kind === 'SWRASP') {                   /* SW ra, offset(sp) */
            bad = zfield(0xE0, 'SWRASP');
            if (bad) { return bad; }
            off = (ext === null) ? ((insn & 0xFF) << 2) : sx(extImm16(ext, insn), 16);
            return new Insn('sw', ['ra', hexs(off) + '(sp)'], n,
                            {x: {k: 'swrasp', off: off}});
        }

        if (kind === 'ADJSP') {                    /* ADDIU sp, immediate */
            bad = zfield(0xE0, 'ADJSP');
            if (bad) { return bad; }
            imm = (ext === null) ? (sx(insn & 0xFF, 8) << 3)
                                 : sx(extImm16(ext, insn), 16);
            return new Insn('addiu', ['sp', hexs(imm)], n,
                            {x: {k: 'adjsp', imm: imm}});
        }

        if (kind === 'SVRS') { return svrs(ext, insn); }

        if (ext !== null) { return notextInsn('I8 ' + kind); }

        if (kind === 'MOV32R') {                   /* MOVE r32, rz */
            /* [MD00076 3.14.10] bits 7..3 hold r32 as [2:0,4:3] */
            var fld = (insn >> 3) & 0x1F;
            var r32 = ((fld & 3) << 3) | ((fld >> 2) & 7);
            var rz = insn & 7;
            if (r32 === 0 && rz === 0) {
                return new Insn('nop', [], 2, {x: {k: 'move', d: 0, s: 0}});
            }
            return new Insn('move', [rn32(r32), rn(rz)], 2,
                            {x: {k: 'move', d: r32, s: xl(rz)}});
        }

        if (kind === 'MOVR32') {                   /* MOVE ry, r32 */
            return new Insn('move', [rn(ry), rn32(insn & 0x1F)], 2,
                            {x: {k: 'move', d: xl(ry), s: insn & 0x1F}});
        }
    }

    /* ---- 01101 LI / 01110 CMPI ---------------------------------------- */
    if (op === 0x0D) {
        bad = zfield(0xE0, 'LI');
        if (bad) { return bad; }
        imm = (ext === null) ? (insn & 0xFF) : extImm16(ext, insn);
        return new Insn('li', [rn(rx), hexs(imm)], n,
                        {x: {k: 'li', d: xl(rx), imm: imm}});
    }
    if (op === 0x0E) {
        bad = zfield(0xE0, 'CMPI');
        if (bad) { return bad; }
        imm = (ext === null) ? (insn & 0xFF) : extImm16(ext, insn);
        return new Insn('cmpi', [rn(rx), hexs(imm)], n,
                        {x: {k: 'cmpi', s: xl(rx), imm: imm}});
    }

    /* ---- 01111 SD : beta ---------------------------------------------- */
    if (op === 0x0F) {
        off = (ext === null) ? ((insn & 0x1F) << 3) : sx(extImm16(ext, insn), 16);
        return gate(new Insn('sd', [rn(ry), hexs(off) + '(' + rn(rx) + ')'], n,
                             {isa64: true}));
    }

    /* ---- loads / stores with a 5-bit scaled offset --------------------- */
    if (MEM_OPS[op] !== undefined) {
        got = MEM_OPS[op];
        name = got[0];
        off = (ext === null) ? ((insn & 0x1F) << got[1])
                             : sx(extImm16(ext, insn), 16);
        return gate(new Insn(name, [rn(ry), hexs(off) + '(' + rn(rx) + ')'], n,
                             {isa64: got[2],
                              x: {k: name, r: xl(ry), b: xl(rx), off: off}}));
    }

    /* ---- 10010 LWSP / 11010 SWSP --------------------------------------- */
    if (op === 0x12 || op === 0x1A) {
        name = (op === 0x12) ? 'lw' : 'sw';
        bad = zfield(0xE0, 'LWSP/SWSP');
        if (bad) { return bad; }
        off = (ext === null) ? ((insn & 0xFF) << 2) : sx(extImm16(ext, insn), 16);
        return new Insn(name, [rn(rx), hexs(off) + '(sp)'], n,
                        {x: {k: (op === 0x12) ? 'lwsp' : 'swsp',
                             r: xl(rx), off: off}});
    }

    /* ---- 10110 LWPC : LW rx, offset(pc) -------------------------------- */
    if (op === 0x16) {
        bad = zfield(0xE0, 'LWPC');
        if (bad) { return bad; }
        off = (ext === null) ? ((insn & 0xFF) << 2) : sx(extImm16(ext, insn), 16);
        if (pcb === null) {
            return new Insn('lw', [rn(rx), hexs(off) + '(pc)'], n,
                            {x: {k: 'lwpc', d: xl(rx), off: off}});
        }
        t = (((pcb & ~3) >>> 0) + off) >>> 0;
        return new Insn('lw', [rn(rx), uhex(t)], n,
                        {target: t, x: {k: 'lwpc', d: xl(rx), off: off, ea: t}});
    }

    /* ---- 11110 EXTEND --------------------------------------------------- */
    if (op === 0x1E) {
        if (ext !== null) { return notextInsn('EXTEND'); }
        return new Insn('extend', [uhex(insn & 0x7FF)], 2, {x: {k: 'extend'}});
    }

    /* ---- 11111 : the MIPS16e64 "I64" group (isa=64 only) ---------------- */
    if (op === 0x1F) {
        /* Not from MD00076 -- established by differential test against
           binutils 2.42; reachable only with isa=64. */
        funct = (insn >> 8) & 7;
        var imm5 = insn & 0x1F;
        if (funct === 0) {                               /* LDSP */
            off = (ext === null) ? (imm5 << 3) : sx(extImm16(ext, insn), 16);
            return gate(new Insn('ld', [rn(ry), hexs(off) + '(sp)'], n, {isa64: true}));
        }
        if (funct === 1) {                               /* SDSP */
            off = (ext === null) ? (imm5 << 3) : sx(extImm16(ext, insn), 16);
            return gate(new Insn('sd', [rn(ry), hexs(off) + '(sp)'], n, {isa64: true}));
        }
        if (funct === 2) {                               /* SDRASP */
            off = (ext === null) ? ((insn & 0xFF) << 3) : sx(extImm16(ext, insn), 16);
            return gate(new Insn('sd', ['ra', hexs(off) + '(sp)'], n, {isa64: true}));
        }
        if (funct === 3) {                               /* DADJSP */
            imm = (ext === null) ? (sx(insn & 0xFF, 8) << 3)
                                 : sx(extImm16(ext, insn), 16);
            return gate(new Insn('daddiu', ['sp', hexs(imm)], n, {isa64: true}));
        }
        if (funct === 4) {                               /* LDPC */
            off = (ext === null) ? (imm5 << 3) : sx(extImm16(ext, insn), 16);
            if (addr === null) {
                return gate(new Insn('ld', [rn(ry), hexs(off) + '(pc)'], n,
                                     {isa64: true}));
            }
            t = (((addr & ~7) >>> 0) + off) >>> 0;
            return gate(new Insn('ld', [rn(ry), uhex(t)], n, {isa64: true}));
        }
        if (funct === 5) {                               /* DADDIU5 */
            imm = (ext === null) ? sx(imm5, 5) : sx(extImm16(ext, insn), 16);
            return gate(new Insn('daddiu', [rn(ry), hexs(imm)], n, {isa64: true}));
        }
        if (funct === 6) {                               /* DADDIUPC -> "dla" */
            off = (ext === null) ? (imm5 << 2) : sx(extImm16(ext, insn), 16);
            if (pcb === null) {
                return gate(new Insn('dla', [rn(ry), hexs(off) + '(pc)'], n,
                                     {isa64: true}));
            }
            t = (((pcb & ~3) >>> 0) + off) >>> 0;
            return gate(new Insn('dla', [rn(ry), uhex(t)], n, {isa64: true}));
        }
        /* funct === 7: DADDIUSP */
        off = (ext === null) ? (imm5 << 2) : sx(extImm16(ext, insn), 16);
        return gate(new Insn('daddiu', [rn(ry), 'sp', hexs(off)], n, {isa64: true}));
    }

    /* ---- 11100 RRR ------------------------------------------------------ */
    if (op === 0x1C) {
        f = insn & 3;                              /* [MD00076 Table 3.23] */
        name = RRR_NAMES[f];
        var rIsa64 = (f === 0 || f === 2);         /* beta */
        if (ext !== null) { return notextInsn('RRR'); }
        var rz = (insn >> 2) & 7;
        ops = (rz === rx) ? [rn(rz), rn(ry)] : [rn(rz), rn(rx), rn(ry)];
        return gate(new Insn(name, ops, 2,
                             {isa64: rIsa64,
                              x: {k: name, d: xl(rz), s: xl(rx), t: xl(ry)}}));
    }

    /* ---- 11101 RR -------------------------------------------------------- */
    if (op === 0x1D) {
        if (ext !== null) { return notextInsn('RR'); }
        funct = insn & 0x1F;                       /* [MD00076 Table 3.24] */
        got = RR_FUNCT[funct];
        name = got[0];
        kind = got[1];
        var flag = got[2];
        if (name === null) {
            return unk('RR funct=0x' + (funct < 16 ? '0' : '') + funct.toString(16)
                       + ' is Reserved', 2);
        }
        var rIsa64b = (flag === true);

        if (kind === 'd' && name === 'J(AL)R(C)') {   /* [MD00076 Table 3.26] */
            sel = JALRC[ry];
            if (sel === undefined) {
                return unk('J(AL)R(C) ry=' + ry + ' is Reserved', 2);
            }
            if ((sel === 'JR_RA' || sel === 'JRC_RA') && rx) {
                return unk('JR/JRC ra with non-zero rx field', 2);
            }
            var usesRa = (sel === 'JR_RA' || sel === 'JRC_RA');
            return new Insn(JALRC_NAME[sel], [usesRa ? 'ra' : rn(rx)], 2,
                            {x: {k: JALRC_NAME[sel], s: usesRa ? GPR_RA : xl(rx)}});
        }

        if (kind === 'd' && name === 'CNVT') {       /* [MD00076 Table 3.27] */
            got = CNVT[ry];
            if (got === undefined) {
                return unk('CNVT ry=' + ry + ' is Reserved', 2);
            }
            return gate(new Insn(got[0], [rn(rx)], 2,
                                 {isa64: got[1], x: {k: got[0], d: xl(rx)}}));
        }

        if (kind === 'code') {
            code = (insn >> 5) & 0x3F;
            return new Insn(name, code ? [String(code)] : [], 2,
                            {x: {k: 'trap', name: name, code: code}});
        }
        if (kind === 'a2') {                         /* Format: OP rx, ry */
            if ((name === 'neg' || name === 'not') && rx === ry) {
                return gate(new Insn(name, [rn(rx)], 2,
                                     {isa64: rIsa64b,
                                      x: {k: name, d: xl(rx), s: xl(ry)}}));
            }
            return gate(new Insn(name, [rn(rx), rn(ry)], 2,
                                 {isa64: rIsa64b,
                                  x: {k: name, d: xl(rx), s: xl(ry)}}));
        }
        if (kind === 'sv') {                         /* Format: OP ry, rx */
            return gate(new Insn(name, [rn(ry), rn(rx)], 2,
                                 {isa64: rIsa64b,
                                  x: {k: name, d: xl(ry), sh: xl(rx)}}));
        }
        if (kind === 'dv') {                         /* binutils prints hi/lo dest */
            return gate(new Insn(name, ['zero', rn(rx), rn(ry)], 2,
                                 {isa64: rIsa64b,
                                  x: {k: name, s: xl(rx), t: xl(ry)}}));
        }
        if (kind === 'r1') {                         /* Format: OP rx (ry must be 0) */
            if (ry) {
                return unk(name + ' with non-zero ry field', 2);
            }
            return gate(new Insn(name, [rn(rx)], 2,
                                 {isa64: rIsa64b, x: {k: name, d: xl(rx)}}));
        }
        if (kind === 'sh64') {
            /* DSRL/DSRA ry, sa -- the 3-bit sa sits in bits 10..8 and the
               register in bits 7..5, the opposite way round from SHIFT.
               Established by differential test against binutils 2.42. */
            sa = rx || 8;
            return gate(new Insn(name, [rn(ry), String(sa)], 2, {isa64: true}));
        }
    }

    return unk('opcode 0x' + (op < 16 ? '0' : '') + op.toString(16)
               + ' (bits 15..11 = ' + bits5(op) + ') is not a MIPS16e instruction',
               n);
}

function bits5(v) {
    var s = (v & 0x1F).toString(2);
    while (s.length < 5) { s = '0' + s; }
    return s;
}

function decodeStrict(hw, extend, nextHw, addr, pcBase, isa) {
    var r = decode(hw, extend, nextHw, addr, pcBase, isa);
    if (r.op === 'UNKNOWN') {
        var h = (hw & 0xFFFF).toString(16);
        while (h.length < 4) { h = '0' + h; }
        var e = new Error('0x' + h + ': ' + r.reason);
        e.name = 'UnknownInstruction';
        throw e;
    }
    return r;
}

/* ------------------------------------------------------------------ */
/* Linear sweep (port of disassemble())                               */
/* ------------------------------------------------------------------ */

/* Jumps with a delay slot.  Branches, JRC and JALRC have none.
   [MD00076 3.13 MIPS16e Jump and Branch Instructions] */
var HAS_DELAY_SLOT = {jal: 1, jalx: 1, jr: 1, jalr: 1};

function disassemble(data, addr, length, isa) {
    if (isNil(addr)) { addr = 0; }
    if (isNil(isa)) { isa = 32; }
    var end = isNil(length) ? data.length : Math.min(data.length, length);
    var i = 0, out = [], hw, nxt, ins, ln;
    var delayFrom = null;
    while (i + 2 <= end) {
        hw = (data[i] << 8) | data[i + 1];
        nxt = (i + 4 <= end) ? ((data[i + 2] << 8) | data[i + 3]) : null;
        if ((hw >> 11) === 0x1E && nxt !== null) {            /* EXTEND prefix */
            ins = decode(nxt, hw, null, addr + i, delayFrom, isa);
            if (!ins.notext) {
                delayFrom = null;
                out.push({addr: addr + i, bytes: slice(data, i, i + 4), ins: ins});
                i += 4;
                continue;
            }
            delayFrom = null;
            out.push({addr: addr + i, bytes: slice(data, i, i + 2),
                      ins: decode(hw, null, null, addr + i, null, isa)});
            i += 2;
            continue;
        }
        ins = decode(hw, null, nxt, addr + i, delayFrom, isa);
        ln = ins.length;
        if (ln === 4 && nxt === null) { ln = 2; }
        delayFrom = HAS_DELAY_SLOT[ins.op] ? (addr + i) : null;
        out.push({addr: addr + i, bytes: slice(data, i, i + ln), ins: ins});
        i += ln;
    }
    return out;
}

function slice(data, a, b) {
    var out = [], i;
    for (i = a; i < b; i++) { out.push(data[i]); }
    return out;
}

/* ------------------------------------------------------------------ */
/* The executor                                                       */
/* ------------------------------------------------------------------ */

function rd(cpu, n) { return n === 0 ? 0 : (cpu.reg[n] | 0); }
function wr(cpu, n, v) { if (n !== 0) { cpu.reg[n] = v | 0; } }

/* 32x32 -> 64 multiply, returning [hi, lo] as int32s.  Done in 16-bit limbs so
   every partial product stays exact in a double (no value exceeds 2^48). */
function mul64(a, b, signed) {
    var neg = false, ua, ub, ah, al, bh, bl, mid, lo, hi, sum, carry;
    ua = signed ? (a | 0) : (a >>> 0);
    ub = signed ? (b | 0) : (b >>> 0);
    if (signed) {
        if (ua < 0) { ua = -ua; neg = !neg; }
        if (ub < 0) { ub = -ub; neg = !neg; }
    }
    ua = ua >>> 0; ub = ub >>> 0;
    ah = ua / 65536 | 0; al = ua & 0xFFFF;
    bh = ub / 65536 | 0; bl = ub & 0xFFFF;
    mid = al * bh + ah * bl;                       /* < 2^33, exact */
    sum = al * bl + (mid % 65536) * 65536;         /* < 2^33, exact */
    lo = sum % 4294967296;
    carry = Math.floor(sum / 4294967296);
    hi = (ah * bh + Math.floor(mid / 65536) + carry) % 4294967296;
    if (neg) {
        /* two's complement negate the 64-bit value */
        if (lo === 0) { lo = 0; hi = (4294967296 - hi) % 4294967296; }
        else { lo = 4294967296 - lo; hi = 4294967295 - hi; }
    }
    return [hi | 0, lo | 0];
}

function trunc(x) { return x < 0 ? Math.ceil(x) : Math.floor(x); }

/* ------------------------------------------------------------------ */
/* Integer opcode ids                                                 */
/* ------------------------------------------------------------------ */

/* WHY THE EXECUTOR DOES NOT SWITCH ON x.k.
   Dispatching on a STRING makes the hot switch a chain of string comparisons -- an engine
   cannot build a jump table out of it, because the cases are not a dense integer range.  The
   firmware this runs is 99.5% MIPS16, so that chain is the interpreter's inner loop.  Each
   decoded instruction therefore carries a small dense integer alongside its k, and exec()
   switches on that.
   k STAYS, and stays authoritative: the disassembler, the tests and the default arm's
   diagnostic all read it, and an id is meaningless to a human reading a decode dump.
   THE TABLE IS THE WHOLE VOCABULARY.  Brute-forcing every one of the 65,536 halfwords,
   bare and EXTEND-prefixed, yields exactly 63 distinct x.k values and they are exactly the
   63 the executor implements -- so every decodable instruction has an id, and id 0 is
   reachable only by a caller synthesising an x by hand.  That case still falls to the same
   default arm with the same message, so nothing silently changes shape. */
var KID = {
    addiusp: 1, addiupc: 2, addiu_rri: 3, addiu8: 4, adjsp: 5, li: 6, move: 7,
    addu: 8, subu: 9, and: 10, or: 11, xor: 12, not: 13, neg: 14,
    cmp: 15, cmpi: 16, slt: 17, sltu: 18, slti: 19, sltiu: 20,
    sll: 21, srl: 22, sra: 23, sllv: 24, srlv: 25, srav: 26,
    mult: 27, multu: 28, div: 29, divu: 30, mfhi: 31, mflo: 32,
    zeb: 33, zeh: 34, seb: 35, seh: 36,
    lb: 37, lbu: 38, lh: 39, lhu: 40, lw: 41, lwsp: 42, lwpc: 43,
    sb: 44, sh: 45, sw: 46, swsp: 47, swrasp: 48,
    b: 49, beqz: 50, bnez: 51, bteqz: 52, btnez: 53,
    jal: 54, jalx: 55, jr: 56, jalr: 57, jrc: 58, jalrc: 59,
    save: 60, restore: 61, trap: 62, extend: 63
};

/* ONE SHAPE FOR EVERY EXECUTION PAYLOAD.
   decode() builds x as whatever object literal each encoding needs -- {k,d,imm} here,
   {k,b,r,off} there -- so by the time the executor has seen a dozen encodings every single
   x.field read in it is a megamorphic load.  Normalising the payload to one field order
   makes them all monomorphic, and it costs one object per DECODE, which the decode cache
   already pays once per address.
   Nothing is dropped: val and ea are carried even though exec() recomputes both, because
   decode()'s output is a published shape and this is not the place to decide they are
   unused.  decode() itself is untouched -- a direct decode() call returns exactly the sparse
   object it always did; only the copy the executor runs from is normalised. */
function xnorm(x) {
    return {
        k: x.k,
        ki: KID[x.k] | 0,
        d: x.d === undefined ? 0 : x.d,
        s: x.s === undefined ? 0 : x.s,
        t: x.t === undefined ? 0 : x.t,
        r: x.r === undefined ? 0 : x.r,
        b: x.b === undefined ? 0 : x.b,
        sh: x.sh === undefined ? 0 : x.sh,
        imm: x.imm === undefined ? 0 : x.imm,
        off: x.off === undefined ? 0 : x.off,
        sa: x.sa === undefined ? 0 : x.sa,
        tgt: x.tgt === undefined ? 0 : x.tgt,
        name: x.name === undefined ? null : x.name,
        code: x.code === undefined ? 0 : x.code,
        framesize: x.framesize === undefined ? 0 : x.framesize,
        ra: x.ra === undefined ? 0 : x.ra,
        s0: x.s0 === undefined ? 0 : x.s0,
        s1: x.s1 === undefined ? 0 : x.s1,
        xsregs: x.xsregs === undefined ? 0 : x.xsregs,
        aregs: x.aregs === undefined ? 0 : x.aregs,
        val: x.val === undefined ? null : x.val,
        ea: x.ea === undefined ? null : x.ea
    };
}

/* ------------------------------------------------------------------ */
/* The address-keyed front cache                                      */
/* ------------------------------------------------------------------ */

/* WHY THERE IS A CACHE IN FRONT OF THE CACHE.
   The decode cache is a Map keyed by address, and profiling the executor over a real
   firmware body puts 30-48% of all time inside V8's FindOrderedHashMapEntry -- more than
   exec() and step() combined.  It is not the hashing, it is the memory behaviour: one
   probe into a table the size of the executed address space, per instruction, with no
   locality.  A direct-mapped array indexed by the address's low bits answers the same
   question with one typed-array load and one integer compare, and falls through to the Map
   on a conflict, so nothing is ever decoded twice that would not have been.

   INVALIDATION IS STILL THE CALLER'S, AND THIS MUST NOT WEAKEN IT.  Code that is written to
   is evicted by the caller calling delete() on the Map, which this array would not see.  So
   the front cache watches the Map's SIZE: outside step() the emulator only ever delete()s or
   clear()s (verified in digibox-boot.html -- there is no external set()), and both of those
   shrink it.  Any size that is not the size step() itself left behind therefore means the
   caller has evicted something, and the whole front cache is dropped by bumping a
   generation.  A caller that ADDED entries behind step()'s back would defeat the check;
   none does, and one that did would be defeating the decode cache's own accounting too.

   Generation rather than clearing: an eviction storm (the firmware relocating code into
   RAM writes thousands of words) must cost O(1) per flush, not a walk of the table. */
var FC_BITS = 14;
var FC_SIZE = 1 << FC_BITS;                  /* 16384 slots: a 32KB contiguous code window */
var FC_MASK = FC_SIZE - 1;

/* The whole safety argument rests on reading a NUMBER back from the cache's size, so a cache
   that does not report one gets no front cache at all rather than a freshness check that
   silently compares undefined with undefined and passes for ever.  A caller handing step() a
   plain object with get/set -- or anything else that is not a Map -- therefore runs exactly
   as it did before this optimisation existed, which is the only honest fallback: slower is a
   cost, stale is a wrong answer. */
function newFront(map) {
    return {
        map: map,
        live: typeof map.size === 'number',
        n: map.size,                         /* the size step() last left behind */
        g: 1,                                /* current generation */
        gen: new Int32Array(FC_SIZE),
        tag: new Int32Array(FC_SIZE),
        ins: new Array(FC_SIZE)
    };
}

/**
 * Execute ONE MIPS16 instruction at cpu.pc.
 *
 * Delay slots are modelled the way hardware does: a jump WITH a delay slot
 * arms cpu.delayed and advances pc past the jump; the following step()
 * executes the delay-slot instruction and only then applies the jump --
 * which is what makes "the delay slot executes in the OLD ISA mode" fall out
 * for free, and what lets a PC-relative load in a delay slot resolve against
 * the JUMP's address (cpu.delayed.from -> decode's pcBase).
 *
 * Returns {ok:true, length, ...} or {ok:false, reason, ...}; on failure the
 * cpu is left untouched.
 */
function step(cpu, mem) {
    if (cpu.isa !== 16) {
        return {ok: false, reason: 'cpu.isa is ' + cpu.isa + ', not 16; '
                                   + 'this executor runs MIPS16 only'};
    }
    if (cpu.delayed === null || cpu.delayed === undefined) { cpu.delayed = null; }

    var pc = cpu.pc >>> 0;
    var pend = cpu.delayed;

    /* OPTIONAL DECODE CACHE.  decode() allocates a fresh object per call and the executor
       has to dispatch on it, so re-decoding every instruction costs three allocations and a
       table walk each time.  99.5% of this firmware's executed instructions are MIPS16, so
       that is essentially the whole interpreter.  A caller that sets cpu.__dcache to a Map
       gets each address decoded once.
       Keyed by ADDRESS, not opcode: decode() resolves branch targets, jump targets and
       PC-relative loads against the address.
       NOT used inside a delay slot -- there decode() resolves PC-relative operands against
       the JUMP's address (pcBase), so the same halfword yields a different instruction.
       Invalidation is the caller's: code that is written to must be evicted.
       cpu.__fc is this executor's own direct-mapped index in front of that Map; see
       newFront() for what keeps the two honest. */
    var dcache = null;
    if (pend === null && cpu.__dcache !== null && cpu.__dcache !== undefined) {
        dcache = cpu.__dcache;
    }
    var fc = null, slot = -1, hit;
    if (dcache !== null) {
        fc = cpu.__fc;
        if (fc === null || fc === undefined || fc.map !== dcache) {
            fc = newFront(dcache);
            cpu.__fc = fc;
        } else if (fc.live && fc.n !== dcache.size) {
            /* The caller has evicted something.  Drop the whole index in O(1). */
            fc.g = (fc.g + 1) | 0;
            if (fc.g > 0x3FFFFFFF) { fc.gen.fill(0); fc.g = 1; }
            fc.n = dcache.size;
        }
        if (fc.live) {
            slot = (pc >>> 1) & FC_MASK;
            if (fc.tag[slot] === (pc | 0) && fc.gen[slot] === fc.g) {
                return exec(cpu, mem, fc.ins[slot], pc);
            }
        }
        hit = dcache.get(pc);
        if (hit !== undefined) {
            if (slot >= 0) { fc.tag[slot] = pc | 0; fc.gen[slot] = fc.g; fc.ins[slot] = hit; }
            return exec(cpu, mem, hit, pc);
        }
    }
    var pcBase = pend !== null ? pend.from : null;

    var hw = mem.read16(pc) & 0xFFFF;
    var ext = null, ins;
    if ((hw >>> 11) === 0x1E) {
        ext = hw;
        hw = mem.read16((pc + 2) >>> 0) & 0xFFFF;
        ins = decode(hw, ext, mem.read16((pc + 4) >>> 0) & 0xFFFF, pc, pcBase, 32);
        if (ins.notext) {
            return {ok: false, pc: pc, reason: ins.reason};
        }
    } else {
        ins = decode(hw, null, mem.read16((pc + 2) >>> 0) & 0xFFFF, pc, pcBase, 32);
    }
    if (ins.op === 'UNKNOWN') {
        return {ok: false, pc: pc, reason: ins.reason, insn: ins};
    }
    if (ins.x === null) {
        return {ok: false, pc: pc, insn: ins,
                reason: ins.op + ' decodes but is not implemented by step()'};
    }
    ins.x = xnorm(ins.x);
    if (dcache !== null) {
        dcache.set(pc, ins);
        if (slot >= 0) {
            fc.tag[slot] = pc | 0; fc.gen[slot] = fc.g; fc.ins[slot] = ins;
            fc.n = dcache.size;
        }
    }
    return exec(cpu, mem, ins, pc);
}

/* The execution half of step(), split out so a decode-cache hit reaches it without
   re-decoding.  Behaviour is identical; step() remains the entry point.
   REGISTER ACCESS IS INLINED rather than routed through rd()/wr().  cpu.reg is an
   Int32Array in the emulator and a plain Array in newCpu(), so the element access is
   polymorphic and a helper call cannot be specialised away; written out, each case reads
   and writes the array directly.  The $zero semantics are preserved exactly and are the
   reason the writes are guarded: reads of GPR 0 answer 0, writes to it are discarded, and
   the `| 0` on every write is what keeps a plain-Array register file holding int32s.
   GPR_T, GPR_SP and GPR_RA are all nonzero, so their accesses need no guard. */
function exec(cpu, mem, ins, pc) {
    var pend = cpu.delayed;
    if (pend === undefined) { pend = null; }
    /* Same as step()'s: inside a delay slot, PC-relative operands resolve against the
       JUMP's address rather than this instruction's. */
    var pcBase = pend !== null ? pend.from : null;
    var x = ins.x;
    var reg = cpu.reg;
    var len = ins.length;
    var nextPc = (pc + len) >>> 0;
    var branchTo = null;        /* taken immediately (no delay slot) */
    var jumpTo = null;          /* taken after the delay slot */
    var jumpIsa = 16;
    var a, b, v, ea, m, d, s, t;

    switch (x.ki) {

    /* --- arithmetic / logic ---------------------------------------- */
    case 1:  /* addiusp */
        d = x.d; if (d !== 0) { reg[d] = ((reg[GPR_SP] | 0) + x.imm) | 0; } break;
    /* [MD00076 ADDIUPC] the base is the address of the JUMP when this sits in
       a jump delay slot, not the address of the ADDIU itself. */
    case 2:  /* addiupc */
        ea = ((pcBase === null ? pc : pcBase) & ~3) >>> 0;
        d = x.d; if (d !== 0) { reg[d] = ((ea + x.imm) >>> 0) | 0; } break;
    case 3:  /* addiu_rri */
        d = x.d; s = x.s;
        if (d !== 0) { reg[d] = ((s === 0 ? 0 : reg[s] | 0) + x.imm) | 0; } break;
    case 4:  /* addiu8 */
        d = x.d; if (d !== 0) { reg[d] = ((reg[d] | 0) + x.imm) | 0; } break;
    case 5:  /* adjsp */
        reg[GPR_SP] = ((reg[GPR_SP] | 0) + x.imm) | 0; break;
    case 6:  /* li */
        d = x.d; if (d !== 0) { reg[d] = x.imm | 0; } break;
    case 7:  /* move */
        d = x.d; s = x.s;
        if (d !== 0) { reg[d] = (s === 0 ? 0 : reg[s] | 0) | 0; } break;
    case 8:  /* addu */
        d = x.d; s = x.s; t = x.t;
        if (d !== 0) {
            reg[d] = ((s === 0 ? 0 : reg[s] | 0) + (t === 0 ? 0 : reg[t] | 0)) | 0;
        }
        break;
    case 9:  /* subu */
        d = x.d; s = x.s; t = x.t;
        if (d !== 0) {
            reg[d] = ((s === 0 ? 0 : reg[s] | 0) - (t === 0 ? 0 : reg[t] | 0)) | 0;
        }
        break;
    case 10: /* and */
        d = x.d; s = x.s;
        if (d !== 0) { reg[d] = ((reg[d] | 0) & (s === 0 ? 0 : reg[s] | 0)) | 0; } break;
    case 11: /* or */
        d = x.d; s = x.s;
        if (d !== 0) { reg[d] = ((reg[d] | 0) | (s === 0 ? 0 : reg[s] | 0)) | 0; } break;
    case 12: /* xor */
        d = x.d; s = x.s;
        if (d !== 0) { reg[d] = ((reg[d] | 0) ^ (s === 0 ? 0 : reg[s] | 0)) | 0; } break;
    case 13: /* not */
        d = x.d; s = x.s;
        if (d !== 0) { reg[d] = (~(s === 0 ? 0 : reg[s] | 0)) | 0; } break;
    case 14: /* neg */
        d = x.d; s = x.s;
        if (d !== 0) { reg[d] = (0 - (s === 0 ? 0 : reg[s] | 0)) | 0; } break;

    /* --- the implicit condition register T (GPR 24) ----------------- */
    case 15: /* cmp */
        d = x.d; s = x.s;
        reg[GPR_T] = ((d === 0 ? 0 : reg[d] | 0) ^ (s === 0 ? 0 : reg[s] | 0)) | 0; break;
    case 16: /* cmpi */
        s = x.s;
        reg[GPR_T] = ((s === 0 ? 0 : reg[s] | 0) ^ x.imm) | 0; break;
    case 17: /* slt */
        d = x.d; s = x.s;
        reg[GPR_T] = ((d === 0 ? 0 : reg[d] | 0) < (s === 0 ? 0 : reg[s] | 0)) ? 1 : 0;
        break;
    case 18: /* sltu */
        d = x.d; s = x.s;
        reg[GPR_T] = (((d === 0 ? 0 : reg[d] | 0) >>> 0)
                      < ((s === 0 ? 0 : reg[s] | 0) >>> 0)) ? 1 : 0;
        break;
    case 19: /* slti */
        s = x.s;
        reg[GPR_T] = ((s === 0 ? 0 : reg[s] | 0) < (x.imm | 0)) ? 1 : 0; break;
    case 20: /* sltiu */
        s = x.s;
        reg[GPR_T] = (((s === 0 ? 0 : reg[s] | 0) >>> 0)
                      < ((x.imm | 0) >>> 0)) ? 1 : 0;
        break;

    /* --- shifts ----------------------------------------------------- */
    case 21: /* sll */
        d = x.d; s = x.s;
        if (d !== 0) { reg[d] = ((s === 0 ? 0 : reg[s] | 0) << (x.sa & 31)) | 0; } break;
    case 22: /* srl */
        d = x.d; s = x.s;
        if (d !== 0) { reg[d] = ((s === 0 ? 0 : reg[s] | 0) >>> (x.sa & 31)) | 0; } break;
    case 23: /* sra */
        d = x.d; s = x.s;
        if (d !== 0) { reg[d] = ((s === 0 ? 0 : reg[s] | 0) >> (x.sa & 31)) | 0; } break;
    case 24: /* sllv */
        d = x.d; s = x.sh;
        if (d !== 0) {
            reg[d] = ((reg[d] | 0) << ((s === 0 ? 0 : reg[s] | 0) & 31)) | 0;
        }
        break;
    case 25: /* srlv */
        d = x.d; s = x.sh;
        if (d !== 0) {
            reg[d] = ((reg[d] | 0) >>> ((s === 0 ? 0 : reg[s] | 0) & 31)) | 0;
        }
        break;
    case 26: /* srav */
        d = x.d; s = x.sh;
        if (d !== 0) {
            reg[d] = ((reg[d] | 0) >> ((s === 0 ? 0 : reg[s] | 0) & 31)) | 0;
        }
        break;

    /* --- hi/lo ------------------------------------------------------ */
    case 27: /* mult */
        d = x.d; s = x.s;
        m = mul64(d === 0 ? 0 : reg[d] | 0, s === 0 ? 0 : reg[s] | 0, true);
        cpu.hi = m[0]; cpu.lo = m[1]; break;
    case 28: /* multu */
        d = x.d; s = x.s;
        m = mul64(d === 0 ? 0 : reg[d] | 0, s === 0 ? 0 : reg[s] | 0, false);
        cpu.hi = m[0]; cpu.lo = m[1]; break;
    case 29: /* div */
        s = x.s; t = x.t;
        a = s === 0 ? 0 : reg[s] | 0; b = t === 0 ? 0 : reg[t] | 0;
        if (b === 0) { cpu.lo = 0; cpu.hi = a | 0; }
        else { cpu.lo = trunc(a / b) | 0; cpu.hi = (a % b) | 0; }
        break;
    case 30: /* divu */
        s = x.s; t = x.t;
        a = (s === 0 ? 0 : reg[s] | 0) >>> 0; b = (t === 0 ? 0 : reg[t] | 0) >>> 0;
        if (b === 0) { cpu.lo = 0; cpu.hi = a | 0; }
        else { cpu.lo = Math.floor(a / b) | 0; cpu.hi = (a % b) | 0; }
        break;
    case 31: /* mfhi */
        d = x.d; if (d !== 0) { reg[d] = cpu.hi | 0; } break;
    case 32: /* mflo */
        d = x.d; if (d !== 0) { reg[d] = cpu.lo | 0; } break;

    /* --- CNVT ------------------------------------------------------- */
    case 33: /* zeb */
        d = x.d; if (d !== 0) { reg[d] = ((reg[d] | 0) & 0xFF) | 0; } break;
    case 34: /* zeh */
        d = x.d; if (d !== 0) { reg[d] = ((reg[d] | 0) & 0xFFFF) | 0; } break;
    case 35: /* seb */
        d = x.d; if (d !== 0) { reg[d] = (((reg[d] | 0) << 24) >> 24) | 0; } break;
    case 36: /* seh */
        d = x.d; if (d !== 0) { reg[d] = (((reg[d] | 0) << 16) >> 16) | 0; } break;

    /* --- loads ------------------------------------------------------ */
    case 37: /* lb */
        b = x.b; ea = ((b === 0 ? 0 : reg[b] | 0) + x.off) >>> 0;
        d = x.r; if (d !== 0) { reg[d] = ((mem.read8(ea) << 24) >> 24) | 0; } break;
    case 38: /* lbu */
        b = x.b; ea = ((b === 0 ? 0 : reg[b] | 0) + x.off) >>> 0;
        d = x.r; if (d !== 0) { reg[d] = (mem.read8(ea) & 0xFF) | 0; } break;
    case 39: /* lh */
        b = x.b; ea = ((b === 0 ? 0 : reg[b] | 0) + x.off) >>> 0;
        d = x.r; if (d !== 0) { reg[d] = ((mem.read16(ea) << 16) >> 16) | 0; } break;
    case 40: /* lhu */
        b = x.b; ea = ((b === 0 ? 0 : reg[b] | 0) + x.off) >>> 0;
        d = x.r; if (d !== 0) { reg[d] = (mem.read16(ea) & 0xFFFF) | 0; } break;
    case 41: /* lw */
        b = x.b; ea = ((b === 0 ? 0 : reg[b] | 0) + x.off) >>> 0;
        d = x.r; if (d !== 0) { reg[d] = mem.read32(ea) | 0; } break;
    case 42: /* lwsp */
        ea = ((reg[GPR_SP] | 0) + x.off) >>> 0;
        d = x.r; if (d !== 0) { reg[d] = mem.read32(ea) | 0; } break;
    case 43: /* lwpc */
        ea = (((pcBase === null ? pc : pcBase) & ~3) >>> 0);
        ea = (ea + x.off) >>> 0;
        d = x.d; if (d !== 0) { reg[d] = mem.read32(ea) | 0; } break;

    /* --- stores ----------------------------------------------------- */
    case 44: /* sb */
        b = x.b; ea = ((b === 0 ? 0 : reg[b] | 0) + x.off) >>> 0;
        s = x.r; mem.write8(ea, (s === 0 ? 0 : reg[s] | 0) & 0xFF); break;
    case 45: /* sh */
        b = x.b; ea = ((b === 0 ? 0 : reg[b] | 0) + x.off) >>> 0;
        s = x.r; mem.write16(ea, (s === 0 ? 0 : reg[s] | 0) & 0xFFFF); break;
    case 46: /* sw */
        b = x.b; ea = ((b === 0 ? 0 : reg[b] | 0) + x.off) >>> 0;
        s = x.r; mem.write32(ea, (s === 0 ? 0 : reg[s] | 0) >>> 0); break;
    case 47: /* swsp */
        ea = ((reg[GPR_SP] | 0) + x.off) >>> 0;
        s = x.r; mem.write32(ea, (s === 0 ? 0 : reg[s] | 0) >>> 0); break;
    case 48: /* swrasp */
        ea = ((reg[GPR_SP] | 0) + x.off) >>> 0;
        mem.write32(ea, (reg[GPR_RA] | 0) >>> 0); break;

    /* --- branches (no delay slot) ----------------------------------- */
    case 49: /* b */
        branchTo = (pc + len + x.off) >>> 0; break;
    case 50: /* beqz */
        s = x.s;
        if ((s === 0 ? 0 : reg[s] | 0) === 0) { branchTo = (pc + len + x.off) >>> 0; }
        break;
    case 51: /* bnez */
        s = x.s;
        if ((s === 0 ? 0 : reg[s] | 0) !== 0) { branchTo = (pc + len + x.off) >>> 0; }
        break;
    case 52: /* bteqz */
        if ((reg[GPR_T] | 0) === 0) { branchTo = (pc + len + x.off) >>> 0; }
        break;
    case 53: /* btnez */
        if ((reg[GPR_T] | 0) !== 0) { branchTo = (pc + len + x.off) >>> 0; }
        break;

    /* --- jumps ------------------------------------------------------ */
    /* [MD00076 JAL/JALX] GPR[31] <- (PC+6)[31:1] || ISAMode.  The ISA bit is
       what makes the MIPS16 `jr ra` at the end of a routine called by a
       MIPS32 jalx return to MIPS32 correctly -- and it is set here from the
       mode we are IN, which is 16. */
    case 54: /* jal */
        reg[GPR_RA] = (((pc + 6) | 1) >>> 0) | 0;
        jumpTo = x.tgt; jumpIsa = 16; break;
    case 55: /* jalx */
        reg[GPR_RA] = (((pc + 6) | 1) >>> 0) | 0;
        jumpTo = x.tgt; jumpIsa = 32; break;   /* ISAMode toggles */
    case 56: /* jr */
        s = x.s; v = (s === 0 ? 0 : reg[s] | 0) >>> 0;
        jumpTo = (v & ~1) >>> 0; jumpIsa = (v & 1) ? 16 : 32; break;
    case 57: /* jalr */
        s = x.s; v = (s === 0 ? 0 : reg[s] | 0) >>> 0;
        reg[GPR_RA] = (((pc + 4) | 1) >>> 0) | 0;
        jumpTo = (v & ~1) >>> 0; jumpIsa = (v & 1) ? 16 : 32; break;
    case 58: /* jrc */
        s = x.s; v = (s === 0 ? 0 : reg[s] | 0) >>> 0;
        branchTo = (v & ~1) >>> 0;
        cpu.isa = (v & 1) ? 16 : 32; break;
    case 59: /* jalrc */
        s = x.s; v = (s === 0 ? 0 : reg[s] | 0) >>> 0;
        reg[GPR_RA] = (((pc + 2) | 1) >>> 0) | 0;
        branchTo = (v & ~1) >>> 0;
        cpu.isa = (v & 1) ? 16 : 32; break;

    /* --- SAVE / RESTORE --------------------------------------------- */
    case 60: /* save */
        execSave(cpu, mem, x); break;
    case 61: /* restore */
        execRestore(cpu, mem, x); break;

    /* --- the rest ---------------------------------------------------- */
    case 62: /* trap */
        return {ok: false, pc: pc, insn: ins,
                reason: x.name + ' (code ' + x.code + ') -- exception, not simulated'};
    case 63: /* extend */
        return {ok: false, pc: pc, insn: ins,
                reason: 'bare EXTEND: not an instruction on its own'};
    default:
        return {ok: false, pc: pc, insn: ins,
                reason: 'unimplemented in step(): ' + x.k};
    }

    var unpredictable = false;
    if (pend !== null) {
        /* We have just executed a delay slot: the pending jump takes effect. */
        if (jumpTo !== null || branchTo !== null) { unpredictable = true; }
        cpu.pc = pend.target >>> 0;
        cpu.isa = pend.isa;
        cpu.delayed = null;
    } else if (branchTo !== null) {
        cpu.pc = branchTo >>> 0;
    } else if (jumpTo !== null) {
        cpu.delayed = {target: jumpTo >>> 0, isa: jumpIsa, from: pc};
        cpu.pc = nextPc;
    } else {
        cpu.pc = nextPc;
    }

    /* Tried reusing a single success object here to save an allocation per instruction.
       Measured: 1,698,551 instructions/sec against 1,683,864 -- noise.  V8 handles
       short-lived objects well, so it bought nothing and a shared mutable result is a real
       aliasing hazard.  Reverted rather than kept on the theory that it ought to help. */
    return {ok: true, length: len, pc: pc, op: ins.op, operands: ins.operands,
            insn: ins, wasDelaySlot: pend !== null, unpredictable: unpredictable};
}

function execSave(cpu, mem, x) {
    /* [MD00076 SAVE Operation / SAVE (Extended) Operation] */
    var temp = rd(cpu, GPR_SP) >>> 0;
    var temp2 = temp;
    var got = AREGS[x.aregs];
    var args = got[0];
    var astatic = got[1].length;
    var i;
    if (args > 0) { mem.write32(temp, rd(cpu, 4) >>> 0); }
    if (args > 1) { mem.write32((temp + 4) >>> 0, rd(cpu, 5) >>> 0); }
    if (args > 2) { mem.write32((temp + 8) >>> 0, rd(cpu, 6) >>> 0); }
    if (args > 3) { mem.write32((temp + 12) >>> 0, rd(cpu, 7) >>> 0); }
    if (x.ra) { temp = (temp - 4) >>> 0; mem.write32(temp, rd(cpu, GPR_RA) >>> 0); }
    var xs = [30, 23, 22, 21, 20, 19, 18];       /* order for xsregs 7..1 */
    for (i = 0; i < 7; i++) {
        if (x.xsregs > (6 - i)) {
            temp = (temp - 4) >>> 0;
            mem.write32(temp, rd(cpu, xs[i]) >>> 0);
        }
    }
    if (x.s1) { temp = (temp - 4) >>> 0; mem.write32(temp, rd(cpu, 17) >>> 0); }
    if (x.s0) { temp = (temp - 4) >>> 0; mem.write32(temp, rd(cpu, 16) >>> 0); }
    var st = [7, 6, 5, 4];
    for (i = 0; i < astatic; i++) {
        temp = (temp - 4) >>> 0;
        mem.write32(temp, rd(cpu, st[i]) >>> 0);
    }
    wr(cpu, GPR_SP, (temp2 - x.framesize) >>> 0);
}

function execRestore(cpu, mem, x) {
    /* [MD00076 RESTORE Operation / RESTORE (Extended) Operation] */
    var temp = (rd(cpu, GPR_SP) + x.framesize) >>> 0;
    var temp2 = temp;
    var got = AREGS[x.aregs];
    var astatic = got[1].length;
    var i;
    if (x.ra) { temp = (temp - 4) >>> 0; wr(cpu, GPR_RA, mem.read32(temp) | 0); }
    var xs = [30, 23, 22, 21, 20, 19, 18];
    for (i = 0; i < 7; i++) {
        if (x.xsregs > (6 - i)) {
            temp = (temp - 4) >>> 0;
            wr(cpu, xs[i], mem.read32(temp) | 0);
        }
    }
    if (x.s1) { temp = (temp - 4) >>> 0; wr(cpu, 17, mem.read32(temp) | 0); }
    if (x.s0) { temp = (temp - 4) >>> 0; wr(cpu, 16, mem.read32(temp) | 0); }
    var st = [7, 6, 5, 4];
    for (i = 0; i < astatic; i++) {
        temp = (temp - 4) >>> 0;
        wr(cpu, st[i], mem.read32(temp) | 0);
    }
    wr(cpu, GPR_SP, temp2);
}

/* ------------------------------------------------------------------ */

function newCpu() {
    var reg = [], i;
    for (i = 0; i < 32; i++) { reg.push(0); }
    return {pc: 0, isa: 16, reg: reg, hi: 0, lo: 0, delayed: null};
}

return {
    decode: decode,
    decodeStrict: decodeStrict,
    disassemble: disassemble,
    step: step,
    newCpu: newCpu,
    XLAT: XLAT,
    REG_NAMES: REG_NAMES
};

})();

if (typeof module !== 'undefined' && module.exports) { module.exports = M16; }
