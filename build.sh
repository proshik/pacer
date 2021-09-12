#!/bin/bash

cd cmd/wasm
GOOS=js GOARCH=wasm go build -o  ../../assets/json.wasm

echo "success build wasm files"