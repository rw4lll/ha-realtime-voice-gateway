# Configuration Guide

Complete configuration reference for the Home Assistant Realtime Voice Gateway.

## Configuration Methods

The gateway supports two configuration methods:

1. **YAML Configuration** (`ha-config.yaml`) - For structured settings, version-controlled config
2. **Environment Variables** (`.env` file) - For secrets and deployment-specific overrides

### Configuration Priority

**Environment variables > YAML > Defaults**

This allows you to:
- Store base configuration in YAML (version controlled)
- Override sensitive data with environment variables  
- Use different configs per environment (dev/staging/prod)

## Quick Start

1. **Copy the example config:**
   ```bash
   cp ha-config.yaml.example ha-config.yaml
   ```

2. **Edit `ha-config.yaml` with your settings:**
   ```yaml
   home_assistant:
     url: "http://homeassistant.local:8123"
     token: ""  # Will be loaded from HA_TOKEN env var
     
     discovery:
       domains:
         include: [light, switch, climate]  # Or [] for all
         exclude: [automation, script, scene]
       services:
         mode: "allow"
         patterns: [light.*, switch.*, climate.*]
   
   backend:
     type: "gemini"
     gemini:
       model: "gemini-2.0-flash-exp"
   ```

3. **Create `.env` file for secrets:**
   ```bash
   # .env - Keep this file gitignored!
   HA_URL=http://homeassistant.local:8123
   HA_TOKEN=your_long_lived_access_token
   GEMINI_API_KEY=your_gemini_api_key
   ```

## Full Environment Variable Reference

Create a `.env` file in the project root with these variables:

### LLM Backend Configuration

```bash
# ============================================
# LLM Backend Configuration
# ============================================
LLM_BACKEND=gemini              # gemini | openai | mock

# Gemini Live (default - free tier available)
GEMINI_API_KEY=your_gemini_api_key
GEMINI_MODEL=gemini-2.0-flash-exp

# Gemini Performance & Reliability (optional, sensible defaults)
GEMINI_CONNECT_TIMEOUT=30s        # Connection timeout
GEMINI_RECEIVE_TIMEOUT=60s        # Message receive timeout  
GEMINI_SEND_TIMEOUT=10s           # Message send timeout
GEMINI_MAX_RETRIES=3              # Max connection retries
GEMINI_RETRY_BACKOFF=1s           # Initial backoff (exponential)
GEMINI_MAX_SESSIONS=5             # Max concurrent sessions

# OpenAI Realtime (optional)
OPENAI_API_KEY=sk-...
OPENAI_REALTIME_MODEL=gpt-4o-realtime-preview-2024-12-17
```

### WebSocket Server Configuration

```bash
# ============================================
# WebSocket Server Configuration
# ============================================
WEBSOCKET_ADDR=0.0.0.0:8080            # Listen address and port (required)
WEBSOCKET_PATH=/voice-stream           # WebSocket endpoint path (required)
WEBSOCKET_MAX_BUFFER_SIZE=32768        # Max WebSocket frame size (bytes)
WEBSOCKET_READ_TIMEOUT=60              # Read timeout (seconds)
WEBSOCKET_WRITE_TIMEOUT=10             # Write timeout (seconds)
```

**Note:** The WebSocket server is always enabled - it's the gateway's core purpose.

**WebSocket URL:** Devices connect to `ws://your-gateway-ip:8080/voice-stream`

### Home Assistant Integration

```bash
# ============================================
# Home Assistant Integration
# ============================================
HA_BASE_URL=http://homeassistant.local:8123
HA_TOKEN=your_long_lived_access_token
HA_WEBSOCKET_RECONNECT=true

# Tool execution safety (see SECURITY.md for details)
ALLOWED_DOMAINS=light,switch,media_player,cover,climate,fan,lock
DENIED_ENTITIES=                # Optional: specific entities to block
REQUIRE_CONFIRMATION=lock       # Domains requiring voice confirmation
READ_ONLY_MODE=false            # If true, only queries allowed
TOOL_TIMEOUT=5s
TOOL_RETRY_ATTEMPTS=2
```

### Audio Configuration

