package firmwaretests_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// TASK-6.9's cross-check, against the BOX rather than against a reference
// decoder.
//
// The task asked for the encoder to be cross-checked against the reference
// DECODER. That instrument is the one that just failed: three defects in
// reading the Huffman dictionary survived a round-trip test, byte-for-byte
// reference vectors AND a decoder transcribed from the reference, because all
// of them share the encoder's reading of the dictionary. They are one
// instrument, not three.
//
// The box is the independent one, and it needs no character recognition to be
// useful. Broadcast a title, press tv guide, hash the frame. Then broadcast the
// title the OLD ENCODER WOULD HAVE PRODUCED -- the same words with the spaces
// removed, and the same words with a trailing "s" -- and require all three
// screens to DIFFER. Under either defect two of them would be identical, which
// is a fact about pixels that no amount of agreement between our encoder and
// our decoder can paper over.
func TestTheDrawnTitleDistinguishesWhatTheEncoderCouldGetWrong(t *testing.T) {
	dict := demoDictionary(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	// One channel, one programme, on now. Everything else about the screen is
	// identical between runs -- same clock, same channel, same layout -- so a
	// difference in the frame is a difference in the drawn title.
	draw := func(t *testing.T, title string) uint32 {
		t.Helper()
		dir := t.TempDir()
		body := `{"bouquet":"Sky Digital","services":[{"name":"Sky One","channel":101,` +
			`"serviceId":100,"listingsId":101,"programmes":[` +
			`{"start":"19:00","minutes":60,"title":"` + title + `"}]}]}`
		if err := os.WriteFile(filepath.Join(dir, "default.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		guide, err := multiplex.LoadGuide(dir)
		if err != nil {
			t.Fatal(err)
		}
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
		if err != nil {
			t.Fatal(err)
		}
		registered := 0
		if at := runUntil(t, box, transmitter, 40_000_000,
			registeringProgrammes(box, 1, &registered)); at < 0 {
			t.Fatalf("%q was never taken by the box, so nothing was drawn from it", title)
		}
		// Let the box finish taking the section before asking to see it, then
		// wait for the drawing to STOP rather than for a guessed number of
		// instructions -- the first version of this caught the banner
		// mid-redraw and hashed a frame that was half of each screen.
		runUntil(t, box, transmitter, 30_000_000, func(int) bool { return false })
		before := screenNow(t, box)
		if err := box.CSI.Key(tvGuideKey, 0); err != nil {
			t.Fatal(err)
		}
		hash := drawnScreen(t, box, transmitter, 60_000_000, before)
		t.Logf("%-14q drew %08X", title, hash)
		return hash
	}

	withSpace := draw(t, "Dream Team")
	withoutSpace := draw(t, "DreamTeam")
	withTrailingS := draw(t, "Dream Teams")

	if withSpace == withoutSpace {
		t.Error(`"Dream Team" and "DreamTeam" draw the same screen, so the space is not reaching ` +
			`the box -- which is the defect where a value's long filler code was emitted instead ` +
			`of its short one`)
	}
	if withSpace == withTrailingS {
		t.Error(`"Dream Team" and "Dream Teams" draw the same screen, so a trailing character is ` +
			`being added -- which is the defect where the final byte was zero-filled and "s" is ` +
			`coded 0000`)
	}
	if withoutSpace == withTrailingS {
		t.Error("two titles that differ in two ways drew the same screen; the instrument is not " +
			"distinguishing titles at all and none of the checks above mean anything")
	}
}
