package demux

import (
	"reflect"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
)

func TestEnableSetsAndStatusClears(t *testing.T) {
	t.Parallel()
	d := New()
	d.Write(0xD8, bus.Word, 1<<22)
	d.Write(0xD8, bus.Word, 1<<23)
	if got := d.Read(0xD8, bus.Word); got != (1<<22)|(1<<23) {
		t.Fatalf("enable %#08x, want both independently armed filters", got)
	}
	if got := d.Read(0xB8, bus.Word); got != 0 {
		t.Fatalf("status %#08x before delivery", got)
	}
	d.complete(22)
	d.complete(23)
	if got := d.Read(0xB8, bus.Word); got != (1<<22)|(1<<23) {
		t.Fatalf("status %#08x after two completions", got)
	}
	d.Write(0xB8, bus.Word, ^uint32(1<<22))
	if got := d.Read(0xB8, bus.Word); got != 1<<23 {
		t.Fatalf("complement acknowledgement left status %#08x", got)
	}
	if got := d.Read(0xD8, bus.Word); got != (1<<22)|(1<<23) {
		t.Fatalf("acknowledgement changed enable to %#08x", got)
	}
	d.Write(0x00, bus.Word, 1)
	if got := d.Read(0xD8, bus.Word) | d.Read(0xB8, bus.Word); got != 0 {
		t.Fatalf("block reset left enable/status %#08x", got)
	}
}

func TestDemuxHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New() },
		Mutate: func(device bus.Device) {
			d := device.(*Demux)
			d.Write(0xD8, bus.Word, 0x00400000)
			d.complete(22)
			d.writePointer[22] = 0x45c24
			d.Write(0x124, bus.Word, 0x4000|(22<<2))
			d.Write(0x6c, bus.Word, 0x14014)
			d.Write(0x94, bus.Word, 0x4101)
			d.Write(0x98, bus.Word, 0x4102)
			d.Write(0x148, bus.Word, 0x4aff)
			d.Write(0x144, bus.Word, 0xc000)
			d.Write(0x140, bus.Word, 1)
			d.Write(0x128, bus.Word, 0x2000)
			d.Write(0x124, bus.Word, 0xc001)
			d.Write(0x124, bus.Word, 0x4000|(22<<2))
			d.Write(0x144, bus.Word, 0xc021)
			d.transportPart[0] = []byte{0, 0xb0, 0x20}
			d.transportPacket = []byte{0x47, 0x40}
			d.programmeTransport = make([]byte, transportPacketSize)
			d.programmeTransport[0] = 0x47
		},
		Disturb: func(device bus.Device) {
			device.Reset()
		},
		Constant: []string{"name", "ram", "interrupt"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPIDChannelsAndMatchUnitsAreIndependent(t *testing.T) {
	t.Parallel()
	d := New()
	// Channel 22 is the TDT PID 0x14. The 16 match units cannot be indexed by 22.
	d.Write(0xD8, bus.Word, 1<<22)
	d.Write(0x14+4*22, bus.Word, 0x14014)
	d.Write(0x14+4*23, bus.Word, 0x14011)
	if got := d.ArmedPIDs(); !reflect.DeepEqual(got, []uint16{0x14}) {
		t.Fatalf("armed PIDs = %v, want TDT only", got)
	}
	d.Write(0xD8, bus.Word, 1<<23)
	if got := d.ArmedPIDs(); !reflect.DeepEqual(got, []uint16{0x14, 0x11}) {
		t.Fatalf("armed PIDs = %v", got)
	}
	d.Write(0x148, bus.Word, 0x4aff)
	d.Write(0x144, bus.Word, 0xc000)
	if got, ok := d.Match(0, 0); !ok || got != (MatchByte{Value: 0x4a, Mask: 0xff}) {
		t.Fatalf("match unit 0 byte 0 = %+v, %t", got, ok)
	}
	if _, ok := d.Match(22, 0); ok {
		t.Fatal("match unit 22 must not exist merely because PID channel 22 exists")
	}
	if got := d.ArmedPIDs(); !reflect.DeepEqual(got, []uint16{0x14, 0x11}) {
		t.Fatalf("programming match unit 0 changed channel PIDs: %v", got)
	}
	d.Write(0x14+4*23, bus.Word, 0x1fff)
	if got := d.ArmedPIDs(); !reflect.DeepEqual(got, []uint16{0x14}) {
		t.Fatalf("disabled channel still armed: %v", got)
	}
}

func TestProgrammePIDsAreSeparateDecoderInputs(t *testing.T) {
	t.Parallel()
	d := New()
	d.Write(0x94, bus.Word, 0x4101)
	if video, audio, ok := d.ProgrammePIDs(); ok || video != 0x101 || audio != 0 {
		t.Fatalf("one programme PID = video %#x audio %#x ready %t", video, audio, ok)
	}
	d.Write(0x98, bus.Word, 0x4102)
	if video, audio, ok := d.ProgrammePIDs(); !ok || video != 0x101 || audio != 0x102 {
		t.Fatalf("programme PIDs = video %#x audio %#x ready %t", video, audio, ok)
	}
	if got := d.ArmedPIDs(); len(got) != 0 {
		t.Fatalf("decoder PIDs appeared as section filters: %v", got)
	}
	d.Reset()
	if video, audio, ok := d.ProgrammePIDs(); ok || video != 0 || audio != 0 {
		t.Fatalf("reset programme PIDs = video %#x audio %#x ready %t", video, audio, ok)
	}
}

func TestPackedMatchWordIsStoredButNotReadBack(t *testing.T) {
	t.Parallel()
	d := New()
	d.Write(0x148, bus.Word, 0x000001ff)
	d.Write(0x144, bus.Word, 0xc020)
	d.Write(0x144, bus.Word, 0x4020)
	if got := d.Read(0x148, bus.Word); got != 0 {
		t.Fatalf("indirect match register unexpectedly read back %#x", got)
	}
	if got, _ := d.Match(0, 2); got != (MatchByte{Value: 1, Mask: 0xff}) {
		t.Fatalf("written low match bank = %+v", got)
	}
	d.Write(0x148, bus.Word, 0x00ff01ff)
	d.Write(0x144, bus.Word, 0xc020)
	if got := d.Read(0x148, bus.Word); got != 0 {
		t.Fatalf("indirect match register unexpectedly read back after second write %#x", got)
	}
	if got, _ := d.Match(0, 2); got != (MatchByte{Value: 1, Mask: 0xff}) {
		t.Fatalf("low match bank = %+v", got)
	}
}

func TestMatchControlSupportsGuestReadModifyWrite(t *testing.T) {
	t.Parallel()
	d := New()
	d.Write(0x140, bus.Word, 1|(1<<18))
	current := d.Read(0x140, bus.Word)
	d.Write(0x140, bus.Word, current&^(1<<18))
	if got := d.Read(0x140, bus.Word); got != 1 {
		t.Fatalf("match control after clearing pair-mode bit = %#08x, want global bit preserved", got)
	}
}

func TestLISRPointerHandshake(t *testing.T) {
	t.Parallel()
	d := New()
	d.writePointer[22] = 0x45c24
	d.writePointer[23] = 0x46c32
	for _, tc := range []struct {
		filter uint32
		want   uint32
	}{
		{22, 0x45c24},
		{23, 0x46c32},
	} {
		d.Write(0x124, bus.Word, 0x4000|(tc.filter<<2))
		cleared := false
		for spin := 0; spin < 8; spin++ {
			if d.Read(0x124, bus.Word)&0x4000 == 0 {
				cleared = true
				break
			}
		}
		if !cleared {
			t.Fatalf("filter %d LISR would hang waiting for command busy bit", tc.filter)
		}
		if got := d.Read(0x128, bus.Word); got != tc.want {
			t.Fatalf("filter %d pointer = %#x, want %#x", tc.filter, got, tc.want)
		}
	}
	d.Reset()
	if got := d.Read(0x128, bus.Word); got != 0 {
		t.Fatalf("reset pointer = %#x", got)
	}
}

// echoingCommandRegister models the tempting but incorrect generic register file:
// the last command written at +0x124 is returned on the next read.
type echoingCommandRegister struct {
	*Demux
	command uint32
}

func (e *echoingCommandRegister) Write(off uint32, size bus.Size, value uint32) {
	e.Demux.Write(off, size, value)
	if off == 0x124 {
		e.command = value
	}
}

func (e *echoingCommandRegister) Read(off uint32, size bus.Size) uint32 {
	if off == 0x124 {
		return e.command
	}
	return e.Demux.Read(off, size)
}

func TestEchoingCommandRegisterStallsLISR(t *testing.T) {
	t.Parallel()
	const command = 0x4000 | (22 << 2)
	working := New()
	working.Write(0x124, bus.Word, command)
	if working.Read(0x124, bus.Word)&0x4000 != 0 {
		t.Fatal("working command register never clears busy")
	}
	echoing := &echoingCommandRegister{Demux: New()}
	echoing.Write(0x124, bus.Word, command)
	for spin := 0; spin < 8; spin++ {
		if echoing.Read(0x124, bus.Word)&0x4000 == 0 {
			t.Fatalf("echoing command register unexpectedly cleared busy after %d spins", spin)
		}
	}
}

func TestJoiningMatchUnitsToPIDChannelsInventsMissingPID(t *testing.T) {
	t.Parallel()
	d := New()
	d.Write(0xD8, bus.Word, 1<<21)
	d.Write(0x14+4*21, bus.Word, 0x14052)
	d.Write(0x148, bus.Word, 0x73ff)
	d.Write(0x144, bus.Word, 0xc005) // Match unit 5, while PID 0x52 is on channel 21.
	if got := d.ArmedPIDs(); !reflect.DeepEqual(got, []uint16{0x52}) {
		t.Fatalf("actual armed PIDs = %v, want PID 0x52", got)
	}
	if got, ok := d.Match(5, 0); !ok || got != (MatchByte{Value: 0x73, Mask: 0xff}) {
		t.Fatalf("match unit 5 = %+v, %t", got, ok)
	}
	if _, ok := d.Match(21, 0); ok {
		t.Fatal("a channel-index join would invent match unit 21")
	}
}
