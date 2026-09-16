package bus_test

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

// The virtual bases this machine actually uses, written the way the measured record writes them
// (docs/reference/digibox-emulation.md, and the oracle's own map at reference/digibox-boot.html).
const (
	dramKseg0   = 0x80000000 // DRAM, cached
	dramKseg1   = 0xA0000000 // DRAM, uncached - the same 32 MB
	dramSize    = 32 << 20
	flashU202   = 0xBFC00000 // the reset vector's chip
	flashU202K0 = 0x9FC00000 // and its cached mirror
	flashU203   = 0xBF800000
	flashSize   = 2 << 20
	asicBase    = 0xB0000000 // the media ASIC, physical 0x10000000 on the external bus
	asicSize    = 0x00010000
)

// probe is a device that answers reads out of a small array and remembers exactly what the bus
// asked it for, so a decode test can assert the OFFSET rather than only the value.
type probe struct {
	label   string
	words   [8]uint32
	lastOff uint32
	lastSz  bus.Size
	reads   uint64
	writes  uint64
}

func newProbe(label string) *probe { return &probe{label: label} }

func (p *probe) Name() string { return p.label }

func (p *probe) Read(off uint32, size bus.Size) uint32 {
	p.lastOff, p.lastSz, p.reads = off, size, p.reads+1
	if i := off >> 2; i < uint32(len(p.words)) {
		return p.words[i]
	}
	return 0
}

func (p *probe) Write(off uint32, size bus.Size, value uint32) {
	p.lastOff, p.lastSz, p.writes = off, size, p.writes+1
	if i := off >> 2; i < uint32(len(p.words)) {
		p.words[i] = value
	}
}

func (p *probe) Reset() {
	label := p.label
	*p = probe{label: label}
}

func (p *probe) Snapshot() ([]byte, error) {
	b := make([]byte, 0, 8*4+4+1+8*2)
	for _, w := range p.words {
		b = binary.BigEndian.AppendUint32(b, w)
	}
	b = binary.BigEndian.AppendUint32(b, p.lastOff)
	b = append(b, byte(p.lastSz))
	b = binary.BigEndian.AppendUint64(b, p.reads)
	return binary.BigEndian.AppendUint64(b, p.writes), nil
}

func (p *probe) Restore(state []byte) error {
	const want = 8*4 + 4 + 1 + 8*2
	if len(state) != want {
		return fmt.Errorf("probe %q: snapshot is %d bytes, want %d", p.label, len(state), want)
	}
	label := p.label
	*p = probe{label: label}
	for i := range p.words {
		p.words[i] = binary.BigEndian.Uint32(state[i*4:])
	}
	p.lastOff = binary.BigEndian.Uint32(state[32:])
	p.lastSz = bus.Size(state[36])
	p.reads = binary.BigEndian.Uint64(state[37:])
	p.writes = binary.BigEndian.Uint64(state[45:])
	return nil
}

func attach(t *testing.T, b *bus.Bus, base, size uint32, d bus.Device) {
	t.Helper()
	if err := b.Attach(base, size, d); err != nil {
		t.Fatalf("Attach(%#08x, %#x, %s): %v", base, size, d.Name(), err)
	}
}

// The device contract, proven against the bus's own test device so that a change to the interface
// cannot pass here while failing everywhere else.
func TestProbeHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return newProbe("probe") },
		Mutate: func(d bus.Device) {
			d.Write(0x00, bus.Word, 0xDEADBEEF)
			d.Read(0x04, bus.Half)
			d.Write(0x08, bus.Byte, 0x5A)
		},
		Disturb: func(d bus.Device) {
			d.Read(0x0C, bus.Word)
			d.Write(0x10, bus.Half, 0x1234)
		},
		// label is the device's identity, not its state: Reset must not change it and neither
		// may a restore, or a snapshot could rename the device it is restoring into.
		Constant: []string{"label"},
	})
	if err != nil {
		t.Fatalf("the bus's own test device must hold the contract every device holds: %v", err)
	}
}

