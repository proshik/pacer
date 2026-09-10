#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 1. Browser bundle: js/wasm, loaded by assets/index.html through wasm_exec.js.
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "${ROOT_DIR}/assets"
echo "copied wasm_exec.js."

GOOS=js GOARCH=wasm go build -o "${ROOT_DIR}/assets/json.wasm" "${ROOT_DIR}/cmd/wasm"
echo "built assets/json.wasm (browser)"

# 2. Server module: a WASI reactor the server runs through wazero when
#    CALC_ENGINE=wasm. Built with -buildmode=c-shared so it exports
#    _initialize and stays callable, instead of exiting like a command.
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared \
	-o "${ROOT_DIR}/pkg/wasmcalc/calc.wasm" "${ROOT_DIR}/cmd/calcwasm"
echo "built pkg/wasmcalc/calc.wasm (server reactor)"

echo "success build wasm files"
