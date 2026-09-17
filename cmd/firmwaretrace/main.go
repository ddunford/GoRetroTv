// Command firmwaretrace runs the Go CPU from the verified reset image and emits checkpoints.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/cpu"
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
	"github.com/ddunford/goretrotv/internal/gdbstub"
	"github.com/ddunford/goretrotv/internal/machine"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/clock"
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
	var sections sectionInputs
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
	demodPolls := flag.Bool("demod-polls", false, "count guest I2C reads of the satellite demodulator")
	demodTraceFrom := flag.Uint64("demod-trace-from", 0, "first instruction included in demodulator read trace (requires -demod-polls)")
	demodTraceTo := flag.Uint64("demod-trace-to", 0, "first instruction excluded from demodulator read trace (requires -demod-polls)")
	sectionHex := flag.String("section-hex", "", "hexadecimal DVB section to deliver through an armed PID filter")
	sectionPID := flag.Uint("section-pid", 0, "PID for the section delivered by -section-hex")
	sectionAt := flag.Uint64("section-at", 0, "retired instruction count at which to deliver -section-hex")
	sectionState := flag.Bool("section-state", false, "print guest-programmed SI PID filters and section interrupt state")
	sectionSamples := flag.Bool("section-samples", false, "sample SI device and guest PC state before each scheduled section batch")
	flag.Var(&sections, "section", "additional DVB section as instruction:pid:hex (repeatable)")
	surfaceHash := flag.Bool("surface-hash", false, "print the raw 720x576 OSD RAM hash and distinct byte count")
	key := flag.Int("key", -1, "raw handset code to send on the CSI link (-1 disables)")
	ackCode := flag.Int("ack-code", -1, "additional CSI command code to acknowledge (-1 keeps measured default)")
	ackAll := flag.Bool("ack-all", false, "diagnostic policy: acknowledge every CSI command")
	keyAt := flag.Uint64("key-at", 0, "instruction at which to queue the handset key")
	nvram := flag.String("nvram", "", "optional persistent 16 KiB EEPROM image path")
	snapshotIn := flag.String("snapshot-in", "", "restore a complete machine snapshot before the run")
	snapshotOut := flag.String("snapshot-out", "", "write a complete machine snapshot after the run")
	recordOut := flag.String("record-out", "", "record timed host inputs and final framebuffer digest")
	replayIn := flag.String("replay-in", "", "replay and verify a prior input recording")
	gdbAddr := flag.String("gdb-addr", "", "serve one GDB session on an explicit loopback TCP address")
	stateHash := flag.Bool("state-hash", false, "print the exact final CPU and DRAM state hash")
	flag.Var(&hits, "pc-hit", "count guest executions of this PC (repeatable, hex or decimal)")
	flag.Parse()
	if *recordOut != "" && *replayIn != "" {
		return fmt.Errorf("record-out and replay-in are mutually exclusive")
	}
	if *gdbAddr != "" && (*recordOut != "" || *replayIn != "" || *key >= 0 || *sectionHex != "" || len(sections) != 0) {
		return fmt.Errorf("gdb-addr cannot be combined with scheduled host inputs or recording")
	}
	if *recordOut != "" || *replayIn != "" {
		if *ackAll || *ackCode >= 0 || *nvram != "" {
			return fmt.Errorf("recording and replay require the default CSI policy and snapshot-contained EEPROM")
		}
	}
	var recording inputRecording
	if *replayIn != "" {
		if *key >= 0 || *sectionHex != "" || len(sections) != 0 {
			return fmt.Errorf("replay-in cannot be combined with live key or section inputs")
		}
		var err error
		recording, err = readRecording(*replayIn)
		if err != nil {
			return fmt.Errorf("read input recording: %w", err)
		}
		*steps = recording.End
		*skyGates = recording.SkyGates
	}
	if *key < -1 || *key > 255 {
		return fmt.Errorf("raw handset key %d is outside 0..255", *key)
	}
	if *ackCode < -1 || *ackCode > 255 {
		return fmt.Errorf("CSI acknowledgement code %d is outside 0..255", *ackCode)
	}
	if (*demodTraceFrom != 0 || *demodTraceTo != 0) && (!*demodPolls || *demodTraceTo <= *demodTraceFrom) {
		return fmt.Errorf("demod read trace requires -demod-polls and a nonempty instruction range")
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
	if (*sectionHex == "") != (*sectionAt == 0) {
		return fmt.Errorf("section-hex and nonzero section-at must be supplied together")
	}
	if *sectionPID > 0x1fff {
		return fmt.Errorf("section PID %#x exceeds DVB range", *sectionPID)
	}
	if *sectionHex != "" {
		if err := sections.Set(fmt.Sprintf("%d:%d:%s", *sectionAt, *sectionPID, *sectionHex)); err != nil {
			return err
		}
	}
	sections.sortByTime()
	for _, section := range sections {
		if section.at >= *steps {
			return fmt.Errorf("section instruction %d must precede steps %d", section.at, *steps)
		}
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
	if err := busMap.Attach(boardlatch.Base, boardlatch.Size, boardlatch.New()); err != nil {
		return err
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
	demodModel := demod.New()
	var instruction uint64
	var demodReadTotal uint64
	demodReadCount := make(map[uint16]uint64)
	demodReadValue := make(map[uint16]uint8)
	if *demodPolls {
		demodModel.SetReadObserver(func(register uint16, value uint8) {
			demodReadTotal++
			demodReadCount[register]++
			demodReadValue[register] = value
			if *demodTraceTo != 0 && instruction >= *demodTraceFrom && instruction < *demodTraceTo {
				fmt.Fprintf(os.Stderr, "demod-read-at instruction=%d register=%d value=%02X\n", instruction, register, value)
			}
		})
	}
	master.BindDemod(demodModel)
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
	// The oracle pumps once per loop iteration. Its MIPS32 branch and slot share one
	// iteration, while an accepted interrupt adds an iteration without retiring a
	// guest instruction. Keep this clock separate from the retired count below.
	loopClock := clock.New()
	const pumpName = "board-pump"
	var boardPump clock.Handler
	boardPump = func(now uint64) error {
		boardTimer.Pump(now + 15)
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
		_, err = loopClock.At(now+16, pumpName, boardPump)
		return err
	}
	if _, err := loopClock.At(1, pumpName, boardPump); err != nil {
		return err
	}
	emulated, err := machine.New(core, busMap, loopClock, &handoff, skyMenu,
		map[string]clock.Handler{pumpName: boardPump})
	if err != nil {
		return err
	}
	pumpBoard := func() error { return loopClock.Advance(1) }
	if *snapshotIn != "" {
		if err := loadSnapshot(*snapshotIn, emulated); err != nil {
			return err
		}
		hasher.Rebuild()
		retired = emulated.Retired
		instruction = retired
	}
	var recordedEvents []recordedInput
	if *recordOut != "" || *replayIn != "" {
		snapshotDigest, err := fileSHA256(*snapshotIn)
		if err != nil {
			return fmt.Errorf("hash initial snapshot: %w", err)
		}
		if *replayIn != "" {
			if recording.SnapshotSHA256 != snapshotDigest || recording.Start != retired {
				return fmt.Errorf("recording initial snapshot or instruction count differs from this machine")
			}
			recordedEvents = recording.Events
		} else {
			recording = inputRecording{Version: 1, SnapshotSHA256: snapshotDigest, Start: retired, End: *steps, SkyGates: *skyGates}
			if *key >= 0 {
				recordedEvents = append(recordedEvents, recordedInput{At: *keyAt, Kind: "key", Key: uint8(*key)}) // #nosec G115 -- validated above.
			}
			for _, section := range sections {
				recordedEvents = append(recordedEvents, recordedInput{At: section.at, Kind: "section", PID: section.pid, Section: hex.EncodeToString(section.bytes)})
			}
			sort.SliceStable(recordedEvents, func(i, j int) bool { return recordedEvents[i].At < recordedEvents[j].At })
			recording.Events = recordedEvents
		}
	}
	if len(sections) != 0 && sections[0].at < retired {
		return fmt.Errorf("section instruction %d precedes restored instruction count %d", sections[0].at, retired)
	}
	nextSection := 0
	nextRecorded := 0
	if *watchWord != 0 {
		previousWord = busMap.Read(uint32(*watchWord), bus.Word)
	} // #nosec G115 -- checked above.
	advance := func(i uint64, observe func(bus.ObservedAccess)) error {
		instruction = i
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
		if observe != nil {
			busMap.SetObserver(observe)
		}
		interruptBoundary := pumpBoard
		if observe != nil {
			interruptBoundary = func() error {
				busMap.SetObserver(nil)
				err := pumpBoard()
				busMap.SetObserver(observe)
				return err
			}
		}
		stepErr := core.StepWithInterruptBoundary(interruptBoundary)
		if observe != nil {
			busMap.SetObserver(nil)
		}
		if err := stepErr; err != nil {
			return fmt.Errorf("after %d instructions: %w", i, err)
		}
		retired = i + 1
		emulated.Retired = retired
		if err := dmaController.Fault(); err != nil {
			return fmt.Errorf("after %d instructions: %w", i, err)
		}
		if err := master.Fault(); err != nil {
			return fmt.Errorf("after %d instructions: %w", i, err)
		}
		return nil
	}
	if *gdbAddr != "" {
		listener, err := gdbstub.Listen(*gdbAddr)
		if err != nil {
			return err
		}
		defer func() { _ = listener.Close() }()
		fmt.Fprintf(os.Stderr, "GDB listening on %s; retired=%d PC=%08X\n", listener.Addr(), retired, core.PC)
		backend := &firmwareDebugBackend{core: core, bus: busMap, retired: &retired, advance: advance}
		if err := gdbstub.New(backend).ServeListener(listener); err != nil {
			return err
		}
		*steps = retired
	}
	for i := retired; i < *steps; i++ {
		instruction = i
		if *scheduler {
			current := ram.Read(0x83d0, bus.Word)
			if current != previousTask {
				fmt.Fprintf(os.Stderr, "scheduler %d PC=%08X from=%08X to=%08X ready=%08X\n", i, core.PC, previousTask, current, ram.Read(0x83cc, bus.Word))
				previousTask = current
			}
		}
		if *recordOut == "" && *replayIn == "" && *key >= 0 && i == *keyAt {
			hits.MarkKey()
			// #nosec G115 -- raw key was checked to be within 0..255 above.
			if err := serial.Key(uint8(*key), 0); err != nil {
				return err
			}
		}
		for nextRecorded < len(recordedEvents) && i == recordedEvents[nextRecorded].At {
			event := recordedEvents[nextRecorded]
			switch event.Kind {
			case "key":
				hits.MarkKey()
				if err := serial.Key(event.Key, 0); err != nil {
					return err
				}
			case "section":
				bytes, err := hex.DecodeString(event.Section)
				if err != nil {
					return err
				}
				if err := sectionDemux.Push(event.PID, bytes); err != nil {
					return fmt.Errorf("recorded section after %d guest instructions: %w", i, err)
				}
			}
			nextRecorded++
		}
		for *recordOut == "" && *replayIn == "" && nextSection < len(sections) && i == sections[nextSection].at {
			if *sectionSamples && (nextSection == 0 || sections[nextSection-1].at != i) {
				last := func(filter uint8) uint32 {
					return ram.Read((demux.RecordBase(filter)&0x1fffffff)+12, bus.Word)
				}
				fmt.Fprintf(os.Stderr, "section-sample at=%d enable=%08X status=%08X armed-pids=%v last22=%08X last23=%08X last24=%08X demod-reads=%d\n",
					i, sectionDemux.Read(0xD8, bus.Word), sectionDemux.Read(0xB8, bus.Word),
					sectionDemux.ArmedPIDs(), last(22), last(23), last(24), demodReadTotal)
				for _, hit := range hits {
					fmt.Fprintf(os.Stderr, "section-sample-hit at=%d pc=%08X total=%d\n", i, hit.address, hit.total)
				}
			}
			section := sections[nextSection]
			if err := sectionDemux.Push(section.pid, section.bytes); err != nil {
				return fmt.Errorf("section delivery after %d guest instructions: %w", i, err)
			}
			fmt.Fprintf(os.Stderr, "section-inject icount=%d pid=%04X bytes=%d\n", i, section.pid, len(section.bytes))
			nextSection++
		}
		if err := advance(i, nil); err != nil {
			halt = err
			break
		}
		if *watchWord != 0 {
			value := busMap.Read(uint32(*watchWord), bus.Word) // #nosec G115 -- checked above.
			if value != previousWord {
				fmt.Fprintf(os.Stderr, "watch %d PC=%08X address=%08X before=%08X after=%08X\n", i+1, core.PC, uint32(*watchWord), previousWord, value) // #nosec G115 -- checked above.
				previousWord = value
			}
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
	if nextSection != len(sections) {
		return fmt.Errorf("section delivery was not reached before run stopped at %d instructions", retired)
	}
	if nextRecorded != len(recordedEvents) {
		return fmt.Errorf("recorded input delivery was not reached before run stopped at %d instructions", retired)
	}
	if *recordOut != "" || *replayIn != "" {
		_, _, surfaceSHA, err := surfaceDigest(ram, surfaceBase, surfaceLength)
		if err != nil {
			return err
		}
		if *replayIn != "" {
			if retired != recording.End || hex.EncodeToString(surfaceSHA[:]) != recording.SurfaceSHA256 {
				return fmt.Errorf("replay diverged: retired=%d surface=%x; recorded retired=%d surface=%s", retired, surfaceSHA, recording.End, recording.SurfaceSHA256)
			}
			fmt.Fprintf(os.Stderr, "replay verified retired=%d surface-sha256=%x\n", retired, surfaceSHA)
		} else {
			recording.End = retired
			recording.SurfaceSHA256 = hex.EncodeToString(surfaceSHA[:])
			if err := writeRecording(*recordOut, recording); err != nil {
				return fmt.Errorf("write input recording: %w", err)
			}
			fmt.Fprintf(os.Stderr, "recorded inputs retired=%d surface-sha256=%x\n", retired, surfaceSHA)
		}
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
	if *demodPolls {
		fmt.Fprintf(os.Stderr, "demod-read total=%d\n", demodReadTotal)
		registers := make([]int, 0, len(demodReadCount))
		for register := range demodReadCount {
			registers = append(registers, int(register))
		}
		sort.Ints(registers)
		for _, register := range registers {
			fmt.Fprintf(os.Stderr, "demod-read register=%d count=%d value=%02X\n", register, demodReadCount[uint16(register)], demodReadValue[uint16(register)]) // #nosec G115 -- register came from uint16 map key.
		}
	}
	if *sectionState {
		fmt.Fprintf(os.Stderr, "section-state enable=%08X status=%08X armed-pids=%v\n",
			sectionDemux.Read(0xD8, bus.Word), sectionDemux.Read(0xB8, bus.Word), sectionDemux.ArmedPIDs())
		for unit := uint8(0); unit < 16; unit++ {
			table, _ := sectionDemux.Match(unit, 0)
			if table.Mask == 0 {
				continue
			}
			extHi, _ := sectionDemux.Match(unit, 1)
			extLo, _ := sectionDemux.Match(unit, 2)
			fmt.Fprintf(os.Stderr, "section-match unit=%d table=%02X/%02X extension=%02X%02X/%02X%02X\n",
				unit, table.Value, table.Mask, extHi.Value, extLo.Value, extHi.Mask, extLo.Mask)
		}
		for _, channel := range []uint8{21, 22, 23, 24} {
			record := demux.RecordBase(channel) & 0x1fffffff
			fmt.Fprintf(os.Stderr, "section-filter channel=%d ring=%08X record-start=%08X record-end=%08X record-current=%08X record-last=%08X context=%08X\n",
				channel, demux.RingBase(channel), ram.Read(record, bus.Word), ram.Read(record+4, bus.Word),
				ram.Read(record+8, bus.Word), ram.Read(record+12, bus.Word), ram.Read(record+16, bus.Word))
		}
	}
	for _, hit := range hits {
		if *key >= 0 {
			fmt.Fprintf(os.Stderr, "pc-hit %08X total=%d before-key=%d after-key=%d\n", hit.address, hit.total, hit.before, hit.total-hit.before)
		} else {
			fmt.Fprintf(os.Stderr, "pc-hit %08X total=%d\n", hit.address, hit.total)
		}
	}
	if *surfaceHash {
		hash, distinct, sha, err := surfaceDigest(ram, surfaceBase, surfaceLength)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "surface hash=%08X distinct=%d bytes=%d base=%08X sha256=%x\n", hash, distinct, surfaceLength, surfaceBase, sha)
	}
	if *stateHash {
		value := hasher.Hash(core.State())
		if err := hasher.Err(); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "state-hash retired=%d hash=%08X\n", retired, value)
	}
	if *snapshotOut != "" {
		if err := writeSnapshot(*snapshotOut, emulated); err != nil {
			return err
		}
	}
	return nil
}
