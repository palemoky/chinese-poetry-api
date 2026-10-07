# syntax=docker/dockerfile:1

# Cross-compilation helpers (xx-go, xx-apk, xx-verify)
FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.9.0 AS xx

# Build stage
# Runs on the build host's own architecture and cross-compiles for the target.
# Building arm64 under QEMU emulation instead took ~16 of the release's ~19 minutes,
# almost all of it compiling the bundled SQLite C source.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

COPY --from=xx / /

WORKDIR /build

# clang/lld cross-compile the cgo parts; the target's C library comes from xx-apk.
# SQLite itself is bundled with mattn/go-sqlite3, so sqlite-dev is not needed.
RUN apk add --no-cache clang lld
ARG TARGETPLATFORM
RUN xx-apk add --no-cache gcc musl-dev

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Copy only necessary source files
COPY cmd/ cmd/
COPY internal/ internal/

# No -a: it forces every package to rebuild and defeats the build cache mount
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=1 xx-go build \
    -tags sqlite_fts5 \
    -ldflags "-extldflags '-static' -s -w" \
    -trimpath \
    -o server ./cmd/server \
    && xx-verify --static server

# Runtime stage
FROM alpine:3.24

# su-exec lets startup.sh drop root after fixing data-volume ownership
RUN apk add --no-cache ca-certificates curl gzip su-exec zstd \
    && addgroup -S poetry \
    && adduser -S -G poetry -H -h /app poetry

WORKDIR /app

RUN mkdir -p data && chown poetry:poetry data

# Copy binary, config, and startup script
COPY --link --from=builder --chmod=755 /build/server .
COPY --link --chmod=644 config.yaml .
COPY --link --chmod=755 scripts/startup.sh .

# Environment variables
ENV PORT=1279 \
    GIN_MODE=release \
    RATE_LIMIT_ENABLED=true \
    RATE_LIMIT_RPS=10 \
    RATE_LIMIT_BURST=20

EXPOSE 1279

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:${PORT}/api/v1/health || exit 1

# The container starts as root only long enough for startup.sh to chown the
# data volume (volumes created by older images are root-owned), then it
# re-executes itself as the unprivileged poetry user.
CMD ["./startup.sh"]
