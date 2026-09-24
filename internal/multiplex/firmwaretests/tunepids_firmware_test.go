package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT THE BOX ASKS FOR WHEN IT TUNES TO A CHANNEL.
//
// "No satellite signal is being received" is the box reporting that it has tuned to a service and
// never seen a stream for it. Two routes a stream could arrive by have now been eliminated as the
// place to look:
//
//   - the demux transport path is gated in THIS MODEL on bit 0 of register 0x140, which the box
//     preserves through read-modify-write -- and the record describes +0x140/+0x144/+0x148 as section-filter MATCH AND
//     MASK programming, so that gate is probably ours rather than the hardware's;
//   - the media DMA is real and used -- 644 writes across opening the guide and viewing -- but
//     ONLY CHANNEL 12 is ever armed, and it is armed when the guide opens, which is the OSD.
//
// So nothing is moving video, and the question narrows to why: a box that means to present a
// service has to know which PIDs carry it, and it learns that from the stream. This asks the
// simplest version of that -- DOES IT ARM A NEW PID WHEN IT TUNES? -- by dumping the demux's armed
// filters before and after.
//
// A new PID would be the box asking, and would name exactly what to send. No new PID means it
// never gets far enough to ask, and then the question is what it is waiting for before it would.
//
// IT ASSERTS ITS OWN SUBJECT: filters must be armed at all, and the box must actually leave the
// guide, or the comparison is between two copies of the same thing.
//
// IT ONLY READS.
func TestWhatPIDsTheBoxArmsWhenItTunes(t *testing.T) {
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
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	armed := func() map[uint16]uint8 {
		out := map[uint16]uint8{}
		for _, f := range box.Demux.ArmedFilters() {
			out[f.PID] = f.Filter
		}
		return out
	}
	show := func(label string, m map[uint16]uint8) {
		pids := make([]uint16, 0, len(m))
		for pid := range m {
			pids = append(pids, pid)
		}
		sort.Slice(pids, func(a, b int) bool { return pids[a] < pids[b] })
		var line string
		for _, pid := range pids {
			line += fmt.Sprintf(" %#04x(ch%d)", pid, m[pid])
		}
		t.Logf("%-28s %d armed:%s", label, len(m), line)
	}
	beforeTune := armed()
	if len(beforeTune) == 0 {
		t.Fatal("harness: no filter is armed after acquisition, so this box has not acquired and " +
			"the comparison below is meaningless")
	}
	show("after acquisition", beforeTune)

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := azPressFunc(t, box, pump)
	openAllChannels(t, press, ".artifacts/tunepids-grid.png", true)
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	inGuide := armed()
	show("with the guide open", inGuide)

	before := screenNow(t, box)
	viewing := uint32(0)
	for attempt := 1; attempt <= 6 && (viewing == 0 || viewing == before); attempt++ {
		viewing = press(keySelect, "select to view the channel", 80_000_000)
	}
	if viewing == 0 || viewing == before {
		t.Fatalf("harness: SELECT never left the grid (%08X), so the box never tuned and there is "+
			"no 'after' to compare", before)
	}
	for i := 0; i < 60_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "tunepids-viewing.png"); err != nil {
		t.Fatal(err)
	}
	afterTune := armed()
	show("while VIEWING the channel", afterTune)

	var added []uint16
	for pid := range afterTune {
		if _, had := beforeTune[pid]; !had {
			added = append(added, pid)
		}
	}
	sort.Slice(added, func(a, b int) bool { return added[a] < added[b] })
	if len(added) == 0 {
		t.Logf("=== THE BOX ARMS NO NEW PID WHEN IT TUNES ===")
		t.Logf("    It viewed %08X and asked the hardware for nothing it was not already "+
			"receiving. So it never gets as far as wanting a stream, and sending one would be "+
			"answering a question it has not asked. What it is waiting for BEFORE it would ask "+
			"is the next thing to find.", viewing)
		return
	}
	t.Logf("=== THE BOX ARMED %d NEW PID(S) ON TUNING ===", len(added))
	for _, pid := range added {
		t.Logf("    PID %#04x on filter %d -- this is the box asking, and it names what to send",
			pid, afterTune[pid])
	}

	// AND WHICH TABLE IT WANTS THERE. A PID says where, a match unit says what: the first byte a
	// unit compares is the table_id, because a match unit skips section_length. All sixteen are
	// dumped UNCONDITIONALLY -- this project's own rule, written after a census that assumed a
	// mask and reported a box as asking for nothing while unit 7 sat there asking for a3/ff.
	t.Logf("=== all sixteen match units while viewing ===")
	for unit := uint8(0); unit < 16; unit++ {
		var line string
		empty := true
		for b := uint8(0); b < 6; b++ {
			m, ok := box.Demux.Match(unit, b)
			if !ok {
				break
			}
			if m.Value != 0 || m.Mask != 0 {
				empty = false
			}
			line += fmt.Sprintf(" %02x/%02x", m.Value, m.Mask)
		}
		if empty {
			continue
		}
		note := ""
		if m, ok := box.Demux.Match(unit, 0); ok {
			switch {
			case m.Value == 0x4e:
				note = "   <- EIT present/following, THIS TRANSPORT"
			case m.Value == 0x4f:
				note = "   <- EIT present/following, another transport"
			case m.Value >= 0x50 && m.Value <= 0x6f:
				note = "   <- EIT schedule"
			case m.Value == 0x42:
				note = "   <- SDT"
			case m.Value == 0x4a:
				note = "   <- BAT"
			case m.Value == 0x40:
				note = "   <- NIT"
			case m.Value == 0x73:
				note = "   <- TOT"
			case m.Value == 0xc1:
				note = "   <- the Sky index"
			case m.Value >= 0xa0 && m.Value <= 0xa4:
				note = "   <- a Sky title table"
			}
		}
		t.Logf("    unit %2d %s%s", unit, line, note)
	}
}
