package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHETHER THE BOX EVER SWITCHES ON THE TRANSPORT PACKET PATH.
//
// "No satellite signal is being received" sits over the picture whenever the demo tunes to a
// channel, and the record has already established what it is: a CHANNEL-level error, one entry in
// a message table beside "There is a technical fault with this channel". The box has tuned to a
// service, has never seen a transport stream carrying it, and says so. On its own terms it is
// right, which is why suppressing it would be the wrong move.
//
// The way to clear it is to send one. This port pushes SECTIONS straight into the demux at the
// section layer; the 188-byte packet path, Demux.PushTransport, exists and nothing in the product
// uses it.
//
// BUT THAT PATH IS GATED BY THE GUEST. PushTransport returns nil and does nothing unless the guest
// has set bit 0 of the register at 0x140 -- correctly, because the ROM's self-test uses the path
// before the application's rings exist. So a transport stream fed at a box that has not enabled it
// goes nowhere and says nothing, and any conclusion drawn from "we sent packets and the message
// stayed" would be about this flag rather than about the box.
//
// So the flag is the first thing to measure, and it decides which piece of work comes next:
// feeding packets, or finding out what makes the box ask for them.
//
// IT ONLY READS.
func TestWhetherTheBoxEnablesTheTransportPath(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("at restore, before anything is transmitted: transport path %s",
		enabled(box.Demux.TransportEnabled()))

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	t.Logf("after acquisition (%d programmes): transport path %s",
		registered, enabled(box.Demux.TransportEnabled()))
	// LET IT GO QUIET before pressing: a key sent the instant acquisition finishes is swallowed.
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	// AND AFTER TUNING, which is when the message appears. Pressing SELECT on the working grid
	// leaves the guide and views the channel, which is the moment the box would want a stream.
	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := azPressFunc(t, box, pump)
	openAllChannelsFinished(t, press, ".artifacts/transportpath-grid.png")
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	before := screenNow(t, box)
	viewing := uint32(0)
	for attempt := 1; attempt <= 6 && (viewing == 0 || viewing == before); attempt++ {
		viewing = press(keySelect, "select to view the channel", 80_000_000)
	}
	for i := 0; i < 30_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "transportpath-viewing.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("while viewing the channel (%08X): transport path %s",
		viewing, enabled(box.Demux.TransportEnabled()))
	if box.Demux.TransportEnabled() {
		t.Logf("SO PACKETS WOULD REACH THE GUEST. Feeding a transport stream is the next piece of " +
			"work, and the message can be answered rather than suppressed.")
		return
	}
	t.Logf("THE BOX NEVER ENABLES IT. So pushing 188-byte packets would be a no-op and would " +
		"prove nothing: the question is not how to build a transport stream but what makes this " +
		"box ask for one. Read .artifacts/transportpath-viewing.png.")
}

func enabled(on bool) string {
	if on {
		return "ENABLED"
	}
	return "off"
}
