# Architecture Guide

This document provides a deep dive into the technical architecture of the Home Assistant Realtime Voice Gateway.

## System Architecture

```
[ESP32 Voice Device]                 [Home Assistant]
   ┌──────────┐                         ┌──────────┐
   │Wake Word │                         │ Services │
   │Mic/Spkr  │                         │REST API  │
   └────┬─────┘                         └────┬─────┘
        │                                    │
        │ WebSocket (ws://gateway:8080)     │ HTTPS
        │ • JSON control messages            │ (persistent)
        │ • Binary PCM audio                 │
        ▼                                    │
   ┌─────────────────────────────────────┐  │
   │   Realtime Voice Gateway            │  │
   │                                     │  │
   │  ┌────────────────────────────┐    │  │
   │  │ WebSocket Server           │    │  │
   │  │ • HTTP upgrade handler     │    │  │
   │  │ • Full-duplex streaming    │    │  │
   │  └────────────┬───────────────┘    │  │
   │               │                     │  │
   │  ┌────────────▼───────────────┐    │  │
   │  │ Session Handler            │    │  │
   │  │ • Per-device state         │    │  │
   │  │ • Lifecycle management     │    │  │
   │  │ • Safety timeouts          │    │  │
   │  └────────────┬───────────────┘    │  │
   │               │                     │  │
   │  ┌────────────▼───────────────┐    │  │
   │  │ Audio Pipeline             │    │  │
   │  │ • Bidirectional streaming  │    │  │
   │  │ • Audio buffering (500ms)  │    │  │
   │  │ • Barge-in support         │    │  │
   │  └────────────┬───────────────┘    │  │
   │               │                     │  │
   │  ┌────────────▼───────────────┐    │  │
   │  │ Backend Manager            │    │  │
   │  │ (Event Router)             │    │  │
   │  └─┬───────────────────────┬──┘    │  │
   │    │                       │        │  │
   │  ┌─▼─────────┐  ┌─────────▼──┐    │  │
   │  │ Gemini    │  │ OpenAI     │    │  │
   │  │ Live      │  │ Realtime   │    │  │
   │  │ (default) │  │ (planned)  │    │  │
   │  └───────────┘  └────────────┘    │  │
   │                                     │  │
   │  ┌─────────────────────────────┐   │  │
   │  │ HA Integration              │◄──┘  │
   │  │ • Tool registry & executor  │      │
   │  │ • Autodiscovery             │      │
   │  │ • Service call safety       │      │
   │  └─────────────────────────────┘      │
   └───────────────────────────────────────┘
```

## Tech Stack

**Language**: Go (Golang) — chosen for performance, concurrency, and single static binary deployment (Linux amd64 & arm64)

### Core Components

| Layer | Responsibility | Status | Key Packages |
|-------|---------------|--------|--------------|
| `internal/websocket` | WebSocket server & session management | ✅ **Complete** | `gorilla/websocket`, `net/http` |
| `internal/session` | Session handler (audio/events/tools) | ✅ **Complete** | channels & goroutines |
| `internal/backend` | LLM abstraction (event-driven interface) | ✅ **Complete** | custom interface |
| `internal/backend/gemini` | Gemini Live WebSocket integration | ✅ **Complete** | `google.golang.org/genai` |
| `internal/backend/openai` | OpenAI Realtime WebSocket | ⏳ Planned | TBD |
| `internal/pipeline` | Audio+event pipeline orchestration | ✅ **Complete** | channels & goroutines |
| `internal/ha` | Home Assistant API client (REST) | ✅ **Complete** | `net/http` |
| `internal/config` | Configuration management | ✅ **Complete** | `gopkg.in/yaml.v3` |
| `internal/metrics` | Prometheus metrics | ⏳ Future | `prometheus/client_golang` |

### Project Structure

