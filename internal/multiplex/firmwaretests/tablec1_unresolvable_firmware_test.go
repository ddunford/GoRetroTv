package firmwaretests_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/broadcast"
)

// WHAT AN INDEX ENTRY DRAWS WHEN THE PROGRAMME IT NAMES IS NOT IN THE STORE.
//
// This decides how much of the day the index may carry, and it is not a matter of taste. An index
// record is a REFERENCE -- the channel's listings id and the programme's event id -- and the box
// stores only the six-hour block its clock is in. So an index covering the whole day names
// programmes the box does not have.
//
// It matters because of what the demo shows. A-Z LISTINGS opens on letter 'A', and in this
// schedule the evening block has no title beginning with A at all -- the initials in it are
// DFGILMNRSTW -- while the whole day has four. An index limited to the block therefore opens on an
// empty page, which is honest and looks broken. Widening it to the day fixes that only if an entry
// the box cannot resolve is SKIPPED rather than drawn as a blank row, and guessing which would put
// either an empty screen or a screen full of gaps in front of a viewer.
//
// ONE SECTION, TWO RECORDS, AND THE DIFFERENCE BETWEEN THEM IS THE ONLY THING THAT VARIES. Both
// name Sky One. One names event 17, "Star Trek Deep Space Nine" at 10.00pm, which is inside the
// block and is demonstrably stored. The other names event 6, "Sally Jessy Raphael" at 11.00am,
// which is outside it and is demonstrably not. If the screen draws one row, unresolvable entries
// are skipped and the index may carry the whole day; if it draws two, it may not.
func TestWhatAnUnresolvableIndexEntryDraws(t *testing.T) {
	const (
		skyOne     = 0x0065
		stored     = 17 // "Star Trek Deep Space Nine", 10.00pm -- inside the 18:00-23:59 block
		notStored  = 6  // "Sally Jessy Raphael", 11.00am -- outside it
		packed     = 0x0f
		selectorHi = 0xc0
	)
	entry := func(event uint16) broadcast.IndexRecord {
		return broadcast.IndexRecord{
			ID: skyOne, Packed: packed, Selector: selectorHi,
			Data: [5]byte{0, byte(event >> 8), byte(event & 0xff), 0, 0}, // #nosec G115 -- masked
		}
	}
	both := indexBeforeOpenRun(t, []broadcast.IndexRecord{entry(stored), entry(notStored)}, 3,
		"unresolvable-both.png")
	onlyStored := indexBeforeOpenRun(t, []broadcast.IndexRecord{entry(stored)}, 3,
		"unresolvable-only-stored.png")
	t.Logf("=== a stored entry and an unstored one: %08X", both)
	t.Logf("=== the stored entry alone:             %08X", onlyStored)
	if both == onlyStored {
		t.Logf("ADDING AN UNRESOLVABLE ENTRY CHANGED NOTHING ON SCREEN, so the box skips what it " +
			"cannot resolve and the index may safely carry the whole day rather than one block")
		return
	}
	t.Logf("the two screens differ, so an entry the box cannot resolve is NOT silently skipped. " +
		"Read .artifacts/unresolvable-both.png against -only-stored.png before widening the index " +
		"beyond the block the box stores")
}
