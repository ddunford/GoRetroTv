#!/usr/bin/env bash
set -euo pipefail
mkdir -p tests/fixtures
python3 - <<'PY'
from pathlib import Path
image = bytearray(2 * 1024 * 1024)
image[0x12584:0x12588] = bytes.fromhex('4a42a007')
Path('tests/fixtures/flash-probe.bin').write_bytes(image)
PY
git add tests/fixtures/flash-probe.bin
