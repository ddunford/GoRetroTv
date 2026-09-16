#!/usr/bin/env bash
set -euo pipefail
python3 - <<'PY'
from pathlib import Path
image = bytearray(2 * 1024 * 1024)
image[0x12584:0x12588] = bytes.fromhex('4a42a007')
Path('flash-probe.bin').write_bytes(image)
p = Path('Dockerfile')
p.write_text(p.read_text() + '\nCOPY flash-probe.bin /flash-fragment\n')
PY
git add Dockerfile
