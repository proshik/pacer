#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${ROOT_DIR}"

# 1. Browser bundle: js/wasm, loaded by assets/index.html through wasm_exec.js.
#    TinyGo by default: the same code comes out about ten times smaller than
#    with the standard toolchain (0.4 MB against 4.5 MB), with byte-identical
#    results. WASM_COMPILER=go builds with the standard toolchain instead.
#    wasm_exec.js always comes from the same toolchain as json.wasm: the two
#    runtimes' glue files are not interchangeable.
#
#    The committed bundle is the one TinyGo builds on Linux, as CI does: the
#    Homebrew build of the same TinyGo on macOS gives different bytes. So away
#    from Linux TinyGo runs in a container with the Go version pinned by the
#    toolchain line in go.mod; TINYGO_NATIVE=1 uses a local tinygo instead.
WASM_COMPILER="${WASM_COMPILER:-tinygo}"
TINYGO_VERSION=0.42.0

case "${WASM_COMPILER}" in
tinygo)
	if [ "$(uname -s)" = Linux ] || [ "${TINYGO_NATIVE:-}" = 1 ]; then
		if ! command -v tinygo >/dev/null 2>&1; then
			echo "tinygo not found: install TinyGo ${TINYGO_VERSION}" >&2
			echo "or build with the standard toolchain: WASM_COMPILER=go ./build.sh" >&2
			exit 1
		fi
		cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" assets/wasm_exec.js
		tinygo build -target=wasm -no-debug -o assets/json.wasm ./cmd/wasm
	else
		if ! command -v docker >/dev/null 2>&1; then
			echo "docker not found: away from Linux the browser bundle is built in a container" >&2
			echo "(TINYGO_NATIVE=1 uses a local tinygo, but its bytes differ from CI)" >&2
			exit 1
		fi
		go_version="$(sed -n 's/^toolchain go//p' go.mod)"
		docker run --rm -v "${ROOT_DIR}:/src" -w /src "golang:${go_version}" bash -euc "
			curl -fsSL -o /tmp/tinygo.deb https://github.com/tinygo-org/tinygo/releases/download/v${TINYGO_VERSION}/tinygo_${TINYGO_VERSION}_\$(dpkg --print-architecture).deb
			dpkg -i /tmp/tinygo.deb >/dev/null
			cp \"\$(tinygo env TINYGOROOT)/targets/wasm_exec.js\" assets/wasm_exec.js
			tinygo build -target=wasm -no-debug -o assets/json.wasm ./cmd/wasm"
	fi
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
