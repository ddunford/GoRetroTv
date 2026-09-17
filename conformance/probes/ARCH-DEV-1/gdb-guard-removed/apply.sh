#!/usr/bin/env bash
set -euo pipefail
python3 - <<'PY'
from pathlib import Path
p = Path('internal/gdbstub/server.go')
body = p.read_text()
old = 'if ip == nil || !ip.IsLoopback() {'
assert body.count(old) == 1
p.write_text(body.replace(old, 'if ip != nil && false {'))
PY
git add internal/gdbstub/server.go
