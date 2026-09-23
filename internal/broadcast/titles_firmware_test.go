package broadcast_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// isOpenTVTitleTable reports whether a table id is one the box's title parsers
// take: 0xA0..0xA4 and 0xB0, per the record's carousel section.
func isOpenTVTitleTable(id byte) bool {
	return (id >= 0xa0 && id <= 0xa4) || id == 0xb0
}

// titleRequest is the listings subscription the box has programmed into its
// own hardware: what it will accept and where. Every field is read off the
// machine rather than chosen, because a section the match unit does not match
// is never delivered, and from outside that is indistinguishable from one the
// box received and ignored.
type titleRequest struct {
	TableID   byte
	Extension uint16
	Filter    [2]byte // data[8..9], the MJD
	PID       uint16
}

// titleSubscription reads the request, or reports that there is not one yet.
//
// It only became readable once Demux.Match learned that units 8..15 carry
// their rule in the HIGH half of the match word. Before that the box's whole
// listings subscription read back as 00/00 -- not as "unprogrammed", but as
// nothing at all, which is why sections addressed by guesswork were collected
// and dropped.
func titleSubscription(box *board.Runtime) (titleRequest, bool) {
	var req titleRequest
	found := false
	for unit := uint8(0); unit < 16; unit++ {
		table, ok := box.Demux.Match(unit, 0)
		// The hardware mask is 0xFE, so the unit accepts a PAIR of table ids,
		// and the pair is not always in the 0xAn family: OpenTV titles run
		// 0xA0..0xA4 and 0xB0. Narrowing this to 0xAn once produced five days
		// out of eight reported as "the box never asked", which was a fact
		// about the census and not about the box.
		if !ok || table.Mask != 0xfe || !isOpenTVTitleTable(table.Value) {
			continue
		}
		high, _ := box.Demux.Match(unit, 1)
		low, _ := box.Demux.Match(unit, 2)
		mjdHigh, _ := box.Demux.Match(unit, 6)
		mjdLow, _ := box.Demux.Match(unit, 7)
		req.TableID = table.Value
		req.Extension = uint16(high.Value)<<8 | uint16(low.Value)
		req.Filter = [2]byte{mjdHigh.Value, mjdLow.Value}
		found = true
	}
	if !found {
		return titleRequest{}, false
	}
	for _, armed := range box.Demux.ArmedPIDs() {
		switch armed {
		case 0x10, 0x11, 0x14, 0x52: // NIT, SDT and BAT, TDT, and the one it boots with
		default:
			req.PID = armed
		}
	}
	// A table id with no PID is half a subscription and cannot be delivered
	// to, so it does not count as one.
	return req, req.PID != 0
}

// askedForTitles is titleSubscription with the harness failure the project
// requires: an instrument that cannot find its subject reports that, rather
// than returning a plausible zero.
func askedForTitles(t *testing.T, box *board.Runtime) titleRequest {
	t.Helper()
	req, ok := titleSubscription(box)
	if !ok {
		t.Fatal("the box is not asking for a title table on any armed PID, so nothing can be delivered to it")
	}
	t.Logf("the box asks for table %#02x extension %#04x MJD %02x%02x on PID %#02x",
		req.TableID, req.Extension, req.Filter[0], req.Filter[1], req.PID)
	return req
}

