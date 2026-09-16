# syntax=docker/dockerfile:1

# The builder runs on the build host's own platform and cross-compiles for the
# target: the binary is pure Go (CGO_ENABLED=0), so no emulation is needed to
# build an amd64 image on arm64 or the other way round.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

WORKDIR /app
ARG TARGETOS
ARG TARGETARCH

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

# Both wasm artifacts are committed, and CI fails when ./build.sh changes them,
# so the image ships exactly what was tested instead of a rebuild whose
# toolchain differs from the one the artifacts were checked with.

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
