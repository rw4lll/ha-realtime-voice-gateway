# Home Assistant Realtime Voice Gateway

**Low latency voice control for Home Assistant using ESP32 devices and LLM streaming.**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8.svg)](https://golang.org)

---

## 🚀 What Is This?

A voice gateway that enables **true realtime, conversational voice control** for Home Assistant. Instead of the traditional STT → Conversation → TTS pipeline, this streams audio directly between your ESP32 device and an LLM (Gemini, OpenAI, etc.), providing:

- ⚡ **Low latency** - Instant responses, feels like talking to a person
- 🎙️ **Full-duplex audio** - Interrupt the assistant anytime (barge-in)
- 🤖 **Smart home control** - LLM can call Home Assistant services as tools
- 🔌 **ESP32 Voice devices** - Works with modified streaming firmware
- 🔄 **Automatic discovery** - No manual tool configuration needed

**Example conversation:**
```
You: "Turn on the living room lights"
Assistant: [responds and turns on lights simultaneously]

You: "Actually, make them dimmer"  ← Can interrupt mid-response!
Assistant: [adjusts brightness while responding]
```

---

## ✨ Key Features

### 🎯 For Users
- **Natural conversations** with instant responses and barge-in support
- **Auto-discovers** all Home Assistant devices and services
- **Test without hardware** using laptop mic/speakers
- **Works with ESP32 devices** running modified streaming firmware

### 🛠️ For Developers  
- **Pluggable backends** (Gemini, OpenAI, Anthropic, local models)
- **Event-driven architecture** for realtime audio + tool calling
- **Production-ready** with retry logic, timeouts, and error handling
- **Comprehensive test suite** (100+ tests)

### 🔒 Security
- **Domain allow-lists** and entity deny-lists
- **Service filtering** - control exactly what LLM can do
- **Audit logging** - track all actions
- **Configurable timeouts** - safety controls

---

## 🏗️ How It Works

```
[Your ESP32 Device]
       │
       │ WebSocket (ws://gateway:8080/voice-stream)
       │ Raw PCM Audio + JSON control
       ↓
[This Gateway]
   ├─> [Gemini/OpenAI/etc.] ← Streaming LLM
   └─> [Home Assistant API]  ← Tool execution (lights, switches, etc.)
```

**The gateway:**
1. Accepts WebSocket connections from ESP32 devices
2. Streams audio to/from LLM backend (Gemini, OpenAI, etc.)
3. Manages conversation state (listening → thinking → speaking → done)
4. Executes Home Assistant tool calls
5. Handles interruptions and full-duplex conversations

---

## 📋 Prerequisites

- **Home Assistant** with Long-Lived Access Token
- **ESP32 Voice device** with modified streaming firmware
  - *Or use laptop mic/speakers for testing*
- **LLM API Key:**
  - [Gemini API Key](https://aistudio.google.com/apikey) ✅ **Free tier available!**
  - OpenAI API Key (coming soon)
- **Docker** (recommended) or Go 1.25+

---

## 🆚 When to Use This?

### **Use This Gateway If:**
- You have ESP32 devices with modified streaming firmware
- You want ultra-low latency conversations (<500ms)
- You want full-duplex, interruptible conversations (barge-in)
- You want LLM to directly control HA without going through HA's pipeline

---

## 🚀 Quick Start

### Option 1: Docker (Recommended)

```bash
# 1. Create docker-compose.yml
cat > docker-compose.yml << 'EOF'
version: '3.8'
services:
  gateway:
    image: ghcr.io/rw4lll/ha-realtime-voice-gateway:latest
    container_name: ha-voice-gateway
    restart: unless-stopped
    environment:
      # Home Assistant
      - HA_URL=http://homeassistant:8123
      - HA_TOKEN=your_long_lived_token_here
      
      # LLM Backend (Gemini example)
      - BACKEND_TYPE=gemini
      - GEMINI_API_KEY=your_gemini_api_key_here
      
      # WebSocket Server (optional customization)
      - WEBSOCKET_ADDR=0.0.0.0:8080
      - WEBSOCKET_PATH=/voice-stream
    ports:
      - "8080:8080"
    networks:
      - homeassistant
EOF

# 2. Edit credentials
nano docker-compose.yml

# 3. Start gateway
docker-compose up -d

# 4. Check logs
docker-compose logs -f
```

### Option 2: From Source

```bash
# 1. Clone repository
git clone https://github.com/rw4lll/ha-realtime-voice-gateway.git
cd ha-realtime-voice-gateway

# 2. Create .env file
cat > .env << 'EOF'
# Home Assistant
HA_URL=http://homeassistant:8123
HA_TOKEN=your_long_lived_token_here

# Backend (Gemini)
BACKEND_TYPE=gemini
GEMINI_API_KEY=your_api_key_here

# WebSocket Server (optional customization)
WEBSOCKET_ADDR=0.0.0.0:8080
WEBSOCKET_PATH=/voice-stream
EOF

# 3. Build and run
go build -o gateway ./cmd/gateway
./gateway
```

**You should see:**
```
INFO  Gateway ready  websocket_addr=0.0.0.0:8080
INFO  WebSocket server listening. Waiting for device connections...
```

---

## 🔌 Connect Your ESP32 Device

### 1. Flash Modified Firmware

Follow the instructions at [ESP32 Streaming Firmware Guide](https://github.com/your-firmware-repo)

### 2. Configure Device

In the device settings, set the gateway URL:
```
ws://YOUR_GATEWAY_IP:8080/voice-stream
```

Example: `ws://192.168.1.100:8080/voice-stream`

### 3. Enter Streaming Mode

**Press the button 4 times** to switch from standard HA Voice Preview mode to direct streaming mode.

### 4. Start Talking!

The device will:
- Send your voice to the gateway via WebSocket
- Receive JSON state updates (`listening`, `thinking`, `speaking`, `done`)
- Play AI responses in real-time

---

## ⚙️ Configuration

### Essential Settings

```bash
# Home Assistant
HA_URL=http://homeassistant:8123          # Your HA URL
HA_TOKEN=your_long_lived_token            # Settings → Profile → Long-Lived Access Tokens

# Backend
BACKEND_TYPE=gemini                       # gemini | openai | mock
GEMINI_API_KEY=your_key                   # Get from https://aistudio.google.com/apikey

# WebSocket Server (always enabled)
WEBSOCKET_ADDR=0.0.0.0:8080              # Listen on all interfaces
WEBSOCKET_PATH=/voice-stream             # WebSocket endpoint
```

### Optional: Security & Filtering

```bash
# Limit which domains the LLM can control
HA_AUTODISCOVERY_DOMAINS=light,switch,climate

# Deny specific entities
HA_AUTODISCOVERY_DENIED=lock.*,alarm_control_panel.*

# Restrict to specific services
HA_ALLOW_LIST=light.turn_on,light.turn_off,switch.toggle
```

### Optional: Performance Tuning

```bash
# Session timeouts
SESSION_SAFETY_TIMEOUT=5m                 # Max conversation duration
SESSION_SILENCE_TIMEOUT=3s                # End after 3s of silence
SESSION_AUDIO_BUFFER_MS=500               # Audio buffering before playback
```

See **[docs/CONFIGURATION.md](docs/CONFIGURATION.md)** for all options.

---

## 🔍 Troubleshooting

### Gateway won't start

```bash
# Check logs
docker-compose logs -f gateway

# Common issues:
# - Invalid HA_TOKEN → Check Settings → Profile → Long-Lived Access Tokens
# - Can't reach Home Assistant → Use http://homeassistant:8123 in Docker network
# - Invalid GEMINI_API_KEY → Get new key from https://aistudio.google.com/apikey
```

### Device won't connect

```bash
# Verify gateway is listening
curl http://YOUR_GATEWAY_IP:8080/voice-stream
# Should return "Upgrade Required" (WebSocket endpoint)

# Check device firmware:
# - Is streaming mode enabled? (4-click button)
# - Is gateway URL correct? ws://IP:8080/voice-stream
# - Is device on same network?
```

### LLM responds but doesn't control HA

```bash
# Check autodiscovery
docker-compose logs gateway | grep "tools_discovered"
# Should show: "Autodiscovery completed  tools_discovered=45"

# Verify HA connection
docker-compose logs gateway | grep "Home Assistant"
# Should show: "Successfully connected to Home Assistant"
```

See **[docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)** for more help.

---

## 🧪 Testing Without Hardware

You can test the gateway using your laptop's mic/speakers:

```bash
cd test/
pip install -r requirements.txt
python3 audio_bridge.py
```

**Speak into your laptop mic - you'll hear the LLM respond!**

See **[test/README.md](test/README.md)** for complete testing guide.

---

## 📚 Documentation

### User Guides
- **[Configuration Guide](docs/CONFIGURATION.md)** - All settings explained
- **[Audio Resampling Config](docs/AUDIO_RESAMPLING_CONFIG.md)** - Configure 24kHz→16kHz conversion
- **[Troubleshooting](docs/TROUBLESHOOTING.md)** - Common issues and fixes
- **[Security Guide](docs/SECURITY.md)** - Securing your gateway

### Technical Docs
- **[Architecture](docs/ARCHITECTURE.md)** - How it works internally
- **[Audio Resampling](docs/AUDIO_RESAMPLING.md)** - Technical deep dive on sample rate conversion
- **[Audio Formats Reference](docs/AUDIO_FORMATS_QUICK_REF.md)** - Format specs and calculations
- **[WebSocket Protocol](docs/STREAMING_GATEWAY_PROTOCOL.md)** - Device communication spec

### Testing
- **[Testing Resampling](test/TESTING_RESAMPLING.md)** - How to test audio resampling
- **[Test Script Guide](test/README.md)** - Using audio_bridge.py

---

## 🤝 Community & Support

- **Issues:** [GitHub Issues](https://github.com/rw4lll/ha-realtime-voice-gateway/issues)
- **Discussions:** [GitHub Discussions](https://github.com/rw4lll/ha-realtime-voice-gateway/discussions)
- **Home Assistant Forum:** [Community Thread](https://community.home-assistant.io/)

---

## 🎯 Supported Backends

| Backend | Status | Latency | Free Tier | Notes |
|---------|--------|---------|-----------|-------|
| **Gemini Live** | ✅ Production | ~200ms | ✅ **Yes** | ✨ Recommended - Free tier available! |
| **OpenAI Realtime** | 🚧 Coming soon | ~250ms | ❌ No | Higher quality, paid only |
| **Mock** | ✅ Testing | N/A | ✅ Yes | For development/testing |

---

## 📄 License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

---

## 🙏 Credits

- **ESP32 Firmware:** [Modified streaming firmware](https://github.com/your-firmware-repo)
- **Home Assistant:** Smart home platform
- **Gemini API:** LLM backend
- **Wyoming Protocol:** Original inspiration (now replaced with WebSocket)

---

**Status:** ✅ **Ready for Use**

Works with ESP32 devices running modified streaming firmware. Supports Gemini backend (free tier available). OpenAI support coming soon.

**Get Started:** [Jump to Quick Start](#-quick-start) ⬆️
