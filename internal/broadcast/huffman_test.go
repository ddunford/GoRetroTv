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
	if dict.Entries() != 447 {
		t.Fatalf("dictionary has %d entries, want the 447 the MANIFEST records", dict.Entries())
	}
	return dict
}

// Byte-for-byte, so that a change to the encoder has to be deliberate.
//
// THESE VECTORS WERE WRONG UNTIL 2026-09-20, and the way they were wrong is
// the lesson. They were taken from a Python codec "validated by round-tripping
// through a transcription of the reference decoder" — which is an encoder
// checked against a decoder, the exact self-consistency the old comment here
// warned about while relying on it. Both halves shared two misreadings, so
// they agreed perfectly and the box did not:
//
//   - a space was emitted as one of the dictionary's 27-bit filler leaves
//     instead of its real three-bit code, and every title lost its spaces;
//   - the last byte was zero-filled, and `s` is coded `0000`, so every title
//     gained a trailing "s".
//
// The authority for the current vectors is THE BOX'S SCREEN: with them, the
// guide draws "Dream Team" and "Walker Texas Ranger"; with the old ones it drew
// "DreamTeams" and "WalkerTexasRangers". Pinning the bytes is still worth doing
// — an encoder can be self-consistently wrong in other ways, and packing eight
// bits into byte 0 instead of six is the classic — but a round trip must never
// again be mistaken for validation.
func TestHuffmanMatchesTheValidatedEncoder(t *testing.T) {
	t.Parallel()
	dict := testDictionary(t)
	for _, tc := range []struct{ text, want string }{
		{"The Simpsons", "2ae3015c256b1c2ae3069010"},
		{"Sky News", "04c04755c61fc010"},
		{"Football", "2a866dc2694102"},
		{"News at Ten", "2ae30fe0696c2fdc40"},
		{" leading space", "353e53571829478079e7c408"},
		{"A", "388408"},
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