// TC-1.3's decoding half: the two DRAM aliases are one region, so a write through the cached
// window is readable through the uncached one. The bus decodes physical addresses precisely so
// this cannot drift - a KSEG0 and a KSEG1 registration that are separately maintained is the
// asymmetry that made a write watch and a read watch disagree in the predecessor.
func TestBothDramAliasesReachTheSameDeviceOffset(t *testing.T) {
	t.Parallel()
	b := bus.New()
	dram := newProbe("dram")
	attach(t, b, dramKseg0, dramSize, dram)

	b.Write(dramKseg0+0x10, bus.Word, 0xC0FFEE)
	if off := dram.lastOff; off != 0x10 {
		t.Fatalf("cached write reached offset %#x, want %#x", off, 0x10)
	}
	if got := b.Read(dramKseg1+0x10, bus.Word); got != 0xC0FFEE {
		t.Fatalf("read through the uncached alias got %#x, want %#x", got, 0xC0FFEE)
	}
	if off := dram.lastOff; off != 0x10 {
		t.Fatalf("uncached read reached offset %#x, want %#x", off, 0x10)
	}

	b.Write(dramKseg1+0x1C, bus.Word, 0x42)
	if got := b.Read(dramKseg0+0x1C, bus.Word); got != 0x42 {
		t.Fatalf("read through the cached alias got %#x, want %#x", got, 0x42)
	}
}

// The same for flash: the reset vector's chip is reachable at 0xBFC00000 and mirrored at
// 0x9FC00000, which the oracle's chipAt() spells out as two range checks per chip and the physical
// decode gets for nothing.
func TestBothFlashWindowsReachTheSameDeviceOffset(t *testing.T) {
	t.Parallel()
	b := bus.New()
	u202 := newProbe("U202")
	attach(t, b, flashU202, flashSize, u202)

	u202.words[1] = 0x3C09BFC0
	if got := b.Read(flashU202+4, bus.Word); got != 0x3C09BFC0 {
		t.Fatalf("read at the reset window got %#x", got)
	}
	if off := u202.lastOff; off != 4 {
		t.Fatalf("reset-window read reached offset %#x, want 4", off)
	}
	if got := b.Read(flashU202K0+4, bus.Word); got != 0x3C09BFC0 {
		t.Fatalf("read at the cached mirror got %#x", got)
	}
	if off := u202.lastOff; off != 4 {
		t.Fatalf("cached-mirror read reached offset %#x, want 4", off)
	}
}

// The peripherals sit at KSEG1 0xB0000000, which is physical 0x10000000 - the external system bus,
// not the VR4111's own on-chip registers at physical 0x0B000000 (digibox-emulation.md).
func TestPeripheralWindowDecodesOntoTheExternalBus(t *testing.T) {
	t.Parallel()
	b := bus.New()
	asic := newProbe("asic")
	attach(t, b, asicBase, asicSize, asic)

	b.Read(asicBase+0x14, bus.Word)
	if off := asic.lastOff; off != 0x14 {
		t.Fatalf("read reached offset %#x, want %#x", off, 0x14)
	}
	if got, ok := bus.Physical(asicBase); !ok || got != 0x10000000 {
		t.Fatalf("0xB0000000 translates to (%#08x, %v), want (0x10000000, true)", got, ok)
	}
}

func TestAccessWidthReachesTheDevice(t *testing.T) {
	t.Parallel()
	b := bus.New()
	p := newProbe("p")
	attach(t, b, asicBase, asicSize, p)

	for _, size := range []bus.Size{bus.Byte, bus.Half, bus.Word} {
		b.Read(asicBase+0x122, size)
		if p.lastSz != size {
			t.Fatalf("read width arrived as %s, want %s", p.lastSz, size)
		}
		b.Write(asicBase+0x122, size, 1)
		if p.lastSz != size {
			t.Fatalf("write width arrived as %s, want %s", p.lastSz, size)
		}
	}
}

func TestAttachRefusesRegionsThatOverlap(t *testing.T) {
	t.Parallel()
	b := bus.New()
	attach(t, b, dramKseg0, dramSize, newProbe("dram"))

	// The same region named through its other alias is the same physical memory, and accepting
	// it would leave two devices behind one address with the decode order deciding which wins.
	err := b.Attach(dramKseg1, dramSize, newProbe("dram-uncached"))
	if err == nil {
		t.Fatal("attaching the uncached alias of an already-mapped region must be refused")
	}
	if !strings.Contains(err.Error(), "dram") {
		t.Fatalf("the refusal must name the device already there, got: %v", err)
	}
	t.Logf("caught: %v", err)

	if err := b.Attach(dramKseg0+dramSize-4, 0x1000, newProbe("straddle")); err == nil {
		t.Fatal("a region straddling the end of an existing one must be refused")
	}
}

