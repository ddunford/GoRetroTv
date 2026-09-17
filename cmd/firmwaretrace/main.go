// Command firmwaretrace runs the Go CPU from the verified reset image and emits checkpoints.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/cpu"
	"github.com/ddunford/goretrotv/internal/device/blitter"
	"github.com/ddunford/goretrotv/internal/device/csi"
	"github.com/ddunford/goretrotv/internal/device/demux"
	"github.com/ddunford/goretrotv/internal/device/dma"
	"github.com/ddunford/goretrotv/internal/device/eeprom"
	"github.com/ddunford/goretrotv/internal/device/i2c"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/device/osd"
	"github.com/ddunford/goretrotv/internal/firmware"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "firmwaretrace:", err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("firmware", "firmware", "verified firmware directory")
	steps := flag.Uint64("steps", 100000, "maximum retired instructions")
	interval := flag.Uint64("interval", 1000, "instructions between checkpoints")
	trace := flag.Bool("trace", false, "write per-instruction register state to stderr")
	traceFrom := flag.Uint64("trace-from", 0, "first instruction included in the trace")
	traceTo := flag.Uint64("trace-to", ^uint64(0), "first instruction excluded from the trace")
	unmapped := flag.Bool("unmapped", false, "list unmapped access sites at the end")
	bootState := flag.Bool("boot-state", false, "print bootloader handoff and decompression probes")
	key := flag.Int("key", -1, "raw handset code to send on the CSI link (-1 disables)")
	keyAt := flag.Uint64("key-at", 0, "instruction at which to queue the handset key")
	nvram := flag.String("nvram", "", "optional persistent 16 KiB EEPROM image path")
	flag.Parse()
	if *key < -1 || *key > 255 {
		return fmt.Errorf("raw handset key %d is outside 0..255", *key)
	}
	images, err := firmware.Load(context.Background(), *dir)
	if err != nil {
		return err
	}
	busMap := bus.New()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		return err
	}
	flash0, err := memory.NewFlash("U202", images.U202)
	if err != nil {
		return err
	}
	flash1, err := memory.NewFlash("U203", images.U203)
	if err != nil {
		return err
	}
	for _, m := range []struct {
		base, size uint32
		device     bus.Device
	}{
		{memory.DRAMBase, memory.DRAMSize, ram},
		{memory.FlashU202, memory.FlashSize, flash0},
		{memory.FlashU203, memory.FlashSize, flash1},
	} {
		if err := busMap.Attach(m.base, m.size, m.device); err != nil {
			return err
		}
	}
	core := cpu.New(busMap, memory.FlashU202)
	interrupts := irq.New(core.Interrupt)
	if err := busMap.Attach(irq.Base, irq.Size, interrupts); err != nil {
		return err
	}
	serial := csi.New(interrupts)
	if err := busMap.Attach(csi.Base, csi.Size, serial); err != nil {
		return err
	}
	store := eeprom.New()
	mux := i2c.NewMux()
	master := i2c.New(store, mux, interrupts)
	if *nvram != "" {
		if err := master.BindImage(*nvram); err != nil {
			return err
		}
	}
	if err := busMap.Attach(i2c.MuxBase, i2c.MuxSize, mux); err != nil {
		return err
	}
	if err := busMap.Attach(i2c.Base, i2c.Size, master); err != nil {
		return err
	}
	sectionDemux := demux.New()
	if err := sectionDemux.BindRAM(ram); err != nil {
		return err
	}
	sectionDemux.BindIRQ(interrupts)
	if err := busMap.Attach(demux.MMIOBase, demux.MMIOSize, sectionDemux); err != nil {
		return err
	}
	video := osd.NewVideo()
	if err := busMap.Attach(osd.VideoBase, osd.WindowSize, video); err != nil {
		return err
	}
	display := osd.NewDisplay()
	if err := display.BindRAM(ram); err != nil {
		return err
	}
	if err := busMap.Attach(osd.DisplayBase, osd.WindowSize, display); err != nil {
		return err
	}
	graphics := blitter.New(ram)
	if err := busMap.Attach(blitter.Base, blitter.Size, graphics); err != nil {
		return err
	}
	dmaController := dma.New(ram, graphics, video, interrupts)
	if err := busMap.Attach(dma.Base, dma.Size, dmaController); err != nil {
		return err
	}
	hasher, err := statehash.New(ram)
	if err != nil {
		return err
	}
	emitter, err := statehash.NewEmitter(os.Stdout, *interval, hasher)
	if err != nil {
		return err
	}
	var halt error
	for i := uint64(0); i < *steps; i++ {
		if *key >= 0 && i == *keyAt {
			// #nosec G115 -- raw key was checked to be within 0..255 above.
			if err := serial.Key(uint8(*key), 0); err != nil {
				return err
			}
		}
		serial.Pump(i)
		if *trace && i >= *traceFrom && i < *traceTo {
			fmt.Fprintf(os.Stderr, "%d PC=%08X ISA=%v Count=%08X Status=%08X GPR=%08X\n", i, core.PC, core.ISA, core.COP0[9], core.COP0[12], core.GPR)
		}
		if err := core.ObserveCheckpoint(emitter, i); err != nil {
			return err
		}
		if err := core.Step(); err != nil {
			halt = fmt.Errorf("after %d instructions: %w", i, err)
			break
		}
		if err := dmaController.Fault(); err != nil {
			halt = fmt.Errorf("after %d instructions: %w", i, err)
			break
		}
		if err := master.Fault(); err != nil {
			halt = fmt.Errorf("after %d instructions: %w", i, err)
			break
		}
	}
	if err := emitter.Close(); err != nil {
		return err
	}
	if halt != nil {
		return halt
	}
	fmt.Fprintf(os.Stderr, "retired %d instructions; PC=%08X ISA=%v; unmapped accesses: %+v\n", *steps, core.PC, core.ISA, busMap.UnmappedTotals())
	if *unmapped {
		for _, site := range busMap.Unmapped() {
			fmt.Fprintf(os.Stderr, "unmapped %+v\n", site)
		}
	}
	if *bootState {
		fmt.Fprintf(os.Stderr, "boot ready=%08X current=%08X image=%08X entry=%08X dram-main=%08X\n",
			busMap.Read(0x800083CC, bus.Word), busMap.Read(0x800083D0, bus.Word),
			busMap.Read(0xBFC20000, bus.Word), busMap.Read(0xBFC2002C, bus.Word),
			busMap.Read(0x800009F4, bus.Word))
	}
	return nil
}
