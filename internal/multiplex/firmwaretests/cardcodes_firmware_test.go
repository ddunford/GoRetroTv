package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/csi"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHICH CARD COMMANDS DOES THE BOX SEND WHILE THE EVENT PIPE IS FILLING?
//
// The wedge is caused by this port's card staying silent (cardsilence_firmware_test.go): answering
// two codes it fills after eight presses, answering everything it never fills. The fix is to answer
// what a real card answers -- and before any of that can be decided, it is worth knowing which
// codes are even in play. A code the box never sends during the presses cannot be starving anyone,
// however interesting it looked in the boot capture.
//
// THE BYTES COME OFF THE BUS, not out of the link's own transmit log. The log was the obvious
// source and it is bounded at four kilobytes, which the acquisition fills on its own -- so it held
// exactly as many bytes after twenty presses as before the first one, and a reader who had not
// checked that boundary would have concluded the box sends NOTHING while it wedges. Watching the
// writes to the data register has no ceiling and no boundary to get wrong.
//
// The frames are de-framed the way the record describes them:
//
//	[LEN] [SEQ] [CODE] [args...]   LEN counts the bytes after itself, 0x00 ends a frame,
//	                               0x1B escapes the byte that follows
//
// THE LENGTH FIELD IS THE ORACLE for the escape rule, and this checks it rather than trusting it:
// a de-framed payload whose LEN does not match what follows means the de-framing is wrong, and
// every count below would be of the wrong bytes.
func TestWhichCardCommandsTheBoxSendsWhileTheEventPipeFills(t *testing.T) {
	const watched = "EVQP0002"
	const presses = 12 // the pinned pre-fix policy fills after eight; this is the margin, not a sweep
	const settle = 2_000_000

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

	// THE PRE-FIX POLICY, PINNED, because this measures what the box asks for WHILE IT FAILS and
	// the failure has since been fixed by answering two of those very codes. Left on the build's
	// default this would never fill the pipe and would have nothing to count -- so the arm that
	// reproduces the defect has to keep the card as silent as it was when the defect existed.
	// Narrowing it back here is also the check that csi.DefaultAckPolicy is what fixed it rather
	// than something else that changed along the way.
	box.CSI.SetAckPolicy([]uint8{0x52, 0x18})

	pipe := pipeNamed(t, box, watched)
	free := func() uint32 {
		return box.RAM.Read((pipe&0x1fffffff)+uint32(pipeAvailable)*4, bus.Word) // #nosec G115
	}

	// Collection starts at the first key, so there is no acquisition traffic to subtract.
	var wire []byte
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Write && a.Virtual&^uint32(csi.Size-1) == csi.Base && a.Virtual&0xff == 0x10 {
			wire = append(wire, byte(a.Value&0xff))
		}
	}}
	filled := 0
	for press := 1; press <= presses && filled == 0; press++ {
		if err := box.CSI.Key(0x59, 0); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < settle; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
		}
		if free() == 0 {
			filled = press
		}
	}
	if filled == 0 {
		t.Fatalf("harness: %s did not fill in %d presses, so this did not observe the run-up to "+
			"the wedge and the codes below are from an ordinary session", watched, presses)
	}
	t.Logf("%s filled after %d presses", watched, filled)

	if len(wire) == 0 {
		t.Fatal("harness: the guest wrote nothing to the link's data register during the presses, " +
			"so either the watch is on the wrong address or nothing was sent")
	}
	during, badLength := cardFrames(t, wire)
	if badLength > 0 {
		t.Fatalf("harness: %d de-framed payloads did not match their own LENGTH field, so the "+
			"escape rule is being applied wrongly and these are not the bytes the box sent",
			badLength)
	}
	if len(during) == 0 {
		t.Fatalf("harness: %d bytes were transmitted during the presses and not one whole frame "+
			"came out of them", len(wire))
	}

	counts := map[uint8]int{}
	for _, f := range during {
		counts[f]++
	}
	codes := make([]int, 0, len(counts))
	for code := range counts {
		codes = append(codes, int(code))
	}
	sort.Ints(codes)
	t.Logf("%d frames in %d bytes during the presses:", len(during), len(wire))
	for _, code := range codes {
		answered := "SILENT under the pinned pre-fix policy -- and now in DefaultAckPolicy"
		switch code {
		case 0x52, 0x18:
			answered = "answered, then and now"
		case 0x41, 0x42:
		default:
			answered = "SILENT, and still silent -- the box does not ask for it while it fails"
		}
		t.Logf("    code %02X  x%-4d  %s", code, counts[uint8(code)], answered) // #nosec G115
	}
}

// pipeNamed finds a kernel object by name, or fails saying what it did find.
func pipeNamed(t *testing.T, box *board.Runtime, name string) uint32 {
	t.Helper()
	objects, err := box.NucleusObjects()
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range objects {
		if o.Name == name {
			return o.Address
		}
	}
	t.Fatalf("harness: no %s among %d kernel objects, so there is nothing to watch", name,
		len(objects))
	return 0
}

// cardFrames de-frames the guest's transmit log and returns each frame's command code, plus how
// many frames failed their own length check.
//
// The check is the point. The escape rule and the terminator interact -- a payload byte of 0x00 or
// 0x1B goes out as 1B <byte> -- so a de-framer that gets it wrong still produces frames, just the
// wrong ones. LEN counts the bytes after itself, so it is an independent statement of where the
// frame ends and it either agrees or the de-framing is wrong.
func cardFrames(t *testing.T, wire []byte) (codes []uint8, badLength int) {
	t.Helper()
	var frame []byte
	escaped := false
	for _, b := range wire {
		switch {
		case escaped:
			escaped = false
			frame = append(frame, b)
		case b == 0x1b:
			escaped = true
		case b != 0:
			frame = append(frame, b)
		default:
			if len(frame) >= 3 {
				if int(frame[0]) != len(frame)-1 {
					badLength++
				} else {
					codes = append(codes, frame[2])
				}
			}
			frame = nil
		}
	}
	return codes, badLength
}