func TestAttachRefusesADuplicateName(t *testing.T) {
	t.Parallel()
	b := bus.New()
	attach(t, b, dramKseg0, dramSize, newProbe("dram"))

	// Names are snapshot keys. Two devices sharing one would make a snapshot ambiguous, and the
	// ambiguity would only show up as a restored machine that is subtly not the one saved.
	err := b.Attach(asicBase, asicSize, newProbe("dram"))
	if err == nil {
		t.Fatal("two devices with the same name must be refused: the name is the snapshot key")
	}
	t.Logf("caught: %v", err)
}

func TestAttachRefusesAnImpossibleRegion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		base uint32
		size uint32
	}{
		{"zero size", dramKseg0, 0},
		{"wrapping past the end of the address space", 0xBFFFF000, 0x2000},
		{"outside the unmapped segments", 0x00001000, 0x1000},
		{"in KSEG2, which needs the TLB this machine never programs", 0xC0000000, 0x1000},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := bus.New()
			if err := b.Attach(tc.base, tc.size, newProbe("p")); err == nil {
				t.Fatalf("Attach(%#08x, %#x) must be refused", tc.base, tc.size)
			} else {
				t.Logf("caught: %v", err)
			}
		})
	}
}

func TestAttachRefusesANilOrUnnamedDevice(t *testing.T) {
	t.Parallel()
	b := bus.New()
	if err := b.Attach(asicBase, asicSize, nil); err == nil {
		t.Fatal("a nil device must be refused")
	}
	if err := b.Attach(asicBase, asicSize, newProbe("")); err == nil {
		t.Fatal("an unnamed device must be refused: the name is the snapshot key")
	}
}

// An unmapped read answers zero because that is what this machine does and what every measurement
// in the record was taken against - the oracle's load() returns 0 and counts the site. Answering
// anything else, or faulting, would be a change to what a READ returns, which is the direction
// that has stopped this firmware booting before. What the bus adds is that the access is counted
// rather than silent.
func TestUnmappedReadAnswersZeroAndIsRecorded(t *testing.T) {
	t.Parallel()
	b := bus.New()
	attach(t, b, dramKseg0, dramSize, newProbe("dram"))

	if got := b.Read(asicBase+0x74, bus.Word); got != 0 {
		t.Fatalf("an unmapped read returned %#x, want 0", got)
	}
	b.Write(asicBase+0x74, bus.Word, 0x1234)
	b.Read(asicBase+0x74, bus.Word)

	sites := b.Unmapped()
	if len(sites) != 1 {
		t.Fatalf("the census holds %d sites, want 1: %+v", len(sites), sites)
	}
	got := sites[0]
	if got.Virtual != asicBase+0x74 {
		t.Fatalf("the census recorded virtual %#08x, want %#08x", got.Virtual, asicBase+0x74)
	}
	if got.Physical != 0x10000074 {
		t.Fatalf("the census recorded physical %#08x, want 0x10000074", got.Physical)
	}
	if got.Kind != bus.NoDevice {
		t.Fatalf("the census recorded kind %v, want NoDevice", got.Kind)
	}
	if got.Reads != 2 || got.Writes != 1 {
		t.Fatalf("the census counted %d reads and %d writes, want 2 and 1", got.Reads, got.Writes)
	}

	totals := b.UnmappedTotals()
	if totals.Reads != 2 || totals.Writes != 1 || totals.Sites != 1 || totals.Dropped != 0 {
		t.Fatalf("totals are %+v, want 2 reads, 1 write, 1 site, 0 dropped", totals)
	}
}

