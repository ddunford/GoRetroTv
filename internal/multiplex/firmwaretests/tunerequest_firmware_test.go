package firmwaretests_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/demux"
	"github.com/ddunford/goretrotv/internal/firmware"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

func TestWhereTheRunningFirmwareStoresDemodulatorEntries(t *testing.T) {
	box := restoredBox(t)
	highHandle := box.RAM.Read(0x00105C0C, bus.Word)
	physicalHandle := box.RAM.Read(0x00105C44, bus.Word)
	t.Logf("demodulator registration handles: high=%08X physical=%08X", highHandle, physicalHandle)
	if highHandle != 0xFFFFFFFF {
		t.Fatalf("higher demodulator table handle = %08X, want the measured unregistered marker FFFFFFFF", highHandle)
	}
	if physicalHandle != 0x14 {
		t.Fatalf("physical demodulator handle = %08X, want registered device 14", physicalHandle)
	}
	for _, target := range []uint32{0x8002FEAD, 0x8002FF4D, 0x80032D6D, 0x80032E35} {
		var refs []uint32
		for off := uint32(0); off+4 <= 0x00400000; off += 4 {
			if box.RAM.Read(off, bus.Word) == target {
				refs = append(refs, 0x80000000+off)
			}
		}
		if len(refs) == 0 {
			t.Fatalf("running firmware contains no pointer to demodulator entry %08X", target)
		}
		t.Logf("demodulator entry %08X stored at %08X", target, refs)
	}
}

