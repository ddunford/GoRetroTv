#!/usr/bin/env bash
set -euo pipefail
go mod edit -require=github.com/coder/websocket@v1.8.14
git add go.mod
