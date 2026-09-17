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
			d.Write(0x148, bus.Word, 0x4aff)
			d.Write(0x144, bus.Word, 0xc000)
			d.Write(0x140, bus.Word, 1)
			d.Write(0x128, bus.Word, 0x2000)
			d.Write(0x124, bus.Word, 0xc001)
			d.Write(0x124, bus.Word, 0x4000|(22<<2))
			d.Write(0x144, bus.Word, 0xc021)
			d.transportPart[0] = []byte{0, 0xb0, 0x20}
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

func TestPackedMatchWordPreservesBothMatchBanks(t *testing.T) {
	t.Parallel()
	d := New()
	d.Write(0x148, bus.Word, 0x000001ff)
	d.Write(0x144, bus.Word, 0xc020)
	d.Write(0x144, bus.Word, 0x4020)
	if got := d.Read(0x148, bus.Word); got != 0x000001ff {
		t.Fatalf("indirect match read = %#x", got)
	}
	d.Write(0x148, bus.Word, d.Read(0x148, bus.Word)|0x00ff0000)
	d.Write(0x144, bus.Word, 0xc020)
	if got := d.Read(0x148, bus.Word); got != 0x00ff01ff {
		t.Fatalf("read-modify-write lost a match bank: %#x", got)
	}
	if got, _ := d.Match(0, 2); got != (MatchByte{Value: 1, Mask: 0xff}) {
		t.Fatalf("low match bank = %+v", got)
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
