// Package gdbstub implements the all-stop GDB Remote Serial Protocol for one
// paused, single-threaded MIPS32 emulator. A Server alone drives its Backend.
package gdbstub

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const maxPacket = 4096

// Registers is the architectural state visible to GDB. The ISA bit is stored
// separately because this VR4111 executes both MIPS32 and MIPS16 instructions.
type Registers struct {
	GPR                 [32]uint32
	Status, LO, HI      uint32
	BadVAddr, Cause, PC uint32
	ISA                 bool
}

// MemoryAccess describes a guest data access made during one Step. Instruction
// fetches and debugger-originated memory operations must be excluded.
type MemoryAccess struct {
	Address uint32
	Size    uint32
	Write   bool
}

// Backend is accessed only by the Serve session goroutine, including while
// continuing. Step returns the guest data accesses of exactly one instruction.
type Backend interface {
	Registers() Registers
	SetRegisters(Registers) error
	ReadMemory(addr, length uint32) ([]byte, error)
	WriteMemory(addr uint32, data []byte) error
	Step() ([]MemoryAccess, error)
}

type watchpoint struct {
	address, length uint32
	kind            byte
}

// Server holds breakpoint state for one emulator. It can serve one GDB client
// at a time; clients are handled synchronously to preserve deterministic steps.
type Server struct {
	backend     Backend
	breaks      map[uint32]uint32
	watches     map[watchpoint]struct{}
	last        string
	lastBreakPC uint32
}

// New creates a server for a stopped machine.
func New(backend Backend) *Server {
	return &Server{backend: backend, breaks: make(map[uint32]uint32), watches: make(map[watchpoint]struct{}), last: "S05"}
}

// Serve listens only on an explicitly specified loopback address. Closing the
// listener returned by Listen ends the accept loop; Serve itself is convenient
// for the production command and returns when its listener fails.
func (s *Server) Serve(addr string) error {
	listener, err := Listen(addr)
	if err != nil {
		return err
	}
	serveErr := s.ServeListener(listener)
	return errors.Join(serveErr, listener.Close())
}

// Listen enforces ARCH-DEV-1 before opening a socket. A hostname other than
// literal localhost is refused, including names that currently resolve locally.
func Listen(addr string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("gdbstub: invalid listen address: %w", err)
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil, fmt.Errorf("gdbstub: listen address %q is not loopback", addr)
		}
	}
	if port == "" {
		return nil, fmt.Errorf("gdbstub: listen port is empty")
	}
	if host == "localhost" {
		addr = net.JoinHostPort("127.0.0.1", port)
	}
	return net.Listen("tcp", addr)
}

// ServeListener handles one client at a time and exits when listener closes.
// Callers can own and close the listener for deterministic lifecycle tests.
func (s *Server) ServeListener(listener net.Listener) error {
	if s == nil || s.backend == nil {
		return fmt.Errorf("gdbstub: backend is nil")
	}
	if err := requireLoopback(listener.Addr()); err != nil {
		return err
	}
	conn, err := listener.Accept()
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		return fmt.Errorf("gdbstub: accept: %w", err)
	}
	serveErr := s.ServeConn(conn)
	if errors.Is(serveErr, io.EOF) {
		serveErr = nil
	}
	return errors.Join(serveErr, conn.Close())
}

func requireLoopback(addr net.Addr) error {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || tcp.IP == nil || !tcp.IP.IsLoopback() {
		return fmt.Errorf("gdbstub: listener is not loopback: %v", addr)
	}
	return nil
}

// ServeConn processes one already-connected RSP stream. It is also useful for
// tests; it never starts a goroutine or executes the backend concurrently.
func (s *Server) ServeConn(conn net.Conn) error {
	if s == nil || s.backend == nil {
		return fmt.Errorf("gdbstub: backend is nil")
	}
	r := bufio.NewReader(conn)
	ack := true
	var lastPacket []byte
	for {
		body, interrupt, err := readPacket(r, conn)
		if err != nil {
			return err
		}
		if interrupt {
			lastPacket, err = sendPacket(conn, "S02")
			if err != nil {
				return err
			}
			continue
		}
		if body == nil {
			if ack {
				if _, err = conn.Write([]byte{'-'}); err != nil {
					return err
				}
			}
			continue
		}
		if len(body) == 1 && body[0] == '-' {
			if len(lastPacket) != 0 {
				if _, err = conn.Write(lastPacket); err != nil {
					return err
				}
			}
			continue
		}
		if len(body) == 1 && body[0] == '+' {
			continue
		}
		if ack {
			if _, err = conn.Write([]byte{'+'}); err != nil {
				return err
			}
		}
		reply, closeSession, noAck := s.dispatch(string(body), r, conn)
		lastPacket, err = sendPacket(conn, reply)
		if err != nil {
			return err
		}
		if noAck {
			ack = false
		}
		if closeSession {
			return nil
		}
	}
}

