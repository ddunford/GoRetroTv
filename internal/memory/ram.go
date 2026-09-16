// Package memory is the machine's DRAM and its two flash chips.
//
// The Pace 2500N has 32 MB of DRAM and two 2 MB flash parts. Their addresses are given here the
// way the measured record and the oracle's own memory map give them, in virtual form, and the bus
// turns those into the physical addresses it decodes on - which is what makes the cached and
// uncached windows onto DRAM one device rather than two that have to be kept in step.
package memory

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

// The virtual bases and sizes this board actually has.
//
// DRAM appears twice: KSEG0 at 0x80000000 is cached and KSEG1 at 0xA0000000 is not, and they are
// the same 32 MB of physical memory. The firmware uses both deliberately - it quotes its field
// buffers uncached so the hardware sees writes the CPU has not flushed - so an emulator that
// treats them as separate memories produces a machine whose drawing path is subtly wrong.
//
// The flash chips are at 0xBFC00000 (U202, which holds the reset vector) and 0xBF800000 (U203),
// each mirrored into KSEG0 at 0x9FC00000 and 0x9F800000. The board has two and this matters: when
// the predecessor modelled only one, the second answered zero rather than 0xFF, the resident
// content manager read thirty blocks of zeros, declared the partition unused and set about
// mirroring 1.5 MB into a chip that was not there.
const (
	DRAMBase     = 0x80000000 // cached; 0xA0000000 is the same memory, uncached
	DRAMSize     = 32 << 20
	FlashU202    = 0xBFC00000 // the reset vector's chip; mirrored at 0x9FC00000
	FlashU203    = 0xBF800000 // the second chip; mirrored at 0x9F800000
	FlashSize    = 2 << 20
	DirtyPageLen = 4096 // the granularity the checkpoint hash re-digests
)

// RAM is the machine's DRAM.
//
// It tracks which pages have been written since the dirty set was last cleared. That is not
// bookkeeping for its own sake: the checkpoint hash has to re-digest DRAM every thousand
// instructions, and hashing 32 MB that often would dominate the run (spike 002), so it re-digests
// only what changed. The tracking lives here rather than in the hash because only the memory knows
// when a byte is written.
type RAM struct {
	// name is the device's identity and its key in a machine snapshot, not state.
	name string

	bytes []byte
	dirty []uint64

	// straddled counts accesses that ran past the end of this device. On real hardware the upper
	// bytes of such an access come from whatever is mapped next; here the bus handed the whole
	// access to one device, so the bytes beyond the end read as zero and are counted rather than
	// being silently indistinguishable from real ones.
	straddled uint64
}

// NewRAM returns zeroed DRAM of size bytes, which must be a whole number of dirty pages.
func NewRAM(name string, size uint32) (*RAM, error) {
	if name == "" {
		return nil, fmt.Errorf("memory: RAM needs a name: it is the key in a machine snapshot")
	}
	if size == 0 || size%DirtyPageLen != 0 {
		return nil, fmt.Errorf("memory: RAM size %#x must be a non-zero multiple of the %d-byte "+
			"dirty page", size, DirtyPageLen)
	}
	return &RAM{
		name:  name,
		bytes: make([]byte, size),
		dirty: make([]uint64, (size/DirtyPageLen+63)/64),
	}, nil
}

// Name is the device's identity.
func (r *RAM) Name() string { return r.name }

// Size is how many bytes of DRAM there are.
func (r *RAM) Size() uint32 { return u32len(r.bytes) }

// u32len is len(b) as a uint32.
//
// Both memories here are sized from a uint32 at construction and neither is ever resized, so the
// conversion cannot lose anything. It is one function with one annotation rather than half a dozen
// conversions each carrying their own.
func u32len(b []byte) uint32 {
	return uint32(len(b)) //#nosec G115 -- bounded by the uint32 size the constructor accepted
}

// Read returns size bytes at off, big-endian.
//
// There is no alignment check. MIPS raises an address error for an unaligned load, but that is the
// CPU's exception to raise before the access reaches a bus; and this firmware's memset uses lwl,
// lwr, swl and swr, whose unaligned path performs genuinely unaligned byte accesses. The oracle
// does not check alignment here either, and agreeing with it is the point.
func (r *RAM) Read(off uint32, size bus.Size) uint32 {
	if uint64(off)+uint64(size) <= uint64(len(r.bytes)) {
		b := r.bytes[off:]
		// The bus refuses any other width before an access reaches a device, so the default arm
		// is Word rather than an error case.
		//exhaustive:ignore
		switch size {
		case bus.Byte:
			return uint32(b[0])
		case bus.Half:
			return uint32(b[0])<<8 | uint32(b[1])
		default:
			return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		}
	}
	return r.readStraddling(off, size)
}

// Write stores size bytes at off, big-endian, and marks the pages it touched.
func (r *RAM) Write(off uint32, size bus.Size, value uint32) {
	if uint64(off)+uint64(size) > uint64(len(r.bytes)) {
		r.writeStraddling(off, size, value)
		return
	}
	b := r.bytes[off:]
	//exhaustive:ignore
	switch size {
	case bus.Byte:
		b[0] = byte(value)
	case bus.Half:
		b[0], b[1] = byte(value>>8), byte(value)
	default:
		b[0], b[1], b[2], b[3] = byte(value>>24), byte(value>>16), byte(value>>8), byte(value)
	}
	r.markDirty(off, uint32(size))
}

