package main

import (
	"strings"

	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

// The trace stream predates hexfmt and is parsed as eight uppercase digits
// without 0x. Keep that wire contract while obtaining every digit from the
// canonical formatter; do not build lookup keys from this display form.
func wireWord(value uint32) string { return hexfmt.Word(value)[2:] }
func wireHalf(value uint16) string { return hexfmt.Half(value)[2:] }
func wireByte(value uint8) string  { return hexfmt.Byte(value)[2:] }

func wireRegisters(values [32]uint32) string {
	words := make([]string, len(values))
	for i, value := range values {
		words[i] = wireWord(value)
	}
	return "[" + strings.Join(words, " ") + "]"
}
