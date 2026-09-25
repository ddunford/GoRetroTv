package firmwaretests_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

// The handset map, pinned.
//
// Swept exhaustively on 2026-09-20 -- every code 0x00..0xFF pressed on its own
// restored box. The original reading mislabeled 0x80 as TV Guide and omitted
// 0xCC because it inferred names from idle-screen changes. A tuned-state probe
// now pins the physical handset distinction: 0x80 is Sky and 0xCC is TV Guide.
//
//   - each key draws a DIFFERENT screen, so the labels are not four names for
//     one thing;
//   - the codes around them are inert, so the map is complete rather than the
//     part of it somebody happened to look at.
//
// The five menu-area codes the handset sends, named once so tests say what they mean.
const (
	skyKey         = 0x80
	tvGuideKey     = 0xCC
	boxOfficeKey   = 0x7d
	servicesKey    = 0x7e
	interactiveKey = 0xf5
)

func TestTheHandsetKeysAreTheFiveTheFirmwareAnswers(t *testing.T) {
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
		{"sky", skyKey},
		{"tv guide", tvGuideKey},
		{"box office", boxOfficeKey},
		{"services", servicesKey},
		{"interactive", interactiveKey},
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
	for _, code := range []int{0x7f, 0x82, 0xf6} {
		if screen(code) != idle {
			t.Errorf("%#02x now draws something; the handset map is no longer the five codes the "+
				"sweep found, so sweep 0x00..0xFF again and update the record", code)
		}
	}
}
