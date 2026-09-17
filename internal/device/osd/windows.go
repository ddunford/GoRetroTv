package osd

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

const (
	windowArrayPointer = 0x80105e9c
	windowGateAddress  = 0x80106f20
	windowCountAddress = 0x80106f24
	windowRecordSize   = 100
)

// Window is the firmware's 100-byte plane record, read from its own DRAM.
// The firmware owns and updates these bytes; this view never shadows them.
type Window struct {
	ID              uint32
	Kind            uint32
	Depth           uint32
	Produce         uint32
	Consume         uint32
	BackgroundSet   bool
	BackgroundColor uint32
	RootObject      uint32
}

// WindowTable reads the firmware's live records directly from board DRAM.
type WindowTable struct{ ram *memory.RAM }

// NewWindowTable binds a read-only view to board DRAM.
func NewWindowTable(ram *memory.RAM) *WindowTable { return &WindowTable{ram: ram} }

func (t *WindowTable) word(addr uint32) (uint32, error) {
	if t.ram == nil {
		return 0, fmt.Errorf("osd: board DRAM is not bound")
	}
	off, ok := bus.Physical(addr)
	if !ok {
		return 0, fmt.Errorf("osd: address %#08x is not board DRAM", addr)
	}
	if uint64(off)+4 > uint64(t.ram.Size()) {
		return 0, fmt.Errorf("osd: address %#08x exceeds board DRAM", addr)
	}
	return t.ram.Read(off, bus.Word), nil
}

// Count reports the number of firmware window records after checking their extent.
func (t *WindowTable) Count() (uint32, error) {
	base, err := t.word(windowArrayPointer)
	if err != nil {
		return 0, err
	}
	count, err := t.word(windowCountAddress)
	if err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, nil
	}
	off, ok := bus.Physical(base)
	if !ok || uint64(off)+uint64(count)*windowRecordSize > uint64(t.ram.Size()) {
		return 0, fmt.Errorf("osd: window table pointer %#08x or count %d is outside board DRAM", base, count)
	}
	return count, nil
}

// Read returns a live window record. Window zero is deliberately refused when
// the firmware's gate is zero, matching the validator that otherwise wedges.
func (t *WindowTable) Read(id uint32) (Window, error) {
	count, err := t.Count()
	if err != nil {
		return Window{}, err
	}
	if id >= count {
		return Window{}, fmt.Errorf("osd: window %d outside %d records", id, count)
	}
	gate, err := t.word(windowGateAddress)
	if err != nil {
		return Window{}, err
	}
	if id == 0 && gate == 0 {
		return Window{}, fmt.Errorf("osd: firmware refuses window 0 while gate is zero")
	}
	base, err := t.word(windowArrayPointer)
	if err != nil {
		return Window{}, err
	}
	at := base + id*windowRecordSize
	read := func(offset uint32) (uint32, error) { return t.word(at + offset) }
	var result Window
	result.ID = id
	if result.Kind, err = read(0x00); err != nil {
		return Window{}, err
	}
	if result.Depth, err = read(0x40); err != nil {
		return Window{}, err
	}
	if result.Produce, err = read(0x50); err != nil {
		return Window{}, err
	}
	if result.Consume, err = read(0x54); err != nil {
		return Window{}, err
	}
	flag, err := read(0x58)
	if err != nil {
		return Window{}, err
	}
	result.BackgroundSet = flag != 0
	if result.BackgroundColor, err = read(0x5c); err != nil {
		return Window{}, err
	}
	if result.RootObject, err = read(0x60); err != nil {
		return Window{}, err
	}
	return result, nil
}
