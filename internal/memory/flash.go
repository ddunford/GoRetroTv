package memory

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

// Flash is one of the board's two 2 MB parts, mapped read-only.
//
// Read-only is this phase's shape, not the chip's. The real part speaks the AMD command set and
// the application does program it - its CA task verifies its settings in 256-byte chunks with a
// 16-bit sum and writes the sum back when it disagrees, and when the predecessor's flash swallowed
// that program command the task never agreed, never blocked, and starved everything below it for
// ever. Modelling the command sequencer is TASK-2.8, which REPLACES this device rather than
// sitting beside it: when it lands, the contents stop being configuration and become state, and
// this file goes.
//
// Until then a write is refused and counted. Counted rather than dropped silently, because the
// failure above is what a silently-swallowed program write looks like from the outside: not an
// error, a machine that runs for ever doing something plausible.
type Flash struct {
	// name and bytes are this device's identity and the ROM image it was built around, not
	// state. Reset does not touch either - flash is non-volatile, and a reset that wiped the
	// reset vector would leave the machine with nothing to boot from.
	name  string
	bytes []byte

	refused uint64
}

// NewFlash returns a read-only flash part called name holding image.
//
// image is copied: the loader reads it from a file whose buffer it may reuse, and a flash chip
// whose contents change underneath the machine is the kind of fault that reads as a firmware bug.
func NewFlash(name string, image []byte) (*Flash, error) {
	if name == "" {
		return nil, fmt.Errorf("memory: flash needs a name: it is the key in a machine snapshot")
	}
	if len(image) == 0 {
		return nil, fmt.Errorf("memory: flash %s: image is empty", name)
	}
	bytes := make([]byte, len(image))
	copy(bytes, image)
	return &Flash{name: name, bytes: bytes}, nil
}

// Name is the device's identity.
func (f *Flash) Name() string { return f.name }

// Size is how many bytes the part holds.
func (f *Flash) Size() uint32 { return u32len(f.bytes) }

// Read returns size bytes at off, big-endian. An access running past the end of the part reads
// zero beyond it, matching RAM.
func (f *Flash) Read(off uint32, size bus.Size) uint32 {
	if uint64(off)+uint64(size) <= uint64(len(f.bytes)) {
		b := f.bytes[off:]
		// The bus refuses any other width before an access reaches a device, so the default arm
		// is Word rather than an error case.
		//exhaustive:ignore
		switch size {
		case bus.Byte:
			return uint32(b[0])
		case bus.Half:
			return uint32(b[0])<<8 | uint32(b[1])
		default:
			return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		}
	}
	limit := u32len(f.bytes)
	var v uint32
	for i := uint32(0); i < uint32(size); i++ {
		v <<= 8
		if a := off + i; a < limit {
			v |= uint32(f.bytes[a])
		}
	}
	return v
}

// Write refuses, and counts the refusal. See the type comment: a write this device swallows in
// silence is indistinguishable from one it accepted, and the firmware's response to the difference
// is to spin for ever rather than to complain.
func (f *Flash) Write(uint32, bus.Size, uint32) { f.refused++ }

// Refused is how many writes this part has turned away.
func (f *Flash) Refused() uint64 { return f.refused }

// Bytes exposes the image for instruments that read it wholesale. It is the live array; nothing
// in this phase may write through it, because that would make the contents state without making
// them snapshotted.
func (f *Flash) Bytes() []byte { return f.bytes }

// Reset clears the refusal count and nothing else.
//
// Flash is non-volatile: the oracle's own reset leaves both chips' arrays alone and clears only
// their command-sequencer fields, which is the behaviour every measurement in the record was taken
// against.
func (f *Flash) Reset() { f.refused = 0 }

// flashVersion is this device's snapshot format version.
const flashVersion = 1

// Snapshot writes the refusal count.
//
// It does not write the 2 MB image. In this phase the contents cannot change - the part is
// read-only and the loader verifies the image against firmware/MANIFEST.md before the machine
// starts - so they are configuration rather than state, and four megabytes of unchanging ROM in
// every snapshot would be four megabytes that prove nothing. TASK-2.8 makes the contents writable,
// and when it does they become state and this format changes with it.
func (f *Flash) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(f.name, flashVersion)
	w.Uint64(f.refused)
	blob, err := w.Blob()
	if err != nil {
		return nil, fmt.Errorf("memory: snapshot %s: %w", f.name, err)
	}
	return blob, nil
}

// Restore replaces the refusal count.
func (f *Flash) Restore(state []byte) error {
	rd, err := snapcodec.Open(state)
	if err != nil {
		return fmt.Errorf("memory: restore %s: %w", f.name, err)
	}
	if err := rd.Expect(f.name, flashVersion, flashVersion); err != nil {
		return fmt.Errorf("memory: restore %s: %w", f.name, err)
	}
	refused := rd.Uint64()
	if err := rd.Done(); err != nil {
		return fmt.Errorf("memory: restore %s: %w", f.name, err)
	}
	f.refused = refused
	return nil
}
