package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// DELIVER THE INDEX BEFORE THE SCREEN ASKS FOR IT, WHICH IS WHAT A CAROUSEL DOES.
//
// ALL PROGRAMMES A-Z reads ONE list-head slot -- letter 'A', extension 0x0041 -- once, as it opens,
// and then draws "Searching for listings" for ever. Delivering under 'A' while it was already open
// did not move it, and the slot was null the whole time, so the parser handed its decoded array to
// nothing:
//
//	uVar5 = DAT_800c51a0[ext - 0x40];             // null
//	(*DAT_800c51ac)(uVar5, puVar2, uVar1, iVar3); // handed to null
//
// A real box never meets that order. The index is broadcast on a carousel, so by the time a viewer
// has walked BOX OFFICE -> tv guide -> nine downs -> A-Z LISTINGS -> ALL PROGRAMMES, every letter
// has come round several times. This port has only ever pushed at a moment of its own choosing, and
// always after the screen was already up.
//
// So this delivers all twenty-six letters first, several times over, and only then navigates. It
// also watches the slot for WRITES as well as reads, because the earlier probe watched reads alone
// and therefore could not have seen anything fill it.
//
// THE CONTROL IS THE SAME WALK WITH NOTHING DELIVERED. "Searching for listings" is what this screen
// draws unaided, so a run without a control cannot tell a fix from the status quo -- and on this
// screen it has already fooled one probe, which reported the A-Z category menu appearing as the
// index taking effect when the menu paints itself out of the box's own resources.
func TestTheIndexDeliveredBeforeTheScreenOpens(t *testing.T) {
	control := indexBeforeOpenRun(t, nil, 3, "beforeopen-control.png")
	delivered := indexBeforeOpenRun(t, channelIndexRecords(t), 3, "beforeopen-delivered.png")
	t.Logf("=== control   (nothing delivered): %08X", control)
	t.Logf("=== delivered (26 letters, first): %08X", delivered)
	if control == delivered {
		t.Logf("both walks finished on the same screen, so delivering every letter ahead of the " +
			"screen changed nothing it shows. That is a fact about the payload or the " +
			"registration, NOT about the addressing: the screen demonstrably reads slot 'A' and " +
			"the parser demonstrably decodes what we send")
		return
	}
	t.Logf("THE SCREENS DIFFER. Read .artifacts/beforeopen-control.png and -delivered.png -- the " +
		"hash says something changed, only the picture says whether it is the listings")
}

// channelIndexRecords is one record per announced channel, carrying its listings id and nothing
// else. rec[4..8] are left at zero DELIBERATELY: they have never been swept off zero, so filling
// them with a plausible guess would make a null result unreadable.
func channelIndexRecords(t *testing.T) []broadcast.IndexRecord {
	t.Helper()
	guide := demoGuide(t)
	listings := guide.On(time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC))
	records := make([]broadcast.IndexRecord, 0, len(listings.Services))
	for i := range listings.Services {
		records = append(records, broadcast.IndexRecord{
			ID:       listings.Services[i].ListingsID,
			Packed:   0x0f,
			Selector: 0xc0,
		})
	}
	return records
}

// indexBeforeOpenRun walks to ALL PROGRAMMES A-Z, having first delivered `records` under every
// letter three times over. Nil records is the control: the same walk with nothing delivered.
func indexBeforeOpenRun(t *testing.T, records []broadcast.IndexRecord, rounds int, artefact string) uint32 {
	t.Helper()
	const (
		probePID  = 0x52
		headsPool = 0x800C51A0
	)
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

	word := func(virtual uint32) uint32 { return box.RAM.Read(virtual&0x1fffffff, bus.Word) }
	heads := word(headsPool)
	if heads&0xf0000000 == 0 || heads&3 != 0 {
		t.Fatalf("harness: the list-head table pointer is %08X, not a word-aligned guest address", heads)
	}
	slotOf := func(letter byte) uint32 { return heads + uint32(letter-0x40)*4 }

	// EVERY LETTER, THREE TIMES ROUND, BEFORE A SINGLE KEY IS PRESSED.
	if len(records) > 0 {
		version := byte(0)
		for round := 0; round < rounds; round++ {
			for letter := byte('A'); letter <= 'Z'; letter++ {
				ext, err := broadcast.IndexLetter(letter)
				if err != nil {
					t.Fatal(err)
				}
				section, err := broadcast.IndexSection(ext, version&0x1f, 0, 0, records)
				if err != nil {
					t.Fatal(err)
				}
				if err := box.Demux.Push(probePID, section); err != nil {
					t.Fatalf("harness: letter %q round %d was refused (%v)", letter, round, err)
				}
				version++
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
		filled := 0
		for letter := byte('A'); letter <= 'Z'; letter++ {
			if word(slotOf(letter)) != 0 {
				filled++
			}
		}
		t.Logf("after %d delivered sections, %d of 26 letter slots hold a non-null head "+
			"(slot 'A' = %08X)", rounds*26, filled, word(slotOf('A')))
	}

	// WATCH THE SLOT FOR WRITES AS WELL AS READS from here on: the previous probe watched reads
	// alone, so it could not have seen whatever fills it.
	slotA := slotOf('A')
	reads, writes := 0, 0
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch {
			return
		}
		if at := a.Virtual | 0x80000000; at >= slotA && at < slotA+4 {
			if a.Write {
				writes++
			} else {
				reads++
			}
		}
	}}
	press := azPressFunc(t, box, func() error { return transmitter.Pump(box.Machine.Retired) })

	inner := openAllProgrammesAtoZ(t, press, ".artifacts/"+artefact)

	final := inner
	for i := 0; i < 30_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
		if i%1_000_000 == 0 {
			final = screenNow(t, box)
		}
	}
	if err := dumpScreen(t, box, artefact); err != nil {
		t.Fatal(err)
	}
	t.Logf("slot 'A' saw %d reads and %d WRITES during the walk; it now holds %08X",
		reads, writes, word(slotA))
	t.Logf("the walk finished on %08X (artefact %s)", final, artefact)
	return final
}
