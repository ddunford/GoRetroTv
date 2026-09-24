package broadcast

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/ddunford/goretrotv/internal/dvb"
)

// A FULL LINE-UP SPANS SEVERAL SECTIONS, AND EVERY CHANNEL MUST SURVIVE THE SPLIT.
//
// A DVB section stops at 1021 bytes, which is twenty-five services once each carries a name, a
// private data specifier and its 0xB2 guide row. Sky's real line-up was around a hundred and forty
// channels across seven EPG sections, so it cannot ever have travelled as one section -- a table
// is section_number 0 through last_section_number and the receiver reassembles it. Carrying one
// section per table was this port's ceiling and never the signal's, and this is the test that says
// the ceiling is gone.
//
// WHAT WOULD GO WRONG IF IT WERE NOT CHECKED is not a build failure, it is a line-up that looks
// fine and is short: a splitter that dropped the overflow, repeated a chunk, or numbered the
// sections wrong would put a plausible table on air and the box would simply have fewer channels
// than the schedule names. So this counts every service across every section and requires each one
// exactly once -- the property a reader actually cares about -- rather than checking a length.
func TestALargeLineupSpansSeveralCorrectlyNumberedSections(t *testing.T) {
	t.Parallel()
	const channels = 140 // what Sky claimed at the 1998 digital launch

	services := make([]Service, 0, channels)
	lineup := make([]LineupEntry, 0, channels)
	for i := 0; i < channels; i++ {
		id := uint16(1000 + i) // #nosec G115 -- bounded by the loop
		services = append(services, Service{
			ID: id, Name: fmt.Sprintf("Sky Channel %03d", i), EITSchedule: true,
			Row: &GuideRow{At9: byte(i%7 + 1)}, // #nosec G115 -- one of seven genres
		})
		lineup = append(lineup, LineupEntry{
			ServiceID: id, Kind: 1, Listings: id, Extra: id,
			Channel: uint16(101 + i), Flags: 0x0c, // #nosec G115 -- bounded by the loop
		})
	}

	sdt, err := SDT(1, 2, 0, services)
	if err != nil {
		t.Fatalf("a %d-channel SDT did not build: %v", channels, err)
	}
	if len(sdt) < 2 {
		t.Fatalf("harness: %d channels fitted in %d section(s), so this test is not exercising "+
			"the split it was written for", channels, len(sdt))
	}
	checkNumbering(t, "SDT", sdt, 0x42, 1)
	seen := map[uint16]int{}
	for _, section := range sdt {
		for _, id := range sdtServiceIDs(t, section) {
			seen[id]++
		}
	}
	assertEachExactlyOnce(t, "SDT", seen, services)

	bat, err := BAT(0x0fa1, 0, "Sky", []Transport{{ID: 1, NetworkID: 2, Services: services, Lineup: lineup}})
	if err != nil {
		t.Fatalf("a %d-channel BAT did not build: %v", channels, err)
	}
	if len(bat) < 2 {
		t.Fatalf("harness: a %d-entry line-up fitted in %d BAT section(s), so the split is not "+
			"being exercised", channels, len(bat))
	}
	checkNumbering(t, "BAT", bat, 0x4a, 0x0fa1)
	inLineup := map[uint16]int{}
	for _, section := range bat {
		for _, id := range batLineupServiceIDs(t, section) {
			inLineup[id]++
		}
	}
	assertEachExactlyOnce(t, "BAT line-up", inLineup, services)
	for sectionNumber, section := range bat {
		assertPrivateLineupHasNamespace(t, sectionNumber, section)
	}
	t.Logf("%d channels: SDT %d sections, BAT %d sections", channels, len(sdt), len(bat))
}

// A private_data_specifier applies only within the descriptor loop containing it. A BAT section
// may be byte-perfect and reach the guest while every 0xB1 in it is ignored because namespace 2
// appeared in an earlier section. Check each transport loop independently, as the receiver does.
func assertPrivateLineupHasNamespace(t *testing.T, sectionNumber int, section []byte) {
	t.Helper()
	body := section[8 : len(section)-4]
	bouquetLen := int(body[0]&0x0f)<<8 | int(body[1])
	at := 2 + bouquetLen
	tsLen := int(body[at]&0x0f)<<8 | int(body[at+1])
	at += 2
	end := at + tsLen
	for at < end {
		loopLen := int(body[at+4]&0x0f)<<8 | int(body[at+5])
		loop, namespaced := body[at+6:at+6+loopLen], false
		for i := 0; i < len(loop); {
			tag, n := loop[i], int(loop[i+1])
			if tag == 0x5f && n == 4 && loop[i+5] == 2 {
				namespaced = true
			}
			if tag == 0xb1 && !namespaced {
				t.Fatalf("BAT section %d carries a 0xB1 line-up before declaring private namespace 2", sectionNumber)
			}
			i += 2 + n
		}
		at += 6 + loopLen
	}
}

