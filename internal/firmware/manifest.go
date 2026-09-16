// Package firmware loads the machine's flash images and refuses to start on any image that is not
// the one firmware/MANIFEST.md describes.
//
// The point is not tamper-proofing. It is that running a DIFFERENT ROM silently is the worst
// failure available to this project: the machine boots, behaves almost right, and every divergence
// from the oracle afterwards reads as a bug in the port. Six thousand lines of measured addresses
// in docs/reference/digibox-emulation.md are expressed against these exact bytes, so an image that
// is nearly them invalidates all of it without saying so.
//
// The manifest is parsed rather than compiled in, so there is one record of what the images are
// rather than two that can drift. That makes the parser part of the guarantee: a manifest it
// cannot understand is an error, never an empty list of images that verifies nothing.
package firmware

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ManifestFile is the manifest's name within the firmware directory.
const ManifestFile = "MANIFEST.md"

// Image is one row of the manifest: an image the machine needs and the digests it must have.
type Image struct {
	File   string
	Bytes  int64
	SHA256 string // lower-case hex, 64 characters
	MD5    string // lower-case hex, 32 characters
	Notes  string
}

// Manifest is everything firmware/MANIFEST.md says about the images.
type Manifest struct {
	// Path is where the manifest was read from, so an error can say which manifest it means.
	Path   string
	Images []Image
}

// Image returns the entry for file.
func (m *Manifest) Image(file string) (Image, bool) {
	for _, img := range m.Images {
		if img.File == file {
			return img, true
		}
	}
	return Image{}, false
}

// Files lists the images the manifest describes, in the order it describes them.
func (m *Manifest) Files() []string {
	out := make([]string, 0, len(m.Images))
	for _, img := range m.Images {
		out = append(out, img.File)
	}
	return out
}

var (
	hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// The columns the parser requires, in order. They are the format: reordering the table in the
// manifest changes what this reads, which is why a mismatched header is an error rather than a
// best guess at which column is which.
var manifestColumns = []string{"file", "bytes", "sha-256", "md5", "what it is"}

// ReadManifest reads and parses the manifest at path.
func ReadManifest(path string) (*Manifest, error) {
	f, err := os.Open(path) //#nosec G304 -- the path is the operator's firmware directory, given deliberately
	if err != nil {
		return nil, fmt.Errorf("firmware: manifest: %w", err)
	}
	// The manifest is only read, so a close error says nothing a parse error would not.
	defer func() { _ = f.Close() }()
	return ParseManifest(path, f)
}

// ParseManifest reads a manifest from r. path is used only in error messages.
//
// It is deliberately strict. Every failure below would otherwise become a manifest that parses to
// nothing and a verification step that passes because it checked nothing - the shape of harness
// failure this project has shipped before and the reason this file asserts its own subject.
func ParseManifest(path string, r io.Reader) (*Manifest, error) {
	m := &Manifest{Path: path}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)

	inTable := false
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(text, "|") {
			if inTable {
				break // the table has ended; there is only one
			}
			continue
		}

		cells := tableRow(text)
		switch {
		case !inTable:
			if !headerMatches(cells) {
				continue // a table, but not the one this file is about
			}
			inTable = true
		case isSeparator(cells):
			continue
		default:
			img, err := parseRow(path, line, cells)
			if err != nil {
				return nil, err
			}
			m.Images = append(m.Images, img)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("firmware: manifest %s: %w", path, err)
	}

	if !inTable {
		return nil, fmt.Errorf("firmware: manifest %s: no image table - it must have a row of "+
			"headers reading | %s |, and without one this file verifies nothing",
			path, strings.Join(manifestColumns, " | "))
	}
	if len(m.Images) == 0 {
		return nil, fmt.Errorf("firmware: manifest %s: the image table has a header and no rows, "+
			"so there is nothing to verify against", path)
	}

	seen := make(map[string]int, len(m.Images))
	for i, img := range m.Images {
		if first, dup := seen[img.File]; dup {
			return nil, fmt.Errorf("firmware: manifest %s: %q appears twice, in rows %d and %d, "+
				"and the two rows could disagree", path, img.File, first+1, i+1)
		}
		seen[img.File] = i
	}
	return m, nil
}

func tableRow(text string) []string {
	text = strings.TrimPrefix(text, "|")
	text = strings.TrimSuffix(text, "|")
	cells := strings.Split(text, "|")
	for i, c := range cells {
		cells[i] = strings.Trim(strings.TrimSpace(c), "`")
	}
	return cells
}

func headerMatches(cells []string) bool {
	if len(cells) != len(manifestColumns) {
		return false
	}
	for i, want := range manifestColumns {
		if !strings.EqualFold(strings.TrimSpace(cells[i]), want) {
			return false
		}
	}
	return true
}

func isSeparator(cells []string) bool {
	for _, c := range cells {
		if strings.Trim(c, "-: ") != "" {
			return false
		}
	}
	return len(cells) > 0
}

func parseRow(path string, line int, cells []string) (Image, error) {
	where := fmt.Sprintf("firmware: manifest %s line %d", path, line)
	if len(cells) != len(manifestColumns) {
		return Image{}, fmt.Errorf("%s: %d columns, want %d (%s)",
			where, len(cells), len(manifestColumns), strings.Join(manifestColumns, ", "))
	}

	img := Image{
		File:   strings.TrimSpace(cells[0]),
		SHA256: strings.ToLower(strings.TrimSpace(cells[2])),
		MD5:    strings.ToLower(strings.TrimSpace(cells[3])),
		Notes:  strings.TrimSpace(cells[4]),
	}
	if img.File == "" {
		return Image{}, fmt.Errorf("%s: the file column is empty", where)
	}
	if strings.ContainsAny(img.File, `/\`) {
		return Image{}, fmt.Errorf("%s: %q is a path, and the manifest names files inside the "+
			"firmware directory", where, img.File)
	}

	size, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(cells[1]), ",", ""), 10, 64)
	if err != nil {
		return Image{}, fmt.Errorf("%s: %s: byte count %q is not a number: %w",
			where, img.File, cells[1], err)
	}
	if size <= 0 {
		return Image{}, fmt.Errorf("%s: %s: byte count is %d", where, img.File, size)
	}
	img.Bytes = size

	// A truncated digest is the specific regression this rejects by name. The manifest carried
	// "7541fb4884d7…" for months: it reads as a record, and it cannot verify anything.
	if !hex64.MatchString(img.SHA256) {
		return Image{}, fmt.Errorf("%s: %s: SHA-256 is %q, and a digest that is not 64 hex "+
			"characters cannot verify an image - if it is abbreviated, record it in full",
			where, img.File, cells[2])
	}
	if !hex32.MatchString(img.MD5) {
		return Image{}, fmt.Errorf("%s: %s: MD5 is %q, and a digest that is not 32 hex "+
			"characters cannot verify an image - if it is abbreviated, record it in full",
			where, img.File, cells[3])
	}
	return img, nil
}
