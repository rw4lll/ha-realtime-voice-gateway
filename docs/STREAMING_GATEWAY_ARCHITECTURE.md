# Streaming Voice Gateway - Architecture Overview

## What We Built

A **full-duplex streaming voice system** that allows ESP32 voice devices to bypass Home Assistant's pipeline and connect directly to a custom AI gateway for ultra-low latency voice interactions.

## Key Design Decisions

### 1. Gateway-Controlled Flow ✅

**Decision:** Gateway has complete control over conversation flow, VAD, and session management.

**Why:**
- Device (ESP32) has limited resources - keep it simple
- VAD algorithms are complex and need tuning - centralize them
- Different AI providers have different flow patterns - gateway abstracts this
- Easier to update logic without firmware changes

**How it works:**
```
Device: Just streams audio continuously after wake word
Gateway: Decides when user stopped talking, when to respond, when conversation ends
```

### 2. True Full-Duplex Streaming ✅

**Decision:** Microphone never stops until gateway explicitly says `{"state": "done"}`

**Why:**
- Allows natural interruptions (user can interrupt AI mid-response)
- No "click-to-talk" friction
- More natural conversation flow
- Matches modern AI voice interfaces (Gemini Live, OpenAI Realtime)

**How it works:**
```
Time →
├─ Wake Word
├─ Device: START streaming audio ──────────────────────────>
├─ Gateway: Listening...
├─ Gateway: Thinking... (device still streaming!)
├─ Gateway: Speaking... (device still streaming for interruption!)
├─ Gateway: VAD detects silence for 2-3s
└─ Gateway: {"state": "done"} ──> Device STOPS
```

### 3. WebSocket Protocol ✅

**Decision:** Use WebSocket with mixed binary/JSON frames

**Why:**
- **Lowest latency:** 1-3ms on local network
- **Full-duplex:** Bidirectional audio + control messages
- **Native support:** ESP-IDF has `esp_websocket_client.h`, Go has `gorilla/websocket`
- **Simple:** No custom protocol implementation needed
- **Debuggable:** Can use `wscat` or browser DevTools

**Protocol:**
```
Device → Gateway: Binary frames (16kHz, 16-bit PCM audio)
Gateway → Device: JSON frames (state control) + Binary frames (AI speech)
```

### 4. Dual-Mode System ✅

**Decision:** Keep existing Home Assistant voice assistant, add streaming mode as alternative

**Why:**
- Users might want both (HA for home control, streaming for AI chat)
- Gradual migration path
- Fallback if gateway is down
- Testing and comparison

**How to switch:**
```yaml
# Via Home Assistant UI
switch.voice_kit_use_streaming_gateway: on/off

# Via button press
Quadruple-press button → toggle mode
```

### 5. Safety Timeouts Only ✅

**Decision:** Device has long timeouts (5 min conversation, 60s gateway silence), normal flow controlled by gateway

**Why:**
- Prevents stuck connections (security)
- Gateway might crash/restart
- Network issues
- But normal conversations should NEVER hit these timeouts

**Timeouts:**
```cpp
// Normal: Gateway sends {"state": "done"} after 2-3s silence
// Safety: 5 minutes max conversation (prevents stuck mic)
// Safety: 60 seconds no gateway message (prevents hung connection)
```

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│ ESP32-S3 Voice Device                                           │
│                                                                 │
│  ┌──────────┐      ┌─────────────────┐      ┌────────┐        │
│  │  Micro   │─────>│ streaming_voice │─────>│  LED   │        │
│  │Wake Word │      │    _gateway     │      │ Ring   │        │
│  └──────────┘      │   Component     │      └────────┘        │
│                    │                 │                         │
│  ┌──────────┐      │  ┌──────────┐  │      ┌────────┐        │
│  │   I²S    │<────>│  │WebSocket │  │<────>│  I²S   │        │
│  │   Mic    │      │  │  Client  │  │      │Speaker │        │
│  └──────────┘      │  └──────────┘  │      └────────┘        │
│                    └────────┬────────┘                         │
│                             │ ws://gateway:8080/voice-stream  │
└─────────────────────────────┼─────────────────────────────────┘
                              │
                              │ WebSocket (Binary audio + JSON)
                              │
