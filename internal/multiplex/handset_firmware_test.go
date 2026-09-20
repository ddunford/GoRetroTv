package multiplex_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

// The handset map, pinned.
//
// Swept exhaustively on 2026-09-20 -- every code 0x00..0xFF pressed on its own
// restored box -- and these five are the ONLY codes this firmware reacts to
// from the idle picture. The sweep itself is a one-off measurement and lives in
// the record; what is worth keeping is the result, and the two claims that the
// page's labelling rests on:
//
//   - each key draws a DIFFERENT screen, so the labels are not four names for
//     one thing;
//   - the codes around them are inert, so the map is complete rather than the
//     part of it somebody happened to look at.
//
// The firmware names these keys itself, on the SERVICES help page: "press 'tv
// guide'", "the 'box office' key", "press 'services'". There is no key that
// opens the menu on TV GUIDE -- 0x7D opens it on BOX OFFICE, and TV GUIDE is
// one LEFT of that -- which is why the handset carries these four and not a
// fifth called sky or home.
func TestTheHandsetKeysAreTheFourTheFirmwareAnswers(t *testing.T) {
	const settle = 5_000_000

	screen := func(code int) uint32 {
		box := restoredBox(t)
		if code >= 0 {
			if err := box.CSI.Key(uint8(code), 0); err != nil { // #nosec G115 -- a key code
				t.Fatal(err)
			}
		}
		for i := 0; i < settle; i++ {
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
		}
		picture, err := box.Compose()
		if err != nil {
			t.Fatal(err)
		}
		return statehash.HashBytes(picture.Pix)
	}

	idle := screen(-1)
	seen := map[uint32]string{idle: "the idle picture"}
	for _, key := range []struct {
		name string
		code int
	}{
		{"tv guide", 0x80},
		{"box office", 0x7d},
		{"services", 0x7e},
		{"interactive", 0xf5},
	} {
		hash := screen(key.code)
		if other, clash := seen[hash]; clash {
			t.Errorf("%s (%#02x) draws the same screen as %s, so one of the two labels is wrong",
				key.name, key.code, other)
			continue
		}
		seen[hash] = key.name
	}

	// The neighbours. If one of these started drawing something, the map is no
	// longer complete and the sweep needs re-running -- which is a finding, not
	// a failure, and the message says so.
	for _, code := range []int{0x7f, 0x81, 0xf6} {
		if screen(code) != idle {
			t.Errorf("%#02x now draws something; the handset map is no longer the five codes the "+
				"sweep found, so sweep 0x00..0xFF again and update the record", code)
		}
	}
}
