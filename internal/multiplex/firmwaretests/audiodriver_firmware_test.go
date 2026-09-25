package firmwaretests_test

import (
	"reflect"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

// TestTheLiveAudioDriverDescriptor pins the entry points read from the guest's registered-device
// array. This is the control for probes which count whether an audio path actually reaches the
// driver: the addresses come from the live descriptor, not from proximity to an audio string.
func TestTheLiveAudioDriverDescriptor(t *testing.T) {
	box := restoredBox(t)
	const descriptor = 0x803138F0
	word := func(off uint32) uint32 {
		return box.RAM.Read(descriptor-memory.DRAMBase+off, bus.Word)
	}
	for off, want := range map[uint32]uint32{
		0x00: 0x0000000A,
		0x04: 0x9FC22DEC, // firmware string "audio_encoder"
		0x08: 0x00010000,
		0x0C: 0x80105A80,
		0x10: 0x00000005,
		0x14: 0x8001C47D, // command dispatcher, MIPS16
		0x18: 0x8001C529, // secondary entry, MIPS16
		0x1C: 0x801706E8,
	} {
		if got := word(off); got != want {
			t.Errorf("audio_encoder descriptor +%02X = %08X, want %08X", off, got, want)
		}
	}

	// The adjacent "audio" registration is a resource record, not an alternate playback driver:
	// it has no command entries. Pinning that distinction keeps later probes on audio_encoder's
	// executable path rather than choosing a promising name from the inventory.
	const audioResource = 0x803138C8
	resourceWord := func(off uint32) uint32 {
		return box.RAM.Read(audioResource-memory.DRAMBase+off, bus.Word)
	}
	for off, want := range map[uint32]uint32{
		0x00: 0x0000000B,
		0x04: 0x9FC22DFC, // firmware string "audio"
		0x14: 0,
		0x18: 0,
		0x1C: 0,
	} {
		if got := resourceWord(off); got != want {
			t.Errorf("audio resource +%02X = %08X, want %08X", off, got, want)
		}
	}
}

// TestWhereTheRunningFirmwareStoresAudioStreamEntries finds the live indirection used by callers.
// The flat decompressed image has no aligned static word for the configure callback: the application
// handoff materialises the final callable table in RAM. Searching the restored guest therefore finds
// the addresses a real caller must read without guessing from nearby audio strings.
func TestWhereTheRunningFirmwareStoresAudioStreamEntries(t *testing.T) {
	box := restoredBox(t)
	for target, want := range map[uint32][]uint32{
		0x80105A80: {0x800FC358, 0x803138FC},
		0x8001C605: {0x80105A8C},
		0x8001C6C9: {0x8001C720, 0x80105A90},
	} {
		var refs []uint32
		for off := uint32(0); off+4 <= memory.DRAMSize; off += 4 {
			if box.RAM.Read(off, bus.Word) == target {
				refs = append(refs, memory.DRAMBase+off)
			}
		}
		if len(refs) == 0 {
			t.Fatalf("running firmware contains no pointer to audio entry %08X", target)
		}
		if !reflect.DeepEqual(refs, want) {
			t.Fatalf("audio entry %08X stored at %08X, want measured references %08X", target, refs, want)
		}
		t.Logf("audio entry %08X stored at %08X", target, refs)
	}
}

// TestTheRestoredMediaObjectPool pins the upstream subject of the audio-stream adapter. The
// module registered as 0x210 owns twenty-eight 432-byte records at this address; inspecting the
// pool distinguishes a cold object callback from a callback whose object was created before the
// restored acquisition snapshot.
func TestTheRestoredMediaObjectPool(t *testing.T) {
	box := restoredBox(t)
	const (
		pool    = 0x8012B4D0
		stride  = uint32(432)
		records = 28
	)
	nonempty := 0
	for i := range records {
		base := pool - memory.DRAMBase + uint32(i)*stride // #nosec G115 -- fixed firmware pool
		words := [12]uint32{}
		for j := range words {
			words[j] = box.RAM.Read(base+uint32(j)*4, bus.Word) // #nosec G115 -- fixed record prefix
		}
		active := false
		for _, word := range words {
			active = active || word != 0
		}
		if active {
			nonempty++
			t.Logf("media record %d prefix=%08X", i, words)
		}
	}
	t.Logf("restored media records with a non-zero 48-byte prefix=%d/%d", nonempty, records)
}

// TestTheMediaModuleRegistration pins the live dispatcher entry for module 0x210. The static
// registration only shows the two callback addresses; the copied registry also identifies its
// dispatcher classes and object-handle range, which are the boundaries a stream probe must
// observe rather than calling either callback itself.
func TestTheMediaModuleRegistration(t *testing.T) {
	box := restoredBox(t)
	const (
		registry = uint32(0x801296A0)
		entries  = 38
		stride   = uint32(16)
		moduleID = uint32(0x210)
	)
	for i := range entries {
		base := registry - memory.DRAMBase + uint32(i)*stride // #nosec G115 -- bounded firmware registry
		if box.RAM.Read(base, bus.Word) != moduleID {
			continue
		}
		words := [4]uint32{}
		for j := range words {
			words[j] = box.RAM.Read(base+uint32(j)*4, bus.Word) // #nosec G115 -- four-word entry
		}
		t.Logf("module 0x210 registry index=%d words=%08X", i, words)
		if words[1] != 0x800DB821 || words[2] != 0x800DB9CD {
			t.Fatalf("module 0x210 callbacks=%08X/%08X, want measured event/object entries", words[1], words[2])
		}
		descriptor := [5]uint32{}
		for j := range descriptor {
			descriptor[j] = box.RAM.Read((words[3]-memory.DRAMBase)+uint32(j)*4, bus.Word) // #nosec G115 -- five-word descriptor
		}
		t.Logf("module 0x210 descriptor=%08X", descriptor)
		wantDescriptor := [5]uint32{0x210, 0x80001400, 0x800014FF, 1, 3}
		if descriptor != wantDescriptor {
			t.Fatalf("module 0x210 descriptor=%08X, want measured dispatcher contract %08X",
				descriptor, wantDescriptor)
		}
		return
	}
	t.Fatal("running firmware registry contains no module 0x210 entry")
}

// TestThe0200To0210ChainBelongsToConditionalAccess pins the owner of the event path which was
// previously described only as a media-object path. The sole 0x10000 producer upstream of module
// 0x200 is registered against the firmware's ECM interface; the adjacent registrations are EMM
// and UTIL. A free-to-air decoder must not manufacture this entitlement path to start playback.
func TestThe0200To0210ChainBelongsToConditionalAccess(t *testing.T) {
	box := restoredBox(t)
	for address, want := range map[uint32]uint32{
		0x000E26B0: 0x800E4201, // ECM callback which emits event 0x10000
		0x000E26B4: 0x9FCC3448, // "ECM"
		0x000E26BC: 0x800E788D, // interface registration routine
		0x000E26C0: 0x800E426D, // adjacent EMM callback
		0x000E26C4: 0x9FCC344C, // "EMM"
		0x000E26CC: 0x800DE3C9, // adjacent UTIL callback
		0x000E26D0: 0x9FCC3450, // "UTIL"
	} {
		if got := box.RAM.Read(address, bus.Word); got != want {
			t.Fatalf("conditional-access registration word %08X = %08X, want %08X", address, got, want)
		}
	}
	for address, want := range map[uint32]string{
		0x9FCC3448: "ECM",
		0x9FCC344C: "EMM",
		0x9FCC3450: "UTIL",
	} {
		got := make([]byte, len(want))
		for i := range got {
			got[i] = byte(box.Machine.Bus.Read(address+uint32(i), bus.Byte)) // #nosec G115 -- three fixed firmware strings
		}
		if string(got) != want {
			t.Fatalf("firmware interface at %08X = %q, want %q", address, got, want)
		}
	}
}