┌─────────────────────────────┼─────────────────────────────────┐
│ Go Gateway Server           v                                 │
│                    ┌─────────────────┐                        │
│                    │  WebSocket      │                        │
│                    │  Handler        │                        │
│                    └────────┬────────┘                        │
│                             │                                 │
│            ┌────────────────┼────────────────┐               │
│            │                │                │               │
│     ┌──────v─────┐   ┌──────v─────┐  ┌──────v──────┐       │
│     │    VAD     │   │   Audio    │  │   Session   │       │
│     │  Detector  │   │   Buffer   │  │  Manager    │       │
│     └──────┬─────┘   └──────┬─────┘  └──────┬──────┘       │
│            │                │                │               │
│            └────────────────┼────────────────┘               │
│                             │                                 │
│                    ┌────────v────────┐                        │
│                    │  AI Provider    │                        │
│                    │   Integration   │                        │
│                    └────────┬────────┘                        │
│                             │                                 │
└─────────────────────────────┼─────────────────────────────────┘
                              │
                    ┌─────────┴─────────┐
                    │                   │
              ┌─────v─────┐      ┌─────v─────┐
              │  Gemini   │      │  OpenAI   │
              │   Live    │      │ Realtime  │
              └───────────┘      └───────────┘
```

## State Machine

### Device States

```
IDLE ──(wake word)──> START ──> LISTENING ──> THINKING ──> REPLYING ──> IDLE
                                     │            │            │
                                     └────────────┴────────────┘
                                     (continuous audio stream)
                                     
ERROR ──(cleanup)──> IDLE
```

### Gateway States

```
CONNECTED ──> LISTENING ──(VAD)──> THINKING ──> SPEAKING ──(VAD timeout)──> DONE
                  │                    │            │
                  │                    │            └──(user interrupts)──┐
                  │                    │                                  │
                  └────────────────────┴──────────────────────────────────┘
                  (continuous audio from device)
```

## Protocol Messages

### Device → Gateway

```
Binary WebSocket Frames
├─ Continuous 16-bit PCM audio
├─ 16kHz sample rate
├─ Mono channel
└─ ~1024 bytes per frame
```

### Gateway → Device

```
JSON Control Messages:
├─ {"state": "listening"}  // Gateway is listening
├─ {"state": "thinking"}   // AI is processing
├─ {"state": "speaking"}   // AI is responding
├─ {"state": "done"}       // Conversation complete (stop mic)
├─ {"command": "stop"}     // Emergency stop
└─ {"error": "message"}    // Error occurred

Binary Audio Frames:
└─ 16-bit PCM audio (AI speech, streamed)
```

## File Structure

```
home-assistant-voice-pe/
├── esphome/
│   └── components/
│       └── streaming_voice_gateway/
│           ├── __init__.py              # ESPHome component registration
│           ├── streaming_voice_gateway.h # C++ header (WebSocket client)
│           └── streaming_voice_gateway.cpp # C++ implementation
│
├── streaming-voice-gateway.yaml         # ESPHome configuration package
│
├── examples/
│   └── go-gateway/
│       ├── main.go                      # Go gateway implementation
│       ├── go.mod                       # Go dependencies
│       └── README.md                    # Gateway documentation
│
├── STREAMING_GATEWAY_PROTOCOL.md       # Protocol specification
└── STREAMING_GATEWAY_ARCHITECTURE.md   # This file
```

## Components

### 1. ESP32 Device Component (`streaming_voice_gateway`)

**Purpose:** Manage WebSocket connection and audio streaming

**Key Files:**
- `streaming_voice_gateway.h` - Component interface
- `streaming_voice_gateway.cpp` - WebSocket implementation
- `__init__.py` - ESPHome integration

**Responsibilities:**
- Connect to gateway WebSocket
- Stream audio continuously
- Play AI responses
- Handle state updates from gateway
- Safety timeouts

**Key Code:**
```cpp
// Start conversation (after wake word)
void start() {
  connect_websocket_();
  mic_->start();
  send_state(START);
}

