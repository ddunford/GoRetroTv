package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// EVERY KEY THE PAGE OFFERS MUST BE ONE THE SERVER ACCEPTS, and nothing checked
// that until a viewer pressed one that was not.
//
// The handset in web/index.html carries a `data-raw` per button and app.ts
// sends it verbatim; decodeKey then refuses anything outside handsetRaw and
// CLOSES THE SOCKET with StatusPolicyViolation. So a button whose code is
// missing from the allowlist is not inert -- it disconnects the person who
// presses it.
//
// That is exactly what `services` did. `0x7E` was on the handset from the day
// the key was added and has never been in handsetRaw (`git log -S"0x7e"` on
// this file is empty), so pressing it dropped the viewer's connection on the
// live demo -- measured, 200ms to "The handset is unavailable while
// disconnected." and ~400ms to reconnect. The firmware test next door has
// pressed 0x7E and watched it draw the Services tab for as long as it has
// existed; it calls box.CSI.Key directly, so it never touched this path and the
// two halves could disagree indefinitely with both suites green.
//
// This is the joint. It reads the page the product actually serves rather than
// a list copied from it, because a copied list drifts in exactly the way this
// defect drifted.
func TestEveryHandsetKeyOnThePageIsAcceptedByTheServer(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("..", "..", "web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`data-raw="(0[xX][0-9a-fA-F]+)"`).FindAllSubmatch(page, -1)
	if len(found) < 10 {
		t.Fatalf("harness: only %d data-raw codes found in web/index.html; the handset has more than "+
			"that, so this pattern has stopped matching and the check is not running", len(found))
	}
	for _, m := range found {
		raw, err := strconv.ParseUint(string(m[1]), 0, 8)
		if err != nil {
			t.Errorf("data-raw %q is not an 8-bit code", m[1])
			continue
		}
		if !handsetRaw(uint8(raw)) { // #nosec G115 -- ParseUint bounded it to 8 bits
			t.Errorf("the handset offers %s but handsetRaw refuses it, so pressing that button "+
				"closes the viewer's socket with \"invalid key\"", m[1])
		}
	}
}

func TestPageExposesExplicitAudioStatesAndReconnectReset(t *testing.T) {
	app, err := os.ReadFile(filepath.Join("..", "..", "web", "app.ts"))
	if err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join("..", "..", "web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, subject := range []struct {
		name    string
		body    []byte
		pattern string
	}{
		{"sound control", page, `id="sound-toggle"`},
		{"locked state", app, `showAudioState('locked'`},
		{"unlocked state", app, `showAudioState('unlocked'`},
		{"muted state", app, `showAudioState('muted'`},
		{"reconnect audio reset", app, `stopAudio();`},
	} {
		if !regexp.MustCompile(regexp.QuoteMeta(subject.pattern)).Match(subject.body) {
			t.Errorf("%s is not explicit in the browser contract", subject.name)
		}
	}
}
