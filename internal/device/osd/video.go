// Package osd models the video RAM port and display-list root registers.
package osd

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// VideoBase is the display and video RAM port's MMIO window.
	VideoBase = 0xB0002000
	// DisplayBase is the scaler and OSD display-list register window.
	DisplayBase = 0xB0004000
	// WindowSize is the mapped size of each MMIO window.
	WindowSize = 0x1000
	// VRAMSize is the decoder's one-megabyte video RAM.
	VRAMSize = 1 << 20
	// FramebufferBase is the application's first 8-bit OSD field buffer in board DRAM.
	FramebufferBase uint32 = 0x80584048
	// FrameWidth is the application's OSD width in pixels.
	FrameWidth = 720
	// FrameHeight is the application's OSD height in pixels.
	FrameHeight = 576
)

// Video is the address, mode and data port into decoder RAM.
type Video struct {
	data     [VRAMSize]byte
	addr     uint32
	mode     uint32
	readback [11]uint32
}

// NewVideo returns the reset video RAM port.
func NewVideo() *Video { return &Video{} }

// Name is the snapshot key.
func (*Video) Name() string { return "video-ram" }

var readbackOffsets = [...]uint32{0x90, 0x94, 0x98, 0x9c, 0xa0, 0xa4, 0xa8, 0xc0, 0xc4, 0xd0, 0xd4}

// Read responds only at ports for which read behaviour was measured.
func (v *Video) Read(off uint32, size bus.Size) uint32 {
	if size != bus.Word {
		return 0
	}
	switch off {
	case 0xb0:
		at := v.addr & (VRAMSize - 1)
		word := uint32(v.data[at])<<24 | uint32(v.data[(at+1)&(VRAMSize-1)])<<16 |
			uint32(v.data[(at+2)&(VRAMSize-1)])<<8 | uint32(v.data[(at+3)&(VRAMSize-1)])
		v.addr = (at + 4) & (VRAMSize - 1)
		return word
	case 0xb4:
		return v.mode
	case 0xb8:
		return 0 // never busy; the firmware's two spin loops exit
	case 0xbc:
		return v.addr
	}
	for i, reg := range readbackOffsets {
		if off == reg {
			return v.readback[i]
		}
	}
	return 0
}

// Write programmes the video RAM port and the measured plane-register readbacks.
func (v *Video) Write(off uint32, size bus.Size, value uint32) {
	if size != bus.Word {
		return
	}
	switch off {
	case 0xb0:
		at := v.addr & (VRAMSize - 1)
		for i := uint32(0); i < 4; i++ {
			v.data[(at+i)&(VRAMSize-1)] = byte((value >> (24 - 8*i)) & 0xff)
		}
		v.addr = (at + 4) & (VRAMSize - 1)
	case 0xb4:
		v.mode = value
	case 0xbc:
		v.addr = value & (VRAMSize - 1)
	default:
		for i, reg := range readbackOffsets {
			if off == reg {
				v.readback[i] = value
				return
			}
		}
	}
}

// Upload copies a DMA plane into the decoder's video RAM, wrapping at one megabyte.
// DMA channel 8 supplies the destination address through the measured +0xD0 register.
func (v *Video) Upload(at uint32, plane []byte) {
	for i, value := range plane {
		v.data[(at+uint32(i))&(VRAMSize-1)] = value // #nosec G115 -- DMA bounds each transfer to board DRAM
	}
}

// Reset clears video RAM and every register the port owns.
func (v *Video) Reset() { *v = Video{} }

// Snapshot captures port registers and all video RAM bytes.
func (v *Video) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(v.Name(), 1)
	w.Uint32(v.addr)
	w.Uint32(v.mode)
	w.Words(v.readback[:])
	w.Bytes(v.data[:])
	return w.Blob()
}

// Restore validates the full image before mutating video state.
func (v *Video) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("osd: video restore: %w", err)
	}
	if err := r.Expect(v.Name(), 1, 1); err != nil {
		return fmt.Errorf("osd: video restore: %w", err)
	}
	addr, mode, regs, data := r.Uint32(), r.Uint32(), r.Words(), r.Bytes()
	if err := r.Done(); err != nil {
		return fmt.Errorf("osd: video restore: %w", err)
	}
	if addr >= VRAMSize || len(regs) != len(v.readback) || len(data) != VRAMSize {
		return fmt.Errorf("osd: video restore: invalid register or memory length")
	}
	v.addr, v.mode = addr, mode
	copy(v.readback[:], regs)
	copy(v.data[:], data)
	return nil
}
