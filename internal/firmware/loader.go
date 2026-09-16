package firmware

import (
	"context"
	"crypto/md5" //#nosec G501 -- provenance identifier, not a security control; see hashes
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The images the machine needs, named as the manifest names them.
const (
	FileU202           = "FLASH_U202.bin"
	FileU203           = "FLASH_U203.bin"
	FileApplicationRAM = "application-ram-image.bin"
)

// Required is the set of images without which nothing here runs. A manifest that does not describe
// all three is a manifest that would let the machine start half-verified.
var Required = []string{FileU202, FileU203, FileApplicationRAM}

// Set is firmware that has been read and verified.
type Set struct {
	// Dir is where it came from.
	Dir string

	// Manifest is what it was verified against.
	Manifest *Manifest

	// U202 holds the reset vector and the application image; U203 is the second flash part.
	U202 []byte
	U203 []byte

	// ApplicationRAM is the application as the bootloader decompresses it, based at 0x800009F4.
	// It is a capture rather than part of the machine - it is kept because every Ghidra seed and
	// every address in the record is expressed against this exact copy.
	ApplicationRAM []byte
}

// Load reads every image the manifest in dir describes and verifies each one against it.
//
// It refuses on the first thing that is wrong about the SET rather than the first thing wrong with
// a file: all mismatches are reported together, because an operator with three wrong images should
// learn that once rather than three times. A refusal here must stop the machine starting. Running
// a different ROM is not a degraded mode, it is a machine whose every later disagreement with the
// oracle is unattributable.
func Load(ctx context.Context, dir string) (*Set, error) {
	manifest, err := ReadManifest(filepath.Join(dir, ManifestFile))
	if err != nil {
		return nil, err
	}

	var missing []string
	for _, want := range Required {
		if _, ok := manifest.Image(want); !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("firmware: manifest %s describes %v and says nothing about %v, so "+
			"loading it would verify only part of the machine",
			manifest.Path, manifest.Files(), missing)
	}

	set := &Set{Dir: dir, Manifest: manifest}
	var problems []error
	contents := make(map[string][]byte, len(manifest.Images))
	for _, img := range manifest.Images {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("firmware: loading %s: %w", dir, err)
		}
		data, err := verify(dir, img)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		contents[img.File] = data
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("firmware: refusing to start on the images in %s: %w",
			dir, errors.Join(problems...))
	}

	set.U202 = contents[FileU202]
	set.U203 = contents[FileU203]
	set.ApplicationRAM = contents[FileApplicationRAM]
	return set, nil
}

// verify reads one image and checks it against its manifest row, naming what does not match.
func verify(dir string, img Image) ([]byte, error) {
	path := filepath.Join(dir, img.File)

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", img.File, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s: is a directory", img.File)
	}
	// Size before contents, so a file that is nothing like the image is refused without reading
	// it, and so the error says the useful thing rather than "the digest differs".
	if info.Size() != img.Bytes {
		return nil, fmt.Errorf("%s: is %d bytes and the manifest records %d",
			img.File, info.Size(), img.Bytes)
	}

	data, err := os.ReadFile(path) //#nosec G304 -- the manifest names the file and its size is checked above
	if err != nil {
		return nil, fmt.Errorf("%s: %w", img.File, err)
	}
	if int64(len(data)) != img.Bytes {
		return nil, fmt.Errorf("%s: changed size while being read: %d bytes, manifest records %d",
			img.File, len(data), img.Bytes)
	}

	gotSHA, gotMD5 := hashes(data)
	if gotSHA != img.SHA256 {
		return nil, fmt.Errorf("%s: SHA-256 is %s and the manifest records %s - this is not the "+
			"image the measured record was written against", img.File, gotSHA, img.SHA256)
	}
	if gotMD5 != img.MD5 {
		return nil, fmt.Errorf("%s: MD5 is %s and the manifest records %s", img.File, gotMD5, img.MD5)
	}
	return data, nil
}

// hashes returns an image's SHA-256 and MD5, both lower-case hex.
//
// SHA-256 is the integrity check. MD5 is here because it is how these images are identified
// outside this repository and inside our own record - docs/reference/digibox-emulation.md opens by
// naming FLASH_U202.bin as md5 7541fb4884d72b03858f7175217c2177, Colibri's published JTAG dump of
// a Pace 2500N - so dropping it would break the only link between these bytes and where they came
// from. It is a provenance identifier and nothing here treats it as a security control.
func hashes(data []byte) (sha, sum string) {
	s := sha256.Sum256(data)
	m := md5.Sum(data) //#nosec G401 -- provenance identifier; SHA-256 above is the integrity check
	return hex.EncodeToString(s[:]), hex.EncodeToString(m[:])
}
