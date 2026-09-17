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
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "firmwaretrace:", err)
		os.Exit(1)
	}
}

func run() error {
	var hits pcHits
	dir := flag.String("firmware", "firmware", "verified firmware directory")
	steps := flag.Uint64("steps", 100000, "maximum retired instructions")
	interval := flag.Uint64("interval", 1000, "instructions between checkpoints")
	trace := flag.Bool("trace", false, "write per-instruction register state to stderr")
	traceFrom := flag.Uint64("trace-from", 0, "first instruction included in the trace")
	traceTo := flag.Uint64("trace-to", ^uint64(0), "first instruction excluded from the trace")
	unmapped := flag.Bool("unmapped", false, "list unmapped access sites at the end")
	bootState := flag.Bool("boot-state", false, "print bootloader handoff and decompression probes")
	probe := flag.Uint64("probe", 0, "print one guest word at a virtual address (hex or decimal)")
	watchWord := flag.Uint64("watch-word", 0, "trace changes to one guest word")
	stopPC := flag.Uint64("stop-pc", 0, "stop on the first guest PC match (hex or decimal)")
	tasks := flag.Bool("tasks", false, "print the guest Nucleus task census")
	skyGates := flag.Bool("sky-gates", false, "apply the oracle's declared post-boot Sky menu gate policy")
	scheduler := flag.Bool("scheduler", false, "print guest current-task changes")
	csiWire := flag.Bool("csi-wire", false, "print bytes the guest transmitted on CSI")
	surfaceHash := flag.Bool("surface-hash", false, "print the raw 720x576 OSD RAM hash and distinct byte count")
	key := flag.Int("key", -1, "raw handset code to send on the CSI link (-1 disables)")
	ackCode := flag.Int("ack-code", -1, "additional CSI command code to acknowledge (-1 keeps measured default)")
	ackAll := flag.Bool("ack-all", false, "diagnostic policy: acknowledge every CSI command")
	keyAt := flag.Uint64("key-at", 0, "instruction at which to queue the handset key")
	nvram := flag.String("nvram", "", "optional persistent 16 KiB EEPROM image path")
	flag.Var(&hits, "pc-hit", "count guest executions of this PC (repeatable, hex or decimal)")
	flag.Parse()
	if *key < -1 || *key > 255 {
		return fmt.Errorf("raw handset key %d is outside 0..255", *key)
	}
	if *ackCode < -1 || *ackCode > 255 {
		return fmt.Errorf("CSI acknowledgement code %d is outside 0..255", *ackCode)
	}
	if *ackAll && *ackCode >= 0 {
		return fmt.Errorf("ack-all and ack-code are mutually exclusive")
	}
	if *stopPC > 0xffffffff {
		return fmt.Errorf("stop PC %#x exceeds 32-bit address space", *stopPC)
	}
	if *probe > 0xffffffff {
		return fmt.Errorf("probe address %#x exceeds 32-bit address space", *probe)
	}
	if *watchWord > 0xffffffff {
		return fmt.Errorf("watch address %#x exceeds 32-bit address space", *watchWord)
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
	boardTimer := hwtimer.New(interrupts)
	if err := busMap.Attach(hwtimer.Base, hwtimer.Size, boardTimer); err != nil {
		return err
	}
	serial := csi.New(interrupts)
	if *ackAll {
		serial.AckAll()
	} else if *ackCode >= 0 {
		serial.SetAckPolicy([]uint8{0x52, 0x18, uint8(*ackCode)}) // #nosec G115 -- checked above.
	}
	if err := busMap.Attach(csi.Base, csi.Size, serial); err != nil {
		return err
	}
	modemPort := modem.New(interrupts)
	if err := busMap.Attach(modem.Base, modem.Size, modemPort); err != nil {
		return err
	}
	cardPort := smartcard.New(interrupts)
	if err := busMap.Attach(smartcard.Base, smartcard.Size, cardPort); err != nil {
		return err
	}
	store := eeprom.New()
	mux := i2c.NewMux()
	master := i2c.New(store, interrupts)
	master.BindDemod(demod.New())
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
	var retired uint64
	var previousTask uint32
	var previousWord uint32
	var handoff machine.Handoff
	skyMenu := machine.NewSkyGates(*skyGates)
	var handoffPumpPhase uint8 = 1
	var boardPumpClock uint64
	var instruction uint64
	pumpBoard := func() error {
		handoffPumpPhase--
		if handoffPumpPhase != 0 {
			return nil
		}
		handoffPumpPhase = 16
		boardPumpClock += 16
		boardTimer.Pump(boardPumpClock)
		serial.Pump(instruction, boardTimer.Ticks())
		modemPort.Pump(boardTimer.Ticks())
		cardPort.Pump(16)
		applied, err := handoff.Tick(core, busMap)
		if err != nil {
			return fmt.Errorf("after %d instructions: %w", instruction, err)
		}
		if applied {
			fmt.Fprintf(os.Stderr, "declared host application handoff after %d guest instructions: PC=%08X\n", instruction, core.PC)
		}
		return nil
	}
	if *watchWord != 0 {
		previousWord = busMap.Read(uint32(*watchWord), bus.Word)
	} // #nosec G115 -- checked above.
	for i := uint64(0); i < *steps; i++ {
		instruction = i
		if *scheduler {
			current := ram.Read(0x83d0, bus.Word)
			if current != previousTask {
				fmt.Fprintf(os.Stderr, "scheduler %d PC=%08X from=%08X to=%08X ready=%08X\n", i, core.PC, previousTask, current, ram.Read(0x83cc, bus.Word))
				previousTask = current
			}
		}
		if *key >= 0 && i == *keyAt {
			hits.MarkKey()
			// #nosec G115 -- raw key was checked to be within 0..255 above.
			if err := serial.Key(uint8(*key), 0); err != nil {
				return err
			}
		}
		// The oracle's MIPS32 branch and delay slot occupy one pump iteration.
		// Go executes them as two Steps, so the delay slot does not advance this phase.
		if !core.HasPendingBranch() || core.ISA {
			if err := pumpBoard(); err != nil {
				return err
			}
		}
		if *trace && i >= *traceFrom && i < *traceTo {
			fmt.Fprintf(os.Stderr, "%d PC=%08X ISA=%v Count=%08X Status=%08X GPR=%08X\n", i, core.PC, core.ISA, core.COP0[9], core.COP0[12], core.GPR)
		}
		hits.Observe(core.PC)
		if err := core.ObserveCheckpoint(emitter, i); err != nil {
			return err
		}
		if *skyGates && (!core.HasPendingBranch() || core.ISA) {
			applied, err := skyMenu.Tick(i, handoff.Done(), func() (int, error) {
				offsets, err := createdTaskOffsets(ram)
				return len(offsets), err
			}, busMap, flash0)
			if err != nil {
				return fmt.Errorf("after %d instructions: %w", i, err)
			}
			if applied {
				fmt.Fprintf(os.Stderr, "declared Sky menu gates applied after %d guest instructions\n", i)
			}
		}
		if err := core.StepWithInterruptBoundary(pumpBoard); err != nil {
			halt = fmt.Errorf("after %d instructions: %w", i, err)
			break
		}
		retired = i + 1
		if *watchWord != 0 {
			value := busMap.Read(uint32(*watchWord), bus.Word) // #nosec G115 -- checked above.
			if value != previousWord {
				fmt.Fprintf(os.Stderr, "watch %d PC=%08X address=%08X before=%08X after=%08X\n", i+1, core.PC, uint32(*watchWord), previousWord, value) // #nosec G115 -- checked above.
				previousWord = value
			}
		}
		if err := dmaController.Fault(); err != nil {
			halt = fmt.Errorf("after %d instructions: %w", i, err)
			break
		}
		if err := master.Fault(); err != nil {
			halt = fmt.Errorf("after %d instructions: %w", i, err)
			break
		}
		// #nosec G115 -- stopPC was checked against the 32-bit address space.
		if *stopPC != 0 && core.PC == uint32(*stopPC) {
			fmt.Fprintf(os.Stderr, "reached PC=%08X after %d guest instructions; Cause=%08X EPC=%08X\n", core.PC, i+1, core.COP0[13], core.COP0[14])
			break
		}
	}
	if err := emitter.Close(); err != nil {
		return err
	}
	if halt != nil {
		return halt
	}
	fmt.Fprintf(os.Stderr, "retired %d instructions; PC=%08X ISA=%v; unmapped accesses: %+v\n", retired, core.PC, core.ISA, busMap.UnmappedTotals())
	if *unmapped {
		for _, site := range busMap.Unmapped() {
			fmt.Fprintf(os.Stderr, "unmapped %+v\n", site)
		}
	}
	if *bootState {
		fmt.Fprintf(os.Stderr, "boot ready=%08X current=%08X handoff-gate=%08X image=%08X entry=%08X dram-main=%08X\n",
			busMap.Read(0x800083CC, bus.Word), busMap.Read(0x800083D0, bus.Word),
			busMap.Read(0x800050D0, bus.Word),
			busMap.Read(0xBFC20000, bus.Word), busMap.Read(0xBFC2002C, bus.Word),
			busMap.Read(0x800009F4, bus.Word))
		fmt.Fprintf(os.Stderr, "flash descriptor base=%08X data=%08X step=%08X interrupt pending=%08X enable=%08X\n",
			busMap.Read(0x800050B0, bus.Word), busMap.Read(0x800050B4, bus.Word),
			busMap.Read(0x800050BC, bus.Word), interrupts.Read(0x30, bus.Word), interrupts.Read(0x40, bus.Word))
	}
	if *probe != 0 {
		fmt.Fprintf(os.Stderr, "probe %08X=%08X\n", uint32(*probe), busMap.Read(uint32(*probe), bus.Word)) // #nosec G115 -- checked above.
	}
	if *tasks {
		if err := reportTasks(os.Stderr, ram); err != nil {
			return err
		}
	}
	if *csiWire {
		fmt.Fprintf(os.Stderr, "CSI transmitted % X\n", serial.Transmitted())
	}
	for _, hit := range hits {
		if *key >= 0 {
			fmt.Fprintf(os.Stderr, "pc-hit %08X total=%d before-key=%d after-key=%d\n", hit.address, hit.total, hit.before, hit.total-hit.before)
		} else {
			fmt.Fprintf(os.Stderr, "pc-hit %08X total=%d\n", hit.address, hit.total)
		}
	}
	if *surfaceHash {
		hash, distinct := surfaceDigest(ram, surfaceBase, surfaceLength)
		fmt.Fprintf(os.Stderr, "surface hash=%08X distinct=%d bytes=%d base=%08X\n", hash, distinct, surfaceLength, surfaceBase)
	}
	return nil
}
