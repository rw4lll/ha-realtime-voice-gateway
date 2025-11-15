# Claude.md - AI Assistant Development Guide

**Home Assistant Realtime Voice Gateway** - Context for AI assistants

---

## Project Overview

**Purpose**: Low-latency (<500ms) voice bridge between HA Voice Preview devices and realtime LLMs (Gemini Live, OpenAI Realtime)  
**Language**: Go 1.25+  
**Architecture**: Event-driven, channel-based, async tool execution

### Core Concept

Wyoming protocol bridge that:
1. Receives audio from HA Voice Preview devices (Wyoming TCP protocol)
2. Forwards to realtime LLM backends via WebSocket/WebRTC
3. Streams LLM responses back to devices
4. Enables LLM→HA control via async tool/function calling

**Critical Insight**: Wyoming is event-based (JSONL + binary payloads), perfect for streaming—no device reflashing needed. We expose **two separate Wyoming servers**: STT (10200) for device→gateway, TTS (10201) for gateway→device.

## Tech Stack

**Key Packages**: `nhooyr.io/websocket`, `github.com/pion/webrtc/v4`, `prometheus/client_golang`, `go.uber.org/zap`, `github.com/spf13/viper`

### Unique Design Patterns

1. **Event-Driven Backend**: All LLM backends use channel-based communication (no request-response blocking)
2. **Dual Wyoming Servers**: Separate TCP servers for STT (audio in) and TTS (audio out)
3. **Async Tool Execution**: HA service calls run in goroutines while LLM continues streaming audio
4. **Session-Per-Device**: Each Wyoming connection = isolated session with its own backend connection
5. **Audio Pipeline**: Wyoming → [Transcode?] → Backend → [Transcode?] → Wyoming (transcode only if sample rates differ)

## Project Structure

```
cmd/gateway/main.go              # Entry point
internal/
  protocol/wyoming/              # Wyoming TCP servers (STT:10200, TTS:10201) + codec
  backend/                       # LLM abstraction (interface.go + gemini/ + openai/)
  audio/                         # Transcoding, VAD, buffers
  ha/                            # HA API client, tool registry, executor
  session/                       # Multi-device session manager
  pipeline/                      # Audio+event orchestration
  config/                        # Config loading
  metrics/                       # Prometheus metrics
```

## Project-Specific Conventions

- **Prefer varchar/numbers over enums** in any future database schema (easier updates)
- **Buffered channels**: Default 50 for events, 100 for audio frames (tune via profiling)
- **Context timeouts**: 5s for tool calls, 3s for HA API, 1s for network
- **Naming**: Interfaces describe behavior (`Backend`, `Transcoder`), implementations are nouns (`GeminiBackend`)

## Critical Architecture Decisions

### 1. Event-Driven Backend Interface

**All LLM backends use this channel-based interface**:

```go
type Backend interface {
    StartSession(ctx context.Context, req SessionRequest) (*Session, error)
    RegisterTools(tools []Tool) error
    Close() error
}

type Session struct {
    AudioIn     chan<- AudioFrame    // Gateway → LLM (user speech)
    AudioOut    <-chan AudioFrame    // LLM → Gateway (responses)
    Events      <-chan Event         // Transcripts, tool calls, errors
    ToolResults chan<- ToolResult    // Tool results → LLM
    Interrupt   chan<- struct{}      // Barge-in signal
    Close       func() error
}
```

**Why channels?** Enables non-blocking async communication. Tool calls execute in goroutines while audio continues streaming.

### 2. Wyoming Protocol Implementation

**Format**: JSONL + binary payload
```
{"type": "audio-chunk", "data": {"rate": 16000, "width": 2, "channels": 1}, "payload_length": 4096}\n
<4096 bytes of PCM>
```

**Critical**: Parse JSON header → read exact `payload_length` bytes. Validate audio format (16kHz, 16-bit, mono).

### 3. Audio Transcoding