// readPacket returns nil for a corrupted frame, and raw +/- acknowledgments
// separately. The checksum covers escaped wire bytes, as required by RSP.
func readPacket(r *bufio.Reader, _ net.Conn) ([]byte, bool, error) {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, false, err
		}
		switch b {
		case '+', '-':
			return []byte{b}, false, nil
		case 3:
			return nil, true, nil
		case '$':
			var body []byte
			var sum byte
			for {
				b, err = r.ReadByte()
				if err != nil {
					return nil, false, err
				}
				if b == '#' {
					break
				}
				sum += b
				if len(body) >= maxPacket {
					return nil, false, fmt.Errorf("gdbstub: packet exceeds %d bytes", maxPacket)
				}
				body = append(body, b)
			}
			var check [2]byte
			if _, err = io.ReadFull(r, check[:]); err != nil {
				return nil, false, err
			}
			want, valid := decodeChecksum(check)
			if !valid || want != sum {
				return nil, false, nil
			}
			return body, false, nil
		}
	}
}

func decodeChecksum(check [2]byte) (byte, bool) {
	decoded := make([]byte, 1)
	if _, err := hex.Decode(decoded, check[:]); err != nil {
		return 0, false
	}
	return decoded[0], true
}

func sendPacket(w io.Writer, body string) ([]byte, error) {
	var sum byte
	for i := range len(body) {
		sum += body[i]
	}
	p := []byte(fmt.Sprintf("$%s#%02x", body, sum))
	remaining := p
	for len(remaining) != 0 {
		n, err := w.Write(remaining)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrShortWrite
		}
		remaining = remaining[n:]
	}
	return p, nil
}

func (s *Server) dispatch(cmd string, r *bufio.Reader, conn net.Conn) (string, bool, bool) {
	switch {
	case cmd == "?":
		return s.last, false, false
	case strings.HasPrefix(cmd, "qSupported"):
		return "PacketSize=1000;QStartNoAckMode+;swbreak+;hwbreak+", false, false
	case cmd == "QStartNoAckMode":
		return "OK", false, true
	case cmd == "qAttached":
		return "1", false, false
	case cmd == "qC":
		return "QC1", false, false
	case cmd == "qfThreadInfo":
		return "m1", false, false
	case cmd == "qsThreadInfo":
		return "l", false, false
	case cmd == "vCont?":
		return "vCont;c;s", false, false
	case cmd == "Hc-1" || cmd == "Hc0" || cmd == "Hg0" || cmd == "Hg1" || cmd == "Hc1":
		return "OK", false, false
	case cmd == "T1":
		return "OK", false, false
	case cmd == "g":
		return encodeRegisters(s.backend.Registers()), false, false
	case strings.HasPrefix(cmd, "p") && len(cmd) > 1:
		return s.readRegister(cmd[1:]), false, false
	case strings.HasPrefix(cmd, "P") && len(cmd) > 1:
		return s.writeRegister(cmd[1:]), false, false
	case strings.HasPrefix(cmd, "G"):
		return s.writeRegisters(cmd[1:]), false, false
	case strings.HasPrefix(cmd, "m"):
		return s.readMemory(cmd[1:]), false, false
	case strings.HasPrefix(cmd, "M"):
		return s.writeMemory(cmd[1:]), false, false
	case strings.HasPrefix(cmd, "Z") || strings.HasPrefix(cmd, "z"):
		return s.breakPacket(cmd), false, false
	case cmd == "s" || strings.HasPrefix(cmd, "s") || cmd == "c" || strings.HasPrefix(cmd, "c"):
		return s.resume(cmd, r, conn), false, false
	case strings.HasPrefix(cmd, "vCont;"):
		action := strings.TrimPrefix(cmd, "vCont;")
		verb, thread, hasThread := strings.Cut(action, ":")
		if hasThread && thread != "1" && thread != "-1" && thread != "0" {
			return "E01", false, false
		}
		if verb == "s" || verb == "c" {
			return s.resume(verb, r, conn), false, false
		}
		return "", false, false
	case cmd == "D" || cmd == "k":
		return "OK", true, false
	default:
		return "", false, false
	}
}

