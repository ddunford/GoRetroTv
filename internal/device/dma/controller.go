// Package dma models the media ASIC's descriptor DMA and completion registers.
package dma

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/blitter"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/device/osd"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Base is the DMA controller's uncached MMIO address.
	Base = 0xB0009000
	// Size is the controller's mapped window.
	Size = 0x1000
	// ChannelCount includes the 13 channels scanned by the LISR and one unused slot.
	ChannelCount = 14
	// DescriptorBase is the firmware's 40-byte-per-channel descriptor array in DRAM.
	DescriptorBase = 0x80108A60
	// DescriptorSize is the firmware's descriptor stride.
	DescriptorSize = 40
	// IRQMask is dispatch row two's DMA bit in the board interrupt controller.
	IRQMask = 0x2
)

// Controller owns descriptor pointers, channel enable, completion and fault state.
// DRAM, blitter, video RAM and the board IRQ controller are configuration links.
type Controller struct {
	descriptors [ChannelCount]uint32
	enable      uint32
	statusA     uint32
	statusB     uint32
	fault       string
	ram         *memory.RAM
	blitter     *blitter.Blitter
	video       *osd.Video
	flash       *memory.Flash
	transport   TransportSink
	interrupt   *irq.Controller
}

// TransportSink accepts the transport bytes moved by DMA channel five.
type TransportSink interface{ PushTransport([]byte) error }

// New binds the controller to the devices its measured channels drive.
func New(ram *memory.RAM, graphics *blitter.Blitter, video *osd.Video, interrupt *irq.Controller) *Controller {
	return &Controller{ram: ram, blitter: graphics, video: video, interrupt: interrupt}
}

// BindTransport connects channel five's ROM source and demux destination.
func (d *Controller) BindTransport(flash *memory.Flash, sink TransportSink) {
	d.flash, d.transport = flash, sink
}

// Name is the snapshot key.
func (*Controller) Name() string { return "media-dma" }

func wordPart(word, off uint32, size bus.Size) uint32 {
	switch size {
	case bus.Word:
		return word
	case bus.Half:
		return word >> ((2 - (off & 2)) * 8) & 0xffff
	case bus.Byte:
		return word >> ((3 - (off & 3)) * 8) & 0xff
	default:
		return 0
	}
}

// Read returns the measured enable, completion and descriptor-pointer registers.
func (d *Controller) Read(off uint32, size bus.Size) uint32 {
	reg := off &^ 3
	var word uint32
	switch {
	case reg == 0x10:
		word = d.enable
	case reg == 0x120:
		word = d.statusA
	case reg == 0x140:
		word = d.statusB
	case reg >= 0x40 && reg < 0x40+0x10*ChannelCount && (reg-0x40)%0x10 == 0:
		word = d.descriptors[(reg-0x40)/0x10]
	default:
		return 0
	}
	return wordPart(word, off, size)
}

// Write starts newly enabled channels, acknowledges completion, or sets a descriptor pointer.
func (d *Controller) Write(off uint32, size bus.Size, value uint32) {
	reg := off &^ 3
	switch {
	case reg == 0x10 && size == bus.Word:
		started := value &^ d.enable
		d.enable = value
		for ch := uint8(0); ch < ChannelCount; ch++ {
			if started&(1<<ch) != 0 {
				if err := d.Run(ch); err != nil {
					d.fault = err.Error()
					return
				}
			}
		}
	case reg == 0x130:
		d.statusA &^= value
		d.updateLine()
	case reg == 0x150:
		d.statusB &^= value
		d.updateLine()
	case reg >= 0x40 && reg < 0x40+0x10*ChannelCount && (reg-0x40)%0x10 == 0 && size == bus.Word:
		d.descriptors[(reg-0x40)/0x10] = value
	}
}

func (d *Controller) updateLine() {
	if d.interrupt != nil {
		d.interrupt.SetLine(IRQMask, (d.statusA&^0x20)|d.statusB != 0)
	}
}

// Fault returns a visible error if a guest DMA command could not be executed.
func (d *Controller) Fault() error {
	if d.fault != "" {
		return fmt.Errorf("dma: %s", d.fault)
	}
	return nil
}

// Reset clears every device-owned register and completion bit.
func (d *Controller) Reset() {
	d.descriptors = [ChannelCount]uint32{}
	d.enable, d.statusA, d.statusB = 0, 0, 0
	d.fault = ""
	d.updateLine()
}

// Snapshot captures all DMA register and fault state.
func (d *Controller) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(d.Name(), 1)
	w.Words(d.descriptors[:])
	w.Words([]uint32{d.enable, d.statusA, d.statusB})
	w.String(d.fault)
	return w.Blob()
}

// Restore validates the complete blob before replacing DMA state.
func (d *Controller) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("dma: restore: %w", err)
	}
	if err := r.Expect(d.Name(), 1, 1); err != nil {
		return fmt.Errorf("dma: restore: %w", err)
	}
	descriptors, regs, fault := r.Words(), r.Words(), r.String()
	if err := r.Done(); err != nil {
		return fmt.Errorf("dma: restore: %w", err)
	}
	if len(descriptors) != ChannelCount || len(regs) != 3 || len(fault) > 256 {
		return fmt.Errorf("dma: restore: invalid register state")
	}
	copy(d.descriptors[:], descriptors)
	d.enable, d.statusA, d.statusB, d.fault = regs[0], regs[1], regs[2], fault
	d.updateLine()
	return nil
}
