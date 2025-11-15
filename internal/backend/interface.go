package backend

import (
	"context"
	"fmt"
)

// Backend represents an LLM backend provider (Gemini, OpenAI, etc.)
type Backend interface {
	// Init initializes the backend with configuration
	Init(ctx context.Context, cfg Config) error

	// StartSession starts a new backend session and returns bidirectional channels
	StartSession(ctx context.Context, req SessionConfig) (*Session, error)

	// RegisterTools registers available Home Assistant tools
	RegisterTools(tools []Tool) error

	// Capabilities returns backend capabilities
	Capabilities() Capabilities

	// Close closes the backend and cleans up resources
	Close() error
}

// Session represents an active backend session with bidirectional channels
type Session struct {
	ID       string
	Metadata SessionMetadata

	// Audio channels (bidirectional)
	AudioIn  chan<- AudioFrame // Gateway → Backend (user speech)
	AudioOut <-chan AudioFrame // Backend → Gateway (LLM response audio)

	// Event channel (Backend → Gateway)
	Events <-chan Event // All events from backend (transcripts, tool calls, errors)

	// Tool results channel (Gateway → Backend)
	ToolResults chan<- ToolResult // Results of tool executions

	// Control channels
	Interrupt chan<- struct{}       // Signal to interrupt/stop current response (barge-in)
	Control   chan<- ControlMessage // Send control messages to backend (turn_complete, etc.)
	// TODO: Implement turn_complete handling when backends support explicit turn signaling
	// TODO: Add support for dynamic mute/unmute during active sessions
	// TODO: Consider adding pause/resume controls for streaming responses

	// Close function
	Close func() error

	// Context for session lifecycle
	ctx context.Context
}

// ControlMessage represents a control signal to the backend
// Control messages enable runtime configuration and signaling without audio interruption.
// Examples: signaling end of speaking turn, muting output, adjusting parameters.
type ControlMessage struct {
	Type ControlMessageType
	Data map[string]any
}

// ControlMessageType represents the type of control message
type ControlMessageType string

const (
	ControlTurnComplete ControlMessageType = "turn_complete" // Signal end of user's speaking turn
	ControlMute         ControlMessageType = "mute"          // Mute backend audio output
	ControlUnmute       ControlMessageType = "unmute"        // Unmute backend audio output
)

// SessionRequest contains parameters for starting a session
type SessionConfig struct {
	SessionID    string
	DeviceID     string
	UserID       string
	Model        string
	SystemPrompt string
	Context      map[string]any // Additional context (conversation history, etc.)
	AudioFormat  AudioFormat
	Extra        map[string]any
}

// Config holds backend configuration
type Config struct {
	// Provider-specific config
	APIKey  string
	Model   string
	BaseURL string // Optional custom endpoint

	// Audio configuration
	InputAudioFormat  AudioFormat
	OutputAudioFormat AudioFormat

	// Behavior
	Temperature  float64
	MaxTokens    int
	SystemPrompt string

	// Buffers
	AudioBufferSize int
	EventBufferSize int

	// Provider-specific settings
	Extra map[string]any
}

// Tool represents a tool/function that the LLM can call
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`         // JSON Schema
	Metadata    map[string]any `json:"metadata,omitempty"` // Provider-specific metadata (e.g., Gemini behavior hints)
}

// Capabilities describes what a backend supports
type Capabilities struct {
	SupportsStreaming     bool          // Streams audio in real-time
	SupportsToolCalling   bool          // Can call tools/functions
	SupportsBargeIn       bool          // Supports interruption
	SupportedAudioFormats []AudioFormat // Audio formats backend accepts
	MaxAudioChunkSize     int           // Maximum audio chunk size in bytes
	Provider              string        // "gemini", "openai", etc.
}

// Errors
var (
	ErrBackendNotInitialized = fmt.Errorf("backend not initialized")
	ErrSessionNotFound       = fmt.Errorf("session not found")
	ErrInvalidAudioFormat    = fmt.Errorf("invalid audio format")
	ErrToolNotRegistered     = fmt.Errorf("tool not registered")
	ErrBackendUnavailable    = fmt.Errorf("backend unavailable")
	ErrAuthenticationFailed  = fmt.Errorf("authentication failed")
	ErrRateLimited           = fmt.Errorf("rate limited")
)

// Context returns the session's context
func (s *Session) Context() context.Context {
	return s.ctx
}

// SendAudio sends audio to the backend (helper)
func (s *Session) SendAudio(data []byte) error {
	select {
	case s.AudioIn <- AudioFrame{Data: data}:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// SendToolResult sends a tool result to the backend (helper)
func (s *Session) SendToolResult(result ToolResult) error {
	select {
	case s.ToolResults <- result:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// TriggerInterrupt triggers a barge-in (helper)
func (s *Session) TriggerInterrupt() error {
	select {
	case s.Interrupt <- struct{}{}:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
		// Interrupt channel full or not ready
		return nil
	}
}
