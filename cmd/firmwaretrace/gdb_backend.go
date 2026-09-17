package main

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/cpu"
	"github.com/ddunford/goretrotv/internal/gdbstub"
)

// firmwareDebugBackend runs on the GDB session goroutine only. The regular
// instruction loop is inactive while a debugger owns this backend.
type firmwareDebugBackend struct {
	core    *cpu.Core
	bus     *bus.Bus
	retired *uint64
	advance func(uint64, func(bus.ObservedAccess)) error
}

func (d *firmwareDebugBackend) Registers() gdbstub.Registers {
	return gdbstub.Registers{
		GPR: d.core.GPR, Status: d.core.COP0[12], LO: d.core.LO, HI: d.core.HI,
		BadVAddr: d.core.COP0[8], Cause: d.core.COP0[13], PC: d.core.PC, ISA: d.core.ISA,
	}
}

func (d *firmwareDebugBackend) SetRegisters(regs gdbstub.Registers) error {
	if regs.PC != d.core.PC || regs.ISA != d.core.ISA {
		if err := d.core.SetDebugPC(regs.PC, regs.ISA); err != nil {
			return err
		}
	}
	d.core.GPR = regs.GPR
	d.core.GPR[0] = 0
	d.core.COP0[12], d.core.COP0[8], d.core.COP0[13] = regs.Status, regs.BadVAddr, regs.Cause
	d.core.LO, d.core.HI = regs.LO, regs.HI
	return nil
}

func (d *firmwareDebugBackend) ReadMemory(addr, length uint32) ([]byte, error) {
	if length > 4096 || uint64(addr)+uint64(length) > 1<<32 {
		return nil, fmt.Errorf("gdb: memory read range is too large")
	}
	result := make([]byte, length)
	for i := range result {
		address := addr + uint32(i) // #nosec G115 -- range checked above.
		if _, ok := bus.Physical(address); !ok {
			return nil, fmt.Errorf("gdb: unmapped virtual address %#x", address)
		}
		result[i] = byte(d.bus.Read(address, bus.Byte)) // #nosec G115 -- byte-sized bus read.
	}
	return result, nil
}

func (d *firmwareDebugBackend) WriteMemory(addr uint32, data []byte) error {
	if len(data) > 4096 || uint64(addr)+uint64(len(data)) > 1<<32 {
		return fmt.Errorf("gdb: memory write range is too large")
	}
	for i, value := range data {
		address := addr + uint32(i) // #nosec G115 -- range checked above.
		if _, ok := bus.Physical(address); !ok {
			return fmt.Errorf("gdb: unmapped virtual address %#x", address)
		}
		d.bus.Write(address, bus.Byte, uint32(value))
	}
	return nil
}

func (d *firmwareDebugBackend) Step() ([]gdbstub.MemoryAccess, error) {
	pc := d.core.PC
	fetchSize := bus.Word
	if d.core.ISA {
		fetchSize = bus.Half
	}
	var accesses []gdbstub.MemoryAccess
	fetchSeen := false
	observe := func(access bus.ObservedAccess) {
		if !fetchSeen && !access.Write && access.Virtual == pc && access.Size == fetchSize {
			fetchSeen = true
			return
		}
		accesses = append(accesses, gdbstub.MemoryAccess{
			Address: access.Virtual, Size: uint32(access.Size), Write: access.Write,
		})
	}
	err := d.advance(*d.retired, observe)
	return accesses, err
}
