package demux

import (
	"encoding/binary"
	"testing"

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

func TestTransportRejectsUnrepresentableWritePointer(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	d := New()
	if err := d.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	section := []byte{0x00, 0xb0, 0x04}
	var checksum [4]byte
	binary.BigEndian.PutUint32(checksum[:], mpegCRC(section))
	section = append(section, checksum[:]...)
	d.indirect[0], d.indirect[3], d.writePointer[0] = 0x1ffffc, 0x200010, 0x1ffffc
	if err := d.acceptTransportSection(0, section); err == nil {
		t.Fatal("write pointer escaped its 21-bit range")
	}
}
