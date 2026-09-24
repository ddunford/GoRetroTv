package firmwaretests_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
)

// TestTraceLogicalChannelActivation compares the firmware's internal PAT and CAT
// channel records with the standard-SI records which it goes on to install in the
// physical demux. It is an inert debugger probe: it observes CPU and RAM state at
// measured function entries and never changes guest state.
func TestTraceLogicalChannelActivation(t *testing.T) {
	_ = restoredBox(t) // Load and verify the private firmware through the cached path.
	box, err := board.New(cachedImages, true)
	if err != nil {
		t.Fatal(err)
	}
	warmCopy := copyPrivateNVRAM(t, "acquisition-warm.nvram")
	if err := box.I2C.BindImage(warmCopy); err != nil {
		t.Fatal(err)
	}

	type registration struct {
		channel uint32
		pid     uint32
		caller  uint32
		words   [40]uint32
	}
	registrations := map[uint32]registration{}
	configured := map[uint32]int{}
	managerEntries := 0
	registeredAll := false
	hooks := board.StepHooks{AfterPump: func() error {
		state := box.Machine.Core.State()
		pc := state.PC &^ 1
		switch pc {
		case 0x800A7ACC:
			t.Logf("icount=%d SI-register caller=%08X group=%d index=%d table=%04X a3=%08X t0=%08X",
				box.Machine.Retired, state.GPR[31]&^1, state.GPR[4], state.GPR[5],
				state.GPR[6], state.GPR[7], state.GPR[8])
		case 0x800A7DA8:
			managerEntries++
			t.Logf("icount=%d manager-entry caller=%08X a0=%08X a1=%08X a2=%08X a3=%08X sp=%08X",
				box.Machine.Retired, state.GPR[31]&^1, state.GPR[4], state.GPR[5],
				state.GPR[6], state.GPR[7], state.GPR[29])
			for _, address := range []uint32{0x0010700c, 0x00107010, 0x00107030, 0x00107034} {
				t.Logf("manager-global %08X=%08X", address|0x80000000,
					box.RAM.Read(address, bus.Word))
			}
			for i := uint32(0); i < 9; i++ {
				at := uint32(0x001019c0) + i*12
				t.Logf("SI descriptor %d at=%08X words=%08X,%08X,%08X", i, at|0x80000000,
					box.RAM.Read(at, bus.Word), box.RAM.Read(at+4, bus.Word), box.RAM.Read(at+8, bus.Word))
			}
			for at := uint32(0x002d7bc8); at < 0x002d7c70; at += 16 {
				t.Logf("allocation %08X words=%08X,%08X,%08X,%08X", at|0x80000000,
					box.RAM.Read(at, bus.Word), box.RAM.Read(at+4, bus.Word),
					box.RAM.Read(at+8, bus.Word), box.RAM.Read(at+12, bus.Word))
			}
			for at := uint32(0x002d7abc); at < 0x002d7bc8; at += 12 {
				t.Logf("sorted %08X words=%08X,%08X,%08X", at|0x80000000,
					box.RAM.Read(at, bus.Word), box.RAM.Read(at+4, bus.Word), box.RAM.Read(at+8, bus.Word))
			}
		case 0x8001CEF8:
			channel, pid := state.GPR[4], state.GPR[5]
			if channel > 33 {
				t.Fatalf("logical channel %d outside the measured 0..33 table", channel)
			}
			base := uint32(0x0010CA54) + channel*0x264
			var words [40]uint32
			for i := range words {
				words[i] = box.RAM.Read(base+uint32(i)*4, bus.Word)
			}
			registrations[channel] = registration{
				channel: channel, pid: pid, caller: state.GPR[31] &^ 1, words: words,
			}
			t.Logf("icount=%d logical-register channel=%d pid=%04X caller=%08X words[0:40]=%08X",
				box.Machine.Retired, channel, pid, state.GPR[31]&^1, words)
		case 0x8001C908, 0x8001D4D4:
			t.Logf("icount=%d activation-entry pc=%08X caller=%08X a0=%08X a1=%08X",
				box.Machine.Retired, pc, state.GPR[31]&^1, state.GPR[4], state.GPR[5])
		case 0x800A915C:
			count := state.GPR[6]
			if count > 64 {
				t.Fatalf("activation manager count %d is implausible", count)
			}
			indices := make([]uint32, count)
			for i := range indices {
				indices[i] = box.RAM.Read((state.GPR[5]&0x1fffffff)+uint32(i)*4, bus.Word)
			}
			t.Logf("icount=%d activation-manager caller=%08X group=%d list=%08X count=%d indices=%v",
				box.Machine.Retired, state.GPR[31]&^1, state.GPR[4], state.GPR[5], count, indices)
		case 0x800035A4:
			arg := state.GPR[4] & 0x1fffffff
			var words [16]uint32
			for i := range words {
				words[i] = box.RAM.Read(arg+uint32(i)*4, bus.Word)
			}
			configured[state.GPR[4]]++
			t.Logf("icount=%d hardware-config caller=%08X a0=%08X words=%08X",
				box.Machine.Retired, state.GPR[31]&^1, state.GPR[4], words)
		}
		return nil
	}, Access: func(access bus.ObservedAccess) {
		physical := access.Virtual & 0x1fffffff
		if access.Write && access.Virtual >= 0xb000a0d0 && access.Virtual <= 0xb000a0dc {
			t.Logf("icount=%d interrupt-enable pc=%08X offset=%03X value=%08X",
				box.Machine.Retired, box.Machine.Core.PC&^1,
				access.Virtual-0xb000a000, access.Value)
		}
		if access.Write && registeredAll {
			for _, channel := range []uint32{33, 31, 30, 29, 28, 27} {
				base := uint32(0x0010CA54) + channel*0x264
				if physical >= base && physical < base+0xa0 {
					t.Logf("icount=%d logical-write pc=%08X channel=%d offset=%03X size=%d value=%08X",
						box.Machine.Retired, box.Machine.Core.PC&^1, channel,
						physical-base, access.Size, access.Value)
				}
			}
		}
		if !access.Write || access.Virtual < 0xb000a014 || access.Virtual >= 0xb000a094 {
			return
		}
		t.Logf("icount=%d physical-pid-write pc=%08X channel=%d value=%08X",
			box.Machine.Retired, box.Machine.Core.PC&^1,
			(access.Virtual-0xb000a014)/4, access.Value)
	}}

	for range 90_000_000 {
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
		// PID 0, 1, 0x10, 0x11, 0x12 and 0x14 are the complete fixed-SI set
		// observed at application initialization. Stop on that event rather than
		// spending the remainder of an arbitrary instruction window.
		if len(registrations) >= 6 {
			registeredAll = true
		}
		if registeredAll && hasPIDs(box.Demux.ArmedPIDs(), 0x10, 0x11, 0x14) {
			break
		}
	}
	if len(registrations) < 6 {
		t.Fatalf("harness: observed only %d fixed-SI logical registrations", len(registrations))
	}
	if managerEntries == 0 {
		t.Fatal("harness: SI manager entry was never observed")
	}
	for _, channel := range []uint32{33, 31, 30, 29, 28, 27} {
		r, ok := registrations[channel]
		if !ok {
			t.Errorf("logical channel %d was not registered", channel)
			continue
		}
		t.Logf("summary channel=%d pid=%04X caller=%08X record=%08X", r.channel, r.pid, r.caller, r.words)
		base := uint32(0x0010CA54) + channel*0x264
		var after [40]uint32
		for i := range after {
			after[i] = box.RAM.Read(base+uint32(i)*4, bus.Word)
		}
		t.Logf("after channel=%d record=%08X", channel, after)
	}
	t.Logf("hardware configuration entries=%d distinct-args=%d", len(configured), len(configured))
}

func hasPIDs(got []uint16, want ...uint16) bool {
	for _, pid := range want {
		found := false
		for _, candidate := range got {
			if candidate == pid {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
