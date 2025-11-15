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

### Wyoming Protocol Configuration

```bash
# ============================================
# Wyoming Protocol Configuration
# ============================================
WYOMING_HOST=0.0.0.0
WYOMING_STT_PORT=10200          # Device connects here for audio input
WYOMING_TTS_PORT=10201          # Device connects here for audio output
```

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
SESSION_TIMEOUT=5m              # Idle timeout
MAX_CONVERSATION_HISTORY=50     # Messages to keep in context
MAX_CONCURRENT_SESSIONS=10      # Per gateway instance
CLEANUP_INTERVAL=1m
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

# Wyoming server
wyoming:
  address: "0.0.0.0:10200"

# Audio settings
audio:
  buffer_size: 100

# Session settings
session:
  system_prompt: "You are a helpful voice assistant for Home Assistant."

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

## Home Assistant Device Configuration

Configure your Voice Preview device in Home Assistant to connect to the gateway:

```yaml
# configuration.yaml

# Add Wyoming integration
wyoming:
  - uri: tcp://your-gateway-ip:10200
    name: "Realtime Gateway STT"
  
  - uri: tcp://your-gateway-ip:10201
    name: "Realtime Gateway TTS"

# Create assist pipeline
assist_pipeline:
  - name: "Realtime Voice"
    stt_engine: wyoming.realtime_gateway_stt
    tts_engine: wyoming.realtime_gateway_tts
    conversation_agent: conversation.home_assistant
```

Then in Home Assistant UI:
1. Go to **Settings** → **Devices & Services** → **Wyoming**
2. Verify the integration shows as connected
3. Go to **Settings** → **Voice Assistants** → **Assist**
4. Select your Voice Preview device
5. Assign the "Realtime Voice" pipeline to the device

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
  - WYOMING_STT_PORT must be between 1024 and 65535
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