// runUntilTheBoxAsksForTitles steps until the box has programmed a listings
// match unit, and fails if it never does. Polled rather than watched on a PC,
// because the programming is a sequence of register writes and the thing that
// matters is the state they leave behind.
func runUntilTheBoxAsksForTitles(t *testing.T, box *board.Runtime, budget int) titleRequest {
	t.Helper()
	const pollEvery = 4096
	for i := 0; i < budget; i++ {
		if i%pollEvery == 0 {
			if req, ok := titleSubscription(box); ok {
				t.Logf("the box programmed its listings request by instruction %d", i)
				return req
			}
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatalf("the box had not asked for any listings after %d instructions, so it never acquired", budget)
	return titleRequest{}
}

// Budgets and one settle.
//
// clockSettle is the only guessed number left in this file, and it is guessed
// in the safe direction on purpose: the listings request is day-addressed and
// the box programs it ONCE, so a TDT that has not been applied before the BAT
// arrives produces a box permanently asking for MJD 40587, the Unix epoch,
// with no error anywhere. TC-6.5 asserts the MJD that comes out, which is what
// turns "the settle was long enough" from a hope into a checked claim.
const (
	clockSettle       = 8_000_000
	acquisitionBudget = 40_000_000
	titleWindow       = 4_000_000
)

// feedLineup gets the box to ask for its listings, which it only does once it
// has a clock and a channel list.
func feedLineup(t *testing.T, box *board.Runtime) {
	t.Helper()
	bouquetID, networkID := guestSubscription(t, box)
	run := func(n int) {
		for i := 0; i < n; i++ {
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A TOT and a TDT, in that order, which is what the carousel transmits.
	// The TOT is the one that matters -- the box's match units carry 0x73 and
	// nothing matches 0x70 -- and the day is chosen from the eight-day
	// rotation for one the box actually subscribes on (TC-6.5).
	when := time.Date(1998, 6, 15, 12, 0, 0, 0, time.UTC)
	tot, err := broadcast.TOT(when, broadcast.TimeOffset{Country: "GBR", Region: 0,
		OffsetMinutes: 0, ChangeUTC: when.AddDate(0, 3, 0), NextOffsetMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	if err := box.Demux.Push(0x14, tot); err != nil {
		t.Fatal(err)
	}
	tdt, err := broadcast.TDT(when)
	if err != nil {
		t.Fatal(err)
	}
	if err := box.Demux.Push(0x14, tdt); err != nil {
		t.Fatal(err)
	}
	run(clockSettle)

	sections, err := broadcast.BAT(bouquetID, 1, "Sky", []broadcast.Transport{{
		ID: networkID, NetworkID: networkID, FrequencyMHz: 11778, OrbitTenths: 282,
		SymbolRate: 27500, FEC: 2,
		Services: []broadcast.Service{{ID: 0x0064, Name: "Sky One"}},
		Lineup: []broadcast.LineupEntry{
			{ServiceID: 0x0064, Kind: 1, Listings: 0x0bb8, Extra: 0x1770, Channel: 101},
		},
	}})
	section := oneSection(t, sections, err)
	if err := box.Demux.Push(0x11, section); err != nil {
		t.Fatal(err)
	}
	runUntilTheBoxAsksForTitles(t, box, acquisitionBudget)
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
		out[off+2] = 0xf0 | byte(overstated>>8) // #nosec G115 -- twelve bits
		out[off+3] = byte(overstated)           // #nosec G115 -- twelve bits
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
	for _, tc := range []struct {
		name      string
		overstate bool
	}{
		{"the firmware's length arithmetic", false},
		{"openTVtoXML's length arithmetic", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box := boxWithLineup(t, feedLineup)
			req := askedForTitles(t, box)

			records := make([]broadcast.TitleRecord, 12)
			for i := range records {
				records[i] = broadcast.TitleRecord{
					EventID:  uint16(i + 1),
					Start:    (6 + i) * 3600,
					Duration: 30 * 60,
					Title:    "The Simpsons",
				}
			}
			section, err := broadcast.TitleSection(req.TableID, req.Extension, req.Filter, 0, 0, 0, dict, records)
			if err != nil {
				t.Fatal(err)
			}
			if tc.overstate {
				section = overstateRecordLengths(t, section)
			}
			if err := box.Demux.Push(req.PID, section); err != nil {
				t.Fatal(err)
			}
			registered, lastAt := 0, 0
			for i := 0; i < titleWindow; i++ {
				if box.Machine.Core.State().PC&^1 == pcPerEventRegister {
					registered++
					lastAt = i
				}
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}
			// Both readings are counted over the SAME window, and the window
			// is only fair if a full registration finishes well inside it --
			// otherwise "fewer records" would be a fact about where the
			// counting stopped rather than about the length arithmetic.
			if !tc.overstate {
				t.Logf("the twelfth record registered at instruction %d of %d", lastAt, titleWindow)
				if lastAt > titleWindow/4 {
					t.Errorf("registration ran to instruction %d, within a quarter of the %d-instruction window; "+
						"the overstated case is no longer demonstrably long enough to have registered twelve",
						lastAt, titleWindow)
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
