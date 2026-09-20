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