// Continuous audio streaming
void stream_audio_() {
  mic_->read(buffer, size, &bytes_read);
  convert_32bit_to_16bit(buffer);
  ws_send_binary(buffer);
  // No VAD - just stream!
}

// Handle gateway messages
void handle_websocket_event_(data) {
  if (json) {
    if (state == "done") stop_mic();
  } else {
    speaker_->play(audio_data);
  }
}
```

### 2. Go Gateway Server

**Purpose:** Central intelligence for conversation management

**Key Files:**
- `main.go` - WebSocket server and session management
- `go.mod` - Dependencies (gorilla/websocket)

**Responsibilities:**
- Accept WebSocket connections
- Perform VAD (voice activity detection)
- Interface with AI providers
- Manage conversation state
- Send control messages to device

**Key Code:**
```go
// Handle device connection
func handleVoiceStream(conn) {
  session := NewVoiceSession(conn)
  session.SendState("listening")
  
  // Receive audio continuously
  for {
    _, audio, _ := conn.ReadMessage()
    aiProvider.ProcessAudio(audio)
    
    if vad.DetectSpeechEnd() {
      session.SendState("thinking")
    }
  }
}

// Monitor for conversation end
func monitorVAD(session) {
  if silenceAfterAI > 2*time.Second {
    session.SendState("done")
  }
}
```

### 3. ESPHome Configuration (`streaming-voice-gateway.yaml`)

**Purpose:** Configuration package to add streaming mode to existing firmware

**Key Features:**
- Component configuration
- Mode toggle (HA switch + button)
- Wake word routing
- LED integration

**Key Config:**
```yaml
# Component instance
streaming_voice_gateway:
  id: streaming_gw
  gateway_url: "ws://192.168.1.100:8080/voice-stream"
  microphone: mic_i2s
  speaker: speaker_i2s

# Mode toggle
globals:
  - id: use_streaming_gateway
    type: bool
    initial_value: 'false'

# Wake word routing
on_wake_word_detected:
  - if:
      condition:
        lambda: return id(use_streaming_gateway);
      then:
        - streaming_voice_gateway.start: streaming_gw
      else:
        - voice_assistant.start:  # HA pipeline
```

## Data Flow

### Conversation Lifecycle

1. **Wake Word Detected**
   ```
   User: "Hey Jarvis..."
   Device: Micro Wake Word detects → triggers start()
   ```

2. **Gateway Connection**
   ```
   Device: Connect WebSocket to gateway
   Gateway: Accept connection, initialize session
   Gateway → Device: {"state": "listening"}
   Device: Update LED to "listening" pattern
   ```

3. **Audio Streaming (Continuous)**
   ```
   Device: Read mic → Convert 32→16 bit → Send binary frame
   Device: Read mic → Convert 32→16 bit → Send binary frame
   Device: Read mic → Convert 32→16 bit → Send binary frame
   ... (continuous loop)
   Gateway: Accumulate audio, forward to AI
   ```

4. **VAD: End of Speech**
   ```
   Gateway: VAD detects 1.5s silence
   Gateway → AI: Finalize input
   Gateway → Device: {"state": "thinking"}
   Device: Update LED to "thinking" pattern
   Device: Still streaming audio! (for potential interruption)
   ```

5. **AI Processing**
   ```
   AI: Processing user request...
   AI: Generating response...
   AI: Start streaming TTS audio
   ```

6. **AI Response**
   ```
   Gateway → Device: {"state": "speaking"}
   Gateway → Device: [binary audio frame 1]
   Gateway → Device: [binary audio frame 2]
   Gateway → Device: [binary audio frame 3]
   ...
   Device: Play through speaker
   Device: Still streaming mic audio! (user can interrupt)
   ```

7. **Conversation End**
   ```
   Gateway: VAD detects 2-3s silence after AI finished
   Gateway → Device: {"state": "done"}
   Device: Stop microphone
   Device: Close WebSocket
   Device: Return to IDLE, listening for wake word
   ```

## Comparison: HA Pipeline vs Streaming Gateway

| Feature | Home Assistant Pipeline | Streaming Gateway |
|---------|------------------------|------------------|
| **Latency** | 2-5 seconds | <500ms |
| **Architecture** | Multi-step (STT→Intent→TTS) | Single streaming connection |
| **AI Provider** | HA-supported only | Any (Gemini, OpenAI, custom) |
| **Conversation** | Turn-based | Full-duplex, interruptible |
| **VAD** | On device or cloud | On gateway (flexible) |
| **Home Control** | ✅ Native integration | ⚡ Via gateway tools/API |
| **Setup** | Built-in | Requires gateway server |
| **Network** | Sends to HA server | Sends to custom gateway |

## Configuration Examples

### Basic Setup

```yaml
# home-assistant-voice.yaml (existing)
packages:
  streaming_gateway: !include streaming-voice-gateway.yaml

