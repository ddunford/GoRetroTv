package broadcast

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/demux"
	"github.com/ddunford/goretrotv/internal/dvb"
	"github.com/ddunford/goretrotv/internal/memory"
)

var jan1998 = time.Date(1998, 1, 1, 12, 0, 0, 0, time.UTC)

func sampleService() Service {
	return Service{ID: 100, Name: "Sky One"}
}

func sampleTransport() Transport {
	return Transport{ID: 0x20, NetworkID: 0x20, FrequencyMHz: 11778, OrbitTenths: 282,
		SymbolRate: 27500, FEC: 2, Services: []Service{sampleService()}}
}

// These fixtures were produced by executing the untouched browser oracle's
// __siNIT/__siSDT/__siTDT/__siTOT builders with the same explicit inputs.
func TestOracleSectionVectors(t *testing.T) {
	t.Parallel()
	nit, err := NIT(0x20, 0, "Sky Digital", []Transport{sampleTransport()})
	if err != nil {
		t.Fatal(err)
	}
	sdtSections, err := SDT(0x20, 0x20, 0, []Service{sampleService()})
	sdt := oneSection(t, sdtSections, err)
	tdt, err := TDT(jan1998)
	if err != nil {
		t.Fatal(err)
	}
	tot, err := TOT(jan1998, TimeOffset{Country: "GBR", ChangeUTC: jan1998})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		got  []byte
		want string
		crc  bool
	}{
		{"NIT", nit, "40b0320020c10000f00d400b536b79204469676974616cf01800200020f012430b01177800028281002750024103006401b953f16a", true},
		// THE SDT IS THIS PORT'S, NOT THE ORACLE'S, AND THE DIFFERENCE IS DELIBERATE. It carries a
		// 5f 04 00 00 00 02 private_data_specifier at the head of each service's descriptor loop,
		// which the oracle's builder does not emit. TestTheSDTDivergesByExactlyTheSpecifier below
		// proves that is the ONLY difference, so the oracle keeps its value as an instrument.
		{"SDT", sdt, "42b0280020c100000020ff0064fd80175f0400000002480f010542536b794207536b79204f6e6542e5c7f4", true},
		{"TDT", tdt, "707005c67e120000", false},
		{"TOT", tot, "73701ac67e120000f00f580d474252020000c67e12000000005b445d38", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want, err := hex.DecodeString(tc.want)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(tc.got, want) {
				t.Fatalf("section = %x, oracle = %x", tc.got, want)
			}
			length := int(tc.got[1]&0x0f)<<8 | int(tc.got[2])
			if length+3 != len(tc.got) {
				t.Fatalf("declared length %d, actual %d", length, len(tc.got)-3)
			}
			if tc.crc && dvb.MPEGCRC32(tc.got) != 0 {
				t.Fatalf("whole-section CRC = %#x", dvb.MPEGCRC32(tc.got))
			}
		})
	}
}

func TestSectionsReachArmedDemuxFilters(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	d := demux.New()
	if err := d.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	nit, err := NIT(0x20, 0, "Sky Digital", []Transport{sampleTransport()})
	if err != nil {
		t.Fatal(err)
	}
	sdtSections, err := SDT(0x20, 0x20, 0, []Service{sampleService()})
	sdt := oneSection(t, sdtSections, err)
	tdt, err := TDT(jan1998)
	if err != nil {
		t.Fatal(err)
	}
	tot, err := TOT(jan1998, TimeOffset{Country: "GBR", ChangeUTC: jan1998})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		filter  uint8
		pid     uint16
		section []byte
	}{
		{24, 0x10, nit}, {23, 0x11, sdt}, {22, 0x14, tdt}, {25, 0x14, tot},
	} {
		d.Reset()
		d.Write(0xd8, bus.Word, 1<<tc.filter)
		d.Write(0x14+4*uint32(tc.filter), bus.Word, uint32(tc.pid))
		if err := d.Push(tc.pid, tc.section); err != nil {
			t.Fatalf("table %#x rejected by demux: %v", tc.section[0], err)
		}
		if got := d.Read(0xb8, bus.Word); got != 1<<tc.filter {
			t.Fatalf("table %#x reached status %#x, want filter %d", tc.section[0], got, tc.filter)
		}
		if tc.section[0] == 0x40 {
			broken := append([]byte(nil), tc.section...)
			broken[4] ^= 1
			if err := d.Push(tc.pid, broken); err == nil {
				t.Fatal("demux accepted corrupted NIT CRC")
			}
		}
	}
}

