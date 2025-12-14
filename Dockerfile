# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy only necessary source files
COPY cmd/ cmd/
COPY internal/ internal/

# Build the server binary with optimizations and cache
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-extldflags '-static' -s -w" \
    -trimpath \
    -o server ./cmd/server

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates curl gzip

WORKDIR /app

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

CMD ["./startup.sh"]
