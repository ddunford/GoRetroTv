package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddunford/goretrotv/internal/machine"
)

func writeSnapshot(path string, m *machine.Machine) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".goretrotv-snapshot-*")
	if err != nil {
		return fmt.Errorf("create snapshot in %s: %w", dir, err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := f.Chmod(0600); err != nil {
		return errors.Join(fmt.Errorf("protect snapshot file: %w", err), f.Close())
	}
	if err := m.Snapshot(f); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := f.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync snapshot: %w", err), f.Close())
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close snapshot: %w", err)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("install snapshot at %s: %w", path, err)
	}
	return nil
}

func loadSnapshot(path string, m *machine.Machine) error {
	f, err := os.Open(path) // #nosec G304 -- the developer explicitly names this local snapshot file.
	if err != nil {
		return fmt.Errorf("open snapshot %s: %w", path, err)
	}
	return errors.Join(m.Restore(f), f.Close())
}
