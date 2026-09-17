package gdbstub

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

type fakeBackend struct {
	regs   Registers
	mem    [256]byte
	steps  int
	access []MemoryAccess
}

func (b *fakeBackend) Registers() Registers           { return b.regs }
func (b *fakeBackend) SetRegisters(r Registers) error { b.regs = r; return nil }
func (b *fakeBackend) ReadMemory(a, n uint32) ([]byte, error) {
	if uint64(a)+uint64(n) > uint64(len(b.mem)) {
		return nil, fmt.Errorf("out of range")
	}
	return bytes.Clone(b.mem[a : a+n]), nil
}
func (b *fakeBackend) WriteMemory(a uint32, data []byte) error {
	if uint64(a)+uint64(len(data)) > uint64(len(b.mem)) {
		return fmt.Errorf("out of range")
	}
	copy(b.mem[a:], data)
	return nil
}
func (b *fakeBackend) Step() ([]MemoryAccess, error) {
	b.steps++
	b.regs.PC += 4
	return b.access, nil
}

func TestRegistersMIPS32BigEndianAndISA(t *testing.T) {
	b := &fakeBackend{regs: Registers{PC: 0x80001234, ISA: true}}
	b.regs.GPR[1] = 0x12345678
	s := New(b)
	all, _, _ := s.dispatch("g", nil, nil)
	if len(all) != registerCount*8 || all[8:16] != "12345678" || all[37*8:38*8] != "80001235" || all[38*8:] != strings.Repeat("x", 35*8) {
		t.Fatalf("bad MIPS32 register packet: length=%d, r1=%q, pc=%q", len(all), all[8:16], all[37*8:38*8])
	}
	if got, _, _ := s.dispatch("p25", nil, nil); got != "80001235" {
		t.Fatalf("pc = %q", got)
	}
	if got, _, _ := s.dispatch("P25=80005678", nil, nil); got != "OK" {
		t.Fatalf("P = %q", got)
	}
	if b.regs.PC != 0x80005678 || b.regs.ISA {
		t.Fatalf("P did not split ISA: %+v", b.regs)
	}
	if got, _, _ := s.dispatch("P0=00000001", nil, nil); got != "E01" {
		t.Fatalf("zero register writable: %q", got)
	}
	if got, _, _ := s.dispatch("G"+all, nil, nil); got != "OK" {
		t.Fatalf("G = %q", got)
	}
	if b.regs.PC != 0x80001234 || !b.regs.ISA {
		t.Fatalf("G did not restore ISA: %+v", b.regs)
	}
}

