#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "${ROOT_DIR}/assets"
echo "copied wasm_exec.js."

cd "${ROOT_DIR}/cmd/wasm"
GOOS=js GOARCH=wasm go build -o "${ROOT_DIR}/assets/json.wasm"

echo "success build wasm files"
