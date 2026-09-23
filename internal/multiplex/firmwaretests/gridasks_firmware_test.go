package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT DOES THE BOX ASK THE BROADCAST FOR WHEN THE GRID IS OPEN?
//
// The ALL CHANNELS grid draws a header and no rows, and the search for the fault by WATCHING READS
// has gone as far as it can. The reason is worth stating because it redirects the work: the grid
// never reads the line-up, never handles our channel numbers, and the tables it does walk turn out
// to be a fixed 512-entry resource directory and a widget context holding 720x576. **An empty list
// generates no reads at all**, so a read watch cannot find one -- the absence looks identical to a
// list that is somewhere else.
//
// SO ASK THE BOX INSTEAD. This hardware says out loud what it wants: the demux has sixteen match
// units and the guest PROGRAMS them for the tables it is filtering for. If the grid needs something
// the transmitter has never sent, the box will have armed a filter for it and that filter is
// readable. That is the difference between reverse-engineering the requirement and guessing at an
// input, and this project's Phase 7 rule is the second one written in blood: do not guess a format,
// let the box name what it wants.
//
// THE DIFFERENTIAL IS THE POINT. What the box asks for while merely acquiring is not interesting;
// what it asks for that it DID NOT ask for before the grid opened is. So the armed state is taken
// twice, and the two are compared.
//
// IT ONLY READS. Nothing is written into the guest, and no input is invented to see what sticks.
func TestWhatTheBoxAsksTheBroadcastForWhenTheGridOpens(t *testing.T) {
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

	// asked is every armed PID and every programmed match rule, as one comparable set. The match
	// units are dumped UNCONDITIONALLY, all sixteen: the record's own rule is that "the box is not
	// asking for it" is a symptom with four known causes, and three of them were instruments that
	// looked at some of the units rather than all of them.
	asked := func() map[string]bool {
		out := map[string]bool{}
		for _, pid := range box.Demux.ArmedPIDs() {
			out[fmt.Sprintf("PID %04X armed", pid)] = true
		}
		for unit := uint8(0); unit < 16; unit++ {
			for b := uint8(0); b < 8; b++ {
				m, ok := box.Demux.Match(unit, b)
				if !ok || m.Mask == 0 {
					continue
				}
				out[fmt.Sprintf("unit %2d byte %d matches %02X/%02X", unit, b, m.Value, m.Mask)] = true
			}
		}
		return out
	}

	// LET IT GO QUIET FIRST. The record is explicit that a key sent the instant the box finishes
	// something does nothing at all, and this instrument pressed immediately after acquisition --
	// which was survivable only because the box happened to be idle by then. It is the wedge
	// guard's discipline, and it belongs here too.
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	before := asked()
	if len(before) == 0 {
		t.Fatal("harness: the box has armed nothing at all after acquiring, so either the demux is " +
			"not being read or this box never acquired -- either way the diff below would be empty " +
			"for the wrong reason")
	}
	// THE WHOLE LIST, PRINTED. This is the honest form of "emulate the whole satellite signal":
	// not inventing tables and hoping one sticks, but filling the filters the box has ALREADY
	// PROGRAMMED. A section on a PID with no armed filter is dropped by the hardware before any
	// code sees it, so transmitting beyond this list cannot reach anything at all -- which makes
	// the list both the opportunity and its boundary.
	t.Logf("after acquiring, the box is asking for %d things:", len(before))
	keys := make([]string, 0, len(before))
	for k := range before {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("    %s", k)
	}

	// THE GAP, STATED BY THE INSTRUMENT RATHER THAN SPOTTED BY EYE. The transmitter uses exactly
	// three PIDs: 0x11 for the NIT, SDT and BAT, 0x14 for the clock, and the day's title PID, which
	// is a function of the date (multiplex.TitlePID). Every other PID the box has armed is a filter
	// it programmed and nothing has ever answered -- and because the hardware drops a section on an
	// unarmed PID before any code sees it, this list is not merely where there is room to send
	// more, it is the ONLY place sending more could ever reach.
	mjd := multiplex.MJDOf(day)
	sending := map[uint16]string{
		0x10: "NIT", 0x11: "SDT and BAT", 0x14: "TDT and TOT",
		multiplex.TitlePID(mjd): fmt.Sprintf("titles for MJD %d", mjd),
	}
	var unfed []string
	for _, pid := range box.Demux.ArmedPIDs() {
		if what, ok := sending[pid]; ok {
			t.Logf("    PID %04X armed and FED -- %s", pid, what)
			continue
		}
		unfed = append(unfed, fmt.Sprintf("%04X", pid))
	}
	sort.Strings(unfed)
	if len(unfed) == 0 {
		t.Log("every PID the box has armed is being fed, so there is no unanswered filter to fill")
	} else {
		t.Logf("ARMED AND NEVER FED: PIDs %v. These are inputs the box has asked for in its own "+
			"words, and the only ones a broader broadcast could reach.", unfed)
	}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		settled := pressAndLetItFinish(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, raw, budget)
		t.Logf("%-28s drew %08X", name, settled)
		return settled
	}

	menu := press(keyBoxOffice, "box office", 80_000_000)
	// PROVE WHICH TAB, DO NOT ASSUME IT MOVED. one LEFT from box office draws 0xFE8D1CCC, which is STILL
	// THE BOX OFFICE MENU -- the wedge guard names it as a constant for this reason -- and a route that only checks "the screen changed"
	// accepts it as the tv guide tab and then measures box office for the rest of the run. That is
	// the FOURTH time this shape has cost this project a measurement, and the first three are in
	// the record. The screenshot is what caught it: the artefact said MOVIES BY START TIME.
	// Both verified by eye against the dumped pictures. A hash cannot tell ALL CHANNELS from the
	// same menu with its highlight moved, and an earlier version of this route accepted the BOX
	// OFFICE menu as the tv guide tab and measured MOVIES BY START TIME throughout.
	const tvGuideMenu = tvGuideMenuScreen // the ten-entry TV GUIDE menu, ALL CHANNELS highlighted
	tab := menu
	for attempt := 1; attempt <= 6 && tab != tvGuideMenu; attempt++ {
		tab = press(keyLeft, "left to the tv guide tab", 80_000_000)
	}
	if tab != tvGuideMenu {
		t.Fatalf("harness: never reached the TV GUIDE menu (%08X); settled on %08X",
			uint32(tvGuideMenu), tab)
	}
	// THE DESTINATION IS THE GRID AT EITHER OF ITS TWO SETTLED FRAMES. This used to pin
	// 0x42DBD889 -- ALL CHANNELS with no rows at all -- which was the only thing it ever drew
	// until the 0xB2 descriptor unlocked the listings. That screen cannot be reached any more, so
	// the pin was a loop that pressed SELECT six times and dived past the grid; route_test.go
	// names the two frames it does settle on.
	grid := uint32(0)
	for attempt := 1; attempt <= 6 && !atAllChannels(grid); attempt++ {
		runUntil(t, box, transmitter, 8_000_000, func(int) bool { return false })
		grid = press(keySelect, fmt.Sprintf("select ALL CHANNELS (try %d)", attempt), 60_000_000)
	}
	if !atAllChannels(grid) {
		t.Fatalf("harness: select never reached ALL CHANNELS (%08X searching or %08X filled); "+
			"settled on %08X, and a screen that merely differs from the menu is what has been "+
			"measured by mistake before",
			uint32(allChannelsOpening), uint32(allChannelsFilled), grid)
	}
	if err := dumpScreen(t, box, "grid-asks-all-channels.png"); err != nil {
		t.Fatal(err)
	}
	// Let it sit: a request the screen makes on entry may take a moment to reach the hardware.
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	after := asked()
	var added, dropped []string
	for k := range after {
		if !before[k] {
			added = append(added, k)
		}
	}
	for k := range before {
		if !after[k] {
			dropped = append(dropped, k)
		}
	}
	sort.Strings(added)
	sort.Strings(dropped)

	if len(added) == 0 && len(dropped) == 0 {
		t.Logf("VERDICT: opening the grid changes NOTHING about what the box asks the broadcast "+
			"for -- the same %d filters before and after. So it is not waiting on an input we are "+
			"failing to send: it never asks for one. Whatever the grid needs, it expects to "+
			"already have.", len(before))
		return
	}
	t.Logf("VERDICT: opening the grid CHANGES what the box is filtering for -- %d new, %d dropped. "+
		"That is the box naming an input, and the new filters are the specification for it.",
		len(added), len(dropped))
	for _, k := range added {
		t.Logf("    NEW     %s", k)
	}
	for _, k := range dropped {
		t.Logf("    dropped %s", k)
	}
}
