#!/usr/bin/env bash
set -euo pipefail
cat >internal/multiplex/firmwaretests/press_loop_probe_test.go <<'GO'
package firmwaretests_test

import "testing"

// A probe that settles a screen by hand, which is the thing ARCH-PRESS-1 exists to stop.
func TestHandRolledPressProbe(t *testing.T) {
	stable, last, settled := 0, uint32(0), uint32(0)
	for i := 0; i < 10; i++ {
		now := screenNow(t, nil)
		if now == last {
			stable++
			settled = now
			if stable >= 4 {
				break
			}
			continue
		}
		stable, last = 0, now
	}
	_ = settled
}
GO
git add internal/multiplex/firmwaretests/press_loop_probe_test.go
