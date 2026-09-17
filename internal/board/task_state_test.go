package board_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

func TestTaskStateReadsOnlyTheLiveCreatedList(t *testing.T) {
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	writeTask := func(off, next uint32, name string, status uint8, runs uint32) {
		ram.Write(off+4, bus.Word, memory.DRAMBase+next)
		ram.Write(off+0xc, bus.Word, 0x5441534b)
		for i := range name {
			ram.Write(off+0x10+uint32(i), bus.Byte, uint32(name[i]))
		}
		ram.Write(off+0x18, bus.Byte, uint32(status))
		ram.Write(off+0x1c, bus.Word, runs)
	}
	writeTask(0x1000, 0x2000, "SMTTask", 0, 10)
	writeTask(0x2000, 0x1000, "TASK20", 7, 2583)
	writeTask(0x3000, 0x3000, "STALE", 7, 9000)
	box := &board.Runtime{RAM: ram}
	status, runs, found, err := box.TaskState("TASK20")
	if err != nil || !found || status != 7 || runs != 2583 {
		t.Fatalf("TASK20 state = status %d runs %d found %v err %v", status, runs, found, err)
	}
	_, _, found, err = box.TaskState("STALE")
	if err != nil || found {
		t.Fatalf("unlinked stale task appeared: found %v err %v", found, err)
	}
}
