#!/bin/bash

cp $(go env GOROOT)/misc/wasm/wasm_exec.js assets
echo "copied wasm_exec.js."

cd cmd/wasm
GOOS=js GOARCH=wasm go build -o  ../../assets/json.wasm

echo "success build wasm files"