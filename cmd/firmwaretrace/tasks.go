package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

// reportTasks walks the guest's created-task list from SMTTask. A raw RAM scan
// misses valid tasks whose names are no longer printable, and silently turns a
// complete boot into a plausible lower count.
func reportTasks(out io.Writer, ram *memory.RAM) error {
	offsets, err := createdTaskOffsets(ram)
	if err != nil {
		return err
	}
	if err := instrument.MustFind("Nucleus task census", "SMTTask seed", len(offsets), 1); err != nil {
		return err
	}
	for count, off := range offsets {
		name := taskName(ram, off)
		status := ram.Read(off+0x18, bus.Byte)
		runs := ram.Read(off+0x1c, bus.Word)
		sp := ram.Read(off+0x2c, bus.Word)
		var codeWords []string
		if sp >= memory.DRAMBase && sp-memory.DRAMBase+256 <= ram.Size() {
			for pos := uint32(0); pos < 256 && len(codeWords) < 12; pos += 4 {
				word := ram.Read(sp-memory.DRAMBase+pos, bus.Word)
				if word >= 0x80000000 && word < 0x80120000 || word >= 0x9fc00000 && word < 0x9fc20000 || word >= 0xbfc00000 && word < 0xbfc20000 {
					codeWords = append(codeWords, fmt.Sprintf("+%02X:%08X", pos, word))
				}
			}
		}
		cleanup := ram.Read(off+0x68, bus.Word)
		suspend := ram.Read(off+0x6c, bus.Word)
		if _, err := fmt.Fprintf(out, "task %d TCB=%08X name=%q status=%d runs=%d SP=%08X cleanup=%08X suspend=%08X stack-code-words=[%s]\n", count, memory.DRAMBase+off, name, status, runs, sp, cleanup, suspend, strings.Join(codeWords, " ")); err != nil {
			return err
		}
	}
	for off := uint32(0); off+0x30 <= ram.Size(); off += 4 {
		magic := ram.Read(off+0x0c, bus.Word)
		if magic != 0x53454d41 && magic != 0x45564e54 { // SEMA, EVNT
			continue
		}
		var name [8]byte
		valid := true
		for i := range name {
			name[i] = byte(ram.Read(off+0x10+uint32(i), bus.Byte)) // #nosec G115 -- bus.Byte fits in uint8
			if name[i] != 0 && (name[i] < 0x20 || name[i] > 0x7e) {
				valid = false
			}
		}
		if !valid {
			continue
		}
		waiters := ram.Read(off+0x20, bus.Word)
		if magic == 0x53454d41 && waiters == 0 {
			continue
		}
		if _, err := fmt.Fprintf(out, "wait-object %08X %q name=%q count=%d waiters=%d head=%08X\n", memory.DRAMBase+off, string(magicBytes(magic)), strings.TrimRight(string(name[:]), "\x00"), ram.Read(off+0x18, bus.Word), waiters, ram.Read(off+0x24, bus.Word)); err != nil {
			return err
		}
	}
	if err := instrument.MustFind("Nucleus task census", "TASK control blocks", len(offsets), 1); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "tasks found: %d\n", len(offsets))
	return err
}

func createdTaskOffsets(ram *memory.RAM) ([]uint32, error) {
	var seed uint32
	for off := uint32(0); off+0x38 <= ram.Size(); off += 4 {
		if ram.Read(off+0x0c, bus.Word) != 0x5441534b {
			continue
		}
		if string(taskName(ram, off)) == "SMTTask" {
			seed = memory.DRAMBase + off
			break
		}
	}
	if seed == 0 {
		return nil, nil
	}
	seen := make(map[uint32]bool)
	var offsets []uint32
	for at := seed; ; {
		if at < memory.DRAMBase || at-memory.DRAMBase+0x70 > ram.Size() || at&3 != 0 {
			return nil, fmt.Errorf("task census: created list left DRAM at %08X", at)
		}
		if seen[at] {
			return nil, fmt.Errorf("task census: created list repeated %08X before returning to seed", at)
		}
		if len(offsets) >= 200 {
			return nil, fmt.Errorf("task census: created list exceeds 200 tasks")
		}
		seen[at] = true
		off := at - memory.DRAMBase
		if ram.Read(off+0x0c, bus.Word) != 0x5441534b {
			return nil, fmt.Errorf("task census: created list points to non-task at %08X", at)
		}
		offsets = append(offsets, off)
		next := ram.Read(off+4, bus.Word)
		if next == seed {
			break
		}
		at = next
	}
	return offsets, nil
}

func taskName(ram *memory.RAM, off uint32) []byte {
	var name [8]byte
	end := 0
	for end < len(name) {
		name[end] = byte(ram.Read(off+0x10+uint32(end), bus.Byte)) // #nosec G115 -- bus.Byte fits in uint8
		if name[end] == 0 {
			break
		}
		end++
	}
	return name[:end]
}

func magicBytes(v uint32) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