# streaming-voice-gateway.yaml
streaming_voice_gateway:
  gateway_url: "ws://192.168.1.100:8080/voice-stream"
  microphone: mic_i2s
  speaker: speaker_i2s
  on_error:
    - logger.log: "Gateway error!"
```

### With Home Assistant Controls

```yaml
# Expose toggle to HA
switch:
  - platform: template
    name: "Use Streaming Gateway"
    id: streaming_mode_switch
    lambda: return id(use_streaming_gateway);
    turn_on_action:
      - globals.set:
          id: use_streaming_gateway
          value: 'true'
    turn_off_action:
      - globals.set:
          id: use_streaming_gateway
          value: 'false'

# Add status sensor
sensor:
  - platform: template
    name: "Voice Gateway State"
    lambda: return id(streaming_gateway_phase);
```

### Advanced: Multiple Gateways

```yaml
# Production gateway
streaming_voice_gateway:
  - id: prod_gateway
    gateway_url: "ws://prod.example.com/voice"
    microphone: mic_i2s
    speaker: speaker_i2s

# Development gateway
  - id: dev_gateway
    gateway_url: "ws://localhost:8080/voice-stream"
    microphone: mic_i2s
    speaker: speaker_i2s

# Switch between them
script:
  - id: use_prod
    then:
      - streaming_voice_gateway.start: prod_gateway
  - id: use_dev
    then:
      - streaming_voice_gateway.start: dev_gateway