- **Gemini Live**: 16kHz PCM (matches Wyoming, no transcoding)
- **OpenAI Realtime**: 24kHz PCM (requires resampling both directions)

### 4. Tool Execution Pattern

```go
// Async - doesn't block audio streaming
go func(call ToolCall) {
    // Validate allow-list → Execute HA API → Send result to LLM
    session.ToolResults <- ToolResult{CallID: call.ID, Result: result}
}(call)
```

## Testing Strategy

### Mock Backend Pattern

```go
type MockBackend struct {
    sessions map[string]*Session
}

func (m *MockBackend) StartSession(ctx context.Context, req SessionRequest) (*Session, error) {
    return &Session{
        AudioIn:  make(chan AudioFrame, 10),
        AudioOut: make(chan AudioFrame, 10),
        Events:   make(chan Event, 10),
    }, nil
}
```

### Critical Test Areas

1. **Wyoming Codec**: Test JSONL + binary roundtrip encoding/decoding
2. **Backend Interface**: Use mock backends to test session lifecycle
3. **Tool Execution**: Verify allow-list validation, async execution
4. **Audio Pipeline**: Benchmark latency (<10ms processing overhead)

**Coverage Goal**: 80%+ for `internal/backend`, `internal/protocol/wyoming`, `internal/ha`

**Commands**:
```bash
go test ./... -v                    # All tests
go test ./... -race                 # Race detection
go test ./internal/audio -bench=.   # Latency benchmarks
```

## Development Phases

**Phase 1**: Wyoming codec + TCP server → Test with synthetic client  
**Phase 2**: Backend interface + mock implementation → Test session lifecycle  
**Phase 3**: Gemini backend → Test audio flows through Gemini  
**Phase 4**: Pipeline orchestration → Test end-to-end with real device  
**Phase 5**: HA integration → Test tool execution

### Logging & Metrics

**Structured logging** (zap):
```go
logger.Info("session started",
    zap.String("session_id", sess.ID),
    zap.String("device_id", req.DeviceID),
    zap.String("backend", "gemini"))
```

**Key metrics**:
- `voice_gateway_latency_seconds{stage="wake_to_response"}` - End-to-end latency
- `voice_gateway_tool_calls_total{domain,result}` - Tool execution
- `voice_gateway_active_sessions` - Current sessions

## Project-Specific Gotchas

1. **Wyoming Binary Payloads**: Must read exact `payload_length` bytes after JSON header. Off-by-one = protocol desync.
2. **Dual Wyoming Servers**: Don't use same port for STT/TTS - devices expect separate TCP connections.
3. **Audio Format Validation**: Always verify 16kHz/16-bit/mono from device before forwarding to backend.
4. **Tool Call Timing**: Execute in goroutine and send result via `ToolResults` channel, don't block on response.
5. **Session Cleanup**: Use `sync.Map` for session storage (concurrent safe), cleanup stale sessions every 1min.

## Security Model

**Tool Execution Safety** (HA service calls):
1. Validate domain against `ALLOWED_DOMAINS` config
2. Check entity against `DENIED_ENTITIES` config  
3. Require voice confirmation for sensitive domains (e.g., `lock`)
4. Rate limit: max 10 tool calls per 60s per session
5. Audit log: Record all tool calls with user/device/timestamp

**Input Validation**:
- Wyoming audio: Reject if not 16kHz/16-bit/mono
- Max audio chunk: 8192 bytes (prevent memory exhaustion)
- HA responses: Sanitize before forwarding to LLM

## Performance Targets

**Latency Budget** (target <500ms, realistic ~610ms):
```
Wake word:        ~300ms (device)
Network:           ~50ms (device→gateway)
Gateway:           <10ms (processing)
LLM:              ~200ms (Gemini/OpenAI)
Network:           ~50ms (gateway→device)
──────────────────────────
Total:            ~610ms
```

