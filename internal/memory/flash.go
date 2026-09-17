package memory

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

// Flash is one of the board's two 2 MB Fujitsu MBM29LV160B parts. Each part owns its AMD
// command sequence; the bus supplies the cached and uncached mirrors of the same part.
type Flash struct {
	// The name is identity. Bytes are non-volatile state: Reset leaves them alone, but Snapshot
	// carries them because program and erase mutate them.
	name    string
	bytes   []byte
	refused uint64
	command uint8 // mode bit 0, unlock position bits 1-2, program bit 3, erase bit 4
}

func (f *Flash) commandState() (mode, seq uint8, program, erase bool) {
	return f.command & 1, (f.command >> 1) & 3, f.command&8 != 0, f.command&16 != 0
}

func (f *Flash) setCommand(mode, seq uint8, program, erase bool) {
	f.command = mode | (seq << 1)
	if program {
		f.command |= 8
	}
	if erase {
		f.command |= 16
	}
}

// NewFlash returns a flash part called name holding image.
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
	if f.command&1 != 0 {
		const manufacturer, device = uint32(0x0004), uint32(0x2249)
		if size == bus.Word {
			return manufacturer<<16 | device
		}
		id := manufacturer
		if off>>1&1 != 0 {
			id = device
		}
		if size == bus.Half {
			return id
		}
		if off&1 != 0 {
			return id & 0xff
		}
		return id >> 8
	}
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

// Write advances the x16 AMD command protocol. Programming only clears bits; an erase sets them.
func (f *Flash) Write(off uint32, size bus.Size, value uint32) {
	mode, seq, program, erase := f.commandState()
	defer func() { f.setCommand(mode, seq, program, erase) }()
	word, command := off>>1&0xfff, value&0xff
	if program {
		program, seq = false, 0
		for i := uint32(0); i < uint32(size); i++ {
			at := off + i
			if uint64(at) < uint64(len(f.bytes)) {
				f.bytes[at] &= byte((value >> ((uint32(size) - 1 - i) * 8)) & 0xff)
			}
		}
		return
	}
	if seq == 2 && erase && command == 0x30 {
		start, end := flashSector(off)
		if end > u32len(f.bytes) {
			end = u32len(f.bytes)
		}
		for i := start; i < end; i++ {
			f.bytes[i] = 0xff
		}
		erase, seq = false, 0
		return
	}
	if command == 0xf0 && !erase {
		mode, seq = 0, 0
		return
	}
	if seq == 0 && command == 0xaa && word == 0x555 {
		seq = 1
		return
	}
	if seq == 1 && command == 0x55 && word == 0x2aa {
		seq = 2
		return
	}
	if seq == 2 && word == 0x555 {
		switch command {
		case 0x90:
			mode = 1
		case 0xa0:
			program = true
		case 0x80:
			erase = true
		case 0x10:
			if erase {
				for i := range f.bytes {
					f.bytes[i] = 0xff
				}
				erase = false
			}
		default:
			f.refused++
		}
		seq = 0
		return
	}
	seq = 0
	f.refused++
}

func flashSector(off uint32) (uint32, uint32) {
	switch {
	case off < 0x4000:
		return 0, 0x4000
	case off < 0x6000:
		return 0x4000, 0x6000
	case off < 0x8000:
		return 0x6000, 0x8000
	case off < 0x10000:
		return 0x8000, 0x10000
	default:
		start := off &^ uint32(0xffff)
		return start, start + 0x10000
	}
}

// Refused is how many writes this part has turned away.
func (f *Flash) Refused() uint64 { return f.refused }

// Bytes exposes the live array to instruments that read it wholesale. Writers use commands.
func (f *Flash) Bytes() []byte { return f.bytes }

// ApplyHostPatch makes one declared presentation-policy change to the flash
// image after checking the exact original byte. Guest writes still use Write.
func (f *Flash) ApplyHostPatch(off uint32, expected, replacement byte) error {
	if uint64(off) >= uint64(len(f.bytes)) {
		return fmt.Errorf("memory: flash %s: host patch offset %X is outside image", f.name, off)
	}
	if f.bytes[off] != expected {
		return fmt.Errorf("memory: flash %s: host patch at %X found %02X, want %02X", f.name, off, f.bytes[off], expected)
	}
	f.bytes[off] = replacement
	return nil
}

// Reset clears command state, not non-volatile contents.
func (f *Flash) Reset() {
	f.refused, f.command = 0, 0
}

// flashVersion is this device's snapshot format version.
const flashVersion = 2

// Snapshot writes the array and every command-sequencer field, including an unlock in progress.
func (f *Flash) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(f.name, flashVersion)
	w.Bytes(f.bytes)
	w.Uint64(f.refused)
	w.Uint8(f.command)
	blob, err := w.Blob()
	if err != nil {
		return nil, fmt.Errorf("memory: snapshot %s: %w", f.name, err)
	}
	return blob, nil
}

// Restore replaces the array and command state atomically after validating the complete blob.
func (f *Flash) Restore(state []byte) error {
	rd, err := snapcodec.Open(state)
	if err != nil {
		return fmt.Errorf("memory: restore %s: %w", f.name, err)
	}
	if err := rd.Expect(f.name, flashVersion, flashVersion); err != nil {
		return fmt.Errorf("memory: restore %s: %w", f.name, err)
	}
	bytes := rd.Bytes()
	refused := rd.Uint64()
	command := rd.Uint8()
	if err := rd.Done(); err != nil {
		return fmt.Errorf("memory: restore %s: %w", f.name, err)
	}
	if len(bytes) != len(f.bytes) || command&^uint8(0x1f) != 0 || (command>>1)&3 > 2 {
		return fmt.Errorf("memory: restore %s: incompatible flash state", f.name)
	}
	copy(f.bytes, bytes)
	f.refused = refused
	f.command = command
	return nil
}
