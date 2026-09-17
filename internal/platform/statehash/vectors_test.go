package statehash

import "testing"

// The vectors that pin the hash's definition.
//
// They exist because the oracle has to reproduce this algorithm in JavaScript (TASK-1.9), and an
// implementation that differs by one byte of ordering produces a divergence at checkpoint zero
// that looks like a CPU fault rather than like a broken instrument. These values were computed
// here and independently by a JavaScript implementation using Math.imul, and the two agreed; the
// same values are checked by the oracle's own self-test, so neither side can drift without a test
// going red.
//
// If one of these ever needs changing, the checkpoint stream format has changed and BOTH emitters
// and every recorded stream change with it.
func TestKnownVectors(t *testing.T) {
	t.Parallel()

	page0 := make([]byte, 4096)
	counting := make([]byte, 4096)
	for i := range counting {
		counting[i] = byte(i)
	}

	t.Run("the primitives", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			got  uint32
			want uint32
		}{
			{"the FNV offset basis", fnvOffset, 2166136261},
			{"the FNV prime", fnvPrime, 16777619},
			{"raw bytes used by the OSD surface", HashBytes([]byte{0x12, 0x34, 0x12, 0xff}), 0xD4A9E9C4},
			{"mixOctet drops all but the low eight bits", mixOctet(fnvOffset, 0xFFFFFF41),
				mixOctet(fnvOffset, 0x41)},
			{"mixWord over an address the record quotes", mixWord(fnvOffset, 0x80081C58), 0xBE2386A9},
			{"a zeroed page 0", pageDigest(0, page0), 0x9B5D3515},
			{"a counting page 3", pageDigest(3, counting), 0xE18B905C},
		}
		for _, tc := range cases {
			if tc.got != tc.want {
				t.Errorf("%s: 0x%08X, want 0x%08X", tc.name, tc.got, tc.want)
			}
		}
	})

	// The page index must change the digest, or two pages swapping contents would cancel under
	// the XOR that makes the DRAM digest incremental.
	t.Run("the page index is part of a page digest", func(t *testing.T) {
		t.Parallel()
		if a, b := pageDigest(0, counting), pageDigest(1, counting); a == b {
			t.Fatalf("pages 0 and 1 with identical contents both digest to 0x%08X", a)
		}
	})

	// The whole-state hash, which pins the field ORDER as well as the arithmetic.
	t.Run("a whole machine", func(t *testing.T) {
		t.Parallel()
		h, err := New(&fixedRAM{pages: 4, len: 4096})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		s := State{PC: 0x80081C58, ISA: 1, HI: 0x0000DEAD, LO: 0x0000BEEF}
		for i := range s.GPR {
			s.GPR[i] = 0x10000000 + uint32(i)*0x11
		}
		for i := range s.COP0 {
			s.COP0[i] = 0x20000000 + uint32(i)*0x13
		}
		// Four zeroed pages, XORed.
		if got, want := h.RAMDigest(), uint32(0x44C40724); got != want {
			t.Fatalf("the DRAM digest of four zeroed pages is 0x%08X, want 0x%08X", got, want)
		}
		const want = 0x3D280665
		if got := h.Hash(s); got != want {
			t.Fatalf("the whole-machine hash is 0x%08X, want 0x%08X. If this changed "+
				"deliberately then the checkpoint format changed: the oracle's emitter and "+
				"every recorded stream change with it.", got, want)
		}
	})
}

// fixedRAM is four zeroed pages that never change: the vector above has to be about the hash, not
// about whatever a memory happened to contain.
type fixedRAM struct {
	pages uint32
	len   uint32
	dirty bool
}

func (r *fixedRAM) Pages() uint32   { return r.pages }
func (r *fixedRAM) PageLen() uint32 { return r.len }
func (r *fixedRAM) ClearDirty()     { r.dirty = false }
func (r *fixedRAM) MarkAllDirty()   { r.dirty = true }
func (r *fixedRAM) EachDirtyPage(fn func(page uint32, data []byte)) {
	if !r.dirty {
		return
	}
	page := make([]byte, r.len)
	for p := uint32(0); p < r.pages; p++ {
		fn(p, page)
	}
}
