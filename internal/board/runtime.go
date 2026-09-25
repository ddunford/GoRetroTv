// Package board assembles and advances the measured Pace 2500N hardware model.
// The web server and diagnostic runner must use the same device wiring and pump cadence.
package board

import (
	"fmt"
	"image"
	"io"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/cpu"
	"github.com/ddunford/goretrotv/internal/device/audio"
	"github.com/ddunford/goretrotv/internal/device/blitter"
	"github.com/ddunford/goretrotv/internal/device/boardlatch"
	"github.com/ddunford/goretrotv/internal/device/csi"
	"github.com/ddunford/goretrotv/internal/device/demod"
	"github.com/ddunford/goretrotv/internal/device/demux"
	"github.com/ddunford/goretrotv/internal/device/dma"
	"github.com/ddunford/goretrotv/internal/device/eeprom"
	"github.com/ddunford/goretrotv/internal/device/hwtimer"
	"github.com/ddunford/goretrotv/internal/device/i2c"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/device/modem"
	"github.com/ddunford/goretrotv/internal/device/osd"
	"github.com/ddunford/goretrotv/internal/device/smartcard"
	"github.com/ddunford/goretrotv/internal/firmware"
	"github.com/ddunford/goretrotv/internal/machine"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/clock"
)

// Runtime owns one single-threaded emulator. Only its instruction-loop owner may
// call Step, Restore, Compose, or mutate the exposed devices.
type Runtime struct {
	Machine   *machine.Machine
	RAM       *memory.RAM
	FlashU202 *memory.Flash
	CSI       *csi.Link
	Display   *osd.Display
	Demux     *demux.Demux
	Demod     *demod.Model
	IRQ       *irq.Controller
	I2C       *i2c.Controller
	DMA       *dma.Controller
	Audio     *audio.Control
	Timer     *hwtimer.Timer
	hooks     StepHooks
}

// New constructs a reset board from firmware already verified by firmware.Load.
// skyGates opts into the oracle's explicitly declared presentation policy.
func New(images *firmware.Set, skyGates bool) (*Runtime, error) {
	if images == nil || len(images.U202) != memory.FlashSize || len(images.U203) != memory.FlashSize {
		return nil, fmt.Errorf("board: verified U202 and U203 firmware images are required")
	}
	r := &Runtime{}
	board := bus.New()
	var err error
	r.RAM, err = memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		return nil, err
	}
	r.FlashU202, err = memory.NewFlash("U202", images.U202)
	if err != nil {
		return nil, err
	}
	flash1, err := memory.NewFlash("U203", images.U203)
	if err != nil {
		return nil, err
	}
	for _, m := range []struct {
		base, size uint32
		device     bus.Device
	}{
		{memory.DRAMBase, memory.DRAMSize, r.RAM},
		{memory.FlashU202, memory.FlashSize, r.FlashU202},
		{memory.FlashU203, memory.FlashSize, flash1},
	} {
		if err := board.Attach(m.base, m.size, m.device); err != nil {
			return nil, err
		}
	}
	if err := board.Attach(boardlatch.Base, boardlatch.Size, boardlatch.New()); err != nil {
		return nil, err
	}
	core := cpu.New(board, memory.FlashU202)
	interrupts := irq.New(core.Interrupt)
	r.IRQ = interrupts
	if err := board.Attach(irq.Base, irq.Size, interrupts); err != nil {
		return nil, err
	}
	r.Timer = hwtimer.New(interrupts)
	if err := board.Attach(hwtimer.Base, hwtimer.Size, r.Timer); err != nil {
		return nil, err
	}
	r.CSI = csi.New(interrupts)
	if err := board.Attach(csi.Base, csi.Size, r.CSI); err != nil {
		return nil, err
	}
	modemPort := modem.New(interrupts)
	if err := board.Attach(modem.Base, modem.Size, modemPort); err != nil {
		return nil, err
	}
	cardPort := smartcard.New(interrupts)
	if err := board.Attach(smartcard.Base, smartcard.Size, cardPort); err != nil {
		return nil, err
	}
	store := eeprom.New()
	mux := i2c.NewMux()
	r.I2C = i2c.New(store, interrupts)
	r.Demod = demod.New()
	r.I2C.BindDemod(r.Demod)
	if err := board.Attach(i2c.MuxBase, i2c.MuxSize, mux); err != nil {
		return nil, err
	}
	if err := board.Attach(i2c.Base, i2c.Size, r.I2C); err != nil {
		return nil, err
	}
	r.Demux = demux.New()
	if err := r.Demux.BindRAM(r.RAM); err != nil {
		return nil, err
	}
	r.Demux.BindIRQ(interrupts)
	if err := board.Attach(demux.MMIOBase, demux.MMIOSize, r.Demux); err != nil {
		return nil, err
	}
	video := osd.NewVideo()
	if err := board.Attach(osd.VideoBase, osd.WindowSize, video); err != nil {
		return nil, err
	}
	r.Display = osd.NewDisplay()
	if err := r.Display.BindRAM(r.RAM); err != nil {
		return nil, err
	}
	if err := board.Attach(osd.DisplayBase, osd.WindowSize, r.Display); err != nil {
		return nil, err
	}
	graphics := blitter.New(r.RAM)
	if err := board.Attach(blitter.Base, blitter.Size, graphics); err != nil {
		return nil, err
	}
	r.DMA = dma.New(r.RAM, graphics, video, interrupts)
	if err := board.Attach(dma.Base, dma.Size, r.DMA); err != nil {
		return nil, err
	}
	r.Audio = audio.New()
	if err := board.Attach(audio.Base, audio.Size, r.Audio); err != nil {
		return nil, err
	}

	var handoff machine.Handoff
	sky := machine.NewSkyGates(skyGates)
	loopClock := clock.New()
	const pumpName = "board-pump"
	var pump clock.Handler
	pump = func(now uint64) error {
		r.Timer.Pump(now + 15)
		if err := r.Demux.Pump(now); err != nil {
			return err
		}
		r.CSI.Pump(r.Machine.Retired, r.Timer.Ticks())
		modemPort.Pump(r.Timer.Ticks())
		cardPort.Pump(16)
		applied, err := handoff.Tick(core, board)
		if err != nil {
			return err
		}
		if applied && r.hooks.Handoff != nil {
			r.hooks.Handoff(r.Machine.Retired, core.PC)
		}
		_, err = loopClock.At(now+16, pumpName, pump)
		return err
	}
	if _, err := loopClock.At(1, pumpName, pump); err != nil {
		return nil, err
	}
	r.Machine, err = machine.New(core, board, loopClock, &handoff, sky,
		map[string]clock.Handler{pumpName: pump})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Step retires one guest instruction. Its two clock boundaries match the oracle:
