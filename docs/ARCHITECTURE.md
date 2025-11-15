# Architecture Guide

This document provides a deep dive into the technical architecture of the Home Assistant Realtime Voice Gateway.

## System Architecture

```
[HA Voice Device]                    [Home Assistant]
   ┌──────────┐                         ┌──────────┐
   │Wake Word │                         │ Services │
   │Mic/Spkr  │                         │WebSocket │
   └────┬─────┘                         └────┬─────┘
        │                                    │
        │ Wyoming Protocol (TCP)             │ WebSocket
        │ • audio-start/chunk/stop           │ (persistent)
        │ • Event-based streaming            │
        ▼                                    │
   ┌─────────────────────────────────────┐  │
   │   Realtime Voice Gateway            │  │
   │                                     │  │
   │  ┌────────────────────────────┐    │  │
   │  │ Wyoming Server (STT+TTS)   │    │  │
   │  │ • Handles device protocol  │    │  │
   │  │ • Audio chunk streaming    │    │  │
   │  └────────────┬───────────────┘    │  │
   │               │                     │  │
   │  ┌────────────▼───────────────┐    │  │
   │  │ Session Manager            │    │  │
   │  │ • Multi-device/user        │    │  │
   │  │ • Conversation history     │    │  │
   │  └────────────┬───────────────┘    │  │
   │               │                     │  │
   │  ┌────────────▼───────────────┐    │  │
   │  │ Audio Pipeline             │    │  │
   │  │ • Format conversion        │    │  │
   │  │ • VAD (barge-in detect)    │    │  │
   │  │ • Buffer management        │    │  │
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
   │  │ (default) │  │ (optional) │    │  │
   │  └───────────┘  └────────────┘    │  │
   │                                     │  │
   │  ┌─────────────────────────────┐   │  │
   │  │ HA Integration              │◄──┘  │
   │  │ • Tool registry & executor  │      │
   │  │ • State cache & queries     │      │
   │  │ • Service call safety       │      │
   │  └─────────────────────────────┘      │
   └───────────────────────────────────────┘
```

## Tech Stack

**Language**: Go (Golang) — chosen for performance, concurrency, and single static binary deployment (Linux amd64 & arm64)

### Core Components

| Layer | Responsibility | Status | Key Packages |
|-------|---------------|--------|--------------|
| `internal/protocol/wyoming` | Wyoming TCP server & event handling | ✅ **Complete** | `net`, `encoding/json` |
| `internal/backend` | LLM abstraction (event-driven interface) | ✅ **Complete** | custom interface |
| `internal/backend/gemini` | Gemini Live WebSocket integration | ✅ **Complete** | `nhooyr.io/websocket` |
| `internal/backend/openai` | OpenAI Realtime WebSocket/WebRTC | ⏳ Planned | `github.com/pion/webrtc/v4` |
| `internal/pipeline` | Audio+event pipeline, barge-in | ✅ **Complete** | channels & goroutines |
| `internal/ha` | Home Assistant API client (REST + WS) | ✅ **Complete** | `net/http`, `gorilla/websocket` |
| `internal/session` | Multi-device session management | ⏳ Planned | `sync.Map` |
| `internal/audio` | Transcoding, VAD, buffer management | ⏳ Future | `github.com/go-audio/audio` |
| `internal/metrics` | Prometheus metrics | ⏳ Future | `prometheus/client_golang` |
| `internal/config` | Configuration management | ✅ **Complete** | `github.com/spf13/viper` |

### Project Structure

