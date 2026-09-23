package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHETHER THE ALL CHANNELS GRID IS WAITING FOR AN INDEX TOO.
//
// The grid draws six correct rows -- right numbers, right names, off the line-up -- and
// "..no listings available" in every cell, while the now-and-next banner on the same box at the
// same instant resolves "Dream Team" at 7.00pm on Sky One off the same broadcast. Everything about
// the DATA has been eliminated by measurement: the box's own match unit asks for a3/ff, extension
// 01ff/fe01, MJD 51171, PID 0x33, which is exactly what we transmit; all six listings ids satisfy
// it; the programmes register; the banner reads them.
//
// WHAT HAS JUST CHANGED IS THAT ANOTHER SCREEN IN THE SAME GUIDE TURNED OUT TO WORK THE SAME WAY.
// ALL PROGRAMMES A-Z said "Searching for listings" for exactly the same reason -- the title store
// was full and the screen drew nothing -- and it was not waiting on titles at all. It was waiting
// on a table 0xC1 INDEX, delivered on PID 0x52 under its own letter, BEFORE the screen opened. Fed
// that, it draws real programmes.
//
// The guide's string table lists its genre screens as ALL CHANNELS, MOVIES, SPORTS, ENTERTAINMENT,
// NEWS & DOCUMENTARIES, KIDS, MUSIC & RADIO, SPECIALIST -- **ALL CHANNELS FIRST IN THE GENRE LIST,
// not separate from it** -- and the 0xC1 dispatch has a sixteen-by-four genre table addressed
// 0x0100 | (block << 6) | category. If the grid is one of those screens, it is empty for the same
// reason A-Z was, and the fix is the same.
//
// ALL SIXTY-FOUR GENRE EXTENSIONS ARE SENT, not a guess at which category ALL CHANNELS is. Which
// number means which genre is not established, and sending one on a hunch would turn a null result
// into "the grid does not want an index" when it only meant "not that category". Sixty-four
// sections cost one acquisition.
//
// THE RECORDS USE THE LAYOUT MEASURED FOR A-Z: rec[0..1] is the channel's listings id and rec[5..6]
// its event id. That was established by sending six candidate layouts pointing at six different
// programmes and reading which titles the screen drew.
//
// THE CONTROL IS THE SAME WALK WITH NOTHING DELIVERED, because "..no listings available" is what
// this screen draws unaided and a run without a control cannot tell a fix from the status quo.
func TestWhetherTheAllChannelsGridWantsAnIndexToo(t *testing.T) {
	control := gridWithIndexRun(t, false, "grid-index-control.png")
	delivered := gridWithIndexRun(t, true, "grid-index-delivered.png")
	t.Logf("=== control   (nothing delivered): %08X", control)
	t.Logf("=== delivered (64 genre indexes):  %08X", delivered)
	if control == delivered {
		t.Logf("the grid finished on the same screen either way, so it is NOT waiting on a table " +
			"0xC1 genre index in any of the sixteen categories -- which is worth knowing, because " +
			"the A-Z screen next door was, and the two failures look identical from outside")
		return
	}
	t.Logf("THE GRID MOVED. Read .artifacts/grid-index-control.png and -delivered.png -- only the " +
		"picture says whether the cells filled")
}

func gridWithIndexRun(t *testing.T, deliver bool, artefact string) uint32 {
	t.Helper()
	const probePID = 0x52
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
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	if deliver {
		// ONE RECORD PER PROGRAMME IN THE BLOCK, across every channel: the grid is a whole page of
		// channels at once, so an index that named only one would answer a question it is not
		// asking. Event ids are the position of the programme in the channel's sorted day, which
		// is how the transmitter assigns them.
		listings := guide.On(day)
		var records []broadcast.IndexRecord
		for i := range listings.Services {
			service := &listings.Services[i]
			for n := range service.Programmes {
				start, err := service.Programmes[n].StartSeconds()
				if err != nil {
					t.Fatal(err)
				}
				if start < 18*3600 {
					continue // outside the block the box subscribes to at 19:00
				}
				event := uint16(n + 1) // #nosec G115 -- a day's programmes
				records = append(records, broadcast.IndexRecord{
					ID:       service.ListingsID,
					Packed:   0x0f,
					Selector: 0xc0,
					Data:     [5]byte{0, byte(event >> 8), byte(event & 0xff), 0, 0}, // #nosec G115 -- masked
				})
			}
		}
		if len(records) == 0 {
			t.Fatal("harness: no programme in the block produced an index record, so nothing " +
				"would be delivered and the grid's emptiness would measure this loop")
		}
		version := byte(0)
		sent := 0
		for category := byte(0); category < 16; category++ {
			for block := byte(0); block < 4; block++ {
				ext, err := broadcast.IndexCategory(category, block)
				if err != nil {
					t.Fatal(err)
				}
				section, err := broadcast.IndexSection(ext, version&0x1f, 0, 0, records)
				if err != nil {
					t.Fatal(err)
				}
				if err := box.Demux.Push(probePID, section); err != nil {
					t.Fatalf("harness: extension %#04x was refused (%v)", ext, err)
				}
				version++
				sent++
				for i := 0; i < 300_000; i++ {
					if err := transmitter.Pump(box.Machine.Retired); err != nil {
						t.Fatal(err)
					}
					if err := box.Step(); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		// DID THE GENRE SLOTS FILL? Everything below is worthless if they did not: an empty grid
		// after a delivery that went nowhere measures the delivery, not the screen. The letter
		// family demonstrably fills its slots this way -- 26 of 26 -- but the genre arm passes a
		// DIFFERENT fourth argument to the same handler, so it does not follow.
		categories := box.RAM.Read(uint32(0x800C51A8)&0x1fffffff, bus.Word)
		if categories&0xf0000000 == 0 || categories&3 != 0 {
			t.Fatalf("harness: the category table pointer is %08X, not a word-aligned guest "+
				"address, so its slots cannot be checked", categories)
		}
		filled := 0
		for slot := uint32(0); slot < 64; slot++ {
			if box.RAM.Read((categories+slot*4)&0x1fffffff, bus.Word) != 0 {
				filled++
			}
		}
		t.Logf("delivered %d records under each of %d genre extensions, before a key was pressed; "+
			"%d of 64 category slots now hold a non-null head", len(records), sent, filled)
		if filled == 0 {
			t.Fatalf("harness: not one of the 64 category slots was filled by %d delivered "+
				"sections, so the arrays were handed to null and the grid below measures the "+
				"delivery rather than the screen. The letter family fills 26 of 26 the same way, "+
				"so this is a real difference between the two arms and not a broken push", sent)
		}
	}

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
		t.Logf("%-32s %08X -> %08X", name, before, settled)
		return settled
	}

	settled := openAllChannelsUnpinned(t, press, ".artifacts/"+artefact)
	// PAST THE SETTLE. Four identical frames is not a finish on this screen: the rows paint in
	// bursts that hold still across four samples and carry on afterwards, and every measurement of
	// the grid taken at a settle in this project's history was taken too early.
	final := settled
	for i := 0; i < 50_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
		if i%1_000_000 == 0 {
			final = screenNow(t, box)
		}
	}
	if err := dumpScreen(t, box, artefact); err != nil {
		t.Fatal(err)
	}
	what := "with NOTHING delivered"
	if deliver {
		what = "with all sixty-four genre indexes delivered first"
	}
	t.Logf("the grid settled on %08X and finished on %08X %s", settled, final, what)
	_ = fmt.Sprint(bus.Byte)
	return final
}
