package main

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// sectionInput is a diagnostic broadcast delivery, timed by guest instruction
// count. Several sections may share one count, as they can on a transport stream.
type sectionInput struct {
	at    uint64
	pid   uint16
	bytes []byte
}

type sectionInputs []sectionInput

func (s *sectionInputs) String() string { return fmt.Sprintf("%d scheduled sections", len(*s)) }

func (s sectionInputs) sortByTime() {
	sort.SliceStable(s, func(i, j int) bool { return s[i].at < s[j].at })
}

func (s *sectionInputs) Set(spec string) error {
	parts := strings.SplitN(spec, ":", 3)
	if len(parts) != 3 {
		return fmt.Errorf("section %q must be instruction:pid:hex", spec)
	}
	at, err := strconv.ParseUint(parts[0], 0, 64)
	if err != nil || at == 0 {
		return fmt.Errorf("section instruction %q must be a positive integer", parts[0])
	}
	pid, err := strconv.ParseUint(parts[1], 0, 13)
	if err != nil {
		return fmt.Errorf("section PID %q is outside 0..8191", parts[1])
	}
	bytes, err := hex.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("section hex: %w", err)
	}
	if len(bytes) < 3 || int(bytes[1]&15)<<8|int(bytes[2]) != len(bytes)-3 {
		return fmt.Errorf("section length field differs from %d bytes", len(bytes))
	}
	*s = append(*s, sectionInput{at: at, pid: uint16(pid), bytes: bytes}) // #nosec G115 -- ParseUint limits PID to 13 bits.
	return nil
}
