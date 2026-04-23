# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder

WORKDIR /app
ARG TARGETOS
ARG TARGETARCH

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

# Build browser runtime files from the current Go toolchain.
RUN cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ./assets/ \
	&& GOOS=js GOARCH=wasm go build -o ./assets/json.wasm ./cmd/wasm

# Build the Linux backend binary with embedded assets.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/pacer .

FROM alpine:3.20

RUN addgroup -S app && adduser -S -G app app \
	&& apk add --no-cache ca-certificates

COPY --from=builder /out/pacer /usr/local/bin/pacer

USER app
EXPOSE 80

CMD ["/usr/local/bin/pacer"]
