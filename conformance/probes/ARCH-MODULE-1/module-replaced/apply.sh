#!/usr/bin/env bash
set -euo pipefail
go mod edit -replace=github.com/coder/websocket@v1.8.15=github.com/coder/websocket@v1.8.14
git add go.mod
