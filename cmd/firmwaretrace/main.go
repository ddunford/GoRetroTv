// Command firmwaretrace runs the Go CPU from the verified reset image and emits checkpoints.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/csi"
	"github.com/ddunford/goretrotv/internal/device/demux"
	"github.com/ddunford/goretrotv/internal/firmware"
	"github.com/ddunford/goretrotv/internal/gdbstub"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
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
	var histogramRanges, readRanges, writeRanges instrumentRanges
	var histogramControls, callTargets pcHits
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
	ackOracle := flag.Bool("ack-oracle", false,
		"restrict the card to the two codes the RECORDED browser oracle answers, for checkpoint comparison")
	keyAt := flag.Uint64("key-at", 0, "instruction at which to queue the handset key")
	nvram := flag.String("nvram", "", "optional persistent 16 KiB EEPROM image path")
	snapshotIn := flag.String("snapshot-in", "", "restore a complete machine snapshot before the run")
	snapshotOut := flag.String("snapshot-out", "", "write a complete machine snapshot after the run")
	recordOut := flag.String("record-out", "", "record timed host inputs and final framebuffer digest")
	replayIn := flag.String("replay-in", "", "replay and verify a prior input recording")
	gdbAddr := flag.String("gdb-addr", "", "serve one GDB session on an explicit loopback TCP address")
	stateHash := flag.Bool("state-hash", false, "print the exact final CPU and DRAM state hash")
	pcHistogram := flag.Bool("pc-hist", false, "report hottest guest instruction PCs")
	flag.Var(&histogramRanges, "pc-range", "executed PC range lo:hi (repeatable, half-open)")
	flag.Var(&histogramControls, "pc-control", "required executed PC for histogram subject (repeatable)")
	flag.Var(&readRanges, "read-watch", "guest data read range lo:hi[:fromPC:toPC] (repeatable)")
	flag.Var(&writeRanges, "write-watch", "guest data write range lo:hi[:fromPC:toPC] (repeatable)")
	flag.Var(&callTargets, "call-trace", "exact guest PC to trace with pre-instruction arguments (repeatable)")
	ocodeTrace := flag.Bool("ocode-trace", false, "trace OpenTV CODE byte reads at the interpreter fetch PC")
	instrumentMax := flag.Int("instrument-max", 4000, "maximum detailed records per instrument")
	flag.Var(&hits, "pc-hit", "count guest executions of this PC (repeatable, hex or decimal)")
	flag.Parse()
	for _, spec := range histogramRanges {
		if spec.toPC != 0 {
			return fmt.Errorf("pc-range accepts only lo:hi")
		}
	}
	traceInstruments, err := newInstrumentRun(*pcHistogram, histogramRanges, readRanges, writeRanges,
		histogramControls, callTargets, *ocodeTrace, *instrumentMax)
	if err != nil {
		return err
	}
	instrumentActive := traceInstruments.requested()
	accessActive := traceInstruments.observeAccesses()
	if *recordOut != "" && *replayIn != "" {
		return fmt.Errorf("record-out and replay-in are mutually exclusive")
	}
	if *gdbAddr != "" && (*recordOut != "" || *replayIn != "" || *key >= 0 || *sectionHex != "" || len(sections) != 0 || instrumentActive) {
		return fmt.Errorf("gdb-addr cannot be combined with scheduled host inputs, recording or trace instruments")
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
	if *ackOracle && (*ackAll || *ackCode >= 0) {
		return fmt.Errorf("ack-oracle sets the whole policy and cannot be combined with ack-all or ack-code")
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
	runtime, err := board.New(images, *skyGates)
	if err != nil {
		return err
	}
	busMap, ram := runtime.Machine.Bus, runtime.RAM
	core, interrupts := runtime.Machine.Core, runtime.IRQ
	serial, master, sectionDemux := runtime.CSI, runtime.I2C, runtime.Demux
	switch {
	case *ackAll:
		serial.AckAll()
	case *ackOracle:
		// THE CARD THE RECORDED ORACLE STREAM WAS MADE WITH, and nothing more.
		//
		// The browser oracle answers card status and the heartbeat and stays silent on everything
		// else. This port used to do the same and no longer does: leaving those two unanswered
		// starves the one task that drains the event queue, and the box stops responding to the
		// handset after a few presses (gort-slq, gort-b9n). The fix is right and the oracle does
		// not have it, so from about twenty million instructions into a cold boot the two machines
		// are legitimately doing different things.
		//
		// A checkpoint comparison needs both sides in the same declared condition -- which is
		// exactly why the oracle comparison already runs WITHOUT -sky-gates. This is the same
		// move on the other axis, and it is NOT editing the oracle to agree: the oracle is
		// untouched and still the independent check on the CPU, the devices and the boot.
		serial.SetAckPolicy([]uint8{0x52, 0x18})
	case *ackCode >= 0:
		serial.SetAckPolicy(append(append([]uint8{}, csi.DefaultAckPolicy...), uint8(*ackCode))) // #nosec G115 -- checked above.
	}
	if *nvram != "" {
		if err := master.BindImage(*nvram); err != nil {
			return err
		}
	}
	var instruction uint64
	var demodReadTotal uint64
	demodReadCount := make(map[uint16]uint64)
	demodReadValue := make(map[uint16]uint8)
	if *demodPolls {
		runtime.Demod.SetReadObserver(func(register uint16, value uint8) {
			demodReadTotal++
			demodReadCount[register]++
			demodReadValue[register] = value
			if *demodTraceTo != 0 && instruction >= *demodTraceFrom && instruction < *demodTraceTo {
				fmt.Fprintf(os.Stderr, "demod-read-at instruction=%d register=%d value=%s\n", instruction, register, wireByte(value))
			}
		})
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
	emulated := runtime.Machine
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
	startRetired := retired
	nextSection := 0
	nextRecorded := 0
	if *watchWord != 0 {
		previousWord = busMap.Read(uint32(*watchWord), bus.Word)
	} // #nosec G115 -- checked above.
	advance := func(i uint64, observe func(bus.ObservedAccess)) error {
		if i != emulated.Retired {
			return fmt.Errorf("firmwaretrace: instruction %d differs from machine retired %d", i, emulated.Retired)
		}
		instruction = i
		var pc uint32
		var accessObserver func(bus.ObservedAccess)
		if accessActive || observe != nil {
			accessObserver = func(access bus.ObservedAccess) {
				if access.Fetch {
					return
				}
				if accessActive {
					traceInstruments.ObserveAccess(i, pc, access)
				}
				if observe != nil {
					observe(access)
				}
			}
		}
		err := runtime.StepWithHooks(board.StepHooks{
			AfterPump: func() error {
				if *trace && i >= *traceFrom && i < *traceTo {
					fmt.Fprintf(os.Stderr, "%d PC=%s ISA=%v Count=%s Status=%s GPR=%s\n", i, wireWord(core.PC), core.ISA, wireWord(core.COP0[9]), wireWord(core.COP0[12]), wireRegisters(core.GPR))
				}
				hits.Observe(core.PC)
				return core.ObserveCheckpoint(emitter, i)
			},
			AfterSky: func() error {
				if instrumentActive {
					traceInstruments.ObserveInstruction(i, core.PC, core.GPR)
				}
				pc = core.PC
				return nil
			},
			Access: accessObserver,
			Handoff: func(retired uint64, entryPC uint32) {
				fmt.Fprintf(os.Stderr, "declared host application handoff after %d guest instructions: PC=%s\n", retired, wireWord(entryPC))
			},
			SkyGate: func(retired uint64) {
				fmt.Fprintf(os.Stderr, "declared Sky menu gates applied after %d guest instructions\n", retired)
			},
		})
		if err != nil {
			return err
		}
		retired = emulated.Retired
		return nil
	}
	if *gdbAddr != "" {
		listener, err := gdbstub.Listen(*gdbAddr)
		if err != nil {
			return err
		}
		defer func() { _ = listener.Close() }()
		fmt.Fprintf(os.Stderr, "GDB listening on %s; retired=%d PC=%s\n", listener.Addr(), retired, wireWord(core.PC))
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
				fmt.Fprintf(os.Stderr, "scheduler %d PC=%s from=%s to=%s ready=%s\n", i, wireWord(core.PC), wireWord(previousTask), wireWord(current), wireWord(ram.Read(0x83cc, bus.Word)))
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
				fmt.Fprintf(os.Stderr, "section-sample at=%d enable=%s status=%s armed-pids=%v last22=%s last23=%s last24=%s demod-reads=%d\n",
					i, wireWord(sectionDemux.Read(0xD8, bus.Word)), wireWord(sectionDemux.Read(0xB8, bus.Word)),
					sectionDemux.ArmedPIDs(), wireWord(last(22)), wireWord(last(23)), wireWord(last(24)), demodReadTotal)
				for _, hit := range hits {
					fmt.Fprintf(os.Stderr, "section-sample-hit at=%d pc=%s total=%d\n", i, wireWord(hit.address), hit.total)
				}
			}
			section := sections[nextSection]
			if err := sectionDemux.Push(section.pid, section.bytes); err != nil {
				return fmt.Errorf("section delivery after %d guest instructions: %w", i, err)
			}
			fmt.Fprintf(os.Stderr, "section-inject icount=%d pid=%s bytes=%d\n", i, wireHalf(section.pid), len(section.bytes))
			nextSection++
		}
		if err := advance(i, nil); err != nil {
			halt = err
			break
		}
		if *watchWord != 0 {
			value := busMap.Read(uint32(*watchWord), bus.Word) // #nosec G115 -- checked above.
			if value != previousWord {
				fmt.Fprintf(os.Stderr, "watch %d PC=%s address=%s before=%s after=%s\n", i+1, wireWord(core.PC), wireWord(uint32(*watchWord)), wireWord(previousWord), wireWord(value)) // #nosec G115 -- checked above.
				previousWord = value
			}
		}
		// #nosec G115 -- stopPC was checked against the 32-bit address space.
		if *stopPC != 0 && core.PC == uint32(*stopPC) {
			fmt.Fprintf(os.Stderr, "reached PC=%s after %d guest instructions; Cause=%s EPC=%s\n", wireWord(core.PC), i+1, wireWord(core.COP0[13]), wireWord(core.COP0[14]))
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
	if len(hits) != 0 && retired == startRetired {
		return instrument.MustFind("PC hit trace", "guest instructions", 0, 1)
	}
	if err := traceInstruments.Report(os.Stderr); err != nil {
		return err
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
	fmt.Fprintf(os.Stderr, "retired %d instructions; PC=%s ISA=%v; unmapped accesses: %+v\n", retired, wireWord(core.PC), core.ISA, busMap.UnmappedTotals())
	if *unmapped {
		for _, site := range busMap.Unmapped() {
			fmt.Fprintf(os.Stderr, "unmapped %+v\n", site)
		}
	}
	if *bootState {
		fmt.Fprintf(os.Stderr, "boot ready=%s current=%s handoff-gate=%s image=%s entry=%s dram-main=%s\n",
			wireWord(busMap.Read(0x800083CC, bus.Word)), wireWord(busMap.Read(0x800083D0, bus.Word)),
			wireWord(busMap.Read(0x800050D0, bus.Word)),
			wireWord(busMap.Read(0xBFC20000, bus.Word)), wireWord(busMap.Read(0xBFC2002C, bus.Word)),
			wireWord(busMap.Read(0x800009F4, bus.Word)))
		fmt.Fprintf(os.Stderr, "flash descriptor base=%s data=%s step=%s interrupt pending=%s enable=%s\n",
			wireWord(busMap.Read(0x800050B0, bus.Word)), wireWord(busMap.Read(0x800050B4, bus.Word)),
			wireWord(busMap.Read(0x800050BC, bus.Word)), wireWord(interrupts.Read(0x30, bus.Word)), wireWord(interrupts.Read(0x40, bus.Word)))
	}
	if *probe != 0 {
		fmt.Fprintf(os.Stderr, "probe %s=%s\n", wireWord(uint32(*probe)), wireWord(busMap.Read(uint32(*probe), bus.Word))) // #nosec G115 -- checked above.
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
			fmt.Fprintf(os.Stderr, "demod-read register=%d count=%d value=%s\n", register, demodReadCount[uint16(register)], wireByte(demodReadValue[uint16(register)])) // #nosec G115 -- register came from uint16 map key.
		}
	}
	if *sectionState {
		fmt.Fprintf(os.Stderr, "section-state enable=%s status=%s armed-pids=%v\n",
			wireWord(sectionDemux.Read(0xD8, bus.Word)), wireWord(sectionDemux.Read(0xB8, bus.Word)), sectionDemux.ArmedPIDs())
		for unit := uint8(0); unit < 16; unit++ {
			table, _ := sectionDemux.Match(unit, 0)
			if table.Mask == 0 {
				continue
			}
			extHi, _ := sectionDemux.Match(unit, 1)
			extLo, _ := sectionDemux.Match(unit, 2)
			fmt.Fprintf(os.Stderr, "section-match unit=%d table=%s/%s extension=%s%s/%s%s\n",
				unit, wireByte(table.Value), wireByte(table.Mask), wireByte(extHi.Value), wireByte(extLo.Value), wireByte(extHi.Mask), wireByte(extLo.Mask))
		}
		for _, channel := range []uint8{21, 22, 23, 24} {
			record := demux.RecordBase(channel) & 0x1fffffff
			fmt.Fprintf(os.Stderr, "section-filter channel=%d ring=%s record-start=%s record-end=%s record-current=%s record-last=%s context=%s\n",
				channel, wireWord(demux.RingBase(channel)), wireWord(ram.Read(record, bus.Word)), wireWord(ram.Read(record+4, bus.Word)),
				wireWord(ram.Read(record+8, bus.Word)), wireWord(ram.Read(record+12, bus.Word)), wireWord(ram.Read(record+16, bus.Word)))
		}
	}
	for _, hit := range hits {
		if *key >= 0 {
			fmt.Fprintf(os.Stderr, "pc-hit %s total=%d before-key=%d after-key=%d\n", wireWord(hit.address), hit.total, hit.before, hit.total-hit.before)
		} else {
			fmt.Fprintf(os.Stderr, "pc-hit %s total=%d\n", wireWord(hit.address), hit.total)
		}
	}
	if *surfaceHash {
		hash, distinct, sha, err := surfaceDigest(ram, surfaceBase, surfaceLength)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "surface hash=%s distinct=%d bytes=%d base=%s sha256=%x\n", wireWord(hash), distinct, surfaceLength, wireWord(surfaceBase), sha)
	}
	if *stateHash {
		value := hasher.Hash(core.State())
		if err := hasher.Err(); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "state-hash retired=%d hash=%s\n", retired, wireWord(value))
	}
	if *snapshotOut != "" {
		if err := writeSnapshot(*snapshotOut, emulated); err != nil {
			return err
		}
	}
	return nil
}
