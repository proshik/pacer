#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${ROOT_DIR}"

# 1. Browser bundle: js/wasm, loaded by assets/index.html through wasm_exec.js.
#    TinyGo by default: the same code comes out about five times smaller than
#    with the standard toolchain (0.9 MB against 4.5 MB on Go 1.27), with
#    byte-identical results. WASM_COMPILER=go builds with the standard
#    toolchain instead. wasm_exec.js always comes from the same toolchain as
#    json.wasm: the two runtimes' glue files are not interchangeable.
WASM_COMPILER="${WASM_COMPILER:-tinygo}"

case "${WASM_COMPILER}" in
tinygo)
	if ! command -v tinygo >/dev/null 2>&1; then
		echo "tinygo not found. Install it (brew tap tinygo-org/tools && brew install tinygo)" >&2
		echo "or build with the standard toolchain: WASM_COMPILER=go ./build.sh" >&2
		exit 1
	fi
	cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" assets/wasm_exec.js
	tinygo build -target=wasm -no-debug -o assets/json.wasm ./cmd/wasm
	;;
go)
	cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" assets/wasm_exec.js
	GOOS=js GOARCH=wasm go build -trimpath -buildvcs=false -o assets/json.wasm ./cmd/wasm
	;;
*)
	echo "unknown WASM_COMPILER=${WASM_COMPILER}, want tinygo or go" >&2
	exit 1
	;;
esac
echo "built assets/json.wasm (browser, ${WASM_COMPILER})"

# 2. Server module: a WASI reactor the server runs through wazero when
#    CALC_ENGINE=wasm. Built with the standard toolchain and -buildmode=c-shared
#    so it exports _initialize and stays callable instead of exiting.
#    -buildvcs=false and -trimpath keep the committed artifact reproducible:
#    otherwise Go stamps the current git revision into it, and the file changes
#    with every commit even when no code did.
GOOS=wasip1 GOARCH=wasm go build -trimpath -buildvcs=false -buildmode=c-shared \
	-o pkg/wasmcalc/calc.wasm ./cmd/calcwasm
echo "built pkg/wasmcalc/calc.wasm (server reactor)"

echo "success build wasm files"
