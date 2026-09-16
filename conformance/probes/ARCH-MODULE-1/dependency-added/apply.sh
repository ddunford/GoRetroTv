#!/usr/bin/env bash
set -euo pipefail
go mod edit -require=github.com/google/uuid@v1.6.0
git add go.mod
