#!/usr/bin/env bash
set -euo pipefail
go mod edit -droprequire=github.com/coder/websocket
git add go.mod
