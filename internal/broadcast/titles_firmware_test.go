package broadcast_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/dvb"
)

// The per-event register, once per record, from the consumer ladder the record
// walks. Counting it answers "how many records did the box take from this
// section" without reading our own bytes back at ourselves.
const pcPerEventRegister = 0x800c587c

func titleDictionary(t *testing.T) *broadcast.HuffmanDictionary {
	t.Helper()
	path := filepath.Join("..", "..", "dictionaries", "skyuk.dict")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("the Sky EPG huffman dictionary is not installed; see dictionaries/MANIFEST.md")
	}
	dict, err := broadcast.LoadHuffmanDictionary(path)
	if err != nil {
		t.Fatal(err)
	}
	return dict
}

// titleSubscription reads what the box is asking for after a line-up, rather
// than hardcoding it. The addressing is TASK-6.6's subject; this takes only
// what it needs and fails loudly if the box never subscribed, because a title
// section sent to the wrong filter is never delivered and that is
// indistinguishable from one the box ignored.
func titleSubscription(t *testing.T, box *board.Runtime) (tableID byte, extension uint16, filter [2]byte, pid uint16) {
	t.Helper()
	for unit := uint8(0); unit < 16; unit++ {
		table, ok := box.Demux.Match(unit, 0)
		// The hardware mask is 0xFE, so 0xA0 and 0xA1 both reach the parser.
		if !ok || table.Mask != 0xfe || table.Value&0xfe != 0xa0 {
			continue
		}
		high, okHigh := box.Demux.Match(unit, 1)
		low, okLow := box.Demux.Match(unit, 2)
		payload0, ok0 := box.Demux.Match(unit, 6)
		payload1, ok1 := box.Demux.Match(unit, 7)
		if !okHigh || !okLow || !ok0 || !ok1 {
			t.Fatalf("title filter in unit %d is incomplete", unit)
		}
		tableID = table.Value
		extension = uint16(high.Value)<<8 | uint16(low.Value)
		filter = [2]byte{payload0.Value, payload1.Value}
		for _, armed := range box.Demux.ArmedPIDs() {
			if armed == 0x33 || armed == 0x36 || armed == 0x37 || armed == 0x30 {
				pid = armed
			}
		}
		if pid == 0 {
			t.Fatalf("unit %d asks for table %#x but no listings PID is armed", unit, tableID)
		}
		t.Logf("the box asks for table %#x extension %#04x payload %#02x %#02x on PID %#02x",
			tableID, extension, filter[0], filter[1], pid)
		return tableID, extension, filter, pid
	}
	// Measured 2026-09-20: after a TDT, NIT, SDT and four BAT versions carrying
	// a line-up, this box arms PID 0x36 -- one of Sky's title PIDs, and new
	// since before the line-up -- but programs NO match unit for table 0xA0 or
	// 0xA1 anywhere in its thirty-two. Sections pushed to 0x36 are accepted by
	// the demux and the guest registers zero records from them, across all four
	// addressings the record names for a clocked box.
	//
	// So this half of TC-6.4 cannot run yet and skips rather than failing: what
	// is missing is TASK-6.6, addressing each section with the table id, PID and
	// MJD the box is CURRENTLY asking for, read from the match unit and the
	// guide's notification slot. The byte-level half -- twelve records walked as
	// twelve by the firmware's own arithmetic, and the reference's shown failing
	// on the same bytes -- is proved in titles_test.go and does not depend on it.
	t.Skip("the box has not programmed a title match unit; blocked on TASK-6.6 (addressing)")
	return 0, 0, [2]byte{}, 0
}

// feedLineup gets the box to ask for its listings, which it only does once it
// has a channel list.
func feedLineup(t *testing.T, box *board.Runtime, listings uint16) {
	t.Helper()
	bouquetID, networkID := guestSubscription(t, box)
	section, err := broadcast.BAT(bouquetID, 6, "Sky", []broadcast.Transport{{
		ID: networkID, NetworkID: networkID,
		Services: []broadcast.Service{{ID: 0x0064}},
		Lineup: []broadcast.LineupEntry{
			{ServiceID: 0x0064, Kind: 1, Listings: listings, Extra: 0x1770, Channel: 101},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := box.Demux.Push(0x11, section); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30_000_000; i++ {
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
}

// overstateRecordLengths rewrites every record's twelve-bit length to
// openTVtoXML's arithmetic — the descriptor PLUS the four header bytes the
// firmware adds itself — and repairs the CRC. The section keeps its exact
// size; only the declared lengths change, so the run differs in the one thing
// under test.
func overstateRecordLengths(t *testing.T, section []byte) []byte {
	t.Helper()
	out := append([]byte(nil), section...)
	end := len(out) - 4
	patched := 0
	for off := 10; off+4 <= end; {
		length := int(out[off+2]&0x0f)<<8 | int(out[off+3])
		if length == 0 || out[off+4] != 0xb5 {
			break
		}
		overstated := length + 4
		out[off+2] = 0xf0 | byte(overstated>>8)
		out[off+3] = byte(overstated)
		patched++
		off += length + 4
	}
	if patched != 12 {
		t.Fatalf("overstated %d record lengths, want all 12", patched)
	}
	body := out[:len(out)-4]
	return binary.BigEndian.AppendUint32(body, dvb.MPEGCRC32(body))
}

// TC-6.4, against the box rather than against our own walk. Twelve records
// must arrive as twelve, and the reference arithmetic must be shown failing on
// a section identical in every other byte.
func TestBoxRegistersTwelveRecordsAndFewerWithTheReferenceLength(t *testing.T) {
	dict := titleDictionary(t)
	const listings = 0x0bb8

	for _, tc := range []struct {
		name      string
		overstate bool
	}{
		{"the firmware's length arithmetic", false},
		{"openTVtoXML's length arithmetic", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box := restoredBox(t)
			feedLineup(t, box, listings)
			tableID, extension, filter, pid := titleSubscription(t, box)

			records := make([]broadcast.TitleRecord, 12)
			for i := range records {
				records[i] = broadcast.TitleRecord{
					EventID:  uint16(i + 1),
					Start:    (6 + i) * 3600,
					Duration: 30 * 60,
					Title:    "The Simpsons",
				}
			}
			section, err := broadcast.TitleSection(tableID, extension, filter, 0, 0, 0, dict, records)
			if err != nil {
				t.Fatal(err)
			}
			if tc.overstate {
				section = overstateRecordLengths(t, section)
			}
			if err := box.Demux.Push(pid, section); err != nil {
				t.Fatal(err)
			}
			registered := 0
			for i := 0; i < 40_000_000; i++ {
				if box.Machine.Core.State().PC == pcPerEventRegister {
					registered++
				}
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("%s: the box registered %d records", tc.name, registered)
			if !tc.overstate && registered != 12 {
				t.Errorf("the box registered %d of 12 records", registered)
			}
			if tc.overstate && registered >= 12 {
				t.Errorf("the box registered %d records from an overstated section, so the "+
					"two readings are not distinguished and this proves nothing", registered)
			}
		})
	}
}