const registerCount = 73 // MIPS32: 32 GPR + 6 control + 32 FPR + 3 FP control

func encodeRegisters(reg Registers) string {
	var b strings.Builder
	b.Grow(registerCount * 8)
	for i := range 32 {
		writeWord(&b, reg.GPR[i])
	}
	pc := reg.PC
	if reg.ISA {
		pc |= 1
	}
	for _, word := range [...]uint32{reg.Status, reg.LO, reg.HI, reg.BadVAddr, reg.Cause, pc} {
		writeWord(&b, word)
	}
	for range 35 {
		b.WriteString("xxxxxxxx")
	}
	return b.String()
}

func writeWord(b *strings.Builder, word uint32) {
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], word)
	b.WriteString(hex.EncodeToString(raw[:]))
}

func register(reg Registers, n uint64) (uint32, bool) {
	switch {
	case n < 32:
		return reg.GPR[n], true
	case n == 32:
		return reg.Status, true
	case n == 33:
		return reg.LO, true
	case n == 34:
		return reg.HI, true
	case n == 35:
		return reg.BadVAddr, true
	case n == 36:
		return reg.Cause, true
	case n == 37:
		pc := reg.PC
		if reg.ISA {
			pc |= 1
		}
		return pc, true
	default:
		return 0, false
	}
}

func setRegister(reg *Registers, n uint64, value uint32) bool {
	switch {
	case n == 0:
		return value == 0
	case n < 32:
		reg.GPR[n] = value
	case n == 32:
		reg.Status = value
	case n == 33:
		reg.LO = value
	case n == 34:
		reg.HI = value
	case n == 35:
		reg.BadVAddr = value
	case n == 36:
		reg.Cause = value
	case n == 37:
		reg.PC = value &^ 1
		reg.ISA = value&1 != 0
	default:
		return false
	}
	return true
}

func (s *Server) readRegister(raw string) string {
	n, err := strconv.ParseUint(raw, 16, 8)
	if err != nil {
		return "E01"
	}
	value, ok := register(s.backend.Registers(), n)
	if !ok {
		return "xxxxxxxx"
	}
	var b strings.Builder
	writeWord(&b, value)
	return b.String()
}

