package multiplex

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/demux"
)

func TestSubscriptionReadsTheFirmwareSelectedServiceFromTheEITFilter(t *testing.T) {
	t.Parallel()
	d := demux.New()
	match := func(unit, index uint8, value, mask byte) {
		d.Write(0x148, bus.Word, uint32(value)<<8|uint32(mask))
		d.Write(0x144, bus.Word, 0xc000|uint32(index)<<4|uint32(unit))
	}
	// The baseline subjects Read requires before any viewing state is meaningful.
	match(1, 0, 0x40, 0xff)
	match(1, 1, 0x00, 0xff)
	match(1, 2, 0x20, 0xff)
	match(3, 0, 0x4a, 0xff)
	match(3, 1, 0x00, 0xff)
	match(3, 2, 0x10, 0xff)
	// The measured viewing request: present/following EIT for service_id 100 on PID 0x12.
	match(4, 0, 0x4e, 0xfe)
	match(4, 1, 0x00, 0xff)
	match(4, 2, 0x64, 0xff)
	d.Write(0x14+4*18, bus.Word, 0x14012)
	d.Write(0xd8, bus.Word, 1<<18)

	sub, err := Read(d)
	if err != nil {
		t.Fatal(err)
	}
	if !sub.EITArmed || sub.TunedServiceID != 100 {
		t.Fatalf("viewing subscription = EIT armed %t, service %d", sub.EITArmed, sub.TunedServiceID)
	}
}
