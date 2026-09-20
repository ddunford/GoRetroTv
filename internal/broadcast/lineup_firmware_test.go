package broadcast_test

import (
	"encoding/binary"
	"testing"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/dvb"
)

// The guest's channel-list parser, read off the firmware in record sky-eluc.38.
// Each address runs once per decoded entry, so a hit count IS the entry count —
// which is what makes "did it decode N services" answerable without finding the
// record array in DRAM.
const (
	pcEntryServiceID = 0x800bf826 // reads entry +0..1
	pcRecordStride   = 0x800bf814 // mult by 0x12: eighteen bytes per record
	pcFlagBit3       = 0x800bf860 // (packed & 8) >> 3 -> record[13]
	pcFlagBit2       = 0x800bf86c // (packed & 4) >> 2 -> record[14]
	pcFlagBit1       = 0x800bf878 // (packed & 2) >> 1 -> record[15]
	pcFlagBit0       = 0x800bf896 // (packed & 1)      -> record[16]
)

// breakTheGate rewrites the 0xB1 descriptor's sentinel halfword and repairs the
// CRC, so the section stays well-formed and only the gate is wrong. The builder
// has no option for this on purpose: a knob for emitting a broken gate would be
// production code that exists only to be misused.
func breakTheGate(t *testing.T, section []byte, gate uint16) []byte {
	t.Helper()
	broken := append([]byte(nil), section...)
	found := false
	for i := 0; i+3 < len(broken)-4; i++ {
		if broken[i] == 0xb1 && broken[i+2] == 0xff && broken[i+3] == 0xff {
			binary.BigEndian.PutUint16(broken[i+2:], gate)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no 0xB1 descriptor with an intact gate to break: the fixture is wrong, not the guest")
	}
	body := broken[:len(broken)-4]
	return binary.BigEndian.AppendUint32(body, dvb.MPEGCRC32(body))
}

// TC-6.2. The line-up only decodes when three measured conditions hold at once,
// so each is falsified here rather than assumed: the private namespace must be
// declared with specifier 2, the descriptor's gate halfword must be 0xFFFF, and
// the entries must be nine bytes. Get any of them wrong and the box accepts the
// section and decodes nothing, reporting no error at all — which is exactly why
// the negative cases are part of the test and not a footnote.
func TestGuestDecodesTheLineupOnlyThroughTheMeasuredGate(t *testing.T) {
	const services = 4
	lineup := make([]broadcast.LineupEntry, services)
	for i := range lineup {
		// Each field from its own decade, so any value seen anywhere names the
		// field it came from.
		lineup[i] = broadcast.LineupEntry{
			ServiceID: uint16(0x0064 + i),
			Kind:      1,
			Listings:  uint16(0x0bb8 + i),
			Extra:     uint16(0x1770 + i),
			Channel:   uint16(0x0abc),
			Flags:     0b0101,
		}
	}

	// Measured 2026-09-20 by sweeping the gate, because the record's control
	// had only ever compared 0xFFFF against 0x1234 and this test first asserted
	// the obvious generalisation — that anything but 0xFFFF decodes nothing —
	// and caught itself being wrong. TWO values admit entries, by two different
	// paths through the firmware:
	//
	//	0xFFFF  bteqz at 0x800bf7fa jumps straight to the entry loop at 0x800bf80c
	//	0x0000  falls through 0x800bf7fc..0x800bf80a and then CONTINUES into it
	//	0x1234  falls through the same block and stops, touching no entry byte
	//
	// Everything else swept — 0x0001, 0x00FF, 0xFF00, 0x7FFF, 0x8000, 0xFFFE —
	// decodes nothing, so this is two admitted values and not a range or a mask.
	// The builder sends 0xFFFF; these cases exist so the boundary stays measured
	// rather than assumed, in both directions.
	for _, tc := range []struct {
		name        string
		mutate      func(*testing.T, []byte) []byte
		wantEntries int
	}{
		{"the 0xFFFF sentinel decodes every entry", nil, services},
		{"a zero gate also decodes, by the fall-through path", func(t *testing.T, s []byte) []byte {
			return breakTheGate(t, s, 0x0000)
		}, services},
		{"gate 0x1234 decodes nothing", func(t *testing.T, s []byte) []byte {
			return breakTheGate(t, s, 0x1234)
		}, 0},
		{"gate 0x0001, one off zero, decodes nothing", func(t *testing.T, s []byte) []byte {
			return breakTheGate(t, s, 0x0001)
		}, 0},
		{"gate 0xFFFE, one off the sentinel, decodes nothing", func(t *testing.T, s []byte) []byte {
			return breakTheGate(t, s, 0xfffe)
		}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box := restoredBox(t)
			bouquetID, networkID := guestSubscription(t, box)

			section, err := broadcast.BAT(bouquetID, 3, "Sky", []broadcast.Transport{{
				ID: networkID, NetworkID: networkID, Lineup: lineup,
				Services: []broadcast.Service{{ID: 0x0064}, {ID: 0x0065}, {ID: 0x0066}, {ID: 0x0067}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if tc.mutate != nil {
				section = tc.mutate(t, section)
			}
			if err := box.Demux.Push(0x11, section); err != nil {
				t.Fatal(err)
			}

			hits := map[uint32]int{}
			for i := 0; i < 3_000_000; i++ {
				hits[box.Machine.Core.State().PC]++
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}

			// One hit per entry at the field read, and the same count at the
			// eighteen-byte record stride: N services, 18 bytes apart.
			if got := hits[pcEntryServiceID]; got != tc.wantEntries {
				t.Errorf("entries decoded = %d, want %d", got, tc.wantEntries)
			}
			if got := hits[pcRecordStride]; got != tc.wantEntries {
				t.Errorf("eighteen-byte records built = %d, want %d", got, tc.wantEntries)
			}
			// Each of the four packed bits is unpacked to its own byte, once per
			// entry. Checking all four is the point: three of them firing would
			// still look like success at any coarser granularity.
			for _, pc := range []uint32{pcFlagBit3, pcFlagBit2, pcFlagBit1, pcFlagBit0} {
				if got := hits[pc]; got != tc.wantEntries {
					t.Errorf("flag unpack at %#x ran %d times, want %d", pc, got, tc.wantEntries)
				}
			}
		})
	}
}

// The namespace control, kept separate because it fails one descriptor earlier
// than the gate does: without specifier 2 the guest never looks inside the
// 0xB1 at all, rather than looking and rejecting.
func TestGuestIgnoresTheLineupWithoutTheDeclaredNamespace(t *testing.T) {
	box := restoredBox(t)
	bouquetID, networkID := guestSubscription(t, box)

	section, err := broadcast.BAT(bouquetID, 4, "Sky", []broadcast.Transport{{
		ID: networkID, NetworkID: networkID,
		// The service is here for the linkage to name, not for this test: a
		// BAT whose transport declares nothing has no service to point the
		// guide at, and the builder refuses to invent one.
		Services: []broadcast.Service{{ID: 0x0064}},
		Lineup:   []broadcast.LineupEntry{{ServiceID: 0x0064, Listings: 0x0bb8, Channel: 101}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Change only the specifier VALUE, from 2 to 9, and repair the CRC. The
	// descriptor, its gate and its entries are untouched.
	patched := append([]byte(nil), section...)
	found := false
	for i := 0; i+5 < len(patched)-4; i++ {
		if patched[i] == 0x5f && patched[i+1] == 4 && patched[i+5] == 0x02 {
			patched[i+5] = 0x09
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no 0x5F specifier to repoint: the fixture is wrong, not the guest")
	}
	body := patched[:len(patched)-4]
	patched = binary.BigEndian.AppendUint32(body, dvb.MPEGCRC32(body))

	if err := box.Demux.Push(0x11, patched); err != nil {
		t.Fatal(err)
	}
	hits := 0
	for i := 0; i < 3_000_000; i++ {
		if box.Machine.Core.State().PC == pcEntryServiceID {
			hits++
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if hits != 0 {
		t.Fatalf("the guest read %d line-up entries under specifier 9; only specifier 2 unlocks tag 0xB1", hits)
	}
}