```

## Security Considerations

### Device Side

1. **TLS/WSS Support**
   ```yaml
   streaming_voice_gateway:
     gateway_url: "wss://gateway.example.com/voice"  # Use WSS in production
   ```

2. **Token Authentication**
   ```cpp
   // In streaming_voice_gateway.cpp, add header
   esp_websocket_client_config_t ws_cfg = {};
   ws_cfg.uri = gateway_url.c_str();
   ws_cfg.headers = "Authorization: Bearer device-token-here";
   ```

3. **Certificate Validation**
   ```yaml
   streaming_voice_gateway:
     gateway_url: "wss://gateway.example.com/voice"
     certificate: !secret gateway_cert
   ```

### Gateway Side

1. **Authentication**
   ```go
   func handleVoiceStream(w, r) {
     token := r.Header.Get("Authorization")
     if !validateDeviceToken(token) {
       w.WriteHeader(401)
       return
     }
   }
   ```

2. **Rate Limiting**
   ```go
   var limiter = rate.NewLimiter(10, 50)
   if !limiter.Allow() {
     return error
   }
   ```

3. **Firewall Rules**
   ```bash
   # Only allow known device IPs
   iptables -A INPUT -p tcp --dport 8080 -s 192.168.1.0/24 -j ACCEPT
   iptables -A INPUT -p tcp --dport 8080 -j DROP
   ```

## Performance Metrics

### Expected Performance (Local Network)

| Metric | Value |
|--------|-------|
| WebSocket Latency | 1-3ms |
| Audio Frame Rate | 15.625 fps (16kHz / 1024 bytes) |
| Throughput | ~32 KB/s per direction |
| Max Concurrent Sessions | 100+ (depends on gateway resources) |
| Memory per Session (ESP32) | ~50KB |
| Memory per Session (Gateway) | ~500KB |

### Monitoring

```go
// Add Prometheus metrics
var (
    activeConnections = prometheus.NewGauge(...)
    audioFramesReceived = prometheus.NewCounter(...)
    conversationDuration = prometheus.NewHistogram(...)
    vadDetections = prometheus.NewCounter(...)
)
```

## Testing

### Unit Tests

```go
// Test VAD detection
func TestVADDetection(t *testing.T) {
    vad := NewVAD()
    // Feed silence
    assert.False(t, vad.DetectSpeech(silenceAudio))
    // Feed speech
    assert.True(t, vad.DetectSpeech(speechAudio))
}
```

### Integration Tests

```bash
# Test device connection
wscat -c ws://localhost:8080/voice-stream

# Expected: {"state":"listening"}

# Send audio file
cat test_audio_16khz_16bit_mono.raw | wscat -c ws://localhost:8080/voice-stream -x
```

### End-to-End Tests

```yaml
# ESPHome test configuration
esphome:
  on_boot:
    - delay: 5s
    - streaming_voice_gateway.start: streaming_gw
    - delay: 10s
    - streaming_voice_gateway.stop: streaming_gw
```

## Troubleshooting

### Common Issues

1. **Device can't connect to gateway**
   - Check network connectivity: `ping gateway-host`
   - Verify gateway is running: `curl http://gateway-host:8080/health`
   - Check firewall rules
   - Verify WebSocket URL in device config

2. **Audio sounds distorted**
   - Verify sample rate matches (16kHz)
   - Check bit depth (16-bit)
   - Ensure mono channel
   - Check for buffer overruns in logs

3. **Conversation doesn't end**
   - VAD might be too sensitive (detecting background noise as speech)
   - Check gateway VAD timeout settings
   - Verify `{"state": "done"}` is being sent
   - Check device logs for received messages

4. **High latency**
   - Use local gateway (not cloud)
   - Check network quality
   - Monitor WebSocket ping/pong
   - Reduce audio buffer sizes

## Future Enhancements

- [ ] Multi-language support (detect language, route to appropriate model)
- [ ] Adaptive VAD (learn user's speech patterns)
- [ ] Background noise suppression (on gateway)
- [ ] Speaker identification (multi-user support)
- [ ] Conversation context persistence
- [ ] Offline mode (local AI model)
- [ ] Audio compression (Opus codec for bandwidth savings)
- [ ] IPv6 support
- [ ] mDNS discovery (auto-find gateway)
- [ ] Metrics dashboard

## References

- [STREAMING_GATEWAY_PROTOCOL.md](./STREAMING_GATEWAY_PROTOCOL.md) - Detailed protocol specification
- [examples/go-gateway/README.md](./examples/go-gateway/README.md) - Go gateway documentation
- [ESP-IDF WebSocket Client](https://docs.espressif.com/projects/esp-idf/en/latest/esp32/api-reference/protocols/esp_websocket_client.html)
- [gorilla/websocket](https://github.com/gorilla/websocket)
- [Gemini Live API](https://ai.google.dev/gemini-api/docs/live)
- [OpenAI Realtime API](https://platform.openai.com/docs/guides/realtime)

---

**Architecture Version:** 1.0  
**Last Updated:** November 2025  
**Status:** Production Ready