```
ha-realtime-voice-gateway/
├── cmd/
│   └── gateway/
│       └── main.go                    # Entry point
├── internal/
│   ├── protocol/
│   │   └── wyoming/
│   │       ├── server.go             # Wyoming TCP server
│   │       ├── events.go             # Event types & parsing
│   │       ├── session.go            # Per-connection session
│   │       └── codec.go              # JSONL + binary codec
│   ├── backend/
│   │   ├── interface.go              # Backend interface (event-driven)
│   │   ├── manager.go                # Backend lifecycle & routing
│   │   ├── events.go                 # Common event types
│   │   ├── gemini/
│   │   │   ├── client.go             # Gemini Live WebSocket
│   │   │   ├── session.go            # Gemini session management
│   │   │   └── audio.go              # Gemini audio format handling
│   │   └── openai/
│   │       ├── client.go             # OpenAI Realtime API
│   │       ├── session.go            # OpenAI session
│   │       └── webrtc.go             # WebRTC handling
│   ├── audio/
│   │   ├── transcoder.go             # Format conversion
│   │   ├── vad.go                    # Voice Activity Detection
│   │   ├── buffer.go                 # Ring buffers, frame handling
│   │   └── formats.go                # Audio format definitions
│   ├── ha/
│   │   ├── client.go                 # REST + WebSocket client
│   │   ├── tools.go                  # Tool definitions & registry
│   │   ├── executor.go               # Safe tool execution
│   │   ├── cache.go                  # State caching
│   │   └── types.go                  # HA API types
│   ├── session/
│   │   ├── manager.go                # Multi-device session management
│   │   ├── session.go                # Per-session state
│   │   └── storage.go                # Optional persistence interface
│   ├── pipeline/
│   │   ├── pipeline.go               # Main audio+event pipeline
│   │   ├── barge_in.go               # Interruption handling
│   │   └── latency.go                # Latency tracking
│   ├── config/
│   │   └── config.go                 # Configuration management
│   └── metrics/
│       └── metrics.go                # Prometheus metrics
├── pkg/
│   └── events/                       # Shared event types
│       └── events.go
├── docs/                              # Documentation
├── test/                              # Testing utilities
├── docker-compose.yml
├── Dockerfile
├── .env.example
├── go.mod
└── go.sum
```

## Wyoming Protocol Integration

The gateway implements Wyoming protocol servers to communicate with HA Voice Preview devices. Wyoming is an **event-based streaming protocol** (JSONL + PCM binary payloads) that supports:

- **Continuous audio streaming** via `audio-chunk` events
- **Stream boundaries** with `audio-start` / `audio-stop`
- **Bidirectional communication** (device ↔ gateway)
- **Raw PCM audio** (16kHz, 16-bit, mono)

### Wyoming Protocol Flow

```json
// Device → Gateway (user speaking)
{"type": "audio-start", "data": {"rate": 16000, "width": 2, "channels": 1}}\n
{"type": "audio-chunk", "payload_length": 4096}\n
<4096 bytes of PCM audio>
{"type": "audio-chunk", "payload_length": 4096}\n
<4096 bytes of PCM audio>
{"type": "audio-stop"}\n

// Gateway → Device (LLM responding)
{"type": "audio-start", "data": {"rate": 16000, "width": 2, "channels": 1}}\n
{"type": "audio-chunk", "payload_length": 4096}\n
<4096 bytes of PCM audio from LLM>
{"type": "audio-stop"}\n
```

### Why Wyoming Protocol Works

**Initially, it seemed Wyoming might be too limited** (being designed for traditional STT/TTS pipelines), but it's actually **ideal** for this use case:

- ✅ **Event-based streaming**: Uses JSONL + binary payloads (not request-response)
- ✅ **Continuous audio chunks**: `audio-chunk` events stream PCM audio in real-time
- ✅ **Stream boundaries**: `audio-start` / `audio-stop` for session management
- ✅ **Bidirectional**: Device ↔ Gateway communication works seamlessly
- ✅ **No device reflashing needed**: Works with HA Voice Preview devices out-of-the-box

## Audio Flow

1. **Device detects wake word** → sends `audio-start`, streams `audio-chunk` (PCM) → Gateway
2. **Gateway** transcodes if needed → forwards to LLM backend (Gemini/OpenAI)
3. **LLM** streams back audio in real-time → Gateway receives chunks
4. **Gateway** converts format if needed → sends `audio-chunk` to device (Wyoming)
5. **Device** plays audio immediately (low-latency streaming)
6. **VAD** detects user interruption → triggers barge-in (stops LLM audio)

