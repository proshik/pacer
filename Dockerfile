# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder

WORKDIR /app
ARG TARGETOS
ARG TARGETARCH

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

# Build both wasm artifacts from the current Go toolchain: the browser bundle
# and the WASI reactor the server can run through wazero.
RUN cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ./assets/ \
	&& GOOS=js GOARCH=wasm go build -o ./assets/json.wasm ./cmd/wasm \
	&& GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared \
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
