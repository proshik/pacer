//go:build !wasip1 || !wasm

package main

import "fmt"

// Mirror of cmd/wasm/main_nowasm.go: keeps IDEs and `go build ./...` from
// failing with "build constraints exclude all Go files" on the host platform.
func main() {
	fmt.Println("cmd/calcwasm is a WASI reactor target. Use ./build.sh or " +
		"GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared ./cmd/calcwasm")
}