```
ha-realtime-voice-gateway/
├── cmd/
│   └── gateway/
│       └── main.go                    # Entry point
├── internal/
│   ├── websocket/
│   │   ├── server.go                  # WebSocket HTTP server
│   │   ├── session.go                 # Per-connection session
│   │   └── protocol.go                # Message types & encoding
│   ├── session/
│   │   └── handler.go                 # Session logic (audio/events/tools)
│   ├── backend/
│   │   ├── interface.go               # Backend interface (event-driven)
│   │   ├── events.go                  # Common event types
│   │   ├── mock.go                    # Mock backend for testing
│   │   └── gemini/
│   │       ├── client.go              # Gemini Live API client
│   │       ├── session.go             # Gemini session management
│   │       └── audio.go               # Audio format handling
│   ├── ha/
│   │   ├── client.go                  # REST API client
│   │   ├── executor.go                # Tool executor
│   │   ├── autodiscovery.go           # Entity/service discovery
│   │   └── tools.go                   # Tool definitions
│   ├── pipeline/
│   │   ├── pipeline.go                # Main pipeline orchestration
│   │   └── metrics.go                 # Metrics collection
│   └── config/
│       ├── config.go                  # Configuration structs
│       └── ha_config.go               # HA-specific config (YAML)
├── docs/                               # Documentation
├── test/                               # Testing utilities
├── docker-compose.yml
├── Dockerfile
├── .env.example
├── go.mod
└── go.sum
```

## WebSocket Protocol Integration

The gateway implements a WebSocket server to communicate with ESP32 devices running modified streaming firmware. This is a **message-based protocol** (JSON control + binary PCM audio) that supports:

- **Full-duplex streaming** - Bidirectional audio flow
- **JSON control messages** - State synchronization (`listening`, `thinking`, `speaking`, `done`)
- **Binary audio frames** - Raw PCM audio (16kHz, 16-bit, mono)
- **Session lifecycle** - Start, timeouts, completion
- **Barge-in support** - Interrupt AI mid-response

### WebSocket Protocol Flow

```
// 1. Connection Established
ESP32 → Gateway: WebSocket Upgrade (ws://gateway:8080/voice-stream)
Gateway → ESP32: {"state": "listening"}

// 2. User Speaks
ESP32 → Gateway: Binary PCM audio (continuous stream)
Gateway → Backend: Audio frames forwarded

// 3. Backend Detects End-of-Speech (VAD)
Gateway → ESP32: {"state": "thinking"}

// 4. Backend Starts Speaking
Gateway → ESP32: {"state": "speaking"}
Gateway → ESP32: Binary PCM audio (AI response, continuous)

// 5. Backend Finishes Response
Gateway waits 3s for silence...
Gateway → ESP32: {"state": "done"}

// 6. Optional: Session Continue or Close
ESP32 can send new audio (continue conversation)
OR ESP32 closes WebSocket (end session)
```

### Message Format

**Control Messages (JSON, WebSocket text frames):**
```json
{"state": "listening"}   // Ready for user speech
{"state": "thinking"}    // Processing input
{"state": "speaking"}    // AI is responding
{"state": "done"}        // Conversation complete
{"error": "message"}     // Error occurred
```

**Audio Messages (Binary, WebSocket binary frames):**
- **Format**: Raw PCM, 16kHz, 16-bit, mono (little-endian)
- **Direction**: Bidirectional (device ↔ gateway)
- **Streaming**: Continuous, no frame headers or delimiters
- **Chunk Size**: Variable (typically 320-640 bytes, 10-20ms audio)

### Why WebSocket Works

1. **Low Latency** - Direct connection, persistent, no HTTP overhead per message
2. **Full Duplex** - Can send audio while receiving responses (barge-in)
3. **Simple Protocol** - JSON + binary, easy to implement on ESP32
4. **Browser Compatible** - Can test from web browsers using JavaScript
5. **Firewall Friendly** - Uses standard HTTP/HTTPS ports

### WebSocket vs. Wyoming Protocol

