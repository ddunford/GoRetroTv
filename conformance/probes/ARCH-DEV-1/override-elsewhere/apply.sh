#!/usr/bin/env bash
set -euo pipefail
mkdir -p ops
cat >ops/open-bind.sh <<'SH'
#!/usr/bin/env bash
export GORETROTV_BIND_ALL_INTERFACES=true
SH
git add ops/open-bind.sh
