package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInputRecordingRoundTripAndRejectsChangedEvent(t *testing.T) {
	surface := sha256.Sum256([]byte("framebuffer"))
	recording := inputRecording{
		Version: 1, Start: 10, End: 30,
		Events:        []recordedInput{{At: 10, Kind: "key", Key: 0x7d}, {At: 20, Kind: "section", PID: 0x14, Section: "707000"}},
		SurfaceSHA256: hex.EncodeToString(surface[:]),
	}
	path := filepath.Join(t.TempDir(), "inputs.json")
	if err := writeRecording(path, recording); err != nil {
		t.Fatal(err)
	}
	got, err := readRecording(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Events[0].Key != 0x7d || got.Events[1].PID != 0x14 {
		t.Fatalf("recorded inputs changed: %+v", got.Events)
	}
	got.Events[1].At = 9
	if err := got.validate(); err == nil {
		t.Fatal("accepted input before the snapshot instruction")
	}
	got.Events[1].At = 20
	got.Events[1].Section = "707001"
	if err := got.validate(); err == nil {
		t.Fatal("accepted section with a false length")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, []byte(`{"second":"object"}`)...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRecording(path); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("accepted trailing object: %v", err)
	}
}