| Feature | WebSocket | Wyoming (Old) |
|---------|-----------|---------------|
| **Transport** | WebSocket (HTTP upgrade) | Raw TCP socket |
| **Control Messages** | JSON (text frames) | JSONL + length prefixes |
| **Audio** | Binary frames | JSONL event + binary payload |
| **Connection** | Standard HTTP upgrade | Custom handshake |
| **Browser Testing** | ✅ Native support | ❌ Requires custom client |
| **Device Firmware** | ✅ Modified for streaming | ✅ Standard HA Voice Preview |

**Why We Switched:** WebSocket provides better latency, simpler implementation, and native browser support for testing, while Wyoming required Home Assistant pipeline integration we didn't need.

## Audio Flow

1. **Device detects wake word (or user presses button)** → Opens WebSocket connection
2. **Gateway** sends `{"state": "listening"}` → Device starts streaming PCM audio
3. **Gateway** forwards audio → LLM backend (Gemini/OpenAI)
4. **LLM** processes speech (VAD detects end) → Gateway sends `{"state": "thinking"}`
5. **LLM** starts generating audio response → Gateway sends `{"state": "speaking"}`
6. **Gateway** streams audio back → Device plays immediately (low-latency)
7. **User interrupts (barge-in)** → Device sends new audio → Gateway suppresses old audio
8. **LLM finishes** + 3s silence → Gateway sends `{"state": "done"}`

### Audio Buffering Strategy

To prevent premature audio-start events:
- **Gateway buffers** first 500ms of backend audio (configurable)
- **Ensures** backend is actually speaking (not just a glitch)
- **Flushes buffer** once confident audio is real
- **Prevents** false-positive "speaking" states

## Tool Call Flow

1. **LLM** needs to call Home Assistant service (e.g., "turn on living room light")
2. **Backend** emits `ToolCall` event → Gateway receives via `Events` channel
3. **Gateway** validates against allow-list → Executes HA API call (async)
4. **HA** returns result → Gateway sends to backend via `ToolResults` channel
5. **LLM** continues speaking with result incorporated

**Optimization**: Tool calls happen asynchronously while LLM continues generating audio ("Let me turn that on for you..." plays while API call executes)

## Backend Abstraction Layer

The backend interface is **event-driven** to handle the asynchronous, bidirectional nature of realtime LLMs:

```go
// internal/backend/interface.go

type Backend interface {
    // Initialize with configuration and system prompt
    Init(ctx context.Context, cfg Config) error
    
    // Start a new session and return bidirectional channels
    StartSession(ctx context.Context, req SessionConfig) (*Session, error)
    
    // Register available Home Assistant tools
    RegisterTools(tools []Tool) error
    
    // Get backend capabilities
    Capabilities() Capabilities
    
    // Close backend and cleanup
    Close() error
}

type Session struct {
    ID          string
    
    // Audio channels (bidirectional)
    AudioIn     chan<- AudioFrame    // Gateway → LLM (user speech)
    AudioOut    <-chan AudioFrame    // LLM → Gateway (response audio)
    
    // Event channel (LLM → Gateway)
    Events      <-chan *Event        // Transcripts, tool calls, errors
    
    // Tool results channel (Gateway → LLM)
    ToolResults chan<- ToolResult    // Results of HA tool executions
    
    // Control
    Close       func() error         // Close this session
}

type SessionConfig struct {
    SessionID    string
    DeviceID     string
    SystemPrompt string
    AudioFormat  AudioFormat
}

type Event struct {
    Type      EventType
    Timestamp time.Time
    SessionID string
    Data      interface{}
}

type EventType string
const (
    EventTranscriptDelta    EventType = "transcript_delta"    // Partial transcript
    EventTranscriptDone     EventType = "transcript_done"     // Final transcript
    EventAudioStart         EventType = "audio_start"         // LLM starts speaking
    EventAudioEnd           EventType = "audio_end"           // LLM stops speaking
    EventAudioInterrupted   EventType = "audio_interrupted"   // User interrupted (barge-in)
    EventToolCall           EventType = "tool_call"           // LLM requests tool
    EventToolCallDone       EventType = "tool_call_done"      // Tool execution complete
    EventError              EventType = "error"               // Error occurred
    EventSessionEnd         EventType = "session_end"         // Session terminated
)

type ToolCall struct {
    ID        string                 // Unique call ID
    Name      string                 // e.g., "light.turn_on"
    Arguments map[string]interface{} // Tool parameters
}

type ToolResult struct {
    CallID    string                 // Matches ToolCall.ID
    Result    interface{}            // Success result
    Error     string                 // Error message if failed
    Timestamp time.Time
}

type AudioFrame struct {
    Data      []byte     // Raw PCM audio
    Timestamp time.Time
}

type AudioFormat struct {
    SampleRate    int    // e.g., 16000
    Channels      int    // e.g., 1 (mono)
    BitsPerSample int    // e.g., 16
    Encoding      string // "pcm", "opus", etc.
}

type Capabilities struct {
    SupportsStreaming     bool
    SupportsToolCalling   bool
    SupportsVAD           bool
    SupportedAudioFormats []AudioFormat
}
```

