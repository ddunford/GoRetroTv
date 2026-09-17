// Package demod models the indirect I²C register interface of the satellite front-end.
package demod

import (
	"fmt"
	"sort"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const maxWrittenRegisters = 65536

// Model records writes while returning the measured fixed lock and identity
// responses. There is no RF source: the emulated front-end stays locked.
type Model struct {
	register     uint16
	selector     uint8
	index        uint16
	writes       map[uint32]uint8
	reads        uint64
	readObserver func(register uint16, value uint8)
}

// New creates a power-on front-end.
func New() *Model { return &Model{writes: make(map[uint32]uint8)} }

// Name is the snapshot identity.
func (*Model) Name() string { return "satellite-demod" }

// Read exposes the measured logical register response for the device contract.
func (m *Model) Read(off uint32, _ bus.Size) uint32 {
	// #nosec G115 -- the indirect register address is exactly 16 bits.
	return uint32(Answer(uint16(off)))
}

// Write feeds a byte to the currently selected indirect port.
func (m *Model) Write(_ uint32, _ bus.Size, value uint32) { m.ShiftWrite(byte(value & 0xff)) }

// Answer returns the firmware-observed values. Guest writes do not alter these
// read responses; doing so once stopped the reference machine booting.
func Answer(register uint16) uint8 {
	switch register {
	case 11:
		return 0x3f
	case 34:
		return 0x03
	case 74, 92:
		return 0x80
	case 75:
		return 0x17
	case 78:
		return 0x02
	default:
		return 0
	}
}

// Locked reports the fixed virtual RF condition.
func (*Model) Locked() bool { return true }

// Start begins one I²C transaction, whose first byte selects a port.
func (m *Model) Start() { m.index = 0 }

// ShiftWrite applies the indirect low/high address and data-port protocol.
func (m *Model) ShiftWrite(value uint8) {
	if m.index == 0 {
		m.selector = value
		m.index++
		return
	}
	switch m.selector {
	case 0:
		m.register = m.register&0xff00 | uint16(value)
	case 1:
		m.register = uint16(value)<<8 | m.register&0x00ff
	default:
		key := uint32(m.selector)<<16 | uint32(m.register)
		_, exists := m.writes[key]
		if len(m.writes) < maxWrittenRegisters || exists {
			m.writes[key] = value
		}
		m.register++
	}
	m.index++
}

// ShiftRead returns one indirect register byte and auto-increments the index.
func (m *Model) ShiftRead() uint8 {
	register := m.register
	value := Answer(register)
	m.register++
	m.reads++
	if m.readObserver != nil {
		m.readObserver(register, value)
	}
	return value
}

// SetReadObserver attaches a diagnostic sink to physical I²C read phases.
// The observer is host instrumentation and does not affect guest state.
func (m *Model) SetReadObserver(observer func(register uint16, value uint8)) {
	m.readObserver = observer
}

// Written reports a value the firmware uploaded to a given port and register.
func (m *Model) Written(port uint8, register uint16) (uint8, bool) {
	value, ok := m.writes[uint32(port)<<16|uint32(register)]
	return value, ok
}

// Reset clears the transaction and recorded upload state.
func (m *Model) Reset() {
	m.register, m.selector, m.index, m.reads = 0, 0, 0, 0
	m.writes = make(map[uint32]uint8)
}

// Snapshot captures the indirect pointer and all uploaded register bytes.
func (m *Model) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(m.Name(), 1)
	w.Uint16(m.register)
	w.Uint8(m.selector)
	w.Uint16(m.index)
	w.Uint64(m.reads)
	keys := make([]uint32, 0, len(m.writes))
	for key := range m.writes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	// #nosec G115 -- map is capped at 65536 entries.
	w.Uint32(uint32(len(keys)))
	for _, key := range keys {
		w.Uint32(key)
		w.Uint8(m.writes[key])
	}
	return w.Blob()
}

// Restore validates the complete indirect state before replacing it.
func (m *Model) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("demod: restore: %w", err)
	}
	if err := r.Expect(m.Name(), 1, 1); err != nil {
		return fmt.Errorf("demod: restore: %w", err)
	}
	register, selector, index, reads := r.Uint16(), r.Uint8(), r.Uint16(), r.Uint64()
	count := r.Uint32()
	if count > maxWrittenRegisters {
		return fmt.Errorf("demod: restore: too many register writes")
	}
	writes := make(map[uint32]uint8, count)
	var prior uint32
	for i := uint32(0); i < count; i++ {
		key, value := r.Uint32(), r.Uint8()
		if i > 0 && key <= prior {
			return fmt.Errorf("demod: restore: unordered register keys")
		}
		writes[key], prior = value, key
	}
	if err := r.Done(); err != nil {
		return fmt.Errorf("demod: restore: %w", err)
	}
	m.register, m.selector, m.index, m.reads, m.writes = register, selector, index, reads, writes
	return nil
}
