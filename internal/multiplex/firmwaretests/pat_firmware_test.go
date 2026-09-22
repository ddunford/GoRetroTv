package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// SEND THE BOX A PAT AND SEE WHICH PID IT ASKS FOR NEXT.
//
// PID 0x0000 is armed during acquisition -- transiently, which is why a single sample of the demux
// missed it and a continuous watch across a session found it -- and nothing has ever been sent
// there. A programme association table is the root of a transport stream: it says which programmes
// exist and which PID carries each one's map table. So this is the one place where the next layer
// of the signal can be offered to a filter the box has actually opened.
//
// **THE BOX NAMES THE NEXT STEP, NOT US.** If it reads the PAT and wants the programme map, it will
// ARM THE PID THIS TABLE NAMES -- a PID chosen here and meaningless to the firmware until it is
// told about it, which is what makes an arming of it proof that the PAT was read and believed. That
// is the whole design: nothing downstream is guessed, because the box asks for it by number.
//
// A NEGATIVE IS ALSO A RESULT and must not be dressed up: a box that reads the PAT and arms nothing
// has told us the map PID is not how it proceeds, and that is worth knowing before a PMT is built.
//
// IT ONLY READS, apart from the section it transmits -- which is the point, and is an input on the
// wire rather than a poke into memory.
func TestWhetherAPATMakesTheBoxAskForAProgrammeMap(t *testing.T) {
	// A PID the firmware has no reason to hold until the PAT names it. Inside the thirteen-bit
	// range and clear of everything the box already arms.
	const mapPID = 0x0100
	const programme = 1

	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	armed := func() map[uint16]bool {
		out := map[uint16]bool{}
		for _, pid := range box.Demux.ArmedPIDs() {
			out[pid] = true
		}
		return out
	}

	// Run until the box opens PID 0. It does so while acquiring and closes it again, so this
	// watches rather than samples, and pushes the moment the filter is there.
	pushed, before := false, map[uint16]bool{}
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000, func(i int) bool {
		if i%4096 == 0 && !pushed && armed()[0x0000] {
			sub, err := multiplex.Read(box.Demux)
			if err != nil {
				return false // the box has not settled its subscription yet; try again shortly
			}
			// The transport stream id this transmitter uses everywhere else, so the PAT agrees
			// with the NIT and SDT the box has already taken.
			section, err := broadcast.PAT(sub.NetworkID, 0, []broadcast.Programme{
				{Number: programme, MapPID: mapPID},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := box.Demux.Push(0x0000, section); err != nil {
				t.Fatalf("harness: the box refused the PAT on PID 0 (%v), so PID 0 was not armed "+
					"after all and nothing below was measured", err)
			}
			before, pushed = armed(), true
			t.Logf("PAT sent on PID 0000: transport stream %04X, programme %d -> map PID %04X",
				sub.NetworkID, programme, mapPID)
			// WHAT THE BOX IS FILTERING FOR, dumped at the one moment PID 0 is open.
			//
			// THE TWO HALVES ARE REPORTED SEPARATELY BECAUSE THEY ARE SEPARATE, and conflating
			// them produced a confident nonsense a moment ago. There are 32 PID CHANNELS and only
			// 16 MATCH UNITS; Match refuses any unit index of 16 or more, so passing a channel
			// number (these run 19..24) returned "not set" for every one and read as "this filter
			// takes any table on its PID" -- six times over, uniformly, which is the tell. The
			// mapping from channel to unit is not exposed by the model, so it is not claimed here.
			for _, f := range box.Demux.ArmedFilters() {
				t.Logf("    channel %2d watches PID %04X, whole register word %08X (upper bits %05X)",
					f.Filter, f.PID, f.Word, f.Word>>13)
			}
			for unit := uint8(0); unit < 16; unit++ {
				var rules []string
				for b := uint8(0); b < 16; b++ {
					m, ok := box.Demux.Match(unit, b)
					if !ok || m.Mask == 0 {
						continue
					}
					rules = append(rules, fmt.Sprintf("byte%d %02X/%02X", b, m.Value, m.Mask))
				}
				if len(rules) > 0 {
					t.Logf("    match unit %2d wants %v", unit, rules)
				}
			}
		}
		return registeringProgrammes(box, want, &registered)(i)
	}); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if !pushed {
		t.Fatal("harness: PID 0x0000 was never armed during this acquisition, so the PAT was never " +
			"sent and this measured nothing -- the continuous watch that found it armed must have " +
			"caught a different phase")
	}

	// Give it room to read the table and act on it.
	seen := map[uint16]bool{}
	runUntil(t, box, transmitter, 60_000_000, func(i int) bool {
		if i%4096 == 0 {
			for pid := range armed() {
				seen[pid] = true
			}
		}
		return false
	})

	var added []uint16
	for pid := range seen {
		if !before[pid] {
			added = append(added, pid)
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i] < added[j] })
	// FORMATTED AS HEX. The first version printed a []uint16 with %v and reported that the box
	// armed "82", which is 0x52 -- the one transient PID this project already knows about -- and
	// read for a moment like a new discovery. Every other PID in this repository is written in hex.
	hexPIDs := make([]string, 0, len(added))
	for _, pid := range added {
		hexPIDs = append(hexPIDs, fmt.Sprintf("%04X", pid))
	}

	if seen[mapPID] {
		t.Logf("VERDICT: THE BOX ARMED %04X -- the PID this PAT named and which the firmware had no "+
			"other reason to hold. It read the table, believed it, and is asking for the programme "+
			"map. That is the next layer named by the box rather than guessed, and a PMT on %04X "+
			"is what answers it.", mapPID, mapPID)
		return
	}
	if len(added) == 0 {
		t.Logf("VERDICT: the box armed NOTHING new after the PAT. It either did not read it, did "+
			"not believe it, or does not reach services this way. A PMT on %04X would be talking "+
			"to nobody, so build nothing until this is understood.", mapPID)
		return
	}
	t.Logf("VERDICT: the box did NOT arm %04X, but it did arm %v after the PAT. That is not the "+
		"map PID this table named, so something else changed -- worth following before anything "+
		"is built on it.", mapPID, hexPIDs)
}
