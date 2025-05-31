# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with optimizations and static linking
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -extldflags '-static'" \
    -tags "netgo osusergo static_build" \
    -o filesystem-daemon ./cmd/daemon/main.go

# Final stage - use alpine for better compatibility
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    && rm -rf /var/cache/apk/*

# Copy the binary and make it executable
COPY --from=builder /app/filesystem-daemon /usr/local/bin/filesystem-daemon
RUN chmod +x /usr/local/bin/filesystem-daemon

# Create necessary directories
RUN mkdir -p /var/www/html /etc/filesystem-daemon/certs /var/log/filesystem-daemon

# Create volume mount points
VOLUME ["/var/www/html", "/etc/filesystem-daemon/certs"]

# Expose gRPC and health check ports
EXPOSE 50051 50052

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:50052/health || exit 1

ENTRYPOINT ["/usr/local/bin/filesystem-daemon"] 