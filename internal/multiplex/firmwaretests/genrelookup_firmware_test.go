package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT THE GENRE SCREEN LOOKS UP, AND WHAT IT GETS BACK.
//
// TestWhatTheGenreGridRejects named the branch. ENTERTAINMENT runs 44 native addresses that ALL
// CHANNELS never touches, against a noise floor of EXACTLY ZERO -- two runs of ALL CHANNELS
// execute byte-identical sets -- and they cluster in one place:
//
//	800CB9F4..800CB9FE    800CBA16..800CBA28    800CBAFC..800CBB06
//	800A4D5E..800A4D8A    800A4DFC..800A4E00
//
// 0x800CB is the SI DESCRIPTOR REGION this file already maps: 0xB2 at 0x800CB008, 0xB4 at
// 0x800CB44E, and the search at 0x800CB69C that presets its caller's out-parameter to 0xFFFFFFFF
// before looking. So the genre grid asks the SI for something per channel and the answer is no --
// which is also why the accept side is where the Huffman decompressor at 0x800BECF0 runs and the
// reject side is not: a screen with no rows has no titles to decompress.
//
// NAMING THE BRANCH IS NOT THE ANSWER, only the place to stand. This reads what those instructions
// READ -- address and value -- because the 0xB2's first scalar was found by exactly that second
// step: 0x9FC73961 was the branch, and the byte it had just loaded was the finding.
//
// THE PC SET IS RECOMPUTED IN THIS RUN rather than pasted from the last one. A hardcoded address
// list is a fixture that rots silently, and this package has been wrong about pasted addresses
// before -- TASK-7.1 was written around four flash addresses that turned out never to execute.
//
// IT ASSERTS ITS OWN SUBJECT: the two screens must differ, the exclusive set must be non-empty,
// and those instructions must actually read something.
//
// IT ONLY READS.
func TestWhatTheGenreScreenLooksUp(t *testing.T) {
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	// SIX-BYTE RECORDS, READ FROM THE BOX THAT PRODUCED THE POINTER. 0x800CBA16 yields a pointer
	// per channel and 0x800CBA1A reads +4 of it, so the record is six bytes; dumping it names the
	// structure instead of leaving "a six-byte record" as a shape. It is captured inside the walk
	// because that is where the machine is, and a pointer read out of a box that has moved on is
	// a number rather than a record.
	const recordAt = 0x800CBA16
	var shapes map[uint32][]byte
	walk := func(t *testing.T, key uint8, name string,
		watch func(a bus.ObservedAccess, pc uint32), each func(pc uint32)) uint32 {
		t.Helper()
		guide := demoGuide(t)
		dict := demoDictionary(t)
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day},
			demoSchedule())
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

		pump := func() error { return transmitter.Pump(box.Machine.Retired) }
		press := azPressFunc(t, box, pump)
		menu := uint32(0)
		for attempt := 1; attempt <= attempts && menu != boxOfficeMenu; attempt++ {
			menu = press(keyBoxOffice, "box office", pressBudget)
		}
		if menu != boxOfficeMenu {
			t.Fatalf("harness: box office drew %08X, not %08X", menu, uint32(boxOfficeMenu))
		}
		tab := uint32(0)
		for attempt := 1; attempt <= attempts && tab != tvGuideMenuScreen; attempt++ {
			tab = press(keyLeft, fmt.Sprintf("left to the tv guide menu (%d)", attempt), pressBudget)
		}
		if tab != tvGuideMenuScreen {
			t.Fatalf("harness: never reached the TV GUIDE menu; drew %08X", tab)
		}
		hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
			pc := box.Machine.Core.State().PC &^ 1
			if shapes != nil && pc == recordAt && !a.Fetch && !a.Write {
				if at := a.Value; at >= 0x80000000 && at < 0x80800000 {
					row := make([]byte, 6)
					for i := range row {
						row[i] = byte(box.RAM.Read((at&0x1fffffff)+uint32(i), bus.Byte)) // #nosec G115
					}
					shapes[at] = row
				}
			}
			if watch != nil {
				watch(a, pc)
			}
		}}
		screen := uint32(0)
		for attempt := 1; attempt <= attempts && (screen == 0 || screen == tab); attempt++ {
			screen = pressAndLetItFinishWatching(t, box, pump, hooks, key, pressBudget,
				func(int) { each(box.Machine.Core.State().PC &^ 1) })
		}
		if screen == 0 || screen == tab {
			t.Fatalf("harness: key %d (%s) never left the TV GUIDE menu (%08X)", key, name, tab)
		}
		t.Logf("%-14s drew %08X", name, screen)
		return screen
	}

	// One ALL CHANNELS run for the baseline PC set: what the ACCEPTING screen executes.
	accepting := map[uint32]bool{}
	allScreen := walk(t, 0x01, "ALL CHANNELS", nil, func(pc uint32) { accepting[pc] = true })

	// Then ENTERTAINMENT, TWICE, and the second run is the whole reason this is three boxes
	// rather than two.
	//
	// THE FIRST VERSION KEPT EVERY READ AND FILTERED AFTERWARDS, because the exclusive set is not
	// known until both screens have run. A draw makes millions of reads, so it carried a cap -- and
	// it hit that cap EXACTLY, four million reads in, which stopped the recording before the
	// reject path ran. The probe then reported that none of the 44 addresses read anything, which
	// is true of the truncated recording and says nothing whatever about the box. A guard that
	// fires on the instrument's own limit and blames the subject is worse than no guard.
	//
	// The emulator is deterministic, so a second ENTERTAINMENT run executes exactly what the first
	// did: computing the exclusive set from run one and recording ONLY those instructions' reads
	// in run two needs no cap at all.
	rejecting := map[uint32]bool{}
	genreScreen := walk(t, 0x02, "ENTERTAINMENT", nil, func(pc uint32) { rejecting[pc] = true })

	if allScreen == genreScreen {
		t.Fatalf("harness: both screens drew %08X, so there is no reject to read", allScreen)
	}
	exclusive := map[uint32]bool{}
	for pc := range rejecting {
		if !accepting[pc] {
			exclusive[pc] = true
		}
	}
	t.Logf("%d addresses run only while the genre screen rejects", len(exclusive))
	if len(exclusive) == 0 {
		t.Fatal("harness: the genre screen ran nothing the accepting screen did not, so the " +
			"differential found no reject path and there is nothing to read")
	}

	record := func(t *testing.T, key uint8, name string, want uint32) []load {
		t.Helper()
		var loads []load
		drew := walk(t, key, name, func(a bus.ObservedAccess, pc uint32) {
			if a.Fetch || a.Write || !exclusive[pc] {
				return
			}
			loads = append(loads, load{pc: pc, at: a.Virtual, value: a.Value, size: a.Size})
		}, func(uint32) {})
		if want != 0 && drew != want {
			t.Fatalf("harness: %s drew %08X where the first run drew %08X, so the two are not the "+
				"same experiment and these reads belong to neither", name, drew, want)
		}
		t.Logf("%-20s %d reads by the reject path", name, len(loads))
		return loads
	}
	shapes = map[uint32][]byte{}
	loads := record(t, 0x02, "ENTERTAINMENT again", genreScreen)

	// AND A SECOND GENRE, which is the whole point of having found the sites. The reject path
	// compares against something loaded from 0x80494904, the same four bytes for every channel --
	// so if that is the GENRE the screen is looking for, MOVIES must read a different value there
	// and every other site must stay put. If nothing moves, the selector is somewhere this probe
	// has not looked and saying so is the result.
	movies := record(t, 0x03, "MOVIES", 0)

	// WHAT THOSE INSTRUCTIONS READ, grouped by the instruction, with the values they saw.
	type site struct {
		count  int
		values map[uint32]int
		addrs  map[uint32]int
	}
	sites := map[uint32]*site{}
	for _, l := range loads {
		if !exclusive[l.pc] {
			continue
		}
		s := sites[l.pc]
		if s == nil {
			s = &site{values: map[uint32]int{}, addrs: map[uint32]int{}}
			sites[l.pc] = s
		}
		s.count++
		if len(s.values) < 16 {
			s.values[l.value]++
		}
		if len(s.addrs) < 16 {
			s.addrs[l.at]++
		}
	}
	if len(sites) == 0 {
		t.Fatalf("none of the %d exclusive addresses read memory at all, across a whole "+
			"uncapped run. The reject is therefore a compare on a register loaded EARLIER, by an "+
			"instruction both screens execute -- a different and larger hunt, and one this probe "+
			"is not equipped for. It is not 'the screen looks up nothing'", len(exclusive))
	}
	pcs := make([]uint32, 0, len(sites))
	for pc := range sites {
		pcs = append(pcs, pc)
	}
	sort.Slice(pcs, func(i, j int) bool { return sites[pcs[i]].count > sites[pcs[j]].count })

	t.Logf("=== WHAT THE REJECT PATH READS ===")
	for i, pc := range pcs {
		if i >= 20 {
			t.Logf("    ... %d quieter instructions", len(pcs)-i)
			break
		}
		s := sites[pc]
		t.Logf("    %08X  %d reads", pc, s.count)
		t.Logf("        values: %s", tally(s.values))
		t.Logf("        from:   %s", tally(s.addrs))
	}
	t.Log("a value that is 0xFFFFFFFF or 0 across every channel is the lookup coming back empty; " +
		"the ADDRESS it read is the field, and that is what the broadcast has to fill")

	// WHICH OF THOSE SITES DEPENDS ON WHICH GENRE WAS OPENED.
	byPC := func(ls []load) map[uint32]map[uint32]bool {
		out := map[uint32]map[uint32]bool{}
		for _, l := range ls {
			if out[l.pc] == nil {
				out[l.pc] = map[uint32]bool{}
			}
			out[l.pc][l.value] = true
		}
		return out
	}
	ent, mov := byPC(loads), byPC(movies)
	var moved, fixed []uint32
	for pc := range ent {
		if mov[pc] == nil {
			continue
		}
		same := len(ent[pc]) == len(mov[pc])
		if same {
			for v := range ent[pc] {
				if !mov[pc][v] {
					same = false
					break
				}
			}
		}
		if same {
			fixed = append(fixed, pc)
			continue
		}
		moved = append(moved, pc)
	}
	sort.Slice(moved, func(i, j int) bool { return moved[i] < moved[j] })
	sort.Slice(fixed, func(i, j int) bool { return fixed[i] < fixed[j] })
	t.Logf("=== WHICH SITES DEPEND ON WHICH GENRE WAS OPENED ===")
	if len(moved) == 0 {
		t.Logf("    NONE. Every site reads the same values for ENTERTAINMENT and MOVIES, so the "+
			"thing that distinguishes one genre from another is NOT read by the %d addresses "+
			"unique to the reject path. It is loaded earlier, by an instruction both screens "+
			"execute, and finding it is a different hunt -- do not read this as 'the screens are "+
			"identical', because they draw different titles.", len(exclusive))
		return
	}
	for _, pc := range moved {
		t.Logf("    %08X  ENTERTAINMENT %s", pc, tally(countOf(loads, pc)))
		t.Logf("              MOVIES        %s", tally(countOf(movies, pc)))
	}
	t.Logf("    %d sites read the same values for both genres", len(fixed))

	// AND WHAT THE PER-CHANNEL RECORD ACTUALLY IS. 0x800CBA16 yields six pointers six bytes apart
	// and 0x800CBA1A reads +4 of each, so there are six six-byte records -- one per channel. Their
	// bytes are dumped against what this broadcast sends, because "a six-byte record" is a shape
	// and the field names are the finding.
	if len(shapes) == 0 {
		t.Log("no per-channel record pointers were seen at 800CBA16, so the shape below is skipped")
		return
	}
	records := make([]uint32, 0, len(shapes))
	for at := range shapes {
		records = append(records, at)
	}
	sort.Slice(records, func(i, j int) bool { return records[i] < records[j] })
	t.Logf("=== THE %d PER-CHANNEL RECORDS THE SEARCH WALKS ===", len(records))
	for _, at := range records {
		t.Logf("    %08X  % 02X", at, shapes[at])
	}
	t.Log("match these against the broadcast: service ids 100..105, channel numbers 101 121 251 " +
		"301 401 501, listings ids 0065 0079 00FB 012D 0191 01F5 -- whichever byte is 01 for every " +
		"channel is the one the genre screen wants to equal its own number")
	t.Log("a site that MOVES with the genre is the selector the screen is searching for, and the " +
		"value it holds is that genre's number -- which is the mapping this task needs")
}

func countOf(ls []load, pc uint32) map[uint32]int {
	out := map[uint32]int{}
	for _, l := range ls {
		if l.pc == pc {
			out[l.value]++
		}
	}
	return out
}

// load is one read made by an instruction on the reject path.
type load struct {
	pc, at, value uint32
	size          bus.Size
}

// tally renders a small value histogram, commonest first, without pulling in a dependency.
func tally(m map[uint32]int) string {
	keys := make([]uint32, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	out := ""
	for i, k := range keys {
		if i >= 6 {
			out += fmt.Sprintf(" (+%d more)", len(keys)-i)
			break
		}
		out += fmt.Sprintf(" %08X×%d", k, m[k])
	}
	return out
}
