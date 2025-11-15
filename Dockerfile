# Stage 1: Build
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a \
    -o gateway \
    ./cmd/gateway

# Stage 2: Runtime
FROM scratch

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /build/gateway /gateway

# Copy example config (optional, users can mount their own)
COPY --from=builder /build/env.example /env.example

# Expose Wyoming server port
EXPOSE 10200

# Expose health check port (optional)
EXPOSE 8080

# Expose metrics port (optional)
EXPOSE 9090

# Set working directory
WORKDIR /

# Run as non-root user (scratch image doesn't have users, but we set USER anyway for documentation)
# USER 65534:65534

# Health check (commented out by default for lower resource usage)
# Uncomment if you need Docker-level health monitoring
# HEALTHCHECK --interval=5m --timeout=3s --start-period=5s --retries=3 \
#     CMD ["/gateway", "--health-check"] || exit 1

# Run the gateway
ENTRYPOINT ["/gateway"]

