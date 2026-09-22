package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE FOUR BITS THIS PROJECT HAS NEVER ONCE SET.
//
// The line-up entry carried in the BAT's private 0xB1 descriptor ends with a halfword whose top
// twelve bits are the channel number and whose LOW FOUR BITS the guest unpacks into four SEPARATE
// BYTES of the eighteen-byte record it builds -- record[13], [14], [15] and [16]. A field a
// firmware splits four ways is a field it distinguishes between, and every feed this port has ever
// transmitted left all four clear, because the transmitter hard-coded no flags at all.
//
// That makes them the strongest remaining candidate for the ALL CHANNELS grid, and the reason is
// the shape of the failure rather than a hunch. The grid draws its header, the in-world date, the
// clock and correctly spaced half-hour columns, and no rows. The now-and-next banner, on the same
// box at the same moment, resolves a channel and shows a real programme off the same broadcast. A
// screen that draws its furniture and none of its content is a screen iterating a list that came
// back empty -- and a per-channel flag the box has never seen set is exactly the kind of thing that
// empties one list while leaving the tuned service, which nothing filters, working perfectly.
//
// **ONE RUN READS ALL FOUR BITS.** Each channel is given a DIFFERENT flags value, so the rows that
// appear name the bit rather than merely proving that some bit matters:
//
//	Sky One     101  0x1    Sky Movies  301  0x8
//	Sky News    501  0x2    Sky Travel  251  0xF   (all four, in case they are only meaningful together)
//	Sky Sports  401  0x4    Sky Soap    121  0x0   (the control: today's feed, unchanged)
//
// Six channels, four bits, one bounded control and one belt-and-braces case, in a single run of a
// test that costs a couple of minutes. Sweeping sixteen values one per run would cost half an hour
// and say less, because a run that changes every channel at once cannot tell which change did it.
//
// IT IS ENTIRELY IN THE SIGNAL. The flags ride in the BAT, the box parses them out of a section the
// hardware delivered, and nothing is written into guest memory. That is the binding constraint on
// this whole line of work: a memory injection that produced rows would prove nothing about a
// Digibox and would have to be unpicked later.
//
// IT ASSERTS ITS OWN SUBJECT, and this is the part that makes a null result worth having. Before it
// presses a single key it reads the records the box built and checks that our flag bytes ARRIVED --
// because if they did not, "the grid still draws nothing" measures the transmitter and not the
// grid, and would be filed as a finding about the firmware. It also refuses to accept any screen as
// the grid on a hash alone: the known-empty grid's hash is used as a CONTROL VALUE, never as the
// acceptance gate, since a run that succeeds is precisely a run whose hash is one nobody has seen.
// The screenshot is the proof, exactly as it was when three instruments measured the wrong screen.
func TestWhichLineUpFlagTheAllChannelsGridWants(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	// One distinct value per channel, assigned before the transmitter is built so every wave of the
	// carousel carries them.
	listings := guide.On(day)
	assign := []byte{0x1, 0x2, 0x4, 0x8, 0x0f, 0x0}
	if len(listings.Services) != len(assign) {
		t.Fatalf("harness: this probe assigns %d distinct flag values and the schedule lists %d "+
			"channels, so the mapping from a row back to a bit would not be one to one",
			len(assign), len(listings.Services))
	}
	sent := map[uint16]byte{}
	for i := range listings.Services {
		listings.Services[i].Flags = assign[i]
		sent[listings.Services[i].Channel] = assign[i]
		t.Logf("%-14s channel %4d  flags %#03x", listings.Services[i].Name,
			listings.Services[i].Channel, assign[i])
	}

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
	// LET IT GO QUIET. A key sent the instant the box finishes acquiring is a key it never sees,
	// and three earlier instruments got away with pressing immediately only while the box happened
	// to be idle by then.
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	// DID THE BITS ARRIVE? Everything below is worthless if they did not.
	records := serviceRecords(t, box, listings.Services)
	readByte := func(off uint32) byte {
		return byte(box.RAM.Read(off, bus.Byte)) // #nosec G115 -- byte read
	}
	carried := 0
	for _, rec := range records {
		var got [4]byte
		for i := range got {
			got[i] = readByte(rec.base + 13 + uint32(i)) // #nosec G115 -- small constant
		}
		packed := byte(0)
		for i, b := range got {
			if b != 0 {
				packed |= 1 << uint(i)
			}
		}
		mine := sent[rec.service.Channel]
		t.Logf("channel %4d  record %08X  flag bytes %02X %02X %02X %02X  (sent %#03x)",
			rec.service.Channel, 0x80000000|rec.base, got[0], got[1], got[2], got[3], mine)
		if packed != 0 {
			carried++
		}
	}
	if carried == 0 {
		t.Fatalf("harness: not one of the %d records carries a non-zero flag byte, so the four bits "+
			"never reached the box through the BAT. Whatever the grid does next measures the "+
			"transmitter, not the firmware, and must not be filed as a finding about the screen",
			len(records))
	}
	t.Logf("%d of %d records carry flag bytes, so the bits travelled the signal end to end",
		carried, len(records))

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, settled := 0, before, uint32(0)
		for i := 0; i < budget; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
			if i%65536 != 0 {
				continue
			}
			now := screenNow(t, box)
			if now == last && now != before {
				stable++
				settled = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}

	// The control value this probe is trying to move. It is NOT the acceptance gate: a run that
	// succeeds is precisely a run whose hash nobody has seen, which is why the route is asked for
	// wantChange and the artefact is the proof.
	const emptyGrid = 0x42DBD889

	grid := openAllChannels(t, press, ".artifacts/lineup-flags-all-channels.png", true)
	if err := dumpScreen(t, box, "lineup-flags-all-channels.png"); err != nil {
		t.Fatal(err)
	}

	switch grid {
	case emptyGrid:
		t.Logf("VERDICT: the grid is BYTE FOR BYTE the screen it has always drawn (%08X) with all "+
			"four line-up flag bits exercised across six channels, including one carrying all four. "+
			"The flags are not what it is filtering on, and that closes them. The artefact is "+
			"lineup-flags-all-channels.png.", grid)
	default:
		t.Logf("VERDICT: THE GRID CHANGED -- %08X, where every previous run drew %08X. Read "+
			"lineup-flags-all-channels.png and see WHICH channels appear: each one carries a "+
			"different bit, so the rows name the bit. Do not report which bit from this hash.",
			grid, emptyGrid)
	}
}