## Tool Call Flow

1. **LLM** needs to call Home Assistant service (e.g., "turn on living room light")
2. **Backend** emits `ToolCall` event → Gateway receives
3. **Gateway** validates against allow-list → executes HA API call (async)
4. **HA** returns result → Gateway sends back to LLM
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
    StartSession(ctx context.Context, req SessionRequest) (*Session, error)
    
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
    Events      <-chan Event         // Transcripts, tool calls, errors, etc.
    
    // Tool results channel (Gateway → LLM)
    ToolResults chan<- ToolResult    // Results of HA tool executions
    
    // Control channels
    Interrupt   chan<- struct{}      // Signal barge-in
    Close       func() error         // Close this session
    
    Metadata    SessionMetadata
}

type SessionRequest struct {
    SessionID    string
    DeviceID     string
    UserID       string
    SystemPrompt string
    AudioFormat  AudioFormat
    Context      map[string]interface{} // Previous conversation context
}

type Event struct {
    Type      EventType
    Timestamp time.Time
    SessionID string
    Data      interface{}
}

type EventType string
const (
    EventTranscriptDelta  EventType = "transcript_delta"  // Partial transcript
    EventTranscriptDone   EventType = "transcript_done"   // Final transcript
    EventAudioStart       EventType = "audio_start"       // LLM starts speaking
    EventAudioEnd         EventType = "audio_end"         // LLM stops speaking
    EventToolCall         EventType = "tool_call"         // LLM requests tool
    EventToolCallDone     EventType = "tool_call_done"    // Tool execution complete
    EventError            EventType = "error"             // Error occurred
    EventSessionEnd       EventType = "session_end"       // Session terminated
)

type ToolCall struct {
    ID        string                 // Unique call ID
    Name      string                 // e.g., "homeassistant.turn_on"
    Arguments map[string]interface{} // Tool parameters
}

type ToolResult struct {
    CallID string                 // Matches ToolCall.ID
    Result interface{}            // Success result
    Error  string                 // Error message if failed
}

type AudioFormat struct {
    SampleRate   int    // e.g., 16000
    Channels     int    // e.g., 1 (mono)
    BitsPerSample int   // e.g., 16
    Encoding     string // "pcm_s16le", "opus", etc.
}

type Capabilities struct {
    SupportsStreaming     bool
    SupportsToolCalling   bool
    SupportsBargeIn       bool
    SupportedAudioFormats []AudioFormat
    MaxAudioChunkSize     int
}
```

### Why Event-Driven?

1. **Asynchronous by nature**: LLMs stream responses while processing tool calls
2. **Non-blocking tool execution**: Gateway can execute HA calls while LLM continues talking
3. **Multiple event types**: Transcripts, audio, tool calls, errors all flow through same pattern
4. **Barge-in support**: Can interrupt LLM mid-response
5. **Easy to extend**: New event types don't break the interface

### Example Backend Implementation Flow

```go
// Backend receives audio and emits events
func (g *GeminiBackend) StartSession(ctx context.Context, req SessionRequest) (*Session, error) {
    sess := &Session{
        ID:          req.SessionID,
        AudioIn:     make(chan AudioFrame, 100),
        AudioOut:    make(chan AudioFrame, 100),
        Events:      make(chan Event, 50),
        ToolResults: make(chan ToolResult, 10),
        Interrupt:   make(chan struct{}, 1),
    }
    
    // Connect to Gemini Live WebSocket
    ws, err := g.connectWebSocket(ctx)
    
    // Start goroutines to handle bidirectional streams
    go g.audioInputLoop(sess, ws)   // AudioIn → Gemini
    go g.audioOutputLoop(sess, ws)  // Gemini → AudioOut
    go g.eventLoop(sess, ws)        // Gemini events → Events channel
    go g.toolResultLoop(sess, ws)   // ToolResults → Gemini
    
    return sess, nil
}
```

### Gateway Pipeline Example

```go
// Gateway coordinates between Wyoming device, Backend, and HA
func (p *Pipeline) Run(wyomingSession *wyoming.Session, backendSession *backend.Session) {
    // Audio forwarding
    go p.forwardAudio(wyomingSession.AudioIn, backendSession.AudioIn)
    go p.forwardAudio(backendSession.AudioOut, wyomingSession.AudioOut)
    
    // Event handling
    for event := range backendSession.Events {
        switch event.Type {
        case backend.EventToolCall:
            go p.handleToolCall(event.Data.(backend.ToolCall), backendSession)
        
        case backend.EventTranscriptDone:
            p.logTranscript(event.Data.(string))
        
        case backend.EventError:
            p.handleError(event.Data.(error))
        }
    }
}

