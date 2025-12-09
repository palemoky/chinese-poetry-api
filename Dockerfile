# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags '-extldflags "-static"' \
    -o server ./cmd/server

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates curl gzip

WORKDIR /app

# Copy binary and scripts
COPY --from=builder /build/server .
COPY scripts/startup.sh .
RUN chmod +x startup.sh server

# Environment variables with defaults
ENV DB_TYPE=simplified \
    GITHUB_REPO="" \
    RELEASE_VERSION=latest \
    PORT=8080 \
    GIN_MODE=release \
    RATE_LIMIT_ENABLED=true \
    RATE_LIMIT_RPS=10 \
    RATE_LIMIT_BURST=20 \
    GRAPHQL_PLAYGROUND=false

EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/api/v1/health || exit 1

CMD ["./startup.sh"]
