#!/usr/bin/env bash
set -euo pipefail
cat >internal/memory/untested_probe.go <<'GO'
package memory

import "github.com/ddunford/goretrotv/internal/bus"

type UntestedProbe struct{ sequence uint8 }

func (*UntestedProbe) Name() string { return "untested" }
func (*UntestedProbe) Read(uint32, bus.Size) uint32 { return 0 }
func (*UntestedProbe) Write(uint32, bus.Size, uint32) {}
func (*UntestedProbe) Reset() {}
func (*UntestedProbe) Snapshot() ([]byte, error) { return []byte{0}, nil }
func (*UntestedProbe) Restore([]byte) error { return nil }
GO
git add internal/memory/untested_probe.go
