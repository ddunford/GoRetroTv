package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// inputRecording stores the exact initial machine image, timed host inputs and
// observed result. The image path is deliberately local to each runner.
type inputRecording struct {
	Version        int             `json:"version"`
	SnapshotSHA256 string          `json:"snapshot_sha256,omitempty"`
	Start          uint64          `json:"start"`
	End            uint64          `json:"end"`
	SkyGates       bool            `json:"sky_gates"`
	Events         []recordedInput `json:"events"`
	SurfaceSHA256  string          `json:"surface_sha256"`
}

type recordedInput struct {
	At      uint64 `json:"at"`
	Kind    string `json:"kind"`
	Key     uint8  `json:"key,omitempty"`
	PID     uint16 `json:"pid,omitempty"`
	Section string `json:"section,omitempty"`
}

func (r inputRecording) validate() error {
	if r.Version != 1 || r.End <= r.Start {
		return fmt.Errorf("unsupported recording version or instruction range")
	}
	if r.SnapshotSHA256 != "" && !validSHA256(r.SnapshotSHA256) {
		return fmt.Errorf("invalid snapshot digest")
	}
	if !validSHA256(r.SurfaceSHA256) {
		return fmt.Errorf("invalid surface digest")
	}
	for i, event := range r.Events {
		if event.At < r.Start || event.At >= r.End || i > 0 && event.At < r.Events[i-1].At {
			return fmt.Errorf("input %d has invalid instruction count", i)
		}
		switch event.Kind {
		case "key":
			if event.PID != 0 || event.Section != "" {
				return fmt.Errorf("input %d has unexpected key fields", i)
			}
		case "section":
			bytes, err := hex.DecodeString(event.Section)
			if err != nil || event.PID > 0x1fff || len(bytes) < 3 || int(bytes[1]&15)<<8|int(bytes[2]) != len(bytes)-3 || event.Key != 0 {
				return fmt.Errorf("input %d has invalid DVB section", i)
			}
		default:
			return fmt.Errorf("input %d has unknown kind %q", i, event.Kind)
		}
	}
	return nil
}

func validSHA256(value string) bool {
	bytes, err := hex.DecodeString(value)
	return err == nil && len(bytes) == sha256.Size && hex.EncodeToString(bytes) == value
}

func readRecording(path string) (inputRecording, error) {
	f, err := os.Open(path) // #nosec G304 -- caller explicitly names this local recording.
	if err != nil {
		return inputRecording{}, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return inputRecording{}, err
	}
	if info.Size() > 1<<20 {
		return inputRecording{}, fmt.Errorf("recording exceeds 1 MiB")
	}
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	var result inputRecording
	if err := decoder.Decode(&result); err != nil {
		return result, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return result, fmt.Errorf("recording has trailing data")
	}
	return result, result.validate()
}

func writeRecording(path string, recording inputRecording) error {
	if err := recording.validate(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".goretrotv-recording-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := f.Chmod(0600); err != nil {
		return errors.Join(err, f.Close())
	}
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(recording); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := f.Sync(); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func fileSHA256(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	f, err := os.Open(path) // #nosec G304 -- caller explicitly names this local snapshot.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
