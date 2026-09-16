#!/usr/bin/env bash
set -euo pipefail
python3 - <<'PY'
from pathlib import Path
p = Path('internal/memory/ram.go')
body = p.read_text()
old = 'type RAM struct {\n'
assert body.count(old) == 1
p.write_text(body.replace(old, 'type RAM struct {\n\tUnserializedMarker uint32\n'))
PY
git add internal/memory/ram.go
