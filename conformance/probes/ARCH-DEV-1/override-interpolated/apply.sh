#!/usr/bin/env bash
set -euo pipefail
python3 - <<'PY'
from pathlib import Path
p = Path('docker-compose.yml')
body = p.read_text()
old = 'GORETROTV_BIND_ALL_INTERFACES: "true"'
assert body.count(old) == 1
p.write_text(body.replace(old, 'GORETROTV_BIND_ALL_INTERFACES: ${GORETROTV_BIND_ALL_INTERFACES:-true}'))
PY
git add docker-compose.yml
