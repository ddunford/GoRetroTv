package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// HOW MANY SERVICES DOES THE BOX ACTUALLY HOLD, AND ARE THEY OURS?
//
// The ALL CHANNELS grid draws its header and no rows, and the record's ruling is that the grid
// "COUNTS TWELVE CHANNELS and draws none of them" -- so the question became why a row loop produces
// nothing from a count it has. **That count was never measured on this port.** It was read out of
// the browser oracle's o-code data segment at DS+0x02E20C, and the addresses that write and read it
// are flash addresses that this port never executes while the grid draws (measured: 6,488,065
// instructions, every one of them in RAM). The grid is interpreted OpenTV o-code; those addresses
// belong to the interpreter's world as the oracle instruments it, not to ours.
//
// AND TWELVE IS SUSPICIOUS ON ITS OWN TERMS. The same dump found DS+0x01A270 = 12 "present from
// boot" -- before any broadcast -- while the transmitter announces SIX services. A grid iterating
// twelve entries against a store that knows six is a plausible way to draw a header and no rows,
// and it would be a fault in what we BROADCAST rather than in the firmware. That is worth an hour
// before anyone disassembles anything.
//
// So this counts the services the box is holding, by reading its own records rather than by
// trusting either number. The record gives their shape, located at runtime by content:
//
//	00 20 00 00 | 00 64 0b b8 | 00 00 | 00 65 | 01 | 00 01 00 01 00
//	transport     service id    extra   channel  kind  four flag bits
//
// so a record has its service id at +4 and its channel number at +10, six bytes apart. Both are
// values WE chose in the fixture, which is what makes them findable without knowing where the array
// lives.
//
// IT ASSERTS ITS OWN SUBJECT. If it cannot find the six services the transmitter announced, it has
// not found the line-up and any count it reported would be a number about nothing.
//
// IT ONLY READS. Nothing is written into the guest.
func TestHowManyServicesTheBoxHoldsAfterAcquiring(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}

	listings := guide.On(day)
	hits := serviceRecords(t, box, listings.Services)
	t.Logf("the transmitter announces %d services; the box holds %d service records",
		len(listings.Services), len(hits))

	// THE ARRAY ITSELF. Records sit at a fixed stride, so the gaps between the sites say what it is
	// and how many entries there are. Taking the commonest gap rather than assuming eighteen keeps
	// this a reading.
	gaps := map[uint32]int{}
	for i := 1; i < len(hits); i++ {
		if d := hits[i].base - hits[i-1].base; d > 0 && d <= 64 {
			gaps[d]++
		}
	}
	stride, most := uint32(0), 0
	for d, n := range gaps {
		if n > most || (n == most && d < stride) {
			stride, most = d, n
		}
	}
	t.Logf("commonest gap between adjacent records: %d bytes (seen %d times)", stride, most)
	for _, h := range hits {
		t.Logf("    guest %08X  service %3d  channel %3d  %s",
			0x80000000|h.base, h.service.ServiceID, h.service.Channel, h.service.Name)
	}
}

// serviceRecord is one of the box's 18-byte line-up entries, found by its contents.
type serviceRecord struct {
	base    uint32
	service *multiplex.ListedService
}

// serviceRecords finds the box's own line-up entries by anchoring on values the fixture chose.
//
// The record gives the shape, located at runtime by content rather than by address:
//
//	00 20 00 00 | 00 64 0b b8 | 00 00 | 00 65 | 01 | 00 01 00 01 00
//	transport     service id    extra   channel  kind  four flag bits
//
// so the service id sits at +4 and the channel number at +10, six bytes apart. Both are values the
// fixture chose, which is what makes the array findable without knowing where it lives.
//
// IT ASSERTS ITS OWN SUBJECT: not finding every announced service means this is not the line-up,
// and anything a caller then measured about it would be about nothing.
func serviceRecords(t *testing.T, box *board.Runtime, announced []multiplex.ListedService) []serviceRecord {
	t.Helper()
	if len(announced) == 0 {
		t.Fatal("harness: the fixture announces no services at all, so there is nothing to look for")
	}
	size := box.RAM.Size()
	half := func(off uint32) uint16 {
		return uint16(box.RAM.Read(off, bus.Half)) // #nosec G115 -- half read
	}
	byService := map[uint16]*multiplex.ListedService{}
	for i := range announced {
		byService[announced[i].ServiceID] = &announced[i]
	}
	var hits []serviceRecord
	for off := uint32(4); off+8 <= size; off += 2 {
		svc, ok := byService[half(off)]
		if !ok || half(off+6) != svc.Channel {
			continue
		}
		hits = append(hits, serviceRecord{base: off - 4, service: svc})
	}
	found := map[uint16]bool{}
	for _, h := range hits {
		found[h.service.ServiceID] = true
	}
	if len(found) < len(announced) {
		var missing []string
		for i := range announced {
			if !found[announced[i].ServiceID] {
				missing = append(missing, fmt.Sprintf("%s (service %d, channel %d)",
					announced[i].Name, announced[i].ServiceID, announced[i].Channel))
			}
		}
		t.Fatalf("harness: found records for only %d of the %d announced services, missing %v -- so "+
			"this is not the line-up array and its count would mean nothing", len(found),
			len(announced), missing)
	}
	sort.Slice(hits, func(a, b int) bool { return hits[a].base < hits[b].base })
	return hits
}
