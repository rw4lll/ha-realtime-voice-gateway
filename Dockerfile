# syntax=docker/dockerfile:1

# Stage 1: Build
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

# Build arguments for multi-arch support
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

# Install build dependencies
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata

# Set working directory
WORKDIR /build

# Copy dependency files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies with verification
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && \
    go mod verify

# Copy source code
COPY . .

# Build with optimizations and multi-arch support
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOOS=${TARGETOS:-linux} \
    GOARCH=${TARGETARCH:-amd64} \
    go build \
    -trimpath \
    -ldflags='-w -s -extldflags "-static"' \
    -tags netgo,osusergo \
    -o gateway \
    ./cmd/gateway

# Verify the binary was built
RUN ls -lh gateway

# Stage 2: Runtime
FROM scratch

# Add labels for better container management
LABEL org.opencontainers.image.title="HA Realtime Voice Gateway"
LABEL org.opencontainers.image.description="Wyoming protocol gateway for Home Assistant with AI-powered voice processing"
LABEL org.opencontainers.image.source="https://github.com/rw4lll/ha-realtime-voice-gateway"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.vendor="Sergei Shitikov"

# Copy timezone data for proper time handling
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy CA certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Create passwd/group for non-root user (scratch doesn't have these)
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

# Copy the compiled binary
COPY --from=builder /build/gateway /gateway

# Copy example config for reference
COPY --from=builder /build/env.example /env.example

# Set working directory
WORKDIR /data

# Use non-root user for security (nobody user, UID 65534)
USER 65534:65534

# Expose ports with documentation
EXPOSE 10200/tcp
EXPOSE 8080/tcp
EXPOSE 9090/tcp

# Health check for container orchestration
# Disabled by default - uncomment if using Kubernetes/Docker Swarm
# HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
#     CMD ["/gateway", "--health-check"] || exit 1

# Set default environment variables
ENV TZ=UTC \
    LOG_LEVEL=info \
    LOG_FORMAT=json

# Use array syntax for proper signal handling
ENTRYPOINT ["/gateway"]

# Default command (can be overridden)
CMD []