// readStraddling handles the rare access that runs off the end of DRAM, byte by byte, so that a
// four-byte load at the very last byte answers instead of panicking.
func (r *RAM) readStraddling(off uint32, size bus.Size) uint32 {
	r.straddled++
	limit := u32len(r.bytes)
	var v uint32
	for i := uint32(0); i < uint32(size); i++ {
		v <<= 8
		// off+i cannot wrap: the bus only ever hands a device an offset inside its own region,
		// and every region on this board lies far below the top of the address space.
		if a := off + i; a < limit {
			v |= uint32(r.bytes[a])
		}
	}
	return v
}

func (r *RAM) writeStraddling(off uint32, size bus.Size, value uint32) {
	r.straddled++
	limit := u32len(r.bytes)
	n := uint32(size)
	for i := uint32(0); i < n; i++ {
		a := off + i
		if a >= limit {
			continue
		}
		r.bytes[a] = byte(value >> ((n - 1 - i) * 8))
		r.markDirty(a, 1)
	}
}

func (r *RAM) markDirty(off, n uint32) {
	first := off / DirtyPageLen
	last := (off + n - 1) / DirtyPageLen
	for p := first; p <= last; p++ {
		r.dirty[p/64] |= 1 << (p % 64)
	}
}

// PageLen is the size of one dirty page.
func (r *RAM) PageLen() uint32 { return DirtyPageLen }

// Pages is how many dirty pages DRAM is divided into.
func (r *RAM) Pages() uint32 { return u32len(r.bytes) / DirtyPageLen }

// EachDirtyPage calls fn for every page written since the dirty set was last cleared, in address
// order, with that page's current contents.
//
// Address order rather than map order because the checkpoint hash folds these in sequence, and a
// hash whose inputs arrive in a different order on two runs of the same program is not a hash the
// oracle comparison can use.
func (r *RAM) EachDirtyPage(fn func(page uint32, data []byte)) {
	pages := r.Pages()
	for p := uint32(0); p < pages; p++ {
		if r.dirty[p/64]&(1<<(p%64)) == 0 {
			continue
		}
		fn(p, r.bytes[p*DirtyPageLen:(p+1)*DirtyPageLen])
	}
}

// DirtyCount is how many pages are currently marked.
func (r *RAM) DirtyCount() int {
	n := 0
	r.EachDirtyPage(func(uint32, []byte) { n++ })
	return n
}

// ClearDirty forgets which pages have been written. The checkpoint emitter calls it after folding
// them in, so the next digest covers only what changed since.
func (r *RAM) ClearDirty() {
	for i := range r.dirty {
		r.dirty[i] = 0
	}
}

// MarkAllDirty marks every page, which is what a restore needs: the incremental digest that
// follows one has no earlier state to be incremental against.
func (r *RAM) MarkAllDirty() {
	pages := r.Pages()
	for p := uint32(0); p < pages; p++ {
		r.dirty[p/64] |= 1 << (p % 64)
	}
}

// Bytes exposes DRAM for the loader and for instruments that read it wholesale. It is the live
// array, not a copy: a caller that writes through it bypasses the dirty tracking, so anything that
// does must call MarkAllDirty afterwards.
func (r *RAM) Bytes() []byte { return r.bytes }

// Reset zeroes DRAM, which is what the oracle's reset does - it allocates a fresh array - and so
// is the state every measurement in the record was taken against.
func (r *RAM) Reset() {
	for i := range r.bytes {
		r.bytes[i] = 0
	}
	r.ClearDirty()
	r.straddled = 0
}

// ramVersion is this device's snapshot format version.
const ramVersion = 1

// Snapshot writes DRAM, the dirty set and the straddle count.
//
// The dirty set is in there because it is not derivable: it says what has changed since the last
// checkpoint, and a restore that guessed at it would make the next incremental digest cover the
// wrong pages - which shows up as a checkpoint divergence with no cause anywhere near it.
func (r *RAM) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(r.name, ramVersion)
	w.Bytes(r.bytes)
	w.Uint64(r.straddled)
	w.Uint32(uint32(len(r.dirty))) //#nosec G115 -- one word per 64 pages of a uint32-sized DRAM
	for _, word := range r.dirty {
		w.Uint64(word)
	}
	blob, err := w.Blob()
	if err != nil {
		return nil, fmt.Errorf("memory: snapshot %s: %w", r.name, err)
	}
	return blob, nil
}

// Restore replaces DRAM's contents, dirty set and straddle count.
func (r *RAM) Restore(state []byte) error {
	rd, err := snapcodec.Open(state)
	if err != nil {
		return fmt.Errorf("memory: restore %s: %w", r.name, err)
	}
	if err := rd.Expect(r.name, ramVersion, ramVersion); err != nil {
		return fmt.Errorf("memory: restore %s: %w", r.name, err)
	}

	contents := rd.Bytes()
	straddled := rd.Uint64()
	words := rd.Uint32()
	if err := rd.Err(); err != nil {
		return fmt.Errorf("memory: restore %s: %w", r.name, err)
	}
	if len(contents) != len(r.bytes) {
		return fmt.Errorf("memory: restore %s: snapshot holds %d bytes of DRAM and this machine "+
			"has %d", r.name, len(contents), len(r.bytes))
	}
	if uint64(words) != uint64(len(r.dirty)) {
		return fmt.Errorf("memory: restore %s: snapshot holds %d dirty-set words and this machine "+
			"has %d", r.name, words, len(r.dirty))
	}
	dirty := make([]uint64, words)
	for i := range dirty {
		dirty[i] = rd.Uint64()
	}
	if err := rd.Done(); err != nil {
		return fmt.Errorf("memory: restore %s: %w", r.name, err)
	}

	copy(r.bytes, contents)
	copy(r.dirty, dirty)
	r.straddled = straddled
	return nil
}
