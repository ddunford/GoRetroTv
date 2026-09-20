package broadcast

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// testDictionary loads the local Sky EPG dictionary, or skips. It is
// third-party data handled like the firmware: required, gitignored, never
// committed. See dictionaries/MANIFEST.md.
func testDictionary(t *testing.T) *HuffmanDictionary {
	t.Helper()
	path := filepath.Join("..", "..", "dictionaries", "skyuk.dict")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("the Sky EPG huffman dictionary is not installed; see dictionaries/MANIFEST.md")
	}
	dict, err := LoadHuffmanDictionary(path)
	if err != nil {
		t.Fatal(err)
	}
	if dict.Entries() != 446 {
		t.Fatalf("dictionary has %d entries, want the 446 the MANIFEST records", dict.Entries())
	}
	return dict
}

// Byte-for-byte against the Python codec this is a port of, which was itself
// validated by round-tripping through a transcription of the reference
// DECODER. Pinning the bytes rather than only the round trip matters: an
// encoder can be self-consistently wrong — pack eight bits into byte 0 instead
// of six and encode/decode still agree with each other while the box reads
// nonsense.
func TestHuffmanMatchesTheValidatedEncoder(t *testing.T) {
	t.Parallel()
	dict := testDictionary(t)
	for _, tc := range []struct{ text, want string }{
		{"The Simpsons", "2ae3015ab8c32a256b1c2ae3069010"},
		{"Sky News", "04c046ae30cad5c61fc010"},
		{"Football", "2a866dc2694100"},
		{"News at Ten", "2ae30fe055c619596ab8c32a2fdc40"},
		{" leading space", "2ae30cad3e53571829475718654079e7c400"},
		{"A", "388400"},
	} {
		got, err := dict.Encode(tc.text)
		if err != nil {
			t.Fatalf("%q: %v", tc.text, err)
		}
		if hex.EncodeToString(got) != tc.want {
			t.Errorf("%q\n got %s\nwant %s", tc.text, hex.EncodeToString(got), tc.want)
		}
	}
}

// The round trip is what proves the encoder against the decoder it feeds, and
// the termination flag is half of it: text that decodes correctly but never
// terminates would let the box read whatever followed as more of the title.
func TestHuffmanRoundTripsAndTerminates(t *testing.T) {
	t.Parallel()
	dict := testDictionary(t)
	for _, text := range []string{
		"The Simpsons", "Sky News", "Football", "News at Ten", "The X-Files",
		"Coronation Street", "A", " leading space", "Digits 1998",
	} {
		encoded, err := dict.Encode(text)
		if err != nil {
			t.Fatalf("%q: %v", text, err)
		}
		got, terminated := dict.Decode(encoded)
		if got != text {
			t.Errorf("%q round-tripped to %q", text, got)
		}
		if !terminated {
			t.Errorf("%q encoded without a terminator", text)
		}
	}
}

// The first byte carries SIX bits, not eight. This is the trap the format sets
// and the reason the check above pins bytes: an encoder that fills byte 0
// produces output that is self-consistent and wrong.
func TestHuffmanFirstByteCarriesSixBits(t *testing.T) {
	t.Parallel()
	dict := testDictionary(t)
	encoded, err := dict.Encode("A")
	if err != nil {
		t.Fatal(err)
	}
	if encoded[0]&0xc0 != 0 {
		t.Fatalf("byte 0 = %#02x, but its top two bits are skipped by the decoder and must be clear", encoded[0])
	}
}

func TestHuffmanRefusesWhatItCannotSay(t *testing.T) {
	t.Parallel()
	dict := testDictionary(t)
	// A character with no code must be an error rather than a silently
	// shortened title: broadcasting a programme name that is not the one asked
	// for is worse than refusing to broadcast it.
	if _, err := dict.Encode("Café — ü"); err == nil {
		t.Error("encoded a string the dictionary cannot express")
	}
}