### Why Event-Driven?

1. **Asynchronous by nature**: LLMs stream responses while processing tool calls
2. **Non-blocking tool execution**: Gateway can execute HA calls while LLM continues talking
3. **Multiple event types**: Transcripts, audio, tool calls, errors all flow through same pattern
4. **Barge-in support**: Can interrupt LLM mid-response via `EventAudioInterrupted`
5. **Easy to extend**: New event types don't break the interface

### Example Backend Implementation Flow

```go
// Backend receives audio and emits events
func (g *GeminiBackend) StartSession(ctx context.Context, cfg SessionConfig) (*Session, error) {
    sess := &Session{
        ID:          cfg.SessionID,
        AudioIn:     make(chan AudioFrame, 100),
        AudioOut:    make(chan AudioFrame, 100),
        Events:      make(chan *Event, 50),
        ToolResults: make(chan ToolResult, 10),
    }
    
    // Connect to Gemini Live API
    client, err := g.connectGeminiLive(ctx, cfg)
    
    // Start goroutines to handle bidirectional streams
    go g.audioInputLoop(sess, client)   // AudioIn → Gemini
    go g.audioOutputLoop(sess, client)  // Gemini → AudioOut
    go g.eventLoop(sess, client)        // Gemini events → Events channel
    go g.toolResultLoop(sess, client)   // ToolResults → Gemini
    
    return sess, nil
}
```

### Gateway Pipeline Example

```go
// internal/pipeline/pipeline.go
// Gateway coordinates between WebSocket device, Backend, and HA

func (p *Pipeline) HandleWebSocketSession(ws *websocket.Session) {
    // Start backend session
    backendSession, err := p.backend.StartSession(ctx, backend.SessionConfig{
        SessionID:    ws.ID,
        DeviceID:     "esp32-voice-device",
        SystemPrompt: p.systemPrompt,
        AudioFormat:  backend.AudioFormat{
            SampleRate:    16000,
            Channels:      1,
            BitsPerSample: 16,
            Encoding:      "pcm",
        },
    })
    
    // Create session handler (manages audio/events/tools)
    handler := session.NewHandler(session.Config{
        WebSocketSession: ws,
        BackendSession:   backendSession,
        ToolExecutor:     p.toolExecutor,
        Logger:           p.logger,
        AudioBufferMs:    500,
        SafetyTimeout:    5 * time.Minute,
    })
    
    // Start session (blocks until complete)
    handler.Start()
    handler.Wait()
}
```

### Session Handler Example

