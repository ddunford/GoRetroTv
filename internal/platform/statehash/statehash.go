// Package statehash is the machine-state hash, and there is exactly one of it.
//
// It is the acceptance instrument for three separate requirements - the oracle comparison (FR-6),
// byte-identical replay (FR-7) and snapshot/restore (FR-8) - and if each of them grew its own
// hash, the three would verify against three different notions of "the same machine" and agree
// with each other by accident. It is also what the framebuffer and boot-gate comparisons use.
//
// # The algorithm, written out because the oracle has to reproduce it exactly
//
// TASK-1.9 adds the matching emitter to reference/digibox-boot.html, and a JavaScript
// implementation that differs by one byte of ordering produces a divergence at checkpoint zero
// that looks like a CPU fault. So the definition is here, in terms JavaScript can express with
// 32-bit integer operations:
//
//	FNV-1a, 32-bit:  h = 2166136261
//	                 per byte b:  h = Math.imul(h ^ b, 16777619) >>> 0
//
// Words are fed BIG-ENDIAN, most significant byte first, because the machine is big-endian and a
// hex dump of a snapshot should read the same way round as a hex dump of its memory.
//
// A page digest is FNV-1a over the page's INDEX as a big-endian word, then the page's bytes. The
// index is in there so that two pages swapping contents changes both digests rather than neither.
//
// The DRAM digest is the XOR of every page digest. XOR because it is what makes the digest
// incremental: re-digesting 32 MB every thousand instructions would dominate the run (spike 002),
// and XOR lets a page that changed be taken out and put back in without touching the rest.
//
// The state hash is FNV-1a over, in this order and no other:
//
//	PC, ISA (one byte), HI, LO, GPR[0..31], COP0[0..31], the DRAM digest
//
// # Why the ISA bit is not optional
//
// Omit it and a MIPS16/MIPS32 mode divergence shows up as a wrong instruction rather than as a
// wrong mode, which sends the reader hunting through the decoder for a fault that is not there.
// This firmware reaches MIPS16 through JALX and spends most of its time in it, so mode is a
// first-class part of where the machine is.
package statehash

import "fmt"

// The FNV-1a 32-bit constants, named so the oracle's copy can cite the same ones.
const (
	fnvOffset uint32 = 2166136261
	fnvPrime  uint32 = 16777619
)

// State is the register state a checkpoint covers.
//
// It is a plain struct rather than an interface because the CPU that fills it does not exist yet
// (phase 2), and a struct says exactly which fields a checkpoint is made of instead of leaving the
// shape to be guessed at from an interface's method set.
type State struct {
	// PC is the program counter with the ISA bit already masked off. ISA carries it instead, so
	// that a mode change is visible as a mode change.
	PC uint32

	// ISA is 0 for MIPS32 and 1 for MIPS16.
	ISA uint8

	HI, LO uint32
	GPR    [32]uint32
	COP0   [32]uint32
}

// DirtyRAM is the part of the machine's memory this package needs: which pages have been written
// since it last looked.
//
// The dirty set belongs to the checkpoint hash. Nothing else may clear it, because a page cleared
// by something else is a page this digest never folds in, and the resulting divergence appears
// thousands of instructions away from anything that caused it.
type DirtyRAM interface {
	Pages() uint32
	PageLen() uint32
	EachDirtyPage(fn func(page uint32, data []byte))
	ClearDirty()
	MarkAllDirty()
}

// Hasher computes the machine-state hash, keeping DRAM's digest incrementally.
type Hasher struct {
	ram DirtyRAM

	// pages holds each page's digest so a page that changes can be XORed out of total and the
	// new one XORed in, which is what makes this affordable at one checkpoint per thousand
	// instructions.
	pages []uint32
	total uint32

	// err is sticky. The one thing that can go wrong here - a page index outside the memory
	// this hasher was built for - cannot happen while DRAM is never resized, but folding a page
	// with no slot would silently leave it out of the digest, and a hash that quietly stops
	// covering part of the machine is the worst failure this package has. It is reported
	// through the emitter rather than by panicking: a run that cannot report what it found is
	// worse than one that reports bad news.
	err error
}

// New returns a Hasher over ram, with DRAM's digest already computed.
func New(ram DirtyRAM) (*Hasher, error) {
	if ram == nil {
		return nil, fmt.Errorf("statehash: New: ram is nil")
	}
	pages := ram.Pages()
	if pages == 0 {
		return nil, fmt.Errorf("statehash: New: the memory reports no pages, so this hash would " +
			"cover no DRAM at all and every checkpoint would agree for the wrong reason")
	}
	if ram.PageLen() == 0 {
		return nil, fmt.Errorf("statehash: New: the memory reports a page length of zero")
	}
	h := &Hasher{ram: ram, pages: make([]uint32, pages)}
	h.Rebuild()
	return h, nil
}

// Rebuild recomputes every page digest from scratch.
//
// A restored machine needs this: the incremental digest that follows a restore has no earlier
// state to be incremental against, and carrying the digest across a restore would describe the
// memory that was there before.
func (h *Hasher) Rebuild() {
	for i := range h.pages {
		h.pages[i] = 0
	}
	h.total = 0
	h.ram.MarkAllDirty()
	h.fold()
}

// RAMDigest folds in every page written since the last look and returns the digest of all of DRAM.
func (h *Hasher) RAMDigest() uint32 {
	h.fold()
	return h.total
}

// Hash returns the 32-bit hash of the whole machine: the registers in s and all of DRAM.
func (h *Hasher) Hash(s State) uint32 {
	ram := h.RAMDigest()

	v := fnvOffset
	v = mixWord(v, s.PC)
	v = mixByte(v, s.ISA)
	v = mixWord(v, s.HI)
	v = mixWord(v, s.LO)
	for _, r := range s.GPR {
		v = mixWord(v, r)
	}
	for _, r := range s.COP0 {
		v = mixWord(v, r)
	}
	return mixWord(v, ram)
}

// Err reports the first fault the hasher hit, if any. A non-nil value means the digests it has
// produced since cover less than the whole machine and cannot be compared against anything.
func (h *Hasher) Err() error { return h.err }

func (h *Hasher) fold() {
	h.ram.EachDirtyPage(func(p uint32, data []byte) {
		if uint64(p) >= uint64(len(h.pages)) {
			if h.err == nil {
				h.err = fmt.Errorf("statehash: DRAM offered page %d and this hasher was built "+
					"for %d, so the digest no longer covers the whole machine",
					p, len(h.pages))
			}
			return
		}
		h.total ^= h.pages[p]
		d := pageDigest(p, data)
		h.pages[p] = d
		h.total ^= d
	})
	h.ram.ClearDirty()
}

// pageDigest is FNV-1a over the page index and then the page's bytes.
func pageDigest(page uint32, data []byte) uint32 {
	v := mixWord(fnvOffset, page)
	for _, b := range data {
		v = mixByte(v, b)
	}
	return v
}

// mixOctet folds the low eight bits of x. Everything else here goes through it, so there is one
// place where the FNV step is written down and one place for the oracle's copy to agree with.
func mixOctet(v, x uint32) uint32 { return (v ^ (x & 0xFF)) * fnvPrime }

func mixByte(v uint32, b uint8) uint32 { return mixOctet(v, uint32(b)) }

// mixWord feeds a 32-bit value most significant byte first.
func mixWord(v, w uint32) uint32 {
	v = mixOctet(v, w>>24)
	v = mixOctet(v, w>>16)
	v = mixOctet(v, w>>8)
	return mixOctet(v, w)
}
