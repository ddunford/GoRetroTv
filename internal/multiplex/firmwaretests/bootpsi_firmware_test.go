package firmwaretests_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/dvb"
)

// The boot ROM creates the only measured PID-0/PID-1 window. Put real TS packets on the wire while
// those guest-programmed channels exist, then follow execution into the application. This tests the
// transport boundary; it does not call the section-delivery helper or modify guest memory.
func TestWhetherBootPSIReachesTheApplicationManagers(t *testing.T) {
	_ = restoredBox(t)
	box, err := board.New(cachedImages, true)
	if err != nil {
		t.Fatal(err)
	}
	pat, err := broadcast.PAT(0x20, 0, []broadcast.Programme{{Number: 0x64, MapPID: 0x100}})
	if err != nil {
		t.Fatal(err)
	}
	cat, err := broadcast.CAT(0, nil)
	if err != nil {
		t.Fatal(err)
	}

	const (
		onWireAt   = 3_211_000
		onWireStop = 50_000_000
		period     = 1_000_000
	)
	hits := map[uint32]int{}
	type specialCall struct{ pc, a0, a1 uint32 }
	var specialCalls []specialCall
	var specialWrites []specialCall
	hooks := board.StepHooks{Access: func(access bus.ObservedAccess) {
		if access.Write && (access.Virtual == 0xB000A094 || access.Virtual == 0xB000A098) {
			specialWrites = append(specialWrites, specialCall{box.Machine.Core.PC, access.Virtual, access.Value})
		}
		if !access.Fetch {
			return
		}
		switch pc := access.Virtual &^ 1; pc {
		case 0x800AFE50, 0x800AFF1C, 0x800A9C5C, 0x800A9F70, 0x80004FA6:
			hits[pc]++
		case 0x8001CEF8:
			g := box.Machine.Core.State().GPR
			specialCalls = append(specialCalls, specialCall{pc, g[4], g[5]})
		}
	}}
	transmissions := 0
	for i := 0; i < 120_000_000; i++ {
		if i >= onWireAt && i < onWireStop && (i-onWireAt)%period == 0 {
			if err := box.Demux.PushTransport(dvb.PacketizeSection(0, pat, 0)); err != nil {
				t.Fatalf("PAT transport at ROM window: %v", err)
			}
			if err := box.Demux.PushTransport(dvb.PacketizeSection(1, cat, 0)); err != nil {
				t.Fatalf("CAT transport at ROM window: %v", err)
			}
			transmissions++
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if hits[0x80004FA6] == 0 {
		t.Fatal("harness: application demux initialisation control did not execute")
	}
	t.Logf("transmissions=%d application demux init=%d PAT parser=%d CAT parser=%d PAT completion=%d CAT completion=%d",
		transmissions, hits[0x80004FA6], hits[0x800AFE50], hits[0x800AFF1C], hits[0x800A9C5C], hits[0x800A9F70])
	for _, call := range specialCalls {
		t.Logf("special PID configure PC=%08X channel=%d PID=%04X", call.pc, call.a0, call.a1)
	}
	for _, write := range specialWrites {
		t.Logf("special register PC=%08X address=%08X value=%08X", write.pc, write.a0, write.a1)
	}
}