```bash
# ============================================
# Audio Configuration
# ============================================
AUDIO_INPUT_SAMPLE_RATE=16000
AUDIO_INPUT_CHANNELS=1
AUDIO_INPUT_WIDTH=2             # 16-bit = 2 bytes
AUDIO_OUTPUT_SAMPLE_RATE=16000
AUDIO_BUFFER_SIZE=4096          # bytes per chunk

# Voice Activity Detection
VAD_ENABLED=true
VAD_SENSITIVITY=0.5             # 0.0-1.0 (higher = more sensitive)
VAD_SPEECH_PAD_MS=300           # ms to wait after speech ends
```

**Note**: For detailed VAD tuning, see [VAD_TUNING_GUIDE.md](VAD_TUNING_GUIDE.md)

### Session Management

```bash
# ============================================
# Session Management
# ============================================
SESSION_SYSTEM_PROMPT="You are a helpful voice assistant for Home Assistant."
SESSION_SAFETY_TIMEOUT=5m       # Max session duration (safety limit)
SESSION_SILENCE_TIMEOUT=3s      # End conversation after silence
SESSION_AUDIO_BUFFER_MS=500     # Audio buffering before playback (ms)
```

### Performance & Optimization

```bash
# ============================================
# Performance & Optimization
# ============================================
ENABLE_STATE_CACHE=true
CACHE_TTL=30s                   # How long to cache HA state
ASYNC_TOOL_CALLS=true           # Execute tools while LLM talks
BUFFER_POOL_SIZE=100            # Audio buffer pool
```

### Observability

```bash
# ============================================
# Observability
# ============================================
LOG_LEVEL=info                  # debug | info | warn | error
LOG_FORMAT=json                 # json | console
METRICS_ENABLED=true
METRICS_PORT=9090
TRACING_ENABLED=false

# Metrics to track
TRACK_LATENCY=true              # End-to-end latency
TRACK_AUDIO_QUALITY=false       # CPU intensive
```

## Home Assistant YAML Configuration

The `ha-config.yaml` file provides structured configuration for Home Assistant integration and autodiscovery.

### Complete Example

```yaml
# ha-config.yaml

home_assistant:
  # Connection settings (can override with env vars)
  url: "http://homeassistant.local:8123"
  token: ""  # Loaded from HA_TOKEN env var
  timeout: "10s"
  
  # Autodiscovery configuration
  discovery:
    # Domain filtering
    domains:
      include: []  # Empty = discover all (except excluded)
      exclude:
        - automation
        - script
        - scene
        - group
    
    # Service filtering (applied after domain filtering)
    services:
      mode: "allow"  # or "deny"
      patterns:
        - "light.*"
        - "switch.*"
        - "climate.*"
        - "cover.*"
        - "media_player.media_*"
        - "media_player.volume_*"
        - "fan.*"
        - "lock.*"
    
    # System prompt generation
    prompt:
      include_entities: true
      include_attributes: false
      group_by: "domain"  # "domain", "area", "floor", "none"

# Backend configuration
backend:
  type: "gemini"  # mock, gemini, openai
  
  gemini:
    api_key: ""  # Override with GEMINI_API_KEY env var
    model: "gemini-2.0-flash-exp"
    connect_timeout: "30s"
    receive_timeout: "60s"
    send_timeout: "10s"
    max_retries: 3
    retry_backoff: "1s"
    max_sessions: 5

# WebSocket server
websocket:
  address: "0.0.0.0:8080"
  path: "/voice-stream"
  max_buffer_size: 32768
  read_timeout: 60
  write_timeout: 10

# Audio settings
audio:
  buffer_size: 100

# Session settings
session:
  system_prompt: "You are a helpful voice assistant for Home Assistant."
  safety_timeout: "5m"
  silence_timeout: "3s"
  audio_buffer_ms: 500

# Logging
logging:
  level: "info"
  format: "console"

# Performance tuning
performance:
  audio_buffer_size: 100
  event_buffer_size: 50

# Observability
observability:
  metrics:
    enabled: false
    address: "0.0.0.0:9090"
  health_check:
    enabled: true
    address: "0.0.0.0:8080"
```

## ESP32 Device Configuration

Configure your ESP32 voice device with modified streaming firmware to connect to the gateway:

### Device Settings