```go
// internal/session/handler.go
// Session handler manages a single voice conversation

func (h *Handler) Start() {
    // Send initial state to device
    h.ws.SendState("listening")
    
    // Start goroutines for bidirectional flow
    go h.forwardAudioToBackend()           // Device → Backend
    go h.forwardBackendAudioToDevice()     // Backend → Device (with buffering)
    go h.handleBackendEvents()             // Process backend events
    go h.monitorSessionLifecycle()         // Timeouts and completion
}

func (h *Handler) handleBackendEvents() {
    for event := range h.backendSession.Events {
        switch event.Type {
        case backend.EventTranscriptDone:
            h.ws.SendState("thinking")
            
        case backend.EventAudioStart:
            h.ws.SendState("speaking")
            
        case backend.EventToolCall:
            go h.executeToolCall(event.Data.(backend.ToolCall))
            
        case backend.EventAudioInterrupted:
            h.suppressAudio()  // User interrupted - stop playback
            
        case backend.EventError:
            h.ws.SendError(event.Data.(error).Error())
        }
    }
}

func (h *Handler) executeToolCall(call backend.ToolCall) {
    // Execute via Home Assistant
    result, err := h.toolExecutor.Execute(h.ctx, &call)
    
    // Send result back to backend
    h.backendSession.ToolResults <- backend.ToolResult{
        CallID:    call.ID,
        Result:    result,
        Error:     err.Error(),
        Timestamp: time.Now(),
    }
}
```

## Benefits of This Architecture

✅ **Pluggable backends**: Add Anthropic, Mistral, local models without changing gateway  
✅ **Provider-agnostic**: Same tool schema across all LLMs  
✅ **Low latency**: Async tool calls don't block audio streaming  
✅ **Type-safe**: Strong typing for events and data structures  
✅ **Testable**: Easy to mock backends and WebSocket sessions  
✅ **Observable**: All events flow through channels (easy to log/monitor)  
✅ **Production-ready**: Timeouts, error handling, graceful shutdown

## Session Lifecycle

```
1. Device connects WebSocket
   ↓
2. Gateway sends {"state": "listening"}
   ↓
3. Device streams audio (user speaking)
   ↓
4. Backend detects end-of-speech (VAD)
   ↓
5. Gateway sends {"state": "thinking"}
   ↓
6. Backend generates response
   ↓
7. Gateway sends {"state": "speaking"}
   ↓
8. Gateway streams audio to device
   ↓
9. Backend finishes + 3s silence
   ↓
10. Gateway sends {"state": "done"}
    ↓
11. Device closes OR continues (new audio)
```

### Safety Controls

- **SafetyTimeout** (default: 5 minutes) - Max session duration
- **SilenceTimeout** (default: 3 seconds) - End conversation after silence
- **AudioBufferMs** (default: 500ms) - Buffer before starting playback
- **Read/Write Timeouts** - Prevent hung connections
- **Graceful Shutdown** - Clean up all resources on exit

## Observability & Monitoring

### Structured Logging

```json
{
  "timestamp": "2025-11-17T12:34:56Z",
  "level": "info",
  "msg": "tool_call_executed",
  "session_id": "abc123",
  "device_id": "esp32-living-room",
  "tool": "light.turn_on",
  "entity_id": "light.living_room",
  "duration_ms": 85,
  "result": "success"
}
```

### Metrics (Future)

```prometheus
# Session metrics
voice_gateway_sessions_active 3
voice_gateway_sessions_total{backend="gemini"} 142

# Audio metrics
voice_gateway_audio_frames_in 45823
voice_gateway_audio_frames_out 38192

# Tool call metrics
voice_gateway_tool_calls_total{domain="light",result="success"} 89
voice_gateway_tool_call_duration_seconds{domain="light"} 0.08

# Error metrics
voice_gateway_errors_total{type="backend_error"} 2
```

## Default Provider

The gateway uses **Gemini Live** by default — thanks to its free tier, simple WebSocket API, and excellent voice quality. Switching to OpenAI Realtime (or others) requires only changing `BACKEND_TYPE` in `.env`.

## See Also

- [CONFIGURATION.md](CONFIGURATION.md) - Complete configuration reference
- [SECURITY.md](SECURITY.md) - Security and safety features
- [AUTODISCOVERY.md](AUTODISCOVERY.md) - Home Assistant autodiscovery
- [VAD_TUNING_GUIDE.md](VAD_TUNING_GUIDE.md) - Voice activity detection tuning
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Common issues and solutions
- [STREAMING_GATEWAY_PROTOCOL.md](../STREAMING_GATEWAY_PROTOCOL.md) - WebSocket protocol spec