func (p *Pipeline) handleToolCall(call backend.ToolCall, session *backend.Session) {
    // Validate and execute HA service call
    result, err := p.haClient.ExecuteTool(call.Name, call.Arguments)
    
    // Send result back to LLM (non-blocking)
    session.ToolResults <- backend.ToolResult{
        CallID: call.ID,
        Result: result,
        Error:  err.Error(),
    }
}
```

## Benefits of This Architecture

✅ **Pluggable backends**: Add Anthropic, Mistral, local models without changing gateway  
✅ **Provider-agnostic**: Same tool schema across all LLMs  
✅ **Low latency**: Async tool calls don't block audio streaming  
✅ **Type-safe**: Strong typing for events and data structures  
✅ **Testable**: Easy to mock backends for testing  
✅ **Observable**: All events flow through channels (easy to log/monitor)

## Observability & Monitoring

### Prometheus Metrics

```prometheus
# Latency metrics
voice_gateway_latency_seconds{stage="wake_to_response"} 0.45
voice_gateway_latency_seconds{stage="llm_first_audio"} 0.32
voice_gateway_audio_rtt_seconds 0.15

# Tool call metrics
voice_gateway_tool_calls_total{domain="light",result="success"} 142
voice_gateway_tool_calls_total{domain="lock",result="denied"} 3
voice_gateway_tool_call_duration_seconds{domain="light"} 0.08

# Session metrics
voice_gateway_active_sessions 5
voice_gateway_sessions_total{backend="gemini"} 234
voice_gateway_barge_ins_total 18

# Audio metrics
voice_gateway_audio_chunks_total{direction="in"} 45823
voice_gateway_audio_chunks_total{direction="out"} 38192
voice_gateway_audio_buffer_overruns_total 0

# Error metrics
voice_gateway_errors_total{type="backend_error"} 2
voice_gateway_errors_total{type="tool_denied"} 5
```

### Structured Logging

```json
{
  "timestamp": "2025-11-02T12:34:56Z",
  "level": "info",
  "msg": "tool_call_executed",
  "session_id": "abc123",
  "device_id": "living_room_voice",
  "user_id": "alice",
  "tool": "homeassistant.turn_on",
  "entity_id": "light.living_room",
  "duration_ms": 85,
  "result": "success"
}
```

### Health Checks

```bash
# Health endpoint
GET /health

{
  "status": "healthy",
  "backend": "gemini",
  "backend_connected": true,
  "ha_connected": true,
  "active_sessions": 5,
  "uptime_seconds": 86400
}
```

## Default Provider

By default, the Gateway uses **Gemini Live** — thanks to its free tier and simple WebSocket streaming API — to minimize entry cost during development.

Switching to OpenAI Realtime (or others) requires only changing `LLM_BACKEND` and relevant keys in `.env`.

## See Also

- [CONFIGURATION.md](CONFIGURATION.md) - Complete configuration reference
- [SECURITY.md](SECURITY.md) - Security and safety features
- [AUTODISCOVERY.md](AUTODISCOVERY.md) - Home Assistant autodiscovery
- [VAD_TUNING_GUIDE.md](VAD_TUNING_GUIDE.md) - Voice activity detection tuning
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Common issues and solutions