// a MIPS32 branch and delay slot share the first boundary; an accepted interrupt
// gets its own boundary before the CPU vectors.
func (r *Runtime) Step() error { return r.StepWithHooks(StepHooks{}) }

// StepHooks let diagnostic tools observe the same ordered instruction loop
// without constructing another board or changing guest-visible reads.
type StepHooks struct {
	AfterPump func() error
	AfterSky  func() error
	Access    func(bus.ObservedAccess)
	Handoff   func(retired uint64, entryPC uint32)
	SkyGate   func(retired uint64)
}

// StepWithHooks retires one instruction with optional inert observations.
func (r *Runtime) StepWithHooks(hooks StepHooks) error {
	if r == nil || r.Machine == nil {
		return fmt.Errorf("board: runtime is nil")
	}
	m := r.Machine
	i := m.Retired
	// Held BY VALUE, and deliberately not cleared afterwards. Storing &hooks
	// moved the parameter to the heap -- 'go build -gcflags=-m' says so
	// outright -- and Step calls this once per emulated instruction, so the
	// machine allocated once per instruction and spent a fifth of its time
	// between the allocator and the defer that cleared the field.
	//
	// It is not cleared afterwards, and the defer that used to do it was pure
	// cost rather than safety. r.hooks is read only by the clock pump (see
	// New), the clock is advanced only by the pump closure below, and this
	// line runs before either -- on EVERY entry, since Step passes an empty
	// StepHooks. A leftover hook therefore cannot be read by a later step
	// whether the field holds a pointer or a value, which is a structural
	// property of there being one entry point, not something to defend with
	// a test: a test written for it passes just as happily with the bug it
	// would be guarding against. The guard that does bite is
	// TestStepDoesNotAllocate.
	r.hooks = hooks
	pump := func() error { return m.Clock.Tick() }
	if !m.Core.HasPendingBranch() || m.Core.ISA {
		if err := pump(); err != nil {
			return fmt.Errorf("after %d instructions: %w", i, err)
		}
	}
	if hooks.AfterPump != nil {
		if err := hooks.AfterPump(); err != nil {
			return err
		}
	}
	if !m.Core.HasPendingBranch() || m.Core.ISA {
		applied, err := m.SkyGates.Tick(i, m.Handoff.Done(), r.taskCount, m.Bus, r.FlashU202)
		if err != nil {
			return fmt.Errorf("after %d instructions: %w", i, err)
		}
		if applied && hooks.SkyGate != nil {
			hooks.SkyGate(i)
		}
	}
	if hooks.AfterSky != nil {
		if err := hooks.AfterSky(); err != nil {
			return err
		}
	}
	boundary := pump
	if hooks.Access != nil {
		m.Bus.SetObserver(hooks.Access)
		defer m.Bus.SetObserver(nil)
		boundary = func() error {
			m.Bus.SetObserver(nil)
			err := pump()
			m.Bus.SetObserver(hooks.Access)
			return err
		}
	}
	if err := m.Core.StepWithInterruptBoundary(boundary); err != nil {
		return fmt.Errorf("after %d instructions: %w", i, err)
	}
	m.Retired = i + 1
	if err := r.DMA.Fault(); err != nil {
		return fmt.Errorf("after %d instructions: %w", i, err)
	}
	if err := r.I2C.Fault(); err != nil {
		return fmt.Errorf("after %d instructions: %w", i, err)
	}
	return nil
}

// Restore applies one complete machine snapshot. It must precede any browser
// publication or input, so an invalid image never appears as a working box.
func (r *Runtime) Restore(src io.Reader) error { return r.Machine.Restore(src) }

// Compose returns the firmware-programmed indexed OSD plane and palette.
func (r *Runtime) Compose() (*image.Paletted, error) { return r.Display.Compose() }
