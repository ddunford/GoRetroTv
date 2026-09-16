#!/usr/bin/env bash
set -euo pipefail
cat >internal/platform/hexfmt/domain_import_probe.go <<'GO'
package hexfmt

import _ "github.com/ddunford/goretrotv/internal/device/demux"
GO
git add internal/platform/hexfmt/domain_import_probe.go
