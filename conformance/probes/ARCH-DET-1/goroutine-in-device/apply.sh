#!/usr/bin/env bash
set -euo pipefail
mkdir -p internal/device
cat >internal/device/goroutine_probe.go <<'GO'
package device

func goroutineProbe() { go func() {}() }
GO
git add internal/device/goroutine_probe.go
