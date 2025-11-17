# Docker Deployment Guide

This guide explains how to deploy the Home Assistant Realtime Voice Gateway using Docker with multi-architecture support.

## 🏗️ Multi-Architecture Support

Pre-built images are available for:
- **linux/amd64** (x86_64) - Standard servers and desktops
- **linux/arm64** (aarch64) - Raspberry Pi 4/5, Apple Silicon, AWS Graviton
- **linux/arm/v7** (armhf) - Raspberry Pi 2/3, older ARM devices

Images are automatically built via GitHub Actions on each release.

## Quick Start

### Using Pre-built Images (Recommended)

Pull the latest official image:
```bash
docker pull ghcr.io/rw4lll/ha-realtime-voice-gateway:latest
```

Or use in docker-compose.yml:
```yaml
services:
  gateway:
    image: ghcr.io/rw4lll/ha-realtime-voice-gateway:latest
    # ... rest of configuration
```

### Using Docker Compose (Local Build)

1. **Clone the repository**
```bash
git clone https://github.com/yourusername/ha-realtime-voice-gateway.git
cd ha-realtime-voice-gateway
```

2. **Configure environment variables**
```bash
# Copy example environment file
cp env.example .env

# Edit with your settings
nano .env
```

Required settings:
```bash
HA_URL=http://homeassistant.local:8123
HA_TOKEN=your_long_lived_access_token_here
```

3. **Start the gateway**
```bash
docker-compose up -d
```

4. **Check logs**
```bash
docker-compose logs -f gateway
```

5. **Check status**
```bash
docker-compose ps
```

### Using Docker CLI

**Build the image:**
```bash
docker build -t ha-voice-gateway .
```

**Run the container:**
```bash
docker run -d \
  --name ha-voice-gateway \
  -p 8080:8080 \
  -e HA_URL=http://homeassistant.local:8123 \
  -e HA_TOKEN=your_token_here \
  -e BACKEND_TYPE=mock \
  ha-voice-gateway
```

## Configuration

### Environment Variables

All configuration is done through environment variables. See `env.example` for a complete list.

**Required:**
- `HA_URL` - Your Home Assistant URL
- `HA_TOKEN` - Long-lived access token from HA

**Optional:**
- `BACKEND_TYPE` - LLM backend: `mock`, `gemini`, `openai` (default: `mock`)
- `GEMINI_API_KEY` - Google Gemini API key (if using Gemini)
- `OPENAI_API_KEY` - OpenAI API key (if using OpenAI)
- `LOG_LEVEL` - Log level: `debug`, `info`, `warn`, `error` (default: `info`)
- `WEBSOCKET_ADDR` - WebSocket server address (default: `0.0.0.0:8080`)
- `WEBSOCKET_PATH` - WebSocket endpoint path (default: `/voice-stream`)

### Docker Compose Configuration

The `docker-compose.yml` file includes:
- Automatic restart on failure
- Health checks
- Log rotation
- Network isolation
- Port mappings

You can customize it by editing `docker-compose.yml` or using environment variables.

## Deployment Scenarios

### 1. Standalone Deployment

Deploy the gateway on a separate machine from Home Assistant:

```yaml
# docker-compose.yml
services:
  gateway:
    environment:
      - HA_URL=http://192.168.1.100:8123  # Your HA IP
      - HA_TOKEN=${HA_TOKEN}
    ports:
      - "8080:8080"  # WebSocket server
```

### 2. Same Host as Home Assistant

Deploy on the same machine as Home Assistant:

```yaml
services:
  gateway:
    environment:
      - HA_URL=http://homeassistant:8123  # Use HA container name
      - HA_TOKEN=${HA_TOKEN}
    network_mode: "host"  # Share host network
```

Or add to the same Docker network as HA:

```yaml
services:
  gateway:
    networks:
      - homeassistant
    environment:
      - HA_URL=http://homeassistant:8123

networks:
  homeassistant:
    external: true
```

### 3. Production Deployment

For production with Gemini backend:

```yaml
services:
  gateway:
    environment:
      - HA_URL=http://homeassistant:8123
      - HA_TOKEN=${HA_TOKEN}
      - BACKEND_TYPE=gemini
      - GEMINI_API_KEY=${GEMINI_API_KEY}
      - LOG_LEVEL=info
      - LOG_FORMAT=json
      - METRICS_ENABLED=true
    ports:
      - "8080:8080"   # WebSocket server
      - "9090:9090"   # Prometheus metrics
```

## Ports

- **8080** - WebSocket server (required) - Devices connect to `ws://gateway:8080/voice-stream`
- **9090** - Prometheus metrics (optional)

## Health Checks

The container includes a health check that runs every 30 seconds.

Check health status:
```bash
docker ps  # Look at STATUS column
docker inspect ha-voice-gateway | grep -A 10 Health
```

Health check endpoint (if exposed):
```bash
curl http://localhost:8080/health
```

## Monitoring

### View logs
```bash
# Follow logs
docker-compose logs -f gateway

# Last 100 lines
docker-compose logs --tail=100 gateway

# JSON formatted logs
docker-compose logs -f gateway | jq
```

### Prometheus metrics
If `METRICS_ENABLED=true`, metrics are available at:
```
http://localhost:9090/metrics
```

### Container stats
```bash
docker stats ha-voice-gateway
```

## Troubleshooting

### Gateway won't start

Check logs:
```bash
docker-compose logs gateway
```