// An address in no unmapped segment at all is a different fact about the machine from an address
// in one with no device behind it: the first means the firmware went somewhere it has never gone,
// which in this project usually means a runaway PC. Spike 001's first measurement was worthless
// for exactly that reason and the tell was that the addresses did not exist on this board.
func TestAddressOutsideTheUnmappedSegmentsIsRecordedApart(t *testing.T) {
	t.Parallel()
	b := bus.New()
	attach(t, b, dramKseg0, dramSize, newProbe("dram"))

	for _, addr := range []uint32{0x00001000, 0x7FFFFFFC, 0xC0000000, 0xD10D59F0} {
		if got := b.Read(addr, bus.Word); got != 0 {
			t.Fatalf("read at %#08x returned %#x, want 0", addr, got)
		}
	}
	sites := b.Unmapped()
	if len(sites) != 4 {
		t.Fatalf("the census holds %d sites, want 4", len(sites))
	}
	for _, s := range sites {
		if s.Kind != bus.NoSegment {
			t.Fatalf("%#08x recorded as %v, want NoSegment", s.Virtual, s.Kind)
		}
	}
	// Sorted, so a census printed into a report reads in address order rather than map order.
	for i := 1; i < len(sites); i++ {
		if sites[i-1].Virtual >= sites[i].Virtual {
			t.Fatalf("the census is not sorted by address: %+v", sites)
		}
	}
}

// A runaway PC visits millions of distinct addresses. The census caps the sites it keeps so it
// cannot become the memory leak that ends the run, but the TOTALS must stay exact - a capped
// census that also stopped counting would under-report the very condition it exists to reveal.
func TestUnmappedCensusCapsSitesWithoutLosingTheTotals(t *testing.T) {
	t.Parallel()
	b := bus.New()
	const n = bus.MaxUnmappedSites + 500
	for i := 0; i < n; i++ {
		b.Read(uint32(0xB0000000+i*4), bus.Word)
	}
	totals := b.UnmappedTotals()
	if totals.Reads != n {
		t.Fatalf("totals counted %d reads, want %d", totals.Reads, n)
	}
	if totals.Sites != bus.MaxUnmappedSites {
		t.Fatalf("the census kept %d sites, want the cap of %d", totals.Sites, bus.MaxUnmappedSites)
	}
	if totals.Dropped != 500 {
		t.Fatalf("totals report %d dropped sites, want 500", totals.Dropped)
	}
}

func TestResetResetsEveryAttachedDevice(t *testing.T) {
	t.Parallel()
	b := bus.New()
	dram, asic := newProbe("dram"), newProbe("asic")
	attach(t, b, dramKseg0, dramSize, dram)
	attach(t, b, asicBase, asicSize, asic)

	b.Write(dramKseg0, bus.Word, 1)
	b.Write(asicBase, bus.Word, 1)
	b.Reset()

	if dram.words[0] != 0 || asic.words[0] != 0 {
		t.Fatal("Reset must reach every attached device")
	}
	if len(b.Unmapped()) != 0 || b.UnmappedTotals().Reads != 0 {
		t.Fatal("Reset must clear the unmapped census: it describes the run, not the machine")
	}
}

func TestSnapshotRoundTripsEveryDevice(t *testing.T) {
	t.Parallel()
	build := func() (*bus.Bus, *probe, *probe) {
		b := bus.New()
		dram, asic := newProbe("dram"), newProbe("asic")
		attach(t, b, dramKseg0, dramSize, dram)
		attach(t, b, asicBase, asicSize, asic)
		return b, dram, asic
	}

	saved, savedDram, savedAsic := build()
	saved.Write(dramKseg0+8, bus.Word, 0x80081C58)
	saved.Write(asicBase+0x10, bus.Half, 0xFFC0)
	state, err := saved.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	restored, restoredDram, restoredAsic := build()
	restored.Write(dramKseg0+8, bus.Word, 0xBADF00D)
	if err := restored.Restore(state); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restoredDram.words[2] != savedDram.words[2] {
		t.Fatalf("dram restored %#x, want %#x", restoredDram.words[2], savedDram.words[2])
	}
	if restoredAsic.words[4] != savedAsic.words[4] {
		t.Fatalf("asic restored %#x, want %#x", restoredAsic.words[4], savedAsic.words[4])
	}
	if restoredDram.writes != savedDram.writes {
		t.Fatalf("dram restored %d writes, want %d - the restore merged rather than replaced",
			restoredDram.writes, savedDram.writes)
	}
}

