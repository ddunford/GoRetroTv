// Package bus is the address decoder every part of the machine hangs off, and the one interface
// every device implements.
//
// # Why Snapshot is in the interface from the first line
//
// Spike 003 (.research/003-snapshot-completeness) took the inventory of what a snapshot of this
// machine must carry and found that a partially-correct snapshot does not fail. It produces a
// plausible machine: restore without the demux ring pointers and sections land at the wrong
// offset, without the I2C transaction state the next EEPROM read returns the wrong byte, without
// the ISA mode bit the first instruction decodes in the wrong mode. Each of those reads as a
// firmware fault, in a program whose faults are supposed to be interesting. Retrofitting
// serialisation across eight device models afterwards is the expensive version, so Snapshot and
// Restore are not optional methods a device may grow later - they sit beside Read and Write.
//
// # Why there is no context.Context and no error on Read and Write
//
// This is a hardware model, not a service. The instruction loop is single-threaded and
// deterministic by construction (CLAUDE.md -> Architecture Decisions), there is nothing to cancel
// between two cycles of a bus, and spike 001 measured peripheral dispatch on loads as one of the
// four costs standing between the port and its 15M instructions/s floor. A bus access is closer to
// a memory read than to a request.
//
// A device that cannot answer therefore does not return an error: it does what the hardware does,
// which for this machine's unmapped space is answer zero. See Bus.Read for why that behaviour is
// copied rather than improved on.
package bus

// Size is the width of a bus access, in bytes.
//
// The width is part of the access rather than three separate methods because this machine's
// registers are width-sensitive in ways the measured record states in exactly these terms - DMA
// completion is "read as a halfword at +0x122", and an MMIO word is one word rather than four
// bytes (docs/reference/digibox-emulation.md). A device that only makes sense at one width says so
// itself; the bus does not decide for it.
type Size uint8

// The three widths this machine's loads and stores use. There is no doubleword: the VR4111 is a
// 64-bit part but this firmware is compiled 32-bit and never issues one.
const (
	Byte Size = 1
	Half Size = 2
	Word Size = 4
)

// String renders a width the way the record writes it.
func (s Size) String() string {
	switch s {
	case Byte:
		return "byte"
	case Half:
		return "half"
	case Word:
		return "word"
	default:
		return "invalid"
	}
}

// Valid reports whether s is a width the bus can carry.
func (s Size) Valid() bool {
	return s == Byte || s == Half || s == Word
}

// Device is memory or a peripheral attached to the bus.
//
// Offsets handed to Read and Write are relative to the device's own base, not absolute: every
// peripheral in docs/reference/digibox-emulation.md is written down as a base plus an offset
// ("0xB0009000 ... +0x120 status"), and a device that knew its own absolute address would have to
// be told again whenever a region moved.
//
// Multi-byte values are big-endian, because the machine is.
type Device interface {
	// Name identifies the device in diagnostics and is its key in a machine snapshot. It is
	// therefore part of the snapshot format: renaming a device invalidates existing snapshots,
	// and Bus.Restore says so by name rather than restoring a device to nothing.
	Name() string

	// Read returns the value at off, which is a byte offset within this device's region.
	//
	// It cannot fail. A device with nothing behind an offset answers what the hardware answers -
	// for most of this machine that is zero, and the measured record is explicit that changing
	// what a read RETURNS is the dangerous direction (making the demux register file read back
	// its own writes stopped the RTOS starting at all).
	Read(off uint32, size Size) uint32

	// Write stores value at off. Like Read it cannot fail; a write to a read-only or
	// unimplemented offset is dropped, and the device records it if the record is worth having.
	Write(off uint32, size Size, value uint32)

	// Reset returns the device to power-on state. Whatever Reset clears is, by definition, the
	// state Snapshot must carry - that equivalence is how spike 003 took its inventory, and it
	// is the one available cross-check on a device's own idea of what its state is.
	Reset()

	// Snapshot encodes the device's complete state.
	//
	// "Complete" means every field Reset clears, including the ones no bus read can observe: a
	// flash chip's position in an unlock sequence, an I2C transaction half-finished, a link's
	// partially-shifted byte. Those are the fields that get forgotten, because forgetting them
	// costs nothing until a restored machine quietly behaves differently.
	//
	// bustest.CheckSnapshot is the standing check that this method has not fallen behind the
	// struct it serialises.
	Snapshot() ([]byte, error)

	// Restore replaces the device's state with a snapshot, in full.
	//
	// Restoring into a device that has been running must leave no trace of what it was doing: a
	// STATE field the snapshot does not mention is one Restore must clear, not one it may leave
	// alone. Identity and configuration are the exception and are preserved - a device's name,
	// and a read-only part's contents, are established when it is built and a snapshot that
	// could change them would be a snapshot that swapped one device for another. Which fields
	// are which is declared to bustest.Check as Constant, so the distinction is written down
	// rather than assumed.
	//
	// Restore reports an error rather than restoring part of itself.
	Restore(state []byte) error
}
