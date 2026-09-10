# syntax=docker/dockerfile:1

# Browser bundle. TinyGo makes json.wasm about five times smaller than the
# standard toolchain with byte-identical results. Wasm does not depend on the
# target platform, so this stage runs on the build host's own platform.
FROM --platform=$BUILDPLATFORM tinygo/tinygo:0.42.0 AS wasm

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/wasm ./cmd/wasm
COPY pkg ./pkg
RUN mkdir -p /out \
	&& tinygo build -target=wasm -no-debug -o /out/json.wasm ./cmd/wasm \
	&& cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" /out/wasm_exec.js

FROM golang:1.26-alpine AS builder

WORKDIR /app
ARG TARGETOS
ARG TARGETARCH

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

# The browser bundle comes from the TinyGo stage together with its own
# wasm_exec.js; the server's WASI reactor is built with the standard toolchain.
COPY --from=wasm /out/json.wasm /out/wasm_exec.js ./assets/
RUN GOOS=wasip1 GOARCH=wasm go build -trimpath -buildvcs=false -buildmode=c-shared \
		-o ./pkg/wasmcalc/calc.wasm ./cmd/calcwasm

# Build the Linux backend binary with embedded assets.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/pacer .

FROM alpine:3.20

RUN addgroup -S app && adduser -S -G app app \
	&& apk add --no-cache ca-certificates

COPY --from=builder /out/pacer /usr/local/bin/pacer

# Saved Mini App runs live in SQLite under /data. Mount a volume there to keep
# them across container re-creation; without DB_PATH the history is disabled.
RUN mkdir -p /data && chown app:app /data
ENV DB_PATH=/data/pacer.db
VOLUME /data

USER app
EXPOSE 80

CMD ["/usr/local/bin/pacer"]
