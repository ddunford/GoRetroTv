// Package eeprom models the Digibox's 24C128 nonvolatile 16 KiB store.
package eeprom

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Capacity is the EEPROM image size in bytes.
	Capacity = 16 * 1024
	// PageSize is the write page, within which its address wraps.
	PageSize = 64
)

// Store owns the EEPROM bytes. A fresh chip reads as 0xff.
type Store struct{ image [Capacity]byte }

// New returns a blank 24C128.
func New() *Store { s := &Store{}; s.Reset(); return s }

// Name is the snapshot identity.
func (*Store) Name() string { return "24c128" }

// Read returns a byte at the 14-bit chip address.
func (s *Store) Read(off uint32, _ bus.Size) uint32 { return uint32(s.image[off&(Capacity-1)]) }

// Write programs a byte at the 14-bit chip address.
func (s *Store) Write(off uint32, _ bus.Size, value uint32) { s.image[off&(Capacity-1)] = byte(value) }

// Reset returns the chip to blank state for the device contract.
func (s *Store) Reset() {
	for i := range s.image {
		s.image[i] = 0xff
	}
}

// Image returns a copy of the complete nonvolatile contents.
func (s *Store) Image() []byte { return append([]byte(nil), s.image[:]...) }

// Load reads an exact-size image. A missing file leaves a factory-blank chip.
func (s *Store) Load(path string) error {
	if path == "" {
		return fmt.Errorf("eeprom: image path is empty")
	}
	// #nosec G304 -- path is the explicit NVRAM image chosen by the caller.
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		s.Reset()
		return nil
	}
	if err != nil {
		return fmt.Errorf("eeprom: load %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, Capacity+1))
	if err != nil {
		return fmt.Errorf("eeprom: load %s: %w", path, err)
	}
	if len(data) != Capacity {
		if len(data) > Capacity {
			return fmt.Errorf("eeprom: image %s exceeds %d bytes", path, Capacity)
		}
		return fmt.Errorf("eeprom: image %s has %d bytes; need %d", path, len(data), Capacity)
	}
	copy(s.image[:], data)
	return nil
}

// Persist atomically replaces the exact-size image after a completed write transaction.
func (s *Store) Persist(path string) error {
	if path == "" {
		return fmt.Errorf("eeprom: image path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("eeprom: create image directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".nvram-*")
	if err != nil {
		return fmt.Errorf("eeprom: create temporary image: %w", err)
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(s.image[:]); err != nil {
		_ = f.Close()
		return fmt.Errorf("eeprom: write image: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("eeprom: sync image: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("eeprom: close image: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("eeprom: replace image: %w", err)
	}
	return nil
}

// Snapshot encodes the entire chip, including its factory-blank bytes.
func (s *Store) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(s.Name(), 1)
	w.Bytes(s.image[:])
	return w.Blob()
}

// Restore rejects incomplete images before replacing chip contents.
func (s *Store) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("eeprom: restore: %w", err)
	}
	if err := r.Expect(s.Name(), 1, 1); err != nil {
		return fmt.Errorf("eeprom: restore: %w", err)
	}
	image := r.Bytes()
	if err := r.Done(); err != nil {
		return fmt.Errorf("eeprom: restore: %w", err)
	}
	if len(image) != Capacity {
		return fmt.Errorf("eeprom: restore: image has %d bytes", len(image))
	}
	copy(s.image[:], image)
	return nil
}