Set the WebSocket gateway URL in your ESP32 device configuration:

```
Gateway URL: ws://your-gateway-ip:8080/voice-stream
```

**Examples:**
- Local network: `ws://192.168.1.100:8080/voice-stream`
- Docker network: `ws://ha-voice-gateway:8080/voice-stream`
- With domain: `ws://voice-gateway.local:8080/voice-stream`

### Entering Streaming Mode

**Press the device button 4 times** to switch from standard Home Assistant Voice Preview mode to direct streaming mode.

The device will:
1. Close any existing Home Assistant pipeline connection
2. Open WebSocket connection to gateway
3. Start streaming audio directly to gateway
4. Receive JSON state updates and audio responses

### Troubleshooting Device Connection

**Device won't connect:**
```bash
# Verify gateway is listening
curl http://your-gateway-ip:8080/voice-stream
# Should return "Upgrade Required" (WebSocket endpoint)

# Check gateway logs
docker logs -f ha-voice-gateway | grep "websocket"
```

**Device shows error:**
- Verify gateway URL is correct (`ws://` not `http://`)
- Check firewall allows port 8080
- Ensure gateway is running: `docker ps | grep gateway`

## Configuration Examples

### Example 1: Development Environment

**`.env`:**
```bash
# Development with detailed logging
LOG_LEVEL=debug
LOG_FORMAT=console
BACKEND_TYPE=gemini
GEMINI_API_KEY=your_key
HA_URL=http://localhost:8123
HA_TOKEN=your_token
ALLOWED_DOMAINS=light,switch
```

### Example 2: Production Environment

**`.env`:**
```bash
# Production with metrics and structured logging
LOG_LEVEL=info
LOG_FORMAT=json
METRICS_ENABLED=true
METRICS_PORT=9090

# Use production HA instance
HA_URL=https://ha.example.com
HA_TOKEN=${HA_PRODUCTION_TOKEN}

# More restrictive security
ALLOWED_DOMAINS=light,switch,climate
DENIED_ENTITIES=lock.front_door,cover.garage_door
REQUIRE_CONFIRMATION=lock,cover
```

### Example 3: Multi-User Home

**`ha-config.yaml`:**
```yaml
home_assistant:
  discovery:
    # Include common domains
    domains:
      include: [light, switch, climate, media_player, fan]
    
    # Exclude sensitive domains
    domains:
      exclude: [lock, automation, script]
    
    # System prompt with entity list for clarity
    prompt:
      include_entities: true
      include_attributes: false
      group_by: "area"  # Group by room/area

backend:
  gemini:
    max_sessions: 10  # Support multiple family members
```

### Example 4: Testing/CI Environment

**`.env`:**
```bash
# Use mock backend for testing
BACKEND_TYPE=mock
LOG_LEVEL=debug
LOG_FORMAT=console

# No real HA connection needed
HA_URL=http://mock:8123
HA_TOKEN=mock_token

# Disable metrics
METRICS_ENABLED=false
```

## Configuration Validation

The gateway validates all configuration on startup and will fail fast if required values are missing or invalid:

```
ERROR: Configuration validation failed:
  - HA_URL is required when backend is not mock
  - GEMINI_API_KEY is required when backend is gemini
  - WEBSOCKET_ADDR must be a valid host:port address
  - SESSION_SAFETY_TIMEOUT must be positive
```

## Environment Variable Substitution

The gateway supports environment variable substitution in YAML configuration:

```yaml
home_assistant:
  url: "${HA_URL}"
  token: "${HA_TOKEN}"

backend:
  gemini:
    api_key: "${GEMINI_API_KEY}"
```

This allows you to:
- Keep secrets out of version control
- Use different values per environment
- Override YAML settings with environment variables

## See Also

- [AUTODISCOVERY.md](AUTODISCOVERY.md) - Home Assistant autodiscovery configuration
- [VAD_TUNING_GUIDE.md](VAD_TUNING_GUIDE.md) - Voice activity detection tuning
- [SECURITY.md](SECURITY.md) - Security and safety configuration
- [DOCKER.md](DOCKER.md) - Docker deployment configuration
- [ARCHITECTURE.md](ARCHITECTURE.md) - Technical architecture overview
