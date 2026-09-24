package demux

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/dvb"
	"github.com/ddunford/goretrotv/internal/memory"
)

func TestTransportRetainsPacketAcrossDMATransfers(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	d := New()
	if err := d.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	d.control140 = 1
	packet := make([]byte, transportPacketSize)
	packet[0], packet[1], packet[2], packet[3] = 0x47, 0x1f, 0xff, 0x10
	for i := 4; i < len(packet); i++ {
		packet[i] = 0xff
	}
	if err := d.PushTransport(packet[:94]); err != nil {
		t.Fatal(err)
	}
	if len(d.transportPacket) != 94 {
		t.Fatalf("partial packet has %d bytes", len(d.transportPacket))
	}
	blob, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored := New()
	if err := restored.Restore(blob); err != nil {
		t.Fatal(err)
	}
	if err := restored.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	if err := restored.PushTransport(packet[94:]); err != nil {
		t.Fatal(err)
	}
	if len(restored.transportPacket) != 0 {
		t.Fatal("completed packet left a partial fragment")
	}
}

func TestTransportPIDOnlyChannelIgnoresAnUnrelatedRouteBit(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	d := New()
	if err := d.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	const channel = uint32(15)
	d.Write(0x140, bus.Word, 1)
	d.Write(0xD8, bus.Word, 1<<channel)
	d.Write(0x14+4*channel, bus.Word, 0x14100)
	// Reproduce the live firmware state: unit 3 matches BAT, while its byte-nine value happens
	// to overlap filter 15's route bit. There is deliberately no table-0x02 unit for the PMT.
	d.matchWords[3][0] = 0x4aff
	d.matchWords[3][9] = 1 << channel
	pmt, err := broadcast.PMT(0x64, 0, 0x101, nil,
		[]broadcast.ElementaryStream{{Type: 0x03, PID: 0x102}})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.PushTransport(dvb.PacketizeSection(0x100, pmt, 0)); err != nil {
		t.Fatal(err)
	}
	if got := d.Read(0xB8, bus.Word); got != 1<<channel {
		t.Fatalf("PID-only PMT completion = %#08x", got)
	}
	if got := ram.Read(RingBase(uint8(channel))&0x1fffffff, bus.Byte); got != 0x02 {
		t.Fatalf("PMT first byte = %#x", got)
	}
}

func TestTransportPacketUsesGuestPIDAndSectionFilters(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	d := New()
	if err := d.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	const channel = uint32(22)
	d.Write(0x140, bus.Word, 1)
	d.Write(0xD8, bus.Word, 1<<channel)
	d.Write(0x14+4*channel, bus.Word, 0x14014)
	d.Write(0x148, bus.Word, 0x70ff)
	d.Write(0x144, bus.Word, 0xc005)
	d.Write(0x148, bus.Word, 1<<channel)
	d.Write(0x144, bus.Word, 0xc095)

	section := []byte{0x70, 0x70, 0x05, 0xc3, 0x50, 0, 0, 0}
	packet := make([]byte, transportPacketSize)
	for i := range packet {
		packet[i] = 0xff
	}
	packet[0], packet[1], packet[2], packet[3], packet[4] = 0x47, 0x40, 0x14, 0x10, 0
	copy(packet[5:], section)
	if err := d.PushTransport(packet); err != nil {
		t.Fatal(err)
	}
	if got := d.Read(0xB8, bus.Word); got != 1<<channel {
		t.Fatalf("transport section completion = %#08x", got)
	}
	if got := ram.Read(RingBase(uint8(channel))&0x1fffffff, bus.Byte); got != 0x70 {
		t.Fatalf("transport section first byte = %#x", got)
	}
}
