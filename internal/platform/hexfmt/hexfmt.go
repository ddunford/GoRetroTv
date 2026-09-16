// Package hexfmt renders machine addresses and words as text in exactly one casing and one width.
//
// There is one casing here because the predecessor had two. Its instruments built lookup keys with
// an uppercase formatter and read them back with JavaScript's lowercase `toString(16)`, so every
// address whose digits happened to include a hex letter missed its own map and reported a
// plausible zero. 0x80081C58 was reported as zero twice, a day was spent on it, and two findings
// were withdrawn. A miss like that does not look like a bug; it looks like an answer.
//
// The canonical form is `0x` followed by upper-case digits, zero-padded to the width of the type:
// eight for a word or an address, four for a half, two for a byte. Upper case because the measured
// record in docs/reference/digibox-emulation.md is written that way by a margin of roughly fifty to
// one, and agreeing with the evidence costs nothing. Zero padding because `0x4` and `0x00000004`
// are different map keys for the same address, which is the same failure wearing a different hat.
//
// Format with these functions; never with %x, %X or strconv directly. Where a key arrives as text
// from somewhere else — a source literal, a config value, the measured record — put it through
// NormalizeAddr rather than trusting its casing.
package hexfmt

import (
	"fmt"
	"strconv"
	"strings"
)

// digits is the canonical alphabet: upper case, so the output of every function here agrees.
const digits = "0123456789ABCDEF"

// Widths of the canonical forms, in hex digits.
const (
	byteWidth = 2
	halfWidth = 4
	wordWidth = 8
)

// Addr formats a 32-bit machine address, e.g. 0x80081C58.
//
// This is the canonical map-key form. Anything keying a map, a set or a log field by address
// should key it with this.
func Addr(a uint32) string { return encode(uint64(a), wordWidth) }

// Word formats a 32-bit value, e.g. 0x0000001C. Identical rendering to Addr; the two exist
// separately so a call site says which it means.
func Word(w uint32) string { return encode(uint64(w), wordWidth) }

// Half formats a 16-bit value, e.g. 0x1C58.
func Half(h uint16) string { return encode(uint64(h), halfWidth) }

// Byte formats an 8-bit value, e.g. 0x58.
func Byte(b uint8) string { return encode(uint64(b), byteWidth) }

// Range formats an inclusive address range, e.g. 0x80000000..0x8000FFFF.
func Range(lo, hi uint32) string { return Addr(lo) + ".." + Addr(hi) }

func encode(v uint64, width int) string {
	buf := make([]byte, 2+width)
	buf[0] = '0'
	buf[1] = 'x'
	for i := width - 1; i >= 0; i-- {
		buf[2+i] = digits[v&0xF]
		v >>= 4
	}
	return string(buf)
}

// ParseAddr reads an address written in any casing, with or without an 0x prefix.
//
// Parsing is deliberately lenient where formatting is strict: text arriving from a human, a
// config file or the measured record has whatever casing its author used, and refusing it would
// only push callers into hand-rolling the parse that this package exists to own.
func ParseAddr(s string) (uint32, error) {
	t := strings.TrimSpace(s)
	t = strings.TrimPrefix(t, "0x")
	t = strings.TrimPrefix(t, "0X")
	if t == "" {
		return 0, fmt.Errorf("hexfmt: %q is not an address: no digits", s)
	}
	v, err := strconv.ParseUint(t, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("hexfmt: %q is not a 32-bit address: %w", s, err)
	}
	return uint32(v), nil
}

// NormalizeAddr turns an address written any way into the canonical key form.
//
// Use it on every address that arrives as text and is about to be used as a lookup key. A raw
// string key skips this package entirely and reintroduces the casing mismatch it exists to stop.
func NormalizeAddr(s string) (string, error) {
	a, err := ParseAddr(s)
	if err != nil {
		return "", err
	}
	return Addr(a), nil
}