func TestMemoryBreakpointsWatchpointsAndStep(t *testing.T) {
	b := &fakeBackend{regs: Registers{PC: 0x10}}
	s := New(b)
	for _, tc := range []struct{ command, want string }{
		{"M10,4:11223344", "OK"},
		{"m10,4", "11223344"},
		{"mff,2", "E02"},
		{"M10,4:zzzzzzzz", "E01"},
		{"mfffffffe,4", "E01"},
	} {
		got, _, _ := s.dispatch(tc.command, nil, nil)
		if got != tc.want {
			t.Fatalf("%s = %q, want %q", tc.command, got, tc.want)
		}
	}
	if got := s.breakPacket("Z0,14,4"); got != "OK" {
		t.Fatal(got)
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	if got := s.resume("c", bufio.NewReader(serverConn), serverConn); got != "T05swbreak:;" || b.steps != 1 {
		t.Fatalf("breakpoint stop = %q, steps=%d", got, b.steps)
	}
	if got := s.breakPacket("Z0,18,4"); got != "OK" {
		t.Fatal(got)
	}
	if got := s.resume("c", bufio.NewReader(serverConn), serverConn); got != "T05swbreak:;" || b.steps != 2 {
		t.Fatalf("breakpoint resume = %q, steps=%d", got, b.steps)
	}
}

func TestWatchpointOverlapAndRemoval(t *testing.T) {
	b := &fakeBackend{access: []MemoryAccess{{Address: 0x21, Size: 2, Write: true}}}
	s := New(b)
	if got := s.breakPacket("Z2,20,2"); got != "OK" {
		t.Fatal(got)
	}
	if got := s.watchHit(b.access); got != "T05watch:20;" {
		t.Fatalf("write watch = %q", got)
	}
	if got := s.breakPacket("z2,20,2"); got != "OK" {
		t.Fatal(got)
	}
	if got := s.watchHit(b.access); got != "" {
		t.Fatalf("removed watch = %q", got)
	}
	if got := s.breakPacket("Z3,20,2"); got != "OK" {
		t.Fatal(got)
	}
	if got := s.watchHit(b.access); got != "" {
		t.Fatalf("read watch hit write: %q", got)
	}
	b.access[0].Write = false
	if got := s.watchHit(b.access); got != "T05rwatch:20;" {
		t.Fatalf("read watch = %q", got)
	}
	if got := s.breakPacket("Z4,20,2"); got != "OK" {
		t.Fatal(got)
	}
	if got := s.breakPacket("Z5,20,2"); got != "" {
		t.Fatalf("unknown type = %q", got)
	}
}

func TestPacketFramingAckRetransmitAndDetach(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	end := make(chan error, 1)
	go func() { end <- New(&fakeBackend{}).ServeConn(serverConn); serverConn.Close() }()
	r := bufio.NewReader(clientConn)
	clientConn.Write([]byte("$?#00"))
	if b, _ := r.ReadByte(); b != '-' {
		t.Fatalf("bad checksum ack = %q", b)
	}
	request(t, clientConn, r, "?", "S05")
	if _, err := clientConn.Write([]byte{'-'}); err != nil {
		t.Fatal(err)
	}
	readReply(t, r, "S05")
	request(t, clientConn, r, "qSupported:multiprocess+", "PacketSize=1000;QStartNoAckMode+;swbreak+;hwbreak+")
	request(t, clientConn, r, "D", "OK")
	if err := <-end; err != nil {
		t.Fatal(err)
	}
}

func TestContinueCanBeInterrupted(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	end := make(chan error, 1)
	b := &fakeBackend{}
	go func() { end <- New(b).ServeConn(serverConn); serverConn.Close() }()
	r := bufio.NewReader(clientConn)
	clientConn.SetDeadline(time.Now().Add(time.Second))
	p, _ := sendPacket(io.Discard, "c")
	if _, err := clientConn.Write(p); err != nil {
		t.Fatal(err)
	}
	if ack, err := r.ReadByte(); err != nil || ack != '+' {
		t.Fatalf("continue ack = %q, %v", ack, err)
	}
	if _, err := clientConn.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
	readReply(t, r, "S02")
	request(t, clientConn, r, "D", "OK")
	if err := <-end; err != nil {
		t.Fatal(err)
	}
	if b.steps == 0 {
		t.Fatal("continue did not execute guest steps")
	}
}

func TestDisconnectDuringContinueStopsStepping(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	end := make(chan error, 1)
	b := &fakeBackend{}
	go func() {
		end <- New(b).ServeConn(serverConn)
		serverConn.Close()
	}()
	r := bufio.NewReader(clientConn)
	p, _ := sendPacket(io.Discard, "c")
	if _, err := clientConn.Write(p); err != nil {
		t.Fatal(err)
	}
	if ack, err := r.ReadByte(); err != nil || ack != '+' {
		t.Fatalf("continue ack = %q, %v", ack, err)
	}
	clientConn.Close()
	select {
	case <-end:
	case <-time.After(time.Second):
		t.Fatal("disconnected GDB left the guest running")
	}
	if b.steps == 0 {
		t.Fatal("continue did not execute guest steps")
	}
}

func request(t *testing.T, conn net.Conn, r *bufio.Reader, body, want string) {
	t.Helper()
	p, _ := sendPacket(io.Discard, body)
	if _, err := conn.Write(p); err != nil {
		t.Fatal(err)
	}
	if b, err := r.ReadByte(); err != nil || b != '+' {
		t.Fatalf("ack = %q, %v", b, err)
	}
	readReply(t, r, want)
	conn.Write([]byte{'+'})
}

func readReply(t *testing.T, r *bufio.Reader, want string) {
	t.Helper()
	b, err := r.ReadByte()
	if err != nil || b != '$' {
		t.Fatalf("reply prefix = %q, %v", b, err)
	}
	got, err := r.ReadString('#')
	if err != nil {
		t.Fatal(err)
	}
	check := make([]byte, 2)
	if _, err := io.ReadFull(r, check); err != nil {
		t.Fatal(err)
	}
	if got[:len(got)-1] != want {
		t.Fatalf("reply = %q, want %q", got, want)
	}
}

func TestLoopbackOnly(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", "[::]:0", "192.0.2.1:0", "example.com:0", ":0"} {
		listener, err := Listen(addr)
		if err == nil {
			listener.Close()
			t.Fatalf("accepted %q", addr)
		}
	}
	listener, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener.Close()
}