Common issues:
- Invalid `HA_TOKEN` - Check token in Home Assistant
- `HA_URL` unreachable - Verify network connectivity
- Port 8080 already in use - Change port mapping (`-p 8081:8080`)

### Can't connect to Home Assistant

Test connectivity from container:
```bash
docker-compose exec gateway wget -O- ${HA_URL}/api/
```

### ESP32 device can't connect

Verify WebSocket endpoint is accessible:
```bash
# Should return "Upgrade Required" (WebSocket endpoint)
curl http://localhost:8080/voice-stream

# Or test with wscat (npm install -g wscat)
wscat -c ws://localhost:8080/voice-stream
```

### High memory usage

Adjust buffer sizes:
```yaml
environment:
  - AUDIO_BUFFER_SIZE=50
  - EVENT_BUFFER_SIZE=25
```

## Updating

### Pull latest image
```bash
docker-compose pull gateway
docker-compose up -d
```

### Rebuild from source
```bash
docker-compose build --no-cache
docker-compose up -d
```

## 🚀 CI/CD Automated Builds

### Release Workflow

When a new release is published on GitHub, multi-architecture images are automatically:
1. Built for all supported platforms
2. Tagged with version numbers (e.g., `1.0.0`, `1.0`, `1`, `latest`)
3. Pushed to GitHub Container Registry (ghcr.io)
4. Security scanned with Trivy
5. Attested with SBOM and provenance

### Version Tags

| Release Type | Tag Example | Image Tags Created |
|-------------|-------------|-------------------|
| Stable | `v1.2.3` | `1.2.3`, `1.2`, `1`, `latest` |
| Pre-release | `v1.2.3-beta.1` | `1.2.3-beta.1` only |

Pre-releases do NOT update `latest` or major/minor tags.

### Creating a Release

1. Create and push a version tag:
```bash
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0
```

2. Create GitHub Release from the tag

3. Images are automatically built and published

See `.github/workflows/docker-release.yml` for details.

## Building Custom Images

### Build Locally (Single Architecture)

```bash
# Build for your current platform
docker build -t ha-voice-gateway:dev .
```

### Multi-arch Build (Local)

```bash
# Create buildx builder
docker buildx create --name multiarch --use --bootstrap

# Build for all platforms
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/arm/v7 \
  -t ha-voice-gateway:multiarch \
  --load \
  .
```

### Build for Specific Architecture

**ARM64 (Raspberry Pi 4/5, Apple Silicon):**
```bash
docker buildx build --platform linux/arm64 -t ha-voice-gateway:arm64 --load .
```

**ARMv7 (Raspberry Pi 2/3):**
```bash
docker buildx build --platform linux/arm/v7 -t ha-voice-gateway:armv7 --load .
```

**AMD64 (Standard x86_64):**
```bash
docker buildx build --platform linux/amd64 -t ha-voice-gateway:amd64 --load .
```

## Backup and Restore

### Backup configuration
```bash
# Backup .env file
cp .env .env.backup

# Export container config
docker inspect ha-voice-gateway > gateway-config.json
```

### Restore
```bash
# Restore .env
cp .env.backup .env

# Recreate container
docker-compose up -d
```

## Security Best Practices

1. **Use secrets for sensitive data:**
```yaml
services:
  gateway:
    secrets:
      - ha_token
secrets:
  ha_token:
    file: ./secrets/ha_token.txt
```

2. **Run as non-root** (already configured in Dockerfile)

3. **Network isolation:**
```yaml
networks:
  ha-network:
    internal: true  # No external access
```

4. **Read-only filesystem:**
```yaml
services:
  gateway:
    read_only: true
```

5. **Resource limits:**
```yaml
services:
  gateway:
    deploy:
      resources:
        limits:
          cpus: '1'
          memory: 512M
        reservations:
          cpus: '0.5'
          memory: 256M
```

## Integration with ESP32 Devices

### Configure ESP32 Device

Set the WebSocket gateway URL in your ESP32 device settings:

```
Gateway URL: ws://gateway:8080/voice-stream
```

**Examples based on deployment:**
- Same Docker network: `ws://gateway:8080/voice-stream`
- Bridge network: `ws://192.168.1.100:8080/voice-stream`
- Host network: `ws://localhost:8080/voice-stream`

### Entering Streaming Mode

**Press the device button 4 times** to switch from standard Home Assistant Voice Preview mode to direct streaming mode.

### Verify Connection

```bash
# Check WebSocket endpoint is accessible
curl http://gateway:8080/voice-stream
# Should return "Upgrade Required" (WebSocket endpoint)

# Or from device's network
curl http://192.168.1.100:8080/voice-stream
```

## Advanced Configuration

### Custom network
```yaml
networks:
  ha-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
```

### Volume mounts for logs
```yaml
services:
  gateway:
    volumes:
      - ./logs:/logs
    environment:
      - LOG_FILE=/logs/gateway.log
```

### Multiple instances
```yaml
services:
  gateway-1:
    ports:
      - "8080:8080"
  gateway-2:
    ports:
      - "8081:8080"  # Different external port, same internal port
```

## Performance Tuning

For high-traffic deployments:

```yaml
services:
  gateway:
    environment:
      - AUDIO_BUFFER_SIZE=200
      - EVENT_BUFFER_SIZE=100
    deploy:
      resources:
        limits:
          memory: 1G
```

## Support

- **GitHub Issues**: https://github.com/yourusername/ha-realtime-voice-gateway/issues
- **Discord**: [Join the discussion](https://discord.gg/home-assistant)
- **Documentation**: See [Readme.md](Readme.md)

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details

