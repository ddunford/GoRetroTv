package i2c

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/eeprom"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

// Controller implements the byte-at-a-time I²C master. Attached slaves own
// their contents; this device owns only the transaction in progress.
type Controller struct {
	control, status, clock, data, enable uint32
	startArmed, active, dirty            bool
	slave                                uint8
	pointer                              uint16
	addressBytes                         uint8
	fault                                string
	store                                *eeprom.Store
	mux                                  *Mux
	interrupt                            *irq.Controller
	imagePath                            string
}

// New binds the NVRAM, channel latch and board interrupt controller.
func New(store *eeprom.Store, mux *Mux, interrupt *irq.Controller) *Controller {
	return &Controller{control: 0x99, store: store, mux: mux, interrupt: interrupt}
}

// BindImage makes completed EEPROM write transactions durable at path.
func (c *Controller) BindImage(path string) error {
	if c.store == nil {
		return fmt.Errorf("i2c: EEPROM is not bound")
	}
	if err := c.store.Load(path); err != nil {
		return err
	}
	c.imagePath = path
	return nil
}

// Name is the snapshot identity.
func (*Controller) Name() string { return "i2c-master" }

// Read exposes the measured controller register values.
func (c *Controller) Read(off uint32, size bus.Size) uint32 {
	if size != bus.Word {
		return 0
	}
	switch off {
	case 0x00:
		return c.control
	case 0x10:
		return c.status
	case 0x20:
		return c.clock
	case 0x40:
		return c.data
	case 0x60:
		return c.enable
	default:
		return 0
	}
}

// Write advances one control or data phase; only data and receive phases signal completion.
func (c *Controller) Write(off uint32, size bus.Size, value uint32) {
	if size != bus.Word {
		return
	}
	switch off {
	case 0x00:
		c.control = value & 0xff
		switch c.control {
		case 0x99:
			if c.active && c.isEEPROM() && c.dirty && c.imagePath != "" {
				if err := c.store.Persist(c.imagePath); err != nil {
					c.fault = err.Error()
				}
			}
			c.active, c.startArmed, c.dirty = false, false, false
		case 0x9a:
			c.startArmed = true
		case 0xac, 0xa8:
			switch {
			case c.active && c.isEEPROM() && c.slave&1 != 0 && c.store != nil:
				c.data = c.store.Read(uint32(c.pointer), bus.Byte)
				c.pointer = (c.pointer + 1) & (eeprom.Capacity - 1)
			case c.active:
				c.data = 0
			default:
				c.data = 0xff
			}
			c.complete(true)
		}
	case 0x20:
		c.clock = value & 0xff
	case 0x40:
		// #nosec G115 -- the hardware data register is eight bits.
		c.writeData(uint8(value))
	case 0x50:
		if c.interrupt != nil {
			c.interrupt.SetLine(IRQMask, false)
		}
	case 0x60:
		c.enable = value & 1
	}
}

func (c *Controller) isEEPROM() bool {
	if c.mux == nil {
		return false
	}
	channel, connected := c.mux.Channel()
	return connected && channel == 3 && c.slave&^uint8(1) == 0xa0
}

func (c *Controller) writeData(value uint8) {
	c.data = uint32(value)
	if c.startArmed {
		c.startArmed = false
		c.slave = value
		var channel uint8
		var connected bool
		if c.mux != nil {
			channel, connected = c.mux.Channel()
		}
		c.active = connected && (channel == 3 && value&^uint8(1) == 0xa0 || channel == 0 && value&^uint8(1) == 0x18 || channel == 1 && value&^uint8(1) == 0xca || value == 0)
		if c.isEEPROM() && value&1 == 0 {
			c.addressBytes = 0
		}
		c.complete(c.active)
		return
	}
	if c.active && c.isEEPROM() {
		switch c.addressBytes {
		case 0:
			c.pointer = uint16(value) & 0x3f
			c.addressBytes++
		case 1:
			c.pointer = (c.pointer<<8 | uint16(value)) & (eeprom.Capacity - 1)
			c.addressBytes++
		default:
			if c.store != nil {
				c.store.Write(uint32(c.pointer), bus.Byte, uint32(value))
				c.pointer = c.pointer&^(eeprom.PageSize-1) | (c.pointer+1)&(eeprom.PageSize-1)
				c.dirty = true
			}
		}
	}
	c.complete(c.active)
}

func (c *Controller) complete(ack bool) {
	c.status = 0
	if ack {
		c.status = 4
	}
	if c.enable != 0 && c.interrupt != nil {
		c.interrupt.SetLine(IRQMask, true)
	}
}

// Fault reports any failed persistence operation, which the runner must halt on.
func (c *Controller) Fault() error {
	if c.fault != "" {
		return fmt.Errorf("i2c: %s", c.fault)
	}
	return nil
}

// Reset clears volatile transfer state while leaving EEPROM contents intact.
func (c *Controller) Reset() {
	c.control, c.status, c.clock, c.data, c.enable = 0x99, 0, 0, 0, 0
	c.startArmed, c.active, c.dirty = false, false, false
	c.slave, c.pointer, c.addressBytes, c.fault = 0, 0, 0, ""
	if c.interrupt != nil {
		c.interrupt.SetLine(IRQMask, false)
	}
}

// Snapshot captures every byte of the transfer currently in progress.
func (c *Controller) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(c.Name(), 1)
	w.Words([]uint32{c.control, c.status, c.clock, c.data, c.enable})
	w.Bool(c.startArmed)
	w.Bool(c.active)
	w.Bool(c.dirty)
	w.Uint8(c.slave)
	w.Uint16(c.pointer)
	w.Uint8(c.addressBytes)
	w.String(c.fault)
	return w.Blob()
}

// Restore validates complete volatile state before replacing it.
func (c *Controller) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("i2c: restore: %w", err)
	}
	if err := r.Expect(c.Name(), 1, 1); err != nil {
		return fmt.Errorf("i2c: restore: %w", err)
	}
	regs := r.Words()
	armed, active, dirty := r.Bool(), r.Bool(), r.Bool()
	slave, pointer, addressBytes, fault := r.Uint8(), r.Uint16(), r.Uint8(), r.String()
	if err := r.Done(); err != nil {
		return fmt.Errorf("i2c: restore: %w", err)
	}
	if len(regs) != 5 || addressBytes > 2 || pointer >= eeprom.Capacity || len(fault) > 256 {
		return fmt.Errorf("i2c: restore: invalid state")
	}
	c.control, c.status, c.clock, c.data, c.enable = regs[0], regs[1], regs[2], regs[3], regs[4]
	c.startArmed, c.active, c.dirty, c.slave, c.pointer, c.addressBytes, c.fault = armed, active, dirty, slave, pointer, addressBytes, fault
	if c.interrupt != nil {
		c.interrupt.SetLine(IRQMask, c.enable != 0 && c.status != 0)
	}
	return nil
}