func parseWord(raw string) (uint32, error) {
	if len(raw) != 8 {
		return 0, fmt.Errorf("word length")
	}
	b, err := hex.DecodeString(raw)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (s *Server) writeRegister(raw string) string {
	num, value, ok := strings.Cut(raw, "=")
	if !ok {
		return "E01"
	}
	n, err := strconv.ParseUint(num, 16, 8)
	if err != nil {
		return "E01"
	}
	word, err := parseWord(value)
	if err != nil {
		return "E01"
	}
	reg := s.backend.Registers()
	if !setRegister(&reg, n, word) {
		return "E01"
	}
	if err = s.backend.SetRegisters(reg); err != nil {
		return "E02"
	}
	return "OK"
}

func (s *Server) writeRegisters(raw string) string {
	if len(raw) != registerCount*8 {
		return "E01"
	}
	reg := s.backend.Registers()
	for i := range 38 {
		word, err := parseWord(raw[i*8 : i*8+8])
		if err != nil {
			return "E01"
		}
		if !setRegister(&reg, uint64(i), word) {
			return "E01"
		}
	}
	if err := s.backend.SetRegisters(reg); err != nil {
		return "E02"
	}
	return "OK"
}

func parseRange(raw string) (uint32, uint32, error) {
	addrRaw, lenRaw, ok := strings.Cut(raw, ",")
	if !ok {
		return 0, 0, fmt.Errorf("missing length")
	}
	a, err := strconv.ParseUint(addrRaw, 16, 32)
	if err != nil {
		return 0, 0, err
	}
	n, err := strconv.ParseUint(lenRaw, 16, 32)
	if err != nil || n == 0 || n > maxPacket/2 || a+n > 1<<32 {
		return 0, 0, fmt.Errorf("invalid range")
	}
	return uint32(a), uint32(n), nil
}

func (s *Server) readMemory(raw string) string {
	a, n, err := parseRange(raw)
	if err != nil {
		return "E01"
	}
	data, err := s.backend.ReadMemory(a, n)
	if err != nil || len(data) != int(n) {
		return "E02"
	}
	return hex.EncodeToString(data)
}

func (s *Server) writeMemory(raw string) string {
	location, payload, ok := strings.Cut(raw, ":")
	if !ok {
		return "E01"
	}
	a, n, err := parseRange(location)
	if err != nil || len(payload) != int(n)*2 {
		return "E01"
	}
	data, err := hex.DecodeString(payload)
	if err != nil {
		return "E01"
	}
	if err := s.backend.WriteMemory(a, data); err != nil {
		return "E02"
	}
	return "OK"
}

func (s *Server) breakPacket(raw string) string {
	if len(raw) < 3 {
		return "E01"
	}
	kind := raw[1]
	if kind < '0' || kind > '4' {
		return ""
	}
	if raw[2] != ',' {
		return "E01"
	}
	parts := strings.Split(raw[3:], ",")
	if len(parts) != 2 {
		return "E01"
	}
	address, err := strconv.ParseUint(parts[0], 16, 32)
	if err != nil {
		return "E01"
	}
	length, err := strconv.ParseUint(parts[1], 16, 32)
	if err != nil || length == 0 || address+length > 1<<32 || address > uint64(^uint32(0)) || length > uint64(^uint32(0)) {
		return "E01"
	}
	address32 := uint32(address)
	if kind < '2' {
		if kind == '0' && length != 2 && length != 4 {
			return "E01"
		}
		// MIPS16 code addresses may carry the ISA bit; Core.PC never does.
		address32 &^= 1
		if raw[0] == 'Z' {
			s.breaks[address32] = uint32(length)
		} else {
			delete(s.breaks, address32)
		}
		return "OK"
	}
	w := watchpoint{address32, uint32(length), kind}
	if raw[0] == 'Z' {
		s.watches[w] = struct{}{}
	} else {
		delete(s.watches, w)
	}
	return "OK"
}

func overlaps(a, alen, b, blen uint32) bool {
	return uint64(a) < uint64(b)+uint64(blen) && uint64(b) < uint64(a)+uint64(alen)
}

func (s *Server) watchHit(accesses []MemoryAccess) string {
	for _, access := range accesses {
		if access.Size == 0 {
			continue
		}
		var selected watchpoint
		found := false
		for watch := range s.watches {
			if !overlaps(access.Address, access.Size, watch.address, watch.length) {
				continue
			}
			if watch.kind == '2' && !access.Write || watch.kind == '3' && access.Write {
				continue
			}
			if !found || watch.address < selected.address || watch.address == selected.address && watch.kind < selected.kind {
				selected, found = watch, true
			}
		}
		if found {
			label := "awatch"
			switch selected.kind {
			case '2':
				label = "watch"
			case '3':
				label = "rwatch"
			}
			return fmt.Sprintf("T05%s:%x;", label, selected.address)
		}
	}
	return ""
}

func (s *Server) resume(cmd string, r *bufio.Reader, conn net.Conn) string {
	step := cmd[0] == 's'
	if len(cmd) > 1 {
		pc, err := strconv.ParseUint(cmd[1:], 16, 32)
		if err != nil {
			return "E01"
		}
		reg := s.backend.Registers()
		reg.PC, reg.ISA = uint32(pc)&^1, pc&1 != 0
		if err := s.backend.SetRegisters(reg); err != nil {
			return "E02"
		}
	}
	// Continuing after a breakpoint must execute the stopped instruction once.
	skipFirst := s.last == "T05swbreak:;" && s.backend.Registers().PC == s.lastBreakPC
	for count := uint64(0); ; count++ {
		if !step && (!skipFirst || count != 0) {
			pc := s.backend.Registers().PC
			if _, ok := s.breaks[pc]; ok {
				s.last = "T05swbreak:;"
				s.lastBreakPC = pc
				return s.last
			}
		}
		accesses, err := s.backend.Step()
		if err != nil {
			s.last = "S0b"
			return s.last
		}
		if hit := s.watchHit(accesses); hit != "" {
			s.last = hit
			return hit
		}
		if step {
			s.last = "S05"
			return s.last
		}
		if count%1024 == 0 && interrupted(r, conn) {
			s.last = "S02"
			return s.last
		}
	}
}

func interrupted(r *bufio.Reader, conn net.Conn) bool {
	if err := conn.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
		return true
	}
	b, err := r.ReadByte()
	if resetErr := conn.SetReadDeadline(time.Time{}); resetErr != nil {
		return true
	}
	if err != nil {
		var netErr net.Error
		return !errors.As(err, &netErr) || !netErr.Timeout()
	}
	if b == 3 {
		return true
	}
	if b != '+' && b != '-' {
		_ = r.UnreadByte()
	}
	return false
}
