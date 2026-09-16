#!/usr/bin/env bash
set -euo pipefail
python3 - <<'PY'
from pathlib import Path
p = Path('docker-compose.yml')
body = p.read_text()
old = '"127.0.0.1:${GORETROTV_PORT:-8099}:8099"'
assert body.count(old) == 1
p.write_text(body.replace(old, '"0.0.0.0:${GORETROTV_PORT:-8099}:8099"'))
PY
git add docker-compose.yml
