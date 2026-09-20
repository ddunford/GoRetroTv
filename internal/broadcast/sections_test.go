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
	sdt, err := SDT(0x20, 0x20, 0, []Service{sampleService()})
	if err != nil {
		t.Fatal(err)
	}
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
		{"SDT", sdt, "42b0220020c100000020ff0064fd8011480f010542536b794207536b79204f6e656121a203", true},
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
	sdt, err := SDT(0x20, 0x20, 0, []Service{sampleService()})
	if err != nil {
		t.Fatal(err)
	}
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
	sdt, err := SDT(0x4567, 0x89ab, 7, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sdt[3:6], []byte{0x45, 0x67, 0xcf}) || !bytes.Equal(sdt[8:10], []byte{0x89, 0xab}) {
		t.Fatalf("SDT IDs/version = %x", sdt)
	}
	noPresent, err := SDT(1, 1, 0, []Service{{ID: 100, Name: "Test", NoEITPresent: true}})
	if err != nil {
		t.Fatal(err)
	}
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
	section, err := BAT(0x1000, 3, "Sky", []Transport{transport})
	if err != nil {
		t.Fatal(err)
	}
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
