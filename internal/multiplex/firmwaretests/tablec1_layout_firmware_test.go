package firmwaretests_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/broadcast"
)

// WHAT THE FIVE UNSWEPT BYTES OF AN INDEX RECORD ARE FOR.
//
// The addressing is now settled end to end: table 0xC1 on PID 0x52, extension = the letter, and the
// section must arrive BEFORE the screen opens, because the parser hands its decoded array to
// whatever already sits in the list-head slot. Delivered that way, all twenty-six slots fill and
// ALL PROGRAMMES A-Z stops being bare -- it gains "Page Up" and "Page Down", which is pagination
// furniture a screen only draws once it believes it has a paginated list.
//
// The body still says "Searching for listings", and the reason is that rec[4..8] have never been
// swept off zero. An index entry has to let the screen FIND a programme, and the only key the title
// store has is (listings id, event id). Nine bytes carry: a sixteen-bit id at rec[0..1], a flags
// byte at rec[2], six wire bits at rec[3] the parser does not read, then rec[4], rec[5..6] and
// rec[7..8] -- three fields, into which two known values must go.
//
// SIX LAYOUTS, ONE RECORD EACH, IN ONE SECTION. The payload is an array, so the whole sweep costs
// one delivery instead of six, and a drawn row NAMES its layout by position. Every record points at
// the same programme -- Sky One (listings id 0x0065), "Dream Team" at 7.00pm, event id 14 -- which
// is the one the now-and-next banner already resolves off the same broadcast, so a layout that
// works cannot be failing for want of the programme.
//
//	1  id=listings  rec[5..6]=event
//	2  id=event     rec[5..6]=listings
//	3  id=listings  rec[7..8]=event
//	4  id=event     rec[7..8]=listings
//	5  id=event     rec[5..6]=listings  rec[7..8]=event
//	6  id=listings  rec[5..6]=event     rec[7..8]=listings
//
// THE COMPARISON IS AGAINST THE KNOWN DELIVERED SCREEN, not against an empty one. Channel-id
// records with rec[4..8] zero already move this screen; so a layout that changes nothing shows the
// same hash as that, and only a layout that resolves a programme can show something else. The
// picture decides, as ever.
func TestWhichIndexRecordLayoutResolvesAProgramme(t *testing.T) {
	const (
		skyOne     = 0x0065 // Sky One's listings id
		packed     = 0x0f   // all four flag bits set, the line-up's own "visible" encoding
		selectorHi = 0xc0
	)
	hi := func(v uint16) byte { return byte(v >> 8) }
	lo := func(v uint16) byte { return byte(v & 0xff) }

	// A DIFFERENT PROGRAMME PER LAYOUT, WHICH IS THE WHOLE POINT OF THE SECOND RUN. The first
	// version pointed all six at "Dream Team" and drew ten identical rows -- proof that SOMETHING
	// resolves, and no way at all to say which of the six did it. Each layout now carries its own
	// event, so the titles on screen name the layouts that work, by name, in one picture.
	//
	// All six events are Sky One's and all six are inside the 18:00-23:59 block the box subscribes
	// to, so every one of them is demonstrably in the store: a layout that does not draw is a
	// layout that failed, not a programme that was missing.
	type candidate struct {
		what  string
		event uint16
		rec   broadcast.IndexRecord
	}
	candidates := []candidate{
		{"1  id=listings, rec[5..6]=event", 13, broadcast.IndexRecord{ID: skyOne,
			Data: [5]byte{0, 0, 13, 0, 0}}},
		{"2  id=event, rec[5..6]=listings", 14, broadcast.IndexRecord{ID: 14,
			Data: [5]byte{0, hi(skyOne), lo(skyOne), 0, 0}}},
		{"3  id=listings, rec[7..8]=event", 15, broadcast.IndexRecord{ID: skyOne,
			Data: [5]byte{0, 0, 0, 0, 15}}},
		{"4  id=event, rec[7..8]=listings", 16, broadcast.IndexRecord{ID: 16,
			Data: [5]byte{0, 0, 0, hi(skyOne), lo(skyOne)}}},
		{"5  id=event, rec[5..6]=listings, rec[7..8]=event", 17, broadcast.IndexRecord{ID: 17,
			Data: [5]byte{0, hi(skyOne), lo(skyOne), 0, 17}}},
		{"6  id=listings, rec[5..6]=event, rec[7..8]=listings", 18, broadcast.IndexRecord{ID: skyOne,
			Data: [5]byte{0, 0, 18, hi(skyOne), lo(skyOne)}}},
	}
	titles := map[uint16]string{13: "Friends", 14: "Dream Team", 15: "Walker Texas Ranger",
		16: "The X Files", 17: "Star Trek Deep Space Nine", 18: "Midnight Mass"}
	records := make([]broadcast.IndexRecord, 0, len(candidates))
	for _, c := range candidates {
		r := c.rec
		r.Packed, r.Selector = packed, selectorHi
		records = append(records, r)
		t.Logf("layout %-52s -> %q", c.what, titles[c.event])
	}

	bare := indexBeforeOpenRun(t, channelIndexRecords(t), 3, "layout-bare.png")
	swept := indexBeforeOpenRun(t, records, 3, "layout-swept.png")
	t.Logf("=== channel ids only, rec[4..8] zero: %08X", bare)
	t.Logf("=== six candidate layouts:            %08X", swept)
	if bare == swept {
		t.Logf("none of the six layouts changed what the screen draws, so the programme reference " +
			"is not (listings id, event id) in any of those three field positions -- or it needs " +
			"rec[4] as well, which was held at zero throughout and is therefore NOT eliminated")
		return
	}
	t.Logf("THE LAYOUTS MOVED THE SCREEN. Read .artifacts/layout-bare.png and -swept.png: a drawn " +
		"row names its layout by position, 1 to 6 down the list")
}