// Spike 003's whole point: a snapshot that is missing a device does not fail, it produces a
// plausible machine whose faults read as firmware bugs. So it has to be made to fail here.
func TestRestoreRefusesASnapshotMissingADevice(t *testing.T) {
	t.Parallel()
	partial := bus.New()
	attach(t, partial, dramKseg0, dramSize, newProbe("dram"))
	state, err := partial.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	full := bus.New()
	attach(t, full, dramKseg0, dramSize, newProbe("dram"))
	attach(t, full, asicBase, asicSize, newProbe("asic"))

	err = full.Restore(state)
	if err == nil {
		t.Fatal("restoring a snapshot that does not mention every attached device must be refused")
	}
	if !strings.Contains(err.Error(), "asic") {
		t.Fatalf("the refusal must name the device the snapshot is missing, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

func TestRestoreRefusesASnapshotNamingADeviceThatIsNotThere(t *testing.T) {
	t.Parallel()
	full := bus.New()
	attach(t, full, dramKseg0, dramSize, newProbe("dram"))
	attach(t, full, asicBase, asicSize, newProbe("asic"))
	state, err := full.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	partial := bus.New()
	attach(t, partial, dramKseg0, dramSize, newProbe("dram"))

	err = partial.Restore(state)
	if err == nil {
		t.Fatal("a snapshot naming a device this machine does not have must be refused, not " +
			"silently part-applied")
	}
	if !strings.Contains(err.Error(), "asic") {
		t.Fatalf("the refusal must name the unknown device, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

// A refusal that has already half-applied itself is worse than no refusal: the caller believes
// nothing happened and the machine is a mixture of two states.
func TestARefusedRestoreLeavesEveryDeviceUntouched(t *testing.T) {
	t.Parallel()
	saved := bus.New()
	attach(t, saved, dramKseg0, dramSize, newProbe("dram"))
	attach(t, saved, asicBase, asicSize, newProbe("asic"))
	saved.Write(dramKseg0, bus.Word, 0x11111111)
	saved.Write(asicBase, bus.Word, 0x22222222)
	state, err := saved.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// A machine with the same devices under one different name: every device but that one could
	// be restored, and must not be.
	live := bus.New()
	dram, other := newProbe("dram"), newProbe("demux")
	attach(t, live, dramKseg0, dramSize, dram)
	attach(t, live, asicBase, asicSize, other)
	live.Write(dramKseg0, bus.Word, 0x99999999)

	if err := live.Restore(state); err == nil {
		t.Fatal("the restore must be refused")
	}
	if dram.words[0] != 0x99999999 {
		t.Fatalf("dram was changed by a refused restore: %#x", dram.words[0])
	}
}

func TestRestoreRefusesAMalformedSnapshot(t *testing.T) {
	t.Parallel()
	good := bus.New()
	attach(t, good, dramKseg0, dramSize, newProbe("dram"))
	state, err := good.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	cases := map[string][]byte{
		"empty":                  {},
		"truncated to the magic": state[:4],
		"truncated mid-device":   state[:len(state)-3],
		"wrong magic":            append([]byte("XXXX"), state[4:]...),
	}
	for name, corrupt := range cases {
		name, corrupt := name, corrupt
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := bus.New()
			attach(t, b, dramKseg0, dramSize, newProbe("dram"))
			if err := b.Restore(corrupt); err == nil {
				t.Fatalf("a %s snapshot must be refused", name)
			} else {
				t.Logf("caught: %v", err)
			}
		})
	}
}

// The format carries its own version so that a snapshot taken by a later build is refused by name
// rather than decoded as though it were this one.
func TestRestoreRefusesAFutureFormatVersion(t *testing.T) {
	t.Parallel()
	b := bus.New()
	attach(t, b, dramKseg0, dramSize, newProbe("dram"))
	state, err := b.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	future := append([]byte(nil), state...)
	future[4] = 0xFF // the container version, a big-endian uint16 immediately after the magic

	if err := b.Restore(future); err == nil {
		t.Fatal("a snapshot in a format this build does not know must be refused")
	} else if !strings.Contains(err.Error(), "version") {
		t.Fatalf("the refusal must say it is a version problem, got: %v", err)
	} else {
		t.Logf("caught: %v", err)
	}
}

func TestPhysicalTranslatesTheSegmentsThisMachineUses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		virt  uint32
		phys  uint32
		valid bool
	}{
		{0x80000000, 0x00000000, true}, // DRAM, cached
		{0xA0000000, 0x00000000, true}, // DRAM, uncached - the same byte
		{0x80105C44, 0x00105C44, true}, // an address the record quotes
		{0xA0105C44, 0x00105C44, true}, // and its alias
		{0xBFC00000, 0x1FC00000, true}, // the reset vector
		{0x9FC00000, 0x1FC00000, true}, // and its cached mirror
		{0xB0000000, 0x10000000, true}, // the media ASIC on the external bus
		{0x9FFFFFFF, 0x1FFFFFFF, true}, // the top of KSEG0
		{0xBFFFFFFF, 0x1FFFFFFF, true}, // the top of KSEG1
		{0x00000000, 0, false},         // KUSEG: mapped, and this firmware never uses it
		{0x7FFFFFFF, 0, false},         // the top of KUSEG
		{0xC0000000, 0, false},         // KSEG2
		{0xFFFFFFFF, 0, false},         // KSEG3
	}
	for _, tc := range cases {
		got, ok := bus.Physical(tc.virt)
		if ok != tc.valid {
			t.Fatalf("Physical(%#08x) reported ok=%v, want %v", tc.virt, ok, tc.valid)
		}
		if tc.valid && got != tc.phys {
			t.Fatalf("Physical(%#08x) = %#08x, want %#08x", tc.virt, got, tc.phys)
		}
	}
}

// brittle refuses the next few restores and then behaves, which is the shape of a device that
// rejects an incoming snapshot but is perfectly able to take its own state back. It exists to
// reach the rollback path: the name sets match, so the refusal happens after the bus has already
// restored the devices ahead of it.
type brittle struct {
	probe
	refusals int
}

func (d *brittle) Restore(state []byte) error {
	if d.refusals > 0 {
		d.refusals--
		return fmt.Errorf("brittle: refusing a snapshot it does not recognise")
	}
	return d.probe.Restore(state)
}

// The rollback path: a device that refuses its blob after earlier devices have been restored must
// leave the machine where it was, not part-way between two states. A caller told the restore
// failed will carry on running the machine, and a machine that is a mixture of two states diverges
// from both while looking entirely healthy.
func TestARestoreRefusedPartWayThroughRollsEveryDeviceBack(t *testing.T) {
	t.Parallel()
	// "asic" sorts before "dram", so the bus reaches the brittle device second and has already
	// restored the other one by then.
	saved := bus.New()
	attach(t, saved, asicBase, asicSize, newProbe("asic"))
	attach(t, saved, dramKseg0, dramSize, &brittle{probe: *newProbe("dram")})
	saved.Write(asicBase, bus.Word, 0x11111111)
	saved.Write(dramKseg0, bus.Word, 0x22222222)
	state, err := saved.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	live := bus.New()
	asic := newProbe("asic")
	dram := &brittle{probe: *newProbe("dram"), refusals: 1}
	attach(t, live, asicBase, asicSize, asic)
	attach(t, live, dramKseg0, dramSize, dram)
	live.Write(asicBase, bus.Word, 0x99999999)
	live.Write(dramKseg0, bus.Word, 0x88888888)

	err = live.Restore(state)
	if err == nil {
		t.Fatal("a device that refuses its own blob must fail the restore")
	}
	if !strings.Contains(err.Error(), "dram") {
		t.Fatalf("the failure must name the device that refused, got: %v", err)
	}
	t.Logf("caught: %v", err)

	if asic.words[0] != 0x99999999 {
		t.Fatalf("asic was restored and not rolled back: %#x, want 0x99999999", asic.words[0])
	}
	if dram.words[0] != 0x88888888 {
		t.Fatalf("dram changed despite refusing: %#x, want 0x88888888", dram.words[0])
	}
}

// And when the rollback ITSELF fails there is no state left to return to. The machine must then
// say so and keep saying so, rather than hand out a snapshot of a state that never existed - and
// it must not take the process down doing it, because a run that cannot report what it found is
// worse than one that reports bad news.
func TestAMachineThatCouldNotBeRolledBackRefusesToBeUsed(t *testing.T) {
	t.Parallel()
	saved := bus.New()
	attach(t, saved, asicBase, asicSize, newProbe("asic"))
	attach(t, saved, dramKseg0, dramSize, &brittle{probe: *newProbe("dram")})
	saved.Write(asicBase, bus.Word, 0x11111111)
	state, err := saved.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	live := bus.New()
	attach(t, live, asicBase, asicSize, newProbe("asic"))
	// Refuses the restore and then refuses the rollback too.
	attach(t, live, dramKseg0, dramSize, &brittle{probe: *newProbe("dram"), refusals: 2})

	err = live.Restore(state)
	if err == nil {
		t.Fatal("the restore must fail")
	}
	if !strings.Contains(err.Error(), "no known state") {
		t.Fatalf("the failure must say the machine is in no known state, got: %v", err)
	}
	t.Logf("caught: %v", err)

	if _, err := live.Snapshot(); err == nil {
		t.Fatal("a machine in no known state must refuse to be snapshotted: writing one out is " +
			"how a state that never existed becomes a baseline")
	}
	if err := live.Restore(state); err == nil {
		t.Fatal("and it must refuse another restore rather than layering onto the wreckage")
	}

	live.Reset()
	if _, err := live.Snapshot(); err != nil {
		t.Fatalf("a Reset returns the machine to a state that IS known, so it must be usable "+
			"again afterwards: %v", err)
	}
}

// The one panic this package keeps, and it is kept deliberately.
//
// An invalid bus.Size cannot come from the firmware: only this program's own CPU constructs one,
// from this program's own constants. So it is a bug in OUR code, and the alternative - routing it
// through the CPU's guest halt path - would disguise our bug as a firmware fault, which is this
// project's single worst failure mode. "A wrong register model is a boot that stops at 20 tasks";
// an emulator bug wearing a firmware fault's clothing costs days.
//
// It is tested because an unreachable panic nobody has watched is still an untested branch, and a
// panic whose message does not name the offending value is a stack trace that sends the reader
// looking in the wrong place.
func TestAnInvalidAccessWidthPanicsAndNamesItself(t *testing.T) {
	t.Parallel()
	const bad = bus.Size(3) // not Byte, Half or Word

	cases := map[string]func(b *bus.Bus){
		"read":  func(b *bus.Bus) { b.Read(asicBase+0x74, bad) },
		"write": func(b *bus.Bus) { b.Write(asicBase+0x74, bad, 0x1234) },
	}
	for name, access := range cases {
		name, access := name, access
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := bus.New()
			attach(t, b, asicBase, asicSize, newProbe("asic"))

			var got any
			func() {
				defer func() { got = recover() }()
				access(b)
			}()

			if got == nil {
				t.Fatalf("a %s at an invalid width must panic: it is a bug in our own code, and "+
					"silently reading some other width is the plausible wrongness this machine "+
					"does not report", name)
			}
			msg, ok := got.(string)
			if !ok {
				t.Fatalf("the panic value is %T, want a string a reader can act on: %v", got, got)
			}
			// The offending value AND where it happened, or the reader is left with a stack
			// trace and no idea which access was malformed.
			// Case-SENSITIVE on the address: the canonical form is hexfmt's upper case, and a
			// message that rendered 0xb0000074 would be the second casing this project spent a
			// day on. Checking it case-insensitively here would let that back in.
			for _, want := range []string{"bus:", name, "invalid size", "3"} {
				if !strings.Contains(msg, want) {
					t.Fatalf("the panic message must name %q; got %q", want, msg)
				}
			}
			if !strings.Contains(msg, hexfmt.Addr(asicBase+0x74)) {
				t.Fatalf("the panic message must name the address in the canonical %s form; "+
					"got %q", hexfmt.Addr(asicBase+0x74), msg)
			}
			t.Logf("caught: %s", msg)
		})
	}
}

// And the widths that ARE valid must not panic - or the check above could be satisfied by a bus
// that panics on everything, which would pass while examining nothing.
func TestValidAccessWidthsDoNotPanic(t *testing.T) {
	t.Parallel()
	b := bus.New()
	attach(t, b, asicBase, asicSize, newProbe("asic"))

	for _, size := range []bus.Size{bus.Byte, bus.Half, bus.Word} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("a %s access panicked: %v", size, r)
				}
			}()
			b.Read(asicBase+0x74, size)
			b.Write(asicBase+0x74, size, 0x1234)
		}()
	}
}
