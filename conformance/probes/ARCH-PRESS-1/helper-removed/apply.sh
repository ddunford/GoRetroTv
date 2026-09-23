#!/usr/bin/env bash
set -euo pipefail
git mv internal/multiplex/firmwaretests/rununtil_test.go \
       internal/multiplex/firmwaretests/rununtil_moved_by_probe.go.txt
