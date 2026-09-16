package bus

import (
	"fmt"
	"sort"

	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

// The VR4111's kernel segments, and the only two this firmware uses.
//
// KSEG0 and KSEG1 are unmapped: they reach physical memory directly, by clearing the top three
// bits, with no TLB involved. They are the same 512 MB of physical address space seen twice,
// cached through KSEG0 and uncached through KSEG1, which is why a write through 0x80000000 is
// readable through 0xA0000000 - not because anything copies it, but because they are one address.
//
// KUSEG (0x00000000) and KSEG2/KSEG3 (0xC0000000 and up) are TLB-mapped. This firmware programs no
// TLB entries and never issues an address in them; one that appears is a fact worth reporting
// rather than a translation worth guessing at, so Physical refuses it. Spike 001's first
// throughput measurement was worthless because a runaway PC was executing zeros out of exactly
// this space, and the tell was addresses like 0xD10D59F0 that do not exist on this board.
const (
	segUnmappedLo = 0x80000000 // the bottom of KSEG0
	segUnmappedHi = 0xC0000000 // one past the top of KSEG1
	physMask      = 0x1FFFFFFF
	physSize      = 0x20000000 // 512 MB, the physical space the two segments reach
)

// Physical translates a virtual address to the physical address it reaches, reporting false for an
// address in a TLB-mapped segment, which this machine never uses.
//
// The bus decodes physically rather than virtually so that the DRAM aliases and the flash mirror
// fall out of one registration instead of being two that have to be kept in step. The predecessor
// kept a read watch on virtual addresses and a write watch on physical ones; they silently
// disagreed about whether they were watching the same memory, and that asymmetry was recorded as a
// finding before it was recognised as a bug (docs/reference/digibox-emulation.md).
func Physical(virt uint32) (uint32, bool) {
	if virt >= segUnmappedLo && virt < segUnmappedHi {
		return virt & physMask, true
	}
	return 0, false
}

// MaxUnmappedSites caps how many distinct unmapped addresses the census remembers.
//
// It is a cap rather than no limit because the condition the census exists to reveal - a runaway
// PC - visits millions of distinct addresses, and an uncapped census would turn a diagnosable
// fault into an out-of-memory kill with nothing to read afterwards. The totals keep counting past
// the cap, so a capped census still reports the size of what it stopped recording.
const MaxUnmappedSites = 4096

// UnmappedKind says why an access reached no device, which are two different facts about the
// machine and lead to two different investigations.
type UnmappedKind uint8

const (
	// NoDevice means the address translated, but nothing is mapped at that physical address. On
	// this machine that is ordinary - the oracle answers zero for unmodelled peripheral
	// registers too - and the census is how "unmodelled" stays visible rather than silent.
	NoDevice UnmappedKind = iota

	// NoSegment means the address is outside KSEG0 and KSEG1 and has no physical form at all.
	// The firmware has gone somewhere it has never gone, which in this project usually means a
	// runaway PC rather than a device nobody has modelled yet.
	NoSegment
)

// String names the kind.
func (k UnmappedKind) String() string {
	switch k {
	case NoDevice:
		return "no device"
	case NoSegment:
		return "no segment"
	default:
		return "unknown"
	}
}

// Access is one address the bus could not deliver to a device, and how often.
type Access struct {
	// Virtual is the address the firmware issued. It is the census key, and it is the virtual
	// form because that is what every address in docs/reference/digibox-emulation.md and every
	// address in a disassembly is written as. The two DRAM aliases of one unmapped address
	// therefore count as two sites, which is the more useful answer: it says which window the
	// code used.
	Virtual uint32

	// Physical is where it translated to, and is meaningful only when Kind is NoDevice.
	Physical uint32

	Kind   UnmappedKind
	Reads  uint64
	Writes uint64
}

// Totals summarises the unmapped census, including what it could not keep.
type Totals struct {
	// Reads and Writes count every unmapped access, whether or not its address was kept.
	Reads  uint64
	Writes uint64

	// Sites is how many distinct addresses the census holds.
	Sites uint64

	// Dropped counts accesses at addresses the census had no room for. A non-zero value means
	// Unmapped() is a sample rather than the whole picture, and says how large a sample.
	Dropped uint64
}

type region struct {
	base uint32 // physical
	size uint32
	dev  Device
}

// Bus is the machine's address decoder: it translates a virtual address, finds the device mapped
// there and hands it the offset within that device's own region.
//
// It is not safe for concurrent use, deliberately. The instruction loop is single-threaded and
// deterministic by construction (CLAUDE.md -> Architecture Decisions) and a mutex here would cost
// something on every load and store to defend against a caller this design does not have.
type Bus struct {
	regions []region // sorted by base, non-overlapping
	names   map[string]struct{}
	last    int // index of the last region hit; see find

	sites  map[uint32]*Access
	totals Totals

	// unknownState is set when a failed restore could not be undone, leaving the machine
	// somewhere between two states. Snapshot and Restore refuse while it is set, because the
	// worst thing this package could do at that point is write out a plausible snapshot of a
	// machine that never existed. Reset clears it: a reset genuinely does return the machine to
	// a state that is known.
	unknownState error
}

// New returns a bus with nothing attached.
func New() *Bus {
	return &Bus{
		names: make(map[string]struct{}),
		last:  -1,
		sites: make(map[uint32]*Access),
	}
}

// Attach maps a device over size bytes starting at the virtual address base.
//
// base is virtual so that a call site reads the way the measured record writes the address -
// Attach(0xB0009000, ...) for the DMA block rather than its physical 0x10009000. It is translated
// immediately, which means ONE call covers both windows onto that memory: a device attached at
// 0x80000000 is reachable at 0xA0000000 as well, because those are the same physical bytes.
// Attaching the second window separately is therefore an overlap, and is refused.
func (b *Bus) Attach(base, size uint32, d Device) error {
	if d == nil {
		return fmt.Errorf("bus: attach at %#08x: device is nil", base)
	}
	name := d.Name()
	if name == "" {
		return fmt.Errorf("bus: attach at %#08x: device has no name, and the name is its key in "+
			"a machine snapshot", base)
	}
	if _, taken := b.names[name]; taken {
		return fmt.Errorf("bus: attach %q at %#08x: a device is already called %q, and a snapshot "+
			"keyed by name could not tell them apart", name, base, name)
	}
	if size == 0 {
		return fmt.Errorf("bus: attach %q at %#08x: size is zero", name, base)
	}
	phys, ok := Physical(base)
	if !ok {
		return fmt.Errorf("bus: attach %q at %#08x: not in KSEG0 or KSEG1, the only segments this "+
			"machine addresses without a TLB", name, base)
	}
	if uint64(phys)+uint64(size) > physSize {
		return fmt.Errorf("bus: attach %q at %#08x: %#x bytes runs past the end of physical space "+
			"at %#08x", name, base, size, uint32(physSize-1))
	}
	for _, r := range b.regions {
		if phys < r.base+r.size && r.base < phys+size {
			return fmt.Errorf("bus: attach %q at %#08x (physical %#08x..%#08x): already mapped by "+
				"%q at physical %#08x..%#08x", name, base, phys, phys+size-1,
				r.dev.Name(), r.base, r.base+r.size-1)
		}
	}

	b.regions = append(b.regions, region{base: phys, size: size, dev: d})
	sort.Slice(b.regions, func(i, j int) bool { return b.regions[i].base < b.regions[j].base })
	b.names[name] = struct{}{}
	b.last = -1
	return nil
}

// Read decodes virt and returns what the device there answers.
//
// An address with no device behind it answers zero and is recorded in the unmapped census. That is
// what this machine does and what every measurement in the record was taken against: the oracle's
// load() returns zero for an unmodelled register, and the measured rule is that changing what a
// READ returns is the dangerous direction - making the demux register file read back its own
// writes, which sounds like an improvement, stopped the RTOS starting at all. What the bus adds is
// that the access is counted, so "unmodelled" is visible rather than silent.
//
// It panics on an invalid size. Only this program's own CPU calls it, never anything derived from
// the firmware, so an invalid width is a mistake in our code; and silently reading some other
// width is exactly the kind of plausible wrongness this machine does not report.
func (b *Bus) Read(virt uint32, size Size) uint32 {
	if !size.Valid() {
		panic(fmt.Sprintf("bus: read at %s with invalid size %d", hexfmt.Addr(virt), size))
	}
	phys, ok := Physical(virt)
	if !ok {
		b.note(virt, 0, NoSegment, true)
		return 0
	}
	if r := b.find(phys); r != nil {
		return r.dev.Read(phys-r.base, size)
	}
	b.note(virt, phys, NoDevice, true)
	return 0
}

// Write decodes virt and hands the value to the device there. A write with no device behind it is
// dropped and recorded. Like Read it panics on an invalid size.
func (b *Bus) Write(virt uint32, size Size, value uint32) {
	if !size.Valid() {
		panic(fmt.Sprintf("bus: write at %s with invalid size %d", hexfmt.Addr(virt), size))
	}
	phys, ok := Physical(virt)
	if !ok {
		b.note(virt, 0, NoSegment, false)
		return
	}
	if r := b.find(phys); r != nil {
		r.dev.Write(phys-r.base, size, value)
		return
	}
	b.note(virt, phys, NoDevice, false)
}

// find returns the region containing phys, or nil.
//
// The last region hit is tried first because this machine's access pattern is overwhelmingly
// local: nearly every access is DRAM, and a run of them is a run of hits on the same region.
// Missing that cache costs a binary search over a handful of regions, so the cache is an
// optimisation rather than a correctness concern - and it is not machine state, being derivable
// from the regions at any time, so it stays out of snapshots.
func (b *Bus) find(phys uint32) *region {
	if b.last >= 0 {
		if r := &b.regions[b.last]; phys-r.base < r.size {
			return r
		}
	}
	// The first region starting after phys; the one before it is the only possible container.
	i := sort.Search(len(b.regions), func(i int) bool { return b.regions[i].base > phys })
	if i == 0 {
		return nil
	}
	r := &b.regions[i-1]
	if phys-r.base >= r.size {
		return nil
	}
	b.last = i - 1
	return r
}

func (b *Bus) note(virt, phys uint32, kind UnmappedKind, read bool) {
	if read {
		b.totals.Reads++
	} else {
		b.totals.Writes++
	}
	if s, seen := b.sites[virt]; seen {
		if read {
			s.Reads++
		} else {
			s.Writes++
		}
		return
	}
	if len(b.sites) >= MaxUnmappedSites {
		b.totals.Dropped++
		return
	}
	s := &Access{Virtual: virt, Physical: phys, Kind: kind}
	if read {
		s.Reads = 1
	} else {
		s.Writes = 1
	}
	b.sites[virt] = s
	b.totals.Sites++
}

// Unmapped returns the census of addresses that reached no device, in address order so that a
// census printed into a report reads the same way twice.
func (b *Bus) Unmapped() []Access {
	out := make([]Access, 0, len(b.sites))
	for _, s := range b.sites {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Virtual < out[j].Virtual })
	return out
}

// UnmappedTotals summarises the census, including the accesses it had no room to record.
func (b *Bus) UnmappedTotals() Totals { return b.totals }

// Reset returns every attached device to power-on state and clears the unmapped census, which
// describes a run rather than the machine.
func (b *Bus) Reset() {
	for _, r := range b.regions {
		r.dev.Reset()
	}
	b.sites = make(map[uint32]*Access)
	b.totals = Totals{}
	b.unknownState = nil
}
