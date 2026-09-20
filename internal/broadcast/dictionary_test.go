package broadcast_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddunford/goretrotv/internal/broadcast"
)

// The two things about this table that an encoder has to know, and that a
// round-trip test against our own decoder cannot see -- because an encoder and
// a decoder that share a misreading agree with each other perfectly.
//
// Both of these shipped. Every title on the guide rendered without its spaces
// for as long as titles have been broadcast at all.
func TestTheDictionaryIsReadTheWayAnEncoderNeedsIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "table.dict")
	// A value with several codes, shortest NOT last; the equals character,
	// whose line has '=' in the value position; and a terminator, also with a
	// filler duplicate after it.
	body := " =110\n" +
		" =1110111\n" +
		" =101010111000110000110010101\n" +
		"==1011\n" +
		"A=1111\n" +
		"=0001000\n" +
		"=101010111000110000101101110\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	dict, err := broadcast.LoadHuffmanDictionary(path)
	if err != nil {
		t.Fatal(err)
	}

	// A VALUE'S REAL CODE IS THE SHORTEST ONE. The long duplicates are the
	// flattened tree's padding, and the box does not decode them back to the
	// value they are listed against -- so emitting one puts a symbol on the
	// wire that comes back as something else, or as nothing.
	//
	// "A A" is three symbols. With space at three bits and A at four, that is
	// 3+4+3+4 = 14 bits of text plus a 7-bit terminator: three bytes. With the
	// 27-bit filler it would be more than eight.
	encoded, err := dict.Encode("A A")
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 3 {
		t.Errorf("%q encoded to %d bytes; the shortest code for a space is 3 bits, so this is "+
			"using one of the long filler entries and the box will not read it back as a space",
			"A A", len(encoded))
	}

	// THE EQUALS CHARACTER IS A VALUE, not a malformed empty one. Cutting at
	// the first '=' drops it, and a title containing one then cannot be
	// encoded at all.
	if _, err := dict.Encode("A=A"); err != nil {
		t.Errorf("a title containing '=' could not be encoded: %v", err)
	}

	// The terminator takes the same shortest-wins rule.
	if _, err := dict.Encode("A"); err != nil {
		t.Fatal(err)
	}
	if got, want := dict.Entries(), 3; got != want { // space, "=", "A"
		t.Errorf("loaded %d values, want %d", got, want)
	}
}

// The real table, when it is installed: space must come out as its three-bit
// code. This is the specific regression -- 62 of the 65 space entries in the
// Sky table are 27-bit filler, and taking the last one is what dropped every
// space on screen.
func TestTheRealTableGivesSpaceItsShortCode(t *testing.T) {
	path := filepath.Join("..", "..", "dictionaries", "skyuk.dict")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("the Sky EPG huffman dictionary is not installed; see dictionaries/MANIFEST.md")
	}
	dict, err := broadcast.LoadHuffmanDictionary(path)
	if err != nil {
		t.Fatal(err)
	}
	// "Dream Team" is ten characters with one space. Coded well it is a
	// handful of bytes; with a 27-bit space it is half as long again.
	encoded, err := dict.Encode("Dream Team")
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 10 {
		t.Errorf("%q encoded to %d bytes, which is longer than the text itself -- the space is "+
			"being emitted as a filler code", "Dream Team", len(encoded))
	}
	if text, ok := dict.Decode(encoded); !ok || text != "Dream Team" {
		t.Errorf("round trip gave %q ok=%v", text, ok)
	}
}

// The padding path's other end: a text whose code lands EXACTLY on a byte
// boundary needs no padding at all, and must not acquire a spare byte of it.
//
// It is here because the fix for the trailing "s" is a loop that fills the tail
// of the last byte, and a loop like that is most likely to be wrong when there
// is nothing to do. "NNNNNNN" is seventy bits in the Sky table -- six in the
// first byte and sixty-four after it -- which is the case the six reference
// vectors happen not to cover.
func TestATextThatEndsOnAByteBoundaryGainsNoPadding(t *testing.T) {
	path := filepath.Join("..", "..", "dictionaries", "skyuk.dict")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("the Sky EPG huffman dictionary is not installed; see dictionaries/MANIFEST.md")
	}
	dict, err := broadcast.LoadHuffmanDictionary(path)
	if err != nil {
		t.Fatal(err)
	}
	const text = "NNNNNNN" // seventy bits including the terminator
	encoded, err := dict.Encode(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 9 { // one six-bit byte plus eight
		t.Errorf("%q encoded to %d bytes, want 9; a text that fills its last byte exactly must "+
			"not gain another one of padding", text, len(encoded))
	}
	if back, ok := dict.Decode(encoded); !ok || back != text {
		t.Errorf("round trip gave %q ok=%v", back, ok)
	}
}
