package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT PIDS DOES THIS BOX EVER ARM -- across a whole session, not one snapshot?
//
// Before a transport stream can be sent there is a hard question to answer: a section or a packet
// on a PID with NO ARMED FILTER is dropped by the demux before any code sees it. So a PAT on PID
// 0x0000 reaches nothing unless the box has asked for PID 0x0000, and the armed set taken once
// after acquisition does not include it.
//
// ONE SNAPSHOT IS NOT THE ANSWER THOUGH. Filters are programmed and dropped as the box moves
// between screens and services -- the record already has PID 0x52 as one that "opens in response to
// our NIT and closes again", which a single sample would have missed entirely. So this watches the
// armed set CONTINUOUSLY across a session that moves the box around, and reports the UNION: every
// PID it ever asked for, and at which point it first appeared.
//
// THE ANSWER SHAPES THE WORK EITHER WAY. If PID 0 is never armed, a PAT is not the way in and the
// box must be reaching services by some other route, which is worth knowing before building one.
// If it is armed, even briefly, that is the door.
//
// IT ONLY READS.
func TestEveryPIDTheBoxEverArms(t *testing.T) {
	const tvGuideMenu = 0xDDBC18E9
	const allChannels = 0x42DBD889

	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	firstSeen := map[uint16]string{}
	phase := "acquiring"
	note := func() {
		for _, pid := range box.Demux.ArmedPIDs() {
			if _, ok := firstSeen[pid]; !ok {
				firstSeen[pid] = phase
			}
		}
	}
	// Sampled often enough to catch a filter that opens and closes: the record has one that does.
	watch := func(budget int) {
		for i := 0; i < budget; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
			if i%20000 == 0 {
				note()
			}
		}
		note()
	}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000, func(i int) bool {
		if i%20000 == 0 {
			note()
		}
		return registeringProgrammes(box, want, &registered)(i)
	}); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	phase = "settled after acquiring"
	watch(20_000_000)

	press := func(raw uint8, name string) uint32 {
		t.Helper()
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, settled := 0, before, uint32(0)
		for i := 0; i < 80_000_000; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
			if i%20000 == 0 {
				note()
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
		t.Logf("%-28s drew %08X", name, settled)
		return settled
	}

	phase = "tv guide banner"
	press(0x80, "tv guide")
	watch(10_000_000)

	phase = "box office"
	screen := press(keyBoxOffice, "box office")
	phase = "tv guide menu"
	for attempt := 1; attempt <= 6 && screen != tvGuideMenu; attempt++ {
		screen = press(keyLeft, "left to the tv guide menu")
	}
	phase = "ALL CHANNELS"
	for attempt := 1; attempt <= 6 && screen != allChannels; attempt++ {
		screen = press(keySelect, fmt.Sprintf("select ALL CHANNELS (try %d)", attempt))
	}
	if screen != allChannels {
		t.Logf("note: never reached ALL CHANNELS (settled on %08X); the union below still stands "+
			"for the screens it did reach", screen)
	}
	watch(10_000_000)

	if len(firstSeen) == 0 {
		t.Fatal("harness: the box armed nothing at any point, so either the demux is not being read " +
			"or this box never acquired")
	}
	pids := make([]uint16, 0, len(firstSeen))
	for pid := range firstSeen {
		pids = append(pids, pid)
	}
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	t.Logf("across the whole session the box armed %d distinct PIDs:", len(pids))
	for _, pid := range pids {
		t.Logf("    PID %04X  first seen while %s", pid, firstSeen[pid])
	}
	if _, ok := firstSeen[0]; ok {
		t.Log("VERDICT: PID 0x0000 IS armed at some point, so a PAT can reach the box and the " +
			"transport-stream work has a door.")
		return
	}
	t.Log("VERDICT: PID 0x0000 is NEVER armed, in any phase of this session. A PAT would be dropped " +
		"by the demux before any code saw it, so 'send a PAT' is not the way in -- the box is " +
		"reaching its services by some other route, and finding that route comes before building " +
		"a transport stream.")
}
