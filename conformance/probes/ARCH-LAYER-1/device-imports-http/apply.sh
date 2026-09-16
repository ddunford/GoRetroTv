#!/usr/bin/env bash
set -euo pipefail
mkdir -p internal/device/demux
cat >internal/device/demux/outward_import_probe.go <<'GO'
package demux

import _ "github.com/ddunford/goretrotv/internal/httpx"
GO
git add internal/device/demux/outward_import_probe.go