func TestDateAndOffsetEncoding(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		date time.Time
		mjd  []byte
	}{
		{time.Date(1858, 11, 17, 12, 0, 0, 0, time.UTC), []byte{0, 0}},
		{time.Date(1970, 1, 1, 12, 0, 0, 0, time.UTC), []byte{0x9e, 0x8b}},
		{time.Date(1998, 1, 2, 12, 0, 0, 0, time.UTC), []byte{0xc6, 0x7f}},
	} {
		section, err := TDT(tc.date)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(section[3:5], tc.mjd) {
			t.Fatalf("MJD of %s = %x, want %x", tc.date, section[3:5], tc.mjd)
		}
	}
	section, err := TOT(jan1998, TimeOffset{Country: "GBR", Region: 3,
		OffsetMinutes: -90, ChangeUTC: jan1998, NextOffsetMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(section[12:18], []byte{0x47, 0x42, 0x52, 0x0f, 0x01, 0x30}) {
		t.Fatalf("TOT country/region/polarity/offset = %x", section[12:18])
	}
	if !bytes.Equal(section[len(section)-6:len(section)-4], []byte{0x01, 0x00}) {
		t.Fatalf("TOT next offset = %x", section[len(section)-6:len(section)-4])
	}
	if dvb.MPEGCRC32(section) != 0 {
		t.Fatal("offset change damaged TOT CRC")
	}
}

func TestIdsAndVersionAreCallersInputs(t *testing.T) {
	t.Parallel()
	nit, err := NIT(0x1234, 7, "Network", []Transport{{ID: 0x4567, NetworkID: 0x89ab,
		FrequencyMHz: 11778, OrbitTenths: 282, SymbolRate: 27500, FEC: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(nit[3:6], []byte{0x12, 0x34, 0xcf}) {
		t.Fatalf("NIT ID/version = %x", nit[3:6])
	}
	if !bytes.Contains(nit, []byte{0x45, 0x67, 0x89, 0xab}) {
		t.Fatalf("transport IDs absent from NIT: %x", nit)
	}
	sdtSections, err := SDT(0x4567, 0x89ab, 7, nil)
	sdt := oneSection(t, sdtSections, err)
	if !bytes.Equal(sdt[3:6], []byte{0x45, 0x67, 0xcf}) || !bytes.Equal(sdt[8:10], []byte{0x89, 0xab}) {
		t.Fatalf("SDT IDs/version = %x", sdt)
	}
	noPresentSections, err := SDT(1, 1, 0, []Service{{ID: 100, Name: "Test", NoEITPresent: true}})
	noPresent := oneSection(t, noPresentSections, err)
	if noPresent[13] != 0xfc {
		t.Fatalf("explicitly disabled present/following flag = %#x", noPresent[13])
	}
}

func TestInvalidSectionInputs(t *testing.T) {
	t.Parallel()
	long := bytes.Repeat([]byte{'x'}, 1020)
	tooMany := make([]Transport, 70)
	for i := range tooMany {
		tooMany[i] = sampleTransport()
	}
	for _, tc := range []struct {
		name  string
		build func() error
	}{
		{"long NIT", func() error { _, err := NIT(1, 0, "network", tooMany); return err }},
		{"non-ASCII", func() error { _, err := SDT(1, 1, 0, []Service{{Name: "Café"}}); return err }},
		{"long name", func() error { _, err := SDT(1, 1, 0, []Service{{Name: string(long)}}); return err }},
		{"bad version", func() error { _, err := SDT(1, 1, 32, nil); return err }},
		{"bad date", func() error { _, err := TDT(time.Time{}); return err }},
		{"bad country", func() error { _, err := TOT(jan1998, TimeOffset{Country: "gb", ChangeUTC: jan1998}); return err }},
		{"bad frequency", func() error {
			tr := sampleTransport()
			tr.FrequencyMHz = -1
			_, err := NIT(1, 0, "x", []Transport{tr})
			return err
		}},
		{"bad region", func() error {
			_, err := TOT(jan1998, TimeOffset{Country: "GBR", Region: 64, ChangeUTC: jan1998})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.build(); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}

func sampleLineup() []LineupEntry {
	return []LineupEntry{
		{ServiceID: 0x0064, Kind: 1, Listings: 0x0bb8, Extra: 0x1770, Channel: 0x0abc, Flags: 0b0101},
		{ServiceID: 0x0065, Kind: 1, Listings: 0x0bb9, Extra: 0x1771, Channel: 0x0abd, Flags: 0b0101},
	}
}

// Produced by executing the untouched browser oracle's own lineupDescriptors()
// with these inputs, so the Go builder is pinned to the bytes a box has already
// been measured accepting rather than to a reading of the layout.
func TestOracleLineupDescriptorVector(t *testing.T) {
	t.Parallel()
	got, err := lineupDescriptors(sampleLineup())
	if err != nil {
		t.Fatal(err)
	}
	const want = "5f0400000002b114ffff0064010bb81770abc50065010bb91771abd5"
	if hex.EncodeToString(got) != want {
		t.Fatalf("lineup descriptors\n got %s\nwant %s", hex.EncodeToString(got), want)
	}
}

// The specifier comes first and the gate is the sentinel. This pins the shape
// the oracle emits and the one DVB defines — a private_data_specifier scopes
// what FOLLOWS it in the same loop — and not a requirement of this box, which
// was measured 2026-09-20 decoding the line-up just as happily with the 0x5F
// moved after the 0xB1. The record and the oracle's own comment both say the
// specifier must lead; on this firmware the VALUE is load-bearing and the
// position is not. Emit the correct order anyway: being right by accident on
// one box is not a reason to broadcast a malformed loop.
func TestLineupDeclaresItsNamespaceBeforeThePrivateTag(t *testing.T) {
	t.Parallel()
	got, err := lineupDescriptors(sampleLineup())
	if err != nil {
		t.Fatal(err)
	}
	specifier := bytes.Index(got, []byte{0x5f, 4, 0, 0, 0, 2})
	private := bytes.IndexByte(got, 0xb1)
	if specifier != 0 {
		t.Fatalf("specifier at %d, want first", specifier)
	}
	if private < specifier {
		t.Fatalf("private tag at %d precedes its namespace at %d", private, specifier)
	}
	if got[private+2] != 0xff || got[private+3] != 0xff {
		t.Fatalf("gate = %#x %#x, want the measured 0xFFFF sentinel", got[private+2], got[private+3])
	}
}

// A byte-long length field holds the gate plus twenty-eight nine-byte entries,
// so a real Sky line-up spans several descriptors. The guest appends them into
// one array because its running index lives in the transport context, and one
// declared namespace covers the rest of the loop.
func TestLineupSplitsAcrossDescriptorsAndDeclaresTheNamespaceOnce(t *testing.T) {
	t.Parallel()
	lineup := make([]LineupEntry, maxLineupEntries+1)
	for i := range lineup {
		lineup[i] = LineupEntry{ServiceID: uint16(i), Listings: uint16(i), Channel: uint16(i)}
	}
	got, err := lineupDescriptors(lineup)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(got, []byte{0x5f, 4, 0, 0, 0, 2}); n != 1 {
		t.Fatalf("declared the namespace %d times, want once", n)
	}
	entries, descriptors := 0, 0
	for i := 6; i < len(got); {
		if got[i] != 0xb1 {
			t.Fatalf("unexpected tag %#x at %d", got[i], i)
		}
		length := int(got[i+1])
		if length > 255 || (length-2)%9 != 0 {
			t.Fatalf("descriptor length %d is not a gate plus whole entries", length)
		}
		if got[i+2] != 0xff || got[i+3] != 0xff {
			t.Fatal("a split descriptor lost its gate")
		}
		descriptors++
		entries += (length - 2) / 9
		i += 2 + length
	}
	if descriptors != 2 || entries != len(lineup) {
		t.Fatalf("%d descriptors carrying %d entries, want 2 carrying %d", descriptors, entries, len(lineup))
	}
}

// Twelve bits and four bits. Masking silently would ship a channel number that
// is simply a different channel, which is the class of failure this whole file
// exists to refuse.
func TestLineupRefusesFieldsThatDoNotFit(t *testing.T) {
	t.Parallel()
	if _, err := lineupDescriptors([]LineupEntry{{Channel: 0x1000}}); err == nil {
		t.Error("accepted a channel wider than twelve bits")
	}
	if _, err := lineupDescriptors([]LineupEntry{{Flags: 0x10}}); err == nil {
		t.Error("accepted flags wider than four bits")
	}
}

func TestBATFramesTheBouquetAndItsTransport(t *testing.T) {
	t.Parallel()
	transport := sampleTransport()
	transport.Lineup = sampleLineup()
	sectionSections, err := BAT(0x1000, 3, "Sky", []Transport{transport})
	section := oneSection(t, sectionSections, err)
	if section[0] != 0x4a {
		t.Fatalf("table id %#x, want 0x4a", section[0])
	}
	if got := uint16(section[3])<<8 | uint16(section[4]); got != 0x1000 {
		t.Fatalf("bouquet id %#x, want 0x1000", got)
	}
	if dvb.MPEGCRC32(section) != 0 {
		t.Fatal("BAT CRC does not verify")
	}
	if declared := int(section[1]&0x0f)<<8 | int(section[2]); declared+3 != len(section) {
		t.Fatalf("section_length %d does not describe %d bytes", declared, len(section))
	}
	// The line-up rides in the TRANSPORT loop, because the guest seeds each
	// record it builds from the transport the descriptor arrived on.
	bouquetLen := int(section[8]&0x0f)<<8 | int(section[9])
	if bytes.Contains(section[10:10+bouquetLen], []byte{0xb1}) {
		t.Fatal("the line-up is in the bouquet loop, where the transport context is not available")
	}
	if !bytes.Contains(section[10+bouquetLen:], []byte{0x5f, 4, 0, 0, 0, 2}) {
		t.Fatal("the transport loop does not declare the private namespace")
	}
}

// The guide asks one question and this is it, so the descriptor's bytes are
// the firmware's own field offsets rather than a public table's: desc[2..3]
// tsid, desc[4..5] onid, desc[6..7] service_id, desc[8] linkage_type.
func TestLinkageCarriesTheFieldsTheGuideReads(t *testing.T) {
	t.Parallel()
	got, err := linkageDescriptor(Transport{ID: 0x1234, NetworkID: 0x0020,
		Services: []Service{{ID: 0x0064}, {ID: 0x0065}}})
	if err != nil {
		t.Fatal(err)
	}
	const want = "4a0712340020006491"
	if hex.EncodeToString(got) != want {
		t.Fatalf("linkage\n got %s\nwant %s", hex.EncodeToString(got), want)
	}
}

// In BOTH loops. Which structure the guide's search walks is not established,
// and the measurement that proved the mechanism put it in both; in the
// transport loop alone it changed nothing.
func TestBATCarriesTheLinkageInBothDescriptorLoops(t *testing.T) {
	t.Parallel()
	transport := sampleTransport()
	transport.Lineup = sampleLineup()
	sectionSections, err := BAT(0x1000, 3, "Sky", []Transport{transport})
	section := oneSection(t, sectionSections, err)
	bouquetLen := int(section[8]&0x0f)<<8 | int(section[9])
	bouquetLoop := section[10 : 10+bouquetLen]
	transportLoop := section[10+bouquetLen:]
	linkage := []byte{0x4a, 7}
	if !bytes.Contains(bouquetLoop, linkage) {
		t.Error("the bouquet loop carries no linkage descriptor")
	}
	if !bytes.Contains(transportLoop, linkage) {
		t.Error("the transport loop carries no linkage descriptor")
	}
	if n := bytes.Count(section, []byte{0x4a, 7}); n != 2 {
		t.Fatalf("%d linkage descriptors, want one per loop", n)
	}
	// The type is the whole point: the caller requires 0x91 and takes its
	// not-answered arm for anything else.
	for i := 0; i+8 < len(section); i++ {
		if section[i] == 0x4a && section[i+1] == 7 && section[i+8] != 0x91 {
			t.Fatalf("linkage_type %#x at %d, want 0x91", section[i+8], i)
		}
	}
}

// The linkage must name a service the same BAT declares. Emitting a plausible
// default would answer the guide with a service that does not exist.
func TestBATRefusesATransportWithNoServiceToLink(t *testing.T) {
	t.Parallel()
	if _, err := BAT(0x1000, 0, "Sky", []Transport{{ID: 1, NetworkID: 2}}); err == nil {
		t.Error("built a BAT whose linkage names nothing")
	}
	if _, err := BAT(0x1000, 0, "Sky", nil); err == nil {
		t.Error("built a BAT announcing no transport at all")
	}
}

// THE SDT DIVERGES FROM THE ORACLE BY EXACTLY ONE DESCRIPTOR, AND THIS PROVES IT IS ONLY THAT.
//
// The oracle is a measuring instrument, not a sibling implementation, and the one move that
// destroys its value is editing it to agree with this port. The opposite move -- letting this port
// drift from it silently -- costs the same thing, so a deliberate divergence has to be stated and
// bounded rather than merely allowed.
//
// WHY THE DIVERGENCE EXISTS. Every service this port announces now carries a
// private_data_specifier (0x5F, value 2) at the head of its descriptor loop. The oracle's __siSDT
// emits none, and its ALL CHANNELS grid is empty too -- which is precisely the shape the project's
// own rule warns about: "where both are wrong in the same way they agree".
//
// It was found by measurement, not by reading a standard. The grid's row callback at 0x800CB7B8
// asks its object for descriptor tag 0x5F, and a census of every tag lookup both screens make had
// 0x4A and 0x4D answering non-zero while 0x5F answered ZERO on seven attempts out of seven. With
// the specifier transmitted, the same callback gets 1 back, goes on to the 0xB2 lookup it had never
// reached, and runs 1284 instructions instead of 580.
//
// WHAT THIS TEST GUARANTEES. Take the oracle's own SDT bytes, splice the six-byte specifier in at
// the head of the service's descriptor loop, widen both length fields by six and recompute the CRC
// -- and the result must be byte-for-byte what this port transmits. If anything else about the SDT
// ever drifts from the oracle, this fails.
func TestTheSDTDivergesByExactlyTheSpecifier(t *testing.T) {
	t.Parallel()
	const oracleSDT = "42b0220020c100000020ff0064fd8011480f010542536b794207536b79204f6e656121a203"
	want, err := hex.DecodeString(oracleSDT)
	if err != nil {
		t.Fatal(err)
	}
	// The oracle's layout: 11 bytes of section header, then the service's 3-byte header (id and
	// flags) and its 2-byte descriptors_loop_length. The descriptors begin at 16.
	const (
		sectionLengthAt = 1
		loopLengthAt    = 14
		descriptorsAt   = 16
	)
	specifier := []byte{0x5f, 4, 0x00, 0x00, 0x00, skyPrivateDataSpecifier}

	spliced := make([]byte, 0, len(want)+len(specifier))
	spliced = append(spliced, want[:descriptorsAt]...)
	spliced = append(spliced, specifier...)
	spliced = append(spliced, want[descriptorsAt:len(want)-4]...) // the oracle's CRC is recomputed
	widen := func(at int) {
		v := int(spliced[at]&0x0f)<<8 | int(spliced[at+1])
		v += len(specifier)
		spliced[at] = spliced[at]&0xf0 | byte(v>>8) // #nosec G115 -- masked to four bits
		spliced[at+1] = byte(v)                     // #nosec G115 -- low byte
	}
	widen(sectionLengthAt)
	widen(loopLengthAt)
	spliced = withCRC(spliced)

	gotSections, err := SDT(0x20, 0x20, 0, []Service{sampleService()})
	got := oneSection(t, gotSections, err)
	if !bytes.Equal(got, spliced) {
		t.Fatalf("this port's SDT is not the oracle's plus the specifier and nothing else:\n"+
			"  ours    = %x\n  expected = %x", got, spliced)
	}
}

// THE INDEX SECTION IS CHECKED AGAINST THE CONSUMER'S OWN ARITHMETIC.
//
// A builder tested against what its author meant is a builder that agrees with itself. The box's
// parser at 0x800C4C34 states the format in code: it takes the twelve-bit section length,
// SUBTRACTS NINE and DIVIDES BY NINE to get the record count, and it walks from section+8. So the
// test worth writing is that recomputing the count the way the firmware does gives back the number
// of records that went in -- and that the walk lands on each record's id.
func TestIndexSectionMatchesTheConsumersArithmetic(t *testing.T) {
	t.Parallel()
	records := []IndexRecord{
		{ID: 0xBEEF, Packed: 0x5a, Selector: 0xc0, Data: [5]byte{1, 2, 3, 4, 5}},
		{ID: 0xCAFE},
		{ID: 0xF00D, Data: [5]byte{9, 8, 7, 6, 5}},
	}
	letter, err := IndexLetter('M')
	if err != nil {
		t.Fatal(err)
	}
	section, err := IndexSection(letter, 0, 0, 0, records)
	if err != nil {
		t.Fatal(err)
	}
	if section[0] != 0xc1 {
		t.Fatalf("table id = %#02x, want 0xC1", section[0])
	}
	if got := uint16(section[3])<<8 | uint16(section[4]); got != 'M' {
		t.Fatalf("extension = %#04x, want %#04x for 'M'", got, uint16('M'))
	}
	// The firmware's own count, computed the firmware's own way.
	length := int(section[1]&0x0f)<<8 | int(section[2])
	if length+3 != len(section) {
		t.Fatalf("declared length %d, actual %d", length, len(section)-3)
	}
	if count := (length - 9) / 9; count != len(records) {
		t.Fatalf("the consumer would read %d records from this section, not the %d it carries",
			count, len(records))
	}
	// And the walk, from section+8, nine bytes at a time.
	for i, want := range records {
		at := 8 + i*9
		if got := uint16(section[at])<<8 | uint16(section[at+1]); got != want.ID {
			t.Fatalf("record %d id = %#04x at offset %d, want %#04x", i, got, at, want.ID)
		}
	}
	if dvb.MPEGCRC32(section) != 0 {
		t.Fatalf("whole-section CRC = %#x", dvb.MPEGCRC32(section))
	}
}

// EVERY EXTENSION THE CONSUMER DISPATCHES, AND NOTHING ELSE.
//
// The parser allocates and fills the record array BEFORE it branches on the extension, so a
// mis-addressed section is decoded and then leaked -- indistinguishable from silence at the screen.
// The accepted set is 0x0000, 'A'..'Z', 0x00FF and 0x0100..0x01CF, and this pins all four arms
// because an earlier reading of the same parser admitted only the letters and would have refused
// the sixty-four genre extensions the box actually subscribes to.
func TestIndexSectionAcceptsExactlyWhatTheConsumerDispatches(t *testing.T) {
	t.Parallel()
	for _, good := range []uint16{0x0000, 'A', 'M', 'Z', 0x0100, 0x010f, 0x0140, 0x018f, 0x01cf, 0x00ff} {
		if !IndexDispatched(good) {
			t.Fatalf("IndexDispatched(%#04x) = false, but the consumer routes it", good)
		}
		if _, err := IndexSection(good, 0, 0, 0, []IndexRecord{{ID: 1}}); err != nil {
			t.Fatalf("IndexSection(%#04x) was refused: %v", good, err)
		}
	}
	for _, bad := range []uint16{0x0001, '@', '[', 'a', 0x00fe, 0x01d0, 0x0200, 0xffff} {
		if IndexDispatched(bad) {
			t.Fatalf("IndexDispatched(%#04x) = true, but the consumer falls through and frees "+
				"the list it built", bad)
		}
		if _, err := IndexSection(bad, 0, 0, 0, []IndexRecord{{ID: 1}}); err == nil {
			t.Fatalf("IndexSection(%#04x) was accepted; the consumer would decode and leak it", bad)
		}
	}
}

// THE GENRE EXTENSION IS SIXTEEN CATEGORIES BY FOUR BLOCKS, AND THE SLOT ARITHMETIC IS THE BOX'S.
//
// 0x800C4C34 picks its list head with `(ext & 0x0F) * 4 + ((ext & 0xC0) >> 6)`. Recomputing that
// from every extension IndexCategory builds must land on each of the sixty-four slots exactly once
// -- if two categories aliased onto one slot, one genre screen would quietly overwrite another.
func TestEveryGenreExtensionLandsOnItsOwnSlot(t *testing.T) {
	t.Parallel()
	seen := map[int][]uint16{}
	for category := byte(0); category < 16; category++ {
		for block := byte(0); block < 4; block++ {
			ext, err := IndexCategory(category, block)
			if err != nil {
				t.Fatal(err)
			}
			if !IndexDispatched(ext) {
				t.Fatalf("IndexCategory(%d,%d) = %#04x, which the consumer does not dispatch",
					category, block, ext)
			}
			slot := int(ext&0x0f)*4 + int(ext&0xc0)>>6
			seen[slot] = append(seen[slot], ext)
		}
	}
	if len(seen) != 64 {
		t.Fatalf("the sixty-four category/block pairs land on %d distinct slots", len(seen))
	}
	for slot, exts := range seen {
		if len(exts) != 1 {
			t.Fatalf("slot %d is claimed by %d extensions %#04x -- one genre screen would "+
				"overwrite another", slot, len(exts), exts)
		}
	}
	if _, err := IndexCategory(16, 0); err == nil {
		t.Fatal("category 16 was accepted, but the consumer masks the extension's low nibble")
	}
	if _, err := IndexCategory(0, 4); err == nil {
		t.Fatal("block 4 was accepted, but a day is four six-hour blocks")
	}
}

// oneSection unwraps a table that this fixture expects to be a single section.
//
// SDT and BAT return TABLES now -- section_number 0 through last_section_number -- because a real
// line-up does not fit in 1021 bytes. The fixtures below carry a channel or two and genuinely are
// one section, so this states that expectation instead of indexing [0] and hoping.
func oneSection(t *testing.T, sections [][]byte, err error) []byte {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 {
		t.Fatalf("this fixture was expected to fit one section and built %d; a caller that puts "+
			"only the first on air would drop the rest", len(sections))
	}
	return sections[0]
}
