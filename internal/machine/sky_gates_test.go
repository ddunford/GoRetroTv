package machine

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

func skyGateFixture(t *testing.T) (*bus.Bus, *memory.Flash) {
	t.Helper()
	board := bus.New()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	if err := board.Attach(memory.DRAMBase, memory.DRAMSize, ram); err != nil {
		t.Fatal(err)
	}
	image := make([]byte, skyGateFlashOffset+1)
	image[skyGateFlashOffset] = 0x75
	flash, err := memory.NewFlash("U202", image)
	if err != nil {
		t.Fatal(err)
	}
	board.Write(skyGateRAMAddress, bus.Word, 0xffffffff)
	return board, flash
}

func TestSkyGatesWaitForHandoffAndFortyTwoGuestTasks(t *testing.T) {
	board, flash := skyGateFixture(t)
	g := NewSkyGates(true)
	count := 41
	tasks := func() (int, error) { return count, nil }
	for _, state := range []struct {
		at      uint64
		handoff bool
	}{
		{0, false}, {0, true}, {skyGateCheckInterval - 1, true},
	} {
		applied, err := g.Tick(state.at, state.handoff, tasks, board, flash)
		if err != nil || applied {
			t.Fatalf("at %d: applied=%v err=%v", state.at, applied, err)
		}
	}
	count = 42
	applied, err := g.Tick(skyGateCheckInterval, true, tasks, board, flash)
	if err != nil || !applied || !g.Done() {
		t.Fatalf("42-task gate applied=%v done=%v err=%v", applied, g.Done(), err)
	}
	if board.Read(skyGateRAMAddress, bus.Word) != 0 || flash.Bytes()[skyGateFlashOffset] != 0x76 {
		t.Fatal("declared RAM and flash answers were not applied")
	}
	applied, err = g.Tick(skyGateCheckInterval+1, true, tasks, board, flash)
	if err != nil || applied {
		t.Fatalf("one-shot gate applied again: %v, %v", applied, err)
	}
}

func TestSkyGatesRejectUnexpectedFlashWithoutMutatingRAM(t *testing.T) {
	board, flash := skyGateFixture(t)
	flash.Bytes()[skyGateFlashOffset] = 0x74
	g := NewSkyGates(true)
	applied, err := g.Tick(0, true, func() (int, error) { return 42, nil }, board, flash)
	if applied || err == nil || g.Done() || board.Read(skyGateRAMAddress, bus.Word) != 0xffffffff {
		t.Fatalf("unexpected flash: applied=%v done=%v RAM=%08X err=%v", applied, g.Done(), board.Read(skyGateRAMAddress, bus.Word), err)
	}
}

func TestSkyGatesRestoreKeepsDeadlineAndOneShotState(t *testing.T) {
	board, flash := skyGateFixture(t)
	g := NewSkyGates(true)
	if _, err := g.Tick(0, true, func() (int, error) { return 41, nil }, board, flash); err != nil {
		t.Fatal(err)
	}
	blob, err := g.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored := NewSkyGates(false)
	if err := restored.Restore(blob); err != nil {
		t.Fatal(err)
	}
	count42 := func() (int, error) { return 42, nil }
	if applied, err := restored.Tick(skyGateCheckInterval-1, true, count42, board, flash); err != nil || applied {
		t.Fatalf("restored policy fired early: %v, %v", applied, err)
	}
	if applied, err := restored.Tick(skyGateCheckInterval, true, count42, board, flash); err != nil || !applied {
		t.Fatalf("restored policy missed deadline: %v, %v", applied, err)
	}
}