**Optimization Strategies**:
1. Audio buffer: 4096 bytes (256ms @ 16kHz) - balance latency/efficiency
2. HA state cache: 30s TTL - avoid blocking API calls
3. **Result caching**: 5s TTL prevents duplicate tool calls (SHA-256 keyed)
4. Async tool execution - LLM continues talking while HA API executes
5. Connection pooling - reuse HA WebSocket
6. Prefer Gemini - generally faster than OpenAI for low latency
7. Buffer pools - `sync.Pool` for audio buffers (reduce GC pressure)

### Error Handling Best Practices

**Graceful Disconnect Detection** (implemented in `internal/protocol/wyoming/session.go`):
```go
// ✅ DO: Use proper error type checking
if errors.Is(err, io.EOF) { return true }
if errors.Is(err, net.ErrClosed) { return true }

var opErr *net.OpError
if errors.As(err, &opErr) {
    var errno syscall.Errno
    if errors.As(opErr.Err, &errno) {
        switch errno {
        case syscall.ECONNRESET:  // Connection reset
        case syscall.EPIPE:       // Broken pipe
        // ... handle gracefully at DEBUG level
        }
    }
}

// ❌ DON'T: Use string matching (fragile across platforms/versions)
if strings.Contains(err.Error(), "connection reset") { ... }
```

**Why This Matters**:
- Syscall errors are standardized across platforms
- `errors.Is()` and `errors.As()` are type-safe (Go 1.13+)
- Error messages can change between Go versions
- String matching is error-prone and not idiomatic

### Caching Implementation

**Tool Call Result Caching** (`internal/ha/executor.go`):
```go
// Cache prevents duplicate HA API calls within 5-second window
cacheKey := sha256(tool_name + json(arguments))
if cached := getFromCache(cacheKey); cached != nil {
    return cached  // <1ms response
}

// Execute tool call...
result := executeToolCall(call)

// Cache successful results only (errors never cached)
if result.Error == "" {
    storeInCache(cacheKey, result, 5*time.Second)
}
```

**Cache Characteristics**:
- TTL: 5 seconds (optimal for voice interactions)
- Key: SHA-256 hash of tool name + arguments
- Thread-safe: RWMutex protected
- Automatic cleanup: Every 10 seconds
- Statistics: Track hits/misses via `GetCacheStats()`

## Quick Reference

### Build & Run
```bash
# Local development
go run ./cmd/gateway

# Production build (static binary)
CGO_ENABLED=0 go build -ldflags="-w -s" -o gateway ./cmd/gateway

# Docker
docker-compose up -d
docker logs -f ha-voice-gateway

# Tests
go test ./... -v
go test ./... -race
```

### Health Check
```go
// GET /health endpoint returns:
{
    "status": "healthy",
    "backend_connected": true,
    "ha_connected": true,
    "active_sessions": 5
}
```

### Ports
- `10200` - Wyoming STT (device→gateway)
- `10201` - Wyoming TTS (gateway→device)  
- `9090` - Metrics & health

---

## AI Assistant Checklist

When implementing features:
- [ ] Define interface before implementation
- [ ] Write tests (use mock backends for isolation)
- [ ] Add structured logging (zap) at key points
- [ ] Add metrics if user-facing
- [ ] Wrap errors with context (`fmt.Errorf("...: %w", err)`)
- [ ] Use buffered channels (events: 50, audio: 100)
- [ ] Respect context cancellation
- [ ] Use `defer` for cleanup
- [ ] Document exported functions

## Resources

- **Wyoming Protocol**: https://github.com/rhasspy/wyoming (event-based JSONL + binary)
- **Gemini Live API**: https://ai.google.dev/gemini-api/docs/live-api (16kHz PCM, WebSocket)
- **OpenAI Realtime**: https://platform.openai.com/docs/guides/realtime (24kHz PCM, prefer WebRTC)
- **Pion WebRTC**: https://github.com/pion/webrtc (for OpenAI backend)
- **HA API**: https://developers.home-assistant.io/docs/api/rest (REST + WebSocket)

---

**Status**: Architecture finalized, ready for implementation  
**See**: `Readme.md` for detailed architecture and deployment guide

