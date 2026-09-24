package firmwaretests_test

import (
	"fmt"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
)

// TestTraceNDSSectionFilterLifecycle observes the conditional-access driver's own
// filter records and hardware configurations. It is inert: the probe changes no
// guest register, memory byte, or device response.
func TestTraceNDSSectionFilterLifecycle(t *testing.T) {
	_ = restoredBox(t)
	box, err := board.New(cachedImages, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := box.I2C.BindImage(copyPrivateNVRAM(t, "acquisition-warm.nvram")); err != nil {
		t.Fatal(err)
	}

	type event struct {
		pc, ra uint32
		args   [4]uint32
		words  []uint32
	}
	var events []event
	var messages []string
	var ndsGateWrites []string
	var frontEndCalls []string
	hooks := board.StepHooks{AfterPump: func() error {
		state := box.Machine.Core.State()
		pc := state.PC &^ 1
		switch pc {
		case 0x80032D0C:
			frontEndCalls = append(frontEndCalls, fmt.Sprintf("icount=%d ra=%08X a0=%08X a1=%08X gate=%02X",
				box.Machine.Retired, state.GPR[31], state.GPR[4], state.GPR[5],
				box.RAM.Read(0x00106220, bus.Byte)))
		case 0x800F515C, 0x800F5220, 0x800F5390:
			e := event{pc: pc, ra: state.GPR[31], args: [4]uint32{state.GPR[4], state.GPR[5], state.GPR[6], state.GPR[7]}}
			at := state.GPR[4] & 0x1fffffff
			for off := uint32(0); off < 0x30; off += 4 {
				e.words = append(e.words, box.RAM.Read(at+off, bus.Word))
			}
			events = append(events, e)
		case 0x80002E04:
			if state.GPR[31]&0xFFF00000 == 0x80000000 && state.GPR[31] >= 0x800F0000 {
				e := event{pc: pc, ra: state.GPR[31], args: [4]uint32{state.GPR[4], state.GPR[5], state.GPR[6], state.GPR[7]}}
				at := state.GPR[4] & 0x1fffffff
				for off := uint32(0); off < 0x28; off += 4 {
					e.words = append(e.words, box.RAM.Read(at+off, bus.Word))
				}
				events = append(events, e)
			}
		case 0x800245E4, 0x80024654:
			queue := box.RAM.Read(0x00107370, bus.Word)
			if state.GPR[4] == queue {
				at := state.GPR[5] & 0x1fffffff
				messages = append(messages, fmt.Sprintf("pc=%08X ra=%08X %08X %08X %08X %08X",
					pc, state.GPR[31], box.RAM.Read(at, bus.Word), box.RAM.Read(at+4, bus.Word),
					box.RAM.Read(at+8, bus.Word), box.RAM.Read(at+12, bus.Word)))
			}
		}
		return nil
	}, Access: func(access bus.ObservedAccess) {
		if access.Write && !access.Fetch && access.Virtual&0x1fffffff == 0x00106220 {
			ndsGateWrites = append(ndsGateWrites, fmt.Sprintf("icount=%d pc=%08X size=%d value=%08X",
				box.Machine.Retired, box.Machine.Core.PC&^1, access.Size, access.Value))
		}
	}}

	for range 90_000_000 {
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if len(events) == 0 {
		t.Fatal("harness: observed no NDS filter lifecycle calls")
	}
	for _, e := range events {
		t.Logf("pc=%08X ra=%08X args=%08X,%08X,%08X,%08X words=%08X",
			e.pc, e.ra, e.args[0], e.args[1], e.args[2], e.args[3], e.words)
	}
	for _, message := range messages {
		t.Logf("NDS queue %s", message)
	}
	for _, write := range ndsGateWrites {
		t.Logf("NDS gate write %s", write)
	}
	for _, call := range frontEndCalls {
		t.Logf("front-end command %s", call)
	}
	t.Logf("armed PIDs=%v", box.Demux.ArmedPIDs())
}
