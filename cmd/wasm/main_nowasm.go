//go:build !js || !wasm
// +build !js !wasm

package main

import "fmt"

func main() {
	fmt.Println("cmd/wasm is a WebAssembly target. Use ./build.sh or GOOS=js GOARCH=wasm go build ./cmd/wasm")
}
