package broadcast

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

// The Sky/OpenTV EPG carries programme text Huffman-coded against a dictionary
// the box already holds in flash. Every public implementation of this format
// READS the stream; a listings feed has to WRITE one, which means encoding —
// and an encoder has a property a decoder does not, in that it can be checked
// against the decoder it is meant to feed. So both halves live here and the
// round trip is the test.
//
// The dictionary itself is not in this repository. It is third-party data of
// uncertain provenance and is handled like the firmware: local, gitignored,
// and described in dictionaries/MANIFEST.md.

// HuffmanDictionary is one loaded Sky EPG code table.
type HuffmanDictionary struct {
	codes      map[string]string // value -> bit string
	terminator string            // the empty-value node: emits nothing, ends the text
	lengths    []int             // distinct value lengths, longest first
}

// LoadHuffmanDictionary reads the `<value>=<bits>` table.
//
// The parse mirrors openTVtoXML's huffman_read_dictionary(), which tries three
// scanf patterns IN ORDER: a single character, then a multi-character phrase,
// then an empty value. That order is load-bearing rather than incidental —
// splitting naively on '=' mis-reads both the line that maps SPACE (it begins
// with one) and any phrase containing an '='. Partitioning on the FIRST '='
// reproduces all three cases.
func LoadHuffmanDictionary(path string) (*HuffmanDictionary, error) {
	file, err := os.Open(path) // #nosec G304 -- operator-supplied dictionary, like the firmware
	if err != nil {
		return nil, fmt.Errorf("broadcast: huffman dictionary: %w", err)
	}
	// Read-only, so a close error tells us nothing we can act on, but it is
	// acknowledged rather than dropped.
	defer func() { _ = file.Close() }()

	dict := &HuffmanDictionary{codes: map[string]string{}}
	seen := map[int]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		value, bits, found := strings.Cut(line, "=")
		if !found || bits == "" || strings.Trim(bits, "01") != "" {
			continue
		}
		if value == "" {
			dict.terminator = bits
			continue
		}
		dict.codes[value] = bits
		if !seen[len(value)] {
			seen[len(value)] = true
			dict.lengths = append(dict.lengths, len(value))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("broadcast: huffman dictionary: %w", err)
	}
	if dict.terminator == "" {
		// Without it an encoder cannot say where the text stops, and the box
		// would read whatever followed as more programme title.
		return nil, fmt.Errorf("broadcast: huffman dictionary %s has no terminator entry; refusing to guess one", path)
	}
	if len(dict.codes) == 0 {
		return nil, fmt.Errorf("broadcast: huffman dictionary %s is empty", path)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(dict.lengths)))
	return dict, nil
}

// Entries reports how many values the dictionary can code, so a caller can say
// which table it loaded rather than assuming.
func (d *HuffmanDictionary) Entries() int { return len(d.codes) }

// Encode packs text the way the box's decoder unpacks it.
//
// Longest match first: the dictionary holds phrases as well as characters, and
// a character-only encoding would still decode correctly but be needlessly
// long — which matters because a title section has a 12-bit length and a fixed
// number of records to fit inside it.
func (d *HuffmanDictionary) Encode(text string) ([]byte, error) {
	var bits strings.Builder
	for i := 0; i < len(text); {
		matched := false
		for _, n := range d.lengths {
			if i+n > len(text) {
				continue
			}
			if code, ok := d.codes[text[i:i+n]]; ok {
				bits.WriteString(code)
				i += n
				matched = true
				break
			}
		}
		if !matched {
			// Silently dropping it would broadcast a title that is not the one
			// asked for, which is the failure this whole package is careful
			// about elsewhere.
			return nil, fmt.Errorf("broadcast: no huffman code for %q at %d in %q", text[i], i, text)
		}
	}
	bits.WriteString(d.terminator)
	return packHuffman(bits.String()), nil
}

// packHuffman turns a bit string into the bytes the decoder expects.
//
// THE FIRST BYTE CARRIES ONLY SIX BITS. The decoder starts byte 0 at mask 0x20
// and every later byte at 0x80, so byte 0's top two bits are skipped entirely.
// It is the format, not a bug in the reference reader, and an encoder that
// packs eight bits into byte 0 produces plausible-looking output that decodes
// to nonsense — which is exactly the kind of failure this box gives no error
// for.
func packHuffman(bits string) []byte {
	out := []byte{bitsToByte(bits, 0, 6)}
	for i := 6; i < len(bits); i += 8 {
		out = append(out, bitsToByte(bits, i, 8))
	}
	return out
}

func bitsToByte(bits string, at, width int) byte {
	var value byte
	for i := 0; i < width; i++ {
		value <<= 1
		if at+i < len(bits) && bits[at+i] == '1' {
			value |= 1
		}
	}
	return value
}

// Decode is the box's decoder, reimplemented so the encoder can be proved
// against it rather than trusted. It reports whether the text terminated
// properly: a stream that runs off the tree or out of bits is an encoder bug,
// and saying "here is the text so far" without saying that would hide it.
func (d *HuffmanDictionary) Decode(data []byte) (string, bool) {
	type node struct {
		children map[byte]*node
		value    string
		isLeaf   bool
	}
	root := &node{children: map[byte]*node{}}
	insert := func(code, value string) {
		cur := root
		for i := 0; i < len(code); i++ {
			next, ok := cur.children[code[i]]
			if !ok {
				next = &node{children: map[byte]*node{}}
				cur.children[code[i]] = next
			}
			cur = next
		}
		cur.value, cur.isLeaf = value, true
	}
	for value, code := range d.codes {
		insert(code, value)
	}
	insert(d.terminator, "")

	var out strings.Builder
	cur := root
	for i, b := range data {
		mask := byte(0x80)
		if i == 0 {
			mask = 0x20
		}
		for ; mask != 0; mask >>= 1 {
			bit := byte('0')
			if b&mask != 0 {
				bit = '1'
			}
			next, ok := cur.children[bit]
			if !ok {
				return out.String(), false // ran off the tree
			}
			cur = next
			if cur.isLeaf {
				if cur.value == "" {
					return out.String(), true // the terminator
				}
				out.WriteString(cur.value)
				cur = root
			}
		}
	}
	return out.String(), false // ran out of bits without terminating
}
