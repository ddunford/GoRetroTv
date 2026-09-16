#!/usr/bin/env bash
set -euo pipefail
cat >internal/memory/unserialized_probe.go <<'GO'
package memory

import "github.com/ddunford/goretrotv/internal/bus"

type UnserializedProbe struct{ sequence uint8 }

func (*UnserializedProbe) Name() string { return "unserialized" }
func (*UnserializedProbe) Read(uint32, bus.Size) uint32 { return 0 }
func (*UnserializedProbe) Write(uint32, bus.Size, uint32) {}
func (*UnserializedProbe) Reset() {}
GO
git add internal/memory/unserialized_probe.go