// checkNumbering requires a table's sections to be a complete, correctly numbered, valid set.
func checkNumbering(t *testing.T, what string, sections [][]byte, table byte, extension uint16) {
	t.Helper()
	last := byte(len(sections) - 1) // #nosec G115 -- callers build fewer than 256
	for i, section := range sections {
		switch {
		case section[0] != table:
			t.Errorf("%s section %d has table id %#02x, not %#02x", what, i, section[0], table)
		case binary.BigEndian.Uint16(section[3:5]) != extension:
			t.Errorf("%s section %d has extension %#04x, not %#04x",
				what, i, binary.BigEndian.Uint16(section[3:5]), extension)
		case section[6] != byte(i): // #nosec G115 -- fewer than 256 sections
			t.Errorf("%s section at index %d calls itself section_number %d", what, i, section[6])
		case section[7] != last:
			t.Errorf("%s section %d says last_section_number %d, not %d", what, i, section[7], last)
		case len(section) > maxSectionLength+3:
			t.Errorf("%s section %d is %d bytes, past the %d a section may carry",
				what, i, len(section), maxSectionLength+3)
		}
		body := section[:len(section)-4]
		if got, want := binary.BigEndian.Uint32(section[len(section)-4:]), dvb.MPEGCRC32(body); got != want {
			t.Errorf("%s section %d has CRC %#08x, computed %#08x -- the box drops a section whose "+
				"CRC fails, so a bad one is invisible rather than wrong", what, i, got, want)
		}
	}
}

// assertEachExactlyOnce is the property that matters: no channel lost, none repeated.
func assertEachExactlyOnce(t *testing.T, what string, seen map[uint16]int, services []Service) {
	t.Helper()
	missing, repeated := 0, 0
	for _, svc := range services {
		switch n := seen[svc.ID]; {
		case n == 0:
			if missing < 3 {
				t.Errorf("%s: service %d appears in no section, so the split dropped it", what, svc.ID)
			}
			missing++
		case n > 1:
			if repeated < 3 {
				t.Errorf("%s: service %d appears in %d sections", what, svc.ID, n)
			}
			repeated++
		}
	}
	if missing > 3 || repeated > 3 {
		t.Errorf("%s: %d services missing and %d repeated in total", what, missing, repeated)
	}
	if extra := len(seen) - len(services); extra > 0 {
		t.Errorf("%s: %d service ids appear that were never put in", what, extra)
	}
}

// sdtServiceIDs reads the service ids out of one SDT section.
func sdtServiceIDs(t *testing.T, section []byte) []uint16 {
	t.Helper()
	body := section[:len(section)-4]
	at := 8 + 3 // long-section header, then original_network_id and the reserved byte
	var ids []uint16
	for at+5 <= len(body) {
		ids = append(ids, binary.BigEndian.Uint16(body[at:at+2]))
		length := int(binary.BigEndian.Uint16(body[at+3:at+5]) & 0x0fff)
		at += 5 + length
	}
	if at != len(body) {
		t.Errorf("an SDT section's service loop ended at %d of %d bytes, so it is malformed",
			at, len(body))
	}
	return ids
}

// batLineupServiceIDs reads the service ids out of every 0xB1 line-up descriptor in one BAT section.
func batLineupServiceIDs(t *testing.T, section []byte) []uint16 {
	t.Helper()
	body := section[:len(section)-4]
	at := 8
	bouquetLen := int(binary.BigEndian.Uint16(body[at:at+2]) & 0x0fff)
	at += 2 + bouquetLen
	tsLen := int(binary.BigEndian.Uint16(body[at:at+2]) & 0x0fff)
	at += 2
	end := at + tsLen
	if end > len(body) {
		t.Fatalf("a BAT section claims a %d-byte transport loop with %d bytes left", tsLen, len(body)-at)
	}
	var ids []uint16
	for at+6 <= end {
		descLen := int(binary.BigEndian.Uint16(body[at+4:at+6]) & 0x0fff)
		at += 6
		stop := at + descLen
		for at+2 <= stop {
			tag, length := body[at], int(body[at+1])
			if tag == 0xb1 {
				// The gate is two bytes, then nine bytes an entry, the first two of which are the
				// service id.
				for e := at + 2 + 2; e+9 <= at+2+length; e += 9 {
					ids = append(ids, binary.BigEndian.Uint16(body[e:e+2]))
				}
			}
			at += 2 + length
		}
		at = stop
	}
	return ids
}