// TestTraceServiceSelectionToTuneRequest observes the application boundary between selecting a
// service and displaying its no-signal message. It does not manufacture a service request: every
// record below is an executed guest call, and the type-13 constructor is the live control proving
// that the selected-service route ran even if the fixed-PSI resolver did not.
func TestTraceServiceSelectionToTuneRequest(t *testing.T) {
	type call struct {
		pc, ra, a0, a1, a2, a3 uint32
		ocode                  uint32
		request                [4]uint32
	}
	type mediaCall struct {
		call
		object  [20]uint32
		manager [16]uint32
	}
	guide := demoGuide(t)
	box := restoredBox(t)
	// Pin the live registered descriptor which makes the driver's indirect callback observable.
	// Static literal-pool xrefs cannot find this relocated registration.
	const demodulatorDescriptor = 0x002C1698
	for off, want := range map[uint32]uint32{
		0x00: 0x00000014, 0x04: 0x9FC24008, 0x08: 0x00010000, 0x0C: 0x800FD610,
		0x10: 0x00000004, 0x14: 0x80032D6D, 0x18: 0x80032E35, 0x1C: 0x80170E20,
	} {
		if got := box.RAM.Read(demodulatorDescriptor+off, bus.Word); got != want {
			t.Fatalf("demodulator descriptor +%02X = %08X, want %08X", off, got, want)
		}
	}
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, demoDictionary(t), multiplex.FixedClock{At: day}, demoScheduleWithEvents())
	if err != nil {
		t.Fatal(err)
	}
	flash, err := os.ReadFile(filepath.Join("..", "..", "..", "firmware", firmware.FileU202))
	if err != nil {
		t.Skipf("the flash image is not installed: %v", err)
	}
	word := func(virtual uint32) uint32 {
		if virtual >= 0x9FC00000 && virtual < 0xA0000000 {
			off := int(virtual - 0x9FC00000)
			if off < 0 || off+4 > len(flash) {
				t.Fatalf("harness: flash address %08X is outside the image", virtual)
			}
			return binary.BigEndian.Uint32(flash[off:])
		}
		return box.RAM.Read(virtual&0x1fffffff, bus.Word)
	}
	// Derive every native shim from the running dispatch table. This is deliberately not limited
	// to module 1: service selection is implemented primarily by module 7.
	type native struct{ module, fn int }
	natives := map[uint32][]native{}
	moduleTable := word(0x8006E71C)
	totalNatives := 0
	for module := 0; module < 64; module++ {
		array := word(moduleTable + uint32(module)*8) // #nosec G115 -- module is bounded above
		count := word(moduleTable + uint32(module)*8 + 4)
		if array == 0 || count == 0 {
			continue
		}
		if count > 512 {
			t.Fatalf("harness: module %d claims implausible native count %d", module, count)
		}
		for fn := uint32(0); fn < count; fn++ {
			record := word(array + fn*4)
			shim := word(record) &^ 1
			natives[shim] = append(natives[shim], native{module: module, fn: int(fn)})
			totalNatives++
		}
	}
	if totalNatives != 800 {
		t.Fatalf("harness: runtime native table yielded %d entries, want measured control 800", totalNatives)
	}

	active := false
	noSignalDecision := false
	componentRebuilt := false
	var calls []call
	var serviceResolveResults []uint32
	var serviceLookupResults []uint32
	var servicePropertyResults []uint32
	type nativeResult struct{ ocode, value uint32 }
	var serviceEventResults []nativeResult
	type byteRead struct{ address, value uint32 }
	type ocodeRead struct{ ocode, address, value uint32 }
	type guestRead struct{ pc, address, value uint32 }
	var typedRequestDiscriminatorReads []byteRead
	var typedRequestDiscriminatorWrites []call
	var typedRequestSourceReads []guestRead
	typedRequestSourceWrites := make(map[uint32][]call)
	var latestOCode uint32
	var serviceEventFieldReads []byteRead
	var selectionDecisionReads []ocodeRead
	var serviceLookupReads []guestRead
	var audioInterfaceOwnerReads []guestRead
	var audioInterfaceReads []guestRead
	var frontendTaskStates []uint32
	var frontendDispatchCalls []call
	var registeredDeviceCalls []call
	var allRegisteredDeviceCalls []call
	var allDemodulatorEntryCalls []call
	var allAudioAPICalls []call
	var mediaCalls []mediaCall
	nativeCalls := map[native]int{}
	nativeFirst := map[native]call{}
	ocodeHits := map[uint32]int{}
	hits := map[uint32]int{}
	hooks := board.StepHooks{
		AfterPump: func() error {
			state := box.Machine.Core.State()
			pc := state.PC &^ 1
			if pc == 0x800A127C {
				componentRebuilt = true
			}
			if pc == 0x8007E6D8 {
				c := call{pc: pc,
					ra: state.GPR[31] &^ 1, a0: state.GPR[4], a1: state.GPR[5],
					a2: state.GPR[6], a3: state.GPR[7]}
				allRegisteredDeviceCalls = append(allRegisteredDeviceCalls, c)
			}
			if pc == 0x8002FEAC || pc == 0x8002FF4C || pc == 0x80032D6C || pc == 0x80032E34 {
				c := call{pc: pc,
					ra: state.GPR[31] &^ 1, a0: state.GPR[4], a1: state.GPR[5],
					a2: state.GPR[6], a3: state.GPR[7]}
				allDemodulatorEntryCalls = append(allDemodulatorEntryCalls, c)
			}
			if pc == 0x80088970 || pc == 0x800889BC || pc == 0x80088A08 {
				allAudioAPICalls = append(allAudioAPICalls, call{pc: pc,
					ra: state.GPR[31] &^ 1, a0: state.GPR[4], a1: state.GPR[5],
					a2: state.GPR[6], a3: state.GPR[7], ocode: latestOCode})
			}
			if pc == 0x8009D370 || pc == 0x8001C908 || pc == 0x8001CF8C ||
				pc == 0x800ABB2C || pc == 0x800AFD74 || pc == 0x800AFF1C ||
				pc == 0x8009E140 || pc == 0x8009E250 || pc == 0x8009E290 ||
				pc == 0x8009E47C || pc == 0x800A127C || pc == 0x800A1A48 || pc == 0x8009E764 ||
				pc == 0x8009F4EC || pc == 0x8009F878 || pc == 0x800A03B0 || pc == 0x800A03D0 ||
				pc == 0x800DA436 || pc == 0x800DA452 || pc == 0x800D5C8C || pc == 0x800D5D2C ||
				pc == 0x800DB9CA || pc == 0x800DBF34 || pc == 0x800E8DE8 || pc == 0x800DC014 {
				c := mediaCall{call: call{pc: pc,
					ra: state.GPR[31] &^ 1, a0: state.GPR[4], a1: state.GPR[5],
					a2: state.GPR[6], a3: state.GPR[7], ocode: latestOCode}}
				object := state.GPR[4]
				if pc == 0x8009D370 || pc == 0x8001C908 || pc == 0x8001CF8C ||
					pc == 0x8009F4EC || pc == 0x8009F878 || pc == 0x800D5C8C ||
					pc == 0x800D5D2C || pc == 0x800DB9CA {
					// This entry receives a handle. The record itself is materialised in s0
					// immediately before the adapter call, so there is no object to snapshot yet.
					object = 0
				}
				if object >= 0x80000000 && object+uint32(len(c.object))*4 <= 0x80800000 {
					for i := range c.object {
						c.object[i] = box.RAM.Read((object&0x1fffffff)+uint32(i)*4, bus.Word) // #nosec G115 -- fixed twenty-word observation
					}
				}
				if pc == 0x800A03D0 && object >= 0x80000000 && object+0x120+uint32(len(c.manager))*4 <= 0x80800000 {
					for i := range c.manager {
						c.manager[i] = box.RAM.Read((object&0x1fffffff)+0x120+uint32(i)*4, bus.Word) // #nosec G115 -- fixed manager window
					}
				}
				mediaCalls = append(mediaCalls, c)
			}
			if !active {
				return nil
			}
			if pc == 0x800285B8 {
				frontendTaskStates = append(frontendTaskStates,
					box.RAM.Read(0x000FCDC4, bus.Word), state.GPR[2])
			}
			// This is the registered front-end driver's control entry. Operation 12 dispatches
			// to the normal tune-request constructor at 0x8002F1DC; recording every operation
			// here distinguishes a cold driver interface from a live interface lacking a tune.
			if pc == 0x80032D6C {
				frontendDispatchCalls = append(frontendDispatchCalls, call{pc: pc,
					ra: state.GPR[31] &^ 1, a0: state.GPR[4], a1: state.GPR[5],
					a2: state.GPR[6], a3: state.GPR[7]})
			}
			if pc == 0x8007E6D8 {
				registeredDeviceCalls = append(registeredDeviceCalls, call{pc: pc,
					ra: state.GPR[31] &^ 1, a0: state.GPR[4], a1: state.GPR[5],
					a2: state.GPR[6], a3: state.GPR[7]})
			}
			// Native (7,04)'s shim returns here immediately after its implementation. The o-code
			// caller branches on this value before it can reach the typed-request method.
			if pc == 0x800A2796 {
				serviceResolveResults = append(serviceResolveResults, state.GPR[2])
			}
			// Native (7,17) returns here. Its o-code caller only continues to (7,18) when
			// this status is 3 or 4, making the return the post-EIT service-description gate.
			if pc == 0x800A2C86 {
				serviceLookupResults = append(serviceLookupResults, state.GPR[2])
			}
			if pc == 0x800A2C9E {
				servicePropertyResults = append(servicePropertyResults, state.GPR[2])
			}
			if pc == 0x800A281E {
				serviceEventResults = append(serviceEventResults, nativeResult{latestOCode, state.GPR[2]})
			}
			switch pc {
			case 0x800AD928, // API-object resolver: kind 0 starts PAT, kind 3 starts CAT
				0x800284D8,                         // front-end task request enqueue
				0x8002F81C,                         // normal tune-request builder
				0x800F8690,                         // alternate request builder
				0x800A283C,                         // native wrapper entering the service-state dispatcher
				0x800A280C,                         // native (7,0D), service event classification
				0x800AE3A8,                         // service-state dispatcher
				0x800AF32C,                         // service transition
				0x800AF49C,                         // type-13 viewed-service object constructor (live control)
				0x80032D0C,                         // front-end command
				0x800B0F2C, 0x800B0F8C, 0x800B1064, // PAT, PMT and CAT subscribers
				0x8001C604, 0x8001C6C8, // audio-specific path
				0x800D48D0, 0x800D4A0C: // lower-level calls reached from it
				c := call{pc: pc, ra: state.GPR[31] &^ 1, a0: state.GPR[4], a1: state.GPR[5], a2: state.GPR[6], a3: state.GPR[7]}
				if pc == 0x800AD928 && c.a2 >= 0x80000000 && c.a2 < 0x80800000 {
					base := c.a2 & 0x1fffffff
					for i := range c.request {
						c.request[i] = box.RAM.Read(base+uint32(i)*4, bus.Word) // #nosec G115 -- four fixed words
					}
				}
				calls = append(calls, c)
				hits[pc]++
			}
			return nil
		},
		Access: func(a bus.ObservedAccess) {
			pc := box.Machine.Core.State().PC &^ 1
			physical := a.Virtual & 0x1fffffff
			if active && a.Write && a.Virtual >= 0x80400000 && a.Virtual < 0x80500000 {
				typedRequestSourceWrites[a.Virtual] = append(typedRequestSourceWrites[a.Virtual],
					call{pc: pc, ra: box.Machine.Core.State().GPR[31] &^ 1, a0: a.Value, ocode: latestOCode})
			}
			if active && !a.Fetch && !a.Write && latestOCode == 0x9FC6A5CB &&
				pc >= 0x8006B3D8 && pc < 0x8006B478 {
				typedRequestSourceReads = append(typedRequestSourceReads,
					guestRead{pc: pc, address: a.Virtual, value: a.Value})
			}
			if active && !a.Fetch && !a.Write &&
				(physical == 0x000FC358 || physical == 0x003138FC) {
				audioInterfaceOwnerReads = append(audioInterfaceOwnerReads,
					guestRead{pc: pc, address: a.Virtual, value: a.Value})
			}
			if active && !a.Fetch && !a.Write &&
				(physical == 0x00105A8C || physical == 0x00105A90) {
				audioInterfaceReads = append(audioInterfaceReads,
					guestRead{pc: pc, address: a.Virtual, value: a.Value})
			}
			if active && !a.Fetch && !a.Write && pc >= 0x800A4040 && pc < 0x800A418C {
				serviceLookupReads = append(serviceLookupReads, guestRead{pc: pc, address: a.Virtual, value: a.Value})
			}
			if active && !a.Fetch && !a.Write && a.Virtual >= 0x80400000 && a.Virtual < 0x80500000 &&
				(latestOCode == 0x9FC6ACC9 || latestOCode == 0x9FC6AE50 || latestOCode == 0x9FC6AF52) {
				selectionDecisionReads = append(selectionDecisionReads, ocodeRead{ocode: latestOCode, address: a.Virtual, value: a.Value})
			}
			if active && !a.Fetch && !a.Write && a.Virtual >= 0x80400000 && a.Virtual < 0x80500000 &&
				(latestOCode == 0x9FC9DC79 || latestOCode == 0x9FC9DCAE) {
				serviceEventFieldReads = append(serviceEventFieldReads, byteRead{address: a.Virtual, value: a.Value})
			}
			if active && a.Write && a.Virtual == 0x80430730 {
				state := box.Machine.Core.State()
				typedRequestDiscriminatorWrites = append(typedRequestDiscriminatorWrites, call{pc: state.PC &^ 1,
					ra: state.GPR[31] &^ 1, a0: a.Value, ocode: latestOCode})
			}
			if active && !a.Fetch && !a.Write && (box.Machine.Core.State().PC&^1) == 0x8006D91A &&
				latestOCode == 0x9FC6A9F5 {
				typedRequestDiscriminatorReads = append(typedRequestDiscriminatorReads, byteRead{address: a.Virtual, value: a.Value})
			}
			if active && a.Fetch {
				if ids := natives[a.Virtual&^1]; len(ids) != 0 {
					for _, id := range ids {
						nativeCalls[id]++
						if _, seen := nativeFirst[id]; !seen {
							state := box.Machine.Core.State()
							nativeFirst[id] = call{pc: state.PC &^ 1, ra: state.GPR[31] &^ 1,
								a0: state.GPR[4], a1: state.GPR[5], a2: state.GPR[6], a3: state.GPR[7], ocode: latestOCode}
						}
					}
				}
			}
			if !active || a.Fetch || a.Write || a.Virtual < 0x9FC00000 {
				return
			}
			if box.Machine.Core.State().PC&^1 == 0x80069298 {
				latestOCode = 0x9FC00000 | (a.Virtual & 0x00ffffff)
				ocodeHits[latestOCode]++
			}
			// The o-code interpreter fetches the instruction at this MIPS site. Reaching the
			// already-proven status decision gives this trace an event boundary of its own.
			if box.Machine.Core.State().PC&^1 == 0x80069298 &&
				(0x9FC00000|(a.Virtual&0x00ffffff)) == 0x9FC6ACC9 {
				noSignalDecision = true
			}
		},
	}
	want, registered := programmesInTheBlock(t, guide, day), 0
	if at := runUntilHooked(t, box, transmitter, hooks, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, label string, budget int) uint32 {
		return pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
	}
	openAllChannels(t, press, ".artifacts/tune-request-grid.png", true)
	active = true
	press(keySelect, "select service", 80_000_000)
	// The PAT is sent on the line-up cadence; after the guest parses it and opens the advertised
	// PMT PID, one more cadence is required for the PMT to arrive. Observe longer than that full
	// handshake rather than declaring the component path cold halfway through it.
	if at := runUntilHooked(t, box, transmitter, hooks, 90_000_000,
		func(int) bool { return componentRebuilt }); at < 0 {
		t.Log("component rebuild did not execute within the post-tune observation window")
	} else {
		// The rebuild publishes component records; give their consumers time to make the stream
		// decision before taking the census.
		runUntilHooked(t, box, transmitter, hooks, 30_000_000, func(int) bool { return false })
	}
	postTuneSubscription, err := multiplex.Read(box.Demux)
	if err != nil {
		t.Fatalf("read post-tune subscription: %v", err)
	}
	t.Logf("post-tune armed filters=%#v subscription=%#v", box.Demux.ArmedFilters(), postTuneSubscription)
	for unit := uint8(0); unit < 16; unit++ {
		var matches [10]demux.MatchByte
		for index := uint8(0); index < 10; index++ {
			matches[index], _ = box.Demux.Match(unit, index)
		}
		t.Logf("post-tune match unit %d routes-PMT=%v: %#v", unit, box.Demux.Routes(unit, 15), matches)
	}
	active = false

	if !noSignalDecision {
		t.Fatal("harness: service selection never reached the measured no-signal decision")
	}
	if hits[0x800AF49C] == 0 {
		t.Fatal("harness: service selection did not construct its type-13 viewed-service object")
	}
	if len(nativeCalls) == 0 {
		t.Fatal("harness: selection executed no native shims from the 800-entry runtime table")
	}
	var selectedNatives []native
	for n := range nativeCalls {
		if n.module != 1 { // module 1 is graphics/event plumbing and overwhelms the service calls
			selectedNatives = append(selectedNatives, n)
		}
	}
	sort.Slice(selectedNatives, func(i, j int) bool {
		if selectedNatives[i].module != selectedNatives[j].module {
			return selectedNatives[i].module < selectedNatives[j].module
		}
		return selectedNatives[i].fn < selectedNatives[j].fn
	})
	for _, n := range selectedNatives {
		c := nativeFirst[n]
		t.Logf("native module=%d function=%#02x hits=%d pc=%08X ra=%08X ocode=%08X args=%08X,%08X,%08X,%08X",
			n.module, n.fn, nativeCalls[n], c.pc, c.ra, c.ocode, c.a0, c.a1, c.a2, c.a3)
	}
	for _, op := range []uint32{
		0x9FC6A73C, // native (7,07): create viewed-service object
		0x9FC6A8A9, // native (7,08): create a typed DVB request (cold)
		0x9FC6A8B1, // selected-service event handler
		0x9FC6A957, // native (7,04): resolve viewed-service object
		0x9FC9E10C, // native (7,17): look up the resolved service tuple
		0x9FC9E132, // native (7,18): query the resulting service record
		0x9FC6A9FE, // first direct caller of the typed-request method (record byte +0x88 == 5)
		0x9FC6B5CD, // second direct caller of the typed-request method (record byte +0x88 == 5)
		0x9FC5936F, // database lookup preceding the generic 0x0107 error producer
		0x9FC593A4, // second database lookup preceding the generic 0x0107 error producer
		0x9FC593B9, // pushes error/event value 0x0107 (cold on the selected-service route)
	} {
		t.Logf("o-code %08X hits=%d", op, ocodeHits[op])
	}
	if ocodeHits[0x9FC6A73C] == 0 || ocodeHits[0x9FC6A8B1] == 0 || ocodeHits[0x9FC6A957] == 0 {
		t.Fatal("harness: the measured create, selected-service handler and resolve route did not all execute")
	}
	if nativeCalls[native{module: 7, fn: 0x08}] != 0 || ocodeHits[0x9FC6A8A9] != 0 {
		t.Fatal("the typed DVB-request native unexpectedly executed; inspect its request before retaining this assertion")
	}
	if len(serviceResolveResults) == 0 {
		t.Fatal("harness: native (7,04) returned no observable result")
	}
	pending, resolved := false, false
	for _, status := range serviceResolveResults {
		pending = pending || status == 2
		resolved = resolved || status == 3 || status == 4
	}
	if !pending || !resolved {
		t.Fatalf("present/following EIT did not advance the viewed-service resolver from pending to resolved: %#v",
			serviceResolveResults)
	}
	t.Logf("native module=7 function=04 return values=%#v", serviceResolveResults)
	if len(serviceLookupResults) == 0 {
		t.Fatal("harness: native (7,17) returned no observable result")
	}
	t.Logf("native module=7 function=17 return values=%#v", serviceLookupResults)
	t.Logf("native module=7 function=18 return values=%#v", servicePropertyResults)
	t.Logf("native module=7 function=0D return values=%#v", serviceEventResults)
	t.Logf("service event field reads=%#v", serviceEventFieldReads)
	t.Logf("selection decision reads=%#v", selectionDecisionReads)
	t.Logf("service lookup reads=%#v", serviceLookupReads)
	t.Logf("audio stream interface owner reads=%#v", audioInterfaceOwnerReads)
	t.Logf("audio stream interface reads=%#v", audioInterfaceReads)
	if len(audioInterfaceReads) != 0 {
		t.Fatalf("audio stream interface unexpectedly read before the no-signal decision: %#v",
			audioInterfaceReads)
	}
	t.Logf("front-end task state/receive-result pairs=%#v", frontendTaskStates)
	t.Logf("front-end control dispatch calls=%#v", frontendDispatchCalls)
	t.Logf("registered-device sends=%#v", registeredDeviceCalls)
	t.Logf("all registered-device sends through selection=%#v", allRegisteredDeviceCalls)
	t.Logf("all demodulator entry calls through selection=%#v", allDemodulatorEntryCalls)
	t.Logf("all audio API calls through acquisition and selection=%#v", allAudioAPICalls)
	for _, c := range mediaCalls {
		t.Logf("media boundary pc=%08X ra=%08X a0=%08X a1=%08X a2=%08X a3=%08X ocode=%08X object=%08X manager+120=%08X", c.pc, c.ra, c.a0, c.a1, c.a2, c.a3, c.ocode, c.object, c.manager)
	}
	mediaHits := map[uint32]int{}
	for _, c := range mediaCalls {
		mediaHits[c.pc]++
	}
	if mediaHits[0x800A03B0] == 0 {
		t.Fatalf("harness: MPEG manager queue callback was not a live control: hits=%#v", mediaHits)
	}
	if mediaHits[0x800A1A48] == 0 {
		t.Fatalf("harness: component-refresh task callback was not a live upstream control: hits=%#v", mediaHits)
	}
	if mediaHits[0x8009E764] == 0 {
		t.Fatalf("successful front-end tune did not request the programme refresh: hits=%#v", mediaHits)
	}
	if mediaHits[0x8009E47C] == 0 || mediaHits[0x800A127C] == 0 ||
		mediaHits[0x8009F4EC] == 0 || mediaHits[0x8009F878] == 0 {
		t.Fatalf("PMT did not execute the component rebuild and programme/stream callbacks: hits=%#v", mediaHits)
	}
	stopCalls := 0
	for _, c := range allAudioAPICalls {
		if c.pc == 0x800889BC {
			stopCalls++
		}
		if c.pc == 0x80088970 {
			t.Fatalf("audio start unexpectedly executed before an elementary stream was delivered: %#v", allAudioAPICalls)
		}
	}
	if stopCalls == 0 {
		t.Fatalf("PMT component publication did not reach the measured audio stop decision: %#v", allAudioAPICalls)
	}
	for _, pc := range []uint32{0x800DA436, 0x800DA452, 0x800D5C8C, 0x800D5D2C,
		0x800DB9CA, 0x800DBF34, 0x800E8DE8, 0x800DC014} {
		if mediaHits[pc] != 0 {
			t.Fatalf("media program/stream path %08X unexpectedly became live before the no-signal decision: %#v", pc, mediaCalls)
		}
	}
	if len(frontendDispatchCalls) != 0 {
		t.Fatalf("demodulator control unexpectedly ran before the no-signal decision: %#v", frontendDispatchCalls)
	}
	if len(frontendTaskStates) == 0 {
		t.Fatal("harness: front-end task did not return from its receive boundary")
	}
	receivedSuccess := false
	for i := 0; i+1 < len(frontendTaskStates); i += 2 {
		if frontendTaskStates[i] != 0 {
			t.Fatalf("front-end task left idle state without issuing a command: %#v", frontendTaskStates)
		}
		receivedSuccess = receivedSuccess || frontendTaskStates[i+1] == 1
	}
	if !receivedSuccess {
		t.Fatalf("front-end task receive control did not observe successful message 1: %#v", frontendTaskStates)
	}
	t.Logf("typed-request discriminator byte reads=%#v", typedRequestDiscriminatorReads)
	t.Logf("typed-request discriminator writes=%#v", typedRequestDiscriminatorWrites)
	t.Logf("typed-request source byte reads=%#v", typedRequestSourceReads)
	sourceRead := false
	sourceWrite := false
	for _, r := range typedRequestSourceReads {
		t.Logf("typed-request source writes at %08X=%#v", r.address, typedRequestSourceWrites[r.address])
		if r.pc == 0x8006B400 && r.address == 0x804949AA && r.value == 1 {
			sourceRead = true
			for _, w := range typedRequestSourceWrites[r.address] {
				if w.pc == 0x800AD290 && w.a0 == 1 && w.ocode == 0x9FC6A5BA {
					sourceWrite = true
				}
			}
		}
	}
	if !sourceRead || !sourceWrite {
		t.Fatalf("typed-request discriminator did not originate in native (7,03)'s measured service descriptor byte: reads=%#v writes=%#v",
			typedRequestSourceReads, typedRequestSourceWrites[0x804949AA])
	}
	if ocodeHits[0x9FC6A9FE] != 0 || ocodeHits[0x9FC6B5CD] != 0 {
		t.Fatal("a direct typed-request caller unexpectedly executed; trace its record before retaining this assertion")
	}
	if len(typedRequestDiscriminatorReads) == 0 {
		t.Fatal("typed-request discriminator was never read")
	}
	for _, read := range typedRequestDiscriminatorReads {
		if read.value == 1 {
			continue
		}
		t.Fatalf("typed-request discriminator reads=%#v, want the measured television-service value 1",
			typedRequestDiscriminatorReads)
	}
	for _, c := range calls {
		if c.pc == 0x800AD928 {
			t.Logf("resolver pc=%08X ra=%08X handle=%08X owner=%08X request=%08X %08X %08X %08X out=%08X",
				c.pc, c.ra, c.a0, c.a1, c.request[0], c.request[1], c.request[2], c.request[3], c.a3)
			continue
		}
		t.Logf("call pc=%08X ra=%08X a0=%08X a1=%08X a2=%08X a3=%08X",
			c.pc, c.ra, c.a0, c.a1, c.a2, c.a3)
	}
	for _, pc := range []uint32{0x800AD928, 0x800284D8, 0x8002F81C, 0x800F8690, 0x80032D0C,
		0x800B0F2C, 0x800B0F8C, 0x800B1064,
		0x8001C604, 0x8001C6C8, 0x800D48D0, 0x800D4A0C} {
		t.Logf("PC %08X hits=%d before no-signal decision", pc, hits[pc])
	}
}
