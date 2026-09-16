#!/usr/bin/env bash
set -euo pipefail
mkdir -p internal/cpu
cat >internal/cpu/wall_clock_probe.go <<'GO'
package cpu

import "time"

func clockProbe() time.Time { return time.Now() }
GO
git add internal/cpu/wall_clock_probe.go
