package backend

import (
	"time"
)

// EventType represents the type of backend event
type EventType string

// Backend event types
const (
	// Audio events
	EventAudioStart       EventType = "audio_start"       // Backend starts speaking
	EventAudioEnd         EventType = "audio_end"         // Backend stops speaking
	EventAudioInterrupted EventType = "audio_interrupted" // Backend interrupted mid-response

	// Transcript events
	EventTranscriptDelta EventType = "transcript_delta" // Partial user transcript
	EventTranscriptDone  EventType = "transcript_done"  // Final user transcript

	// Tool call events
	EventToolCall     EventType = "tool_call"      // Backend requests tool execution
	EventToolCallDone EventType = "tool_call_done" // Tool execution complete (from backend perspective)

	// Error and control
	EventError      EventType = "error"       // Error occurred
	EventSessionEnd EventType = "session_end" // Session terminated
)

// Event represents an event from the backend
type Event struct {
	Type      EventType
	SessionID string
	Timestamp time.Time
	Data      any // Type depends on EventType
}

// AudioFrame represents a chunk of audio data
type AudioFrame struct {
	Data      []byte    // PCM audio data
	Timestamp time.Time // When this frame was generated
}

// TranscriptDelta represents partial transcript from user speech
type TranscriptDelta struct {
	Text       string
	IsFinal    bool    // If true, this is the final version
	Confidence float64 // 0.0 to 1.0
}

// Transcript represents final user transcript
type Transcript struct {
	Text       string
	Confidence float64
	Language   string
}

// ToolCall represents a request from the LLM to execute a tool
type ToolCall struct {
	ID        string         // Unique call ID (for matching with results)
	Name      string         // Tool name (e.g., "homeassistant.turn_on")
	Arguments map[string]any // Tool parameters
	Timestamp time.Time
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	CallID    string // Matches ToolCall.ID
	Result    any    // Success result data
	Error     string // Error message if failed
	Timestamp time.Time
	Metadata  map[string]any // Provider-specific metadata (e.g., Gemini scheduling hints)
}

// ErrorEvent represents an error from the backend
type ErrorEvent struct {
	Error   error
	Code    string // Error code for categorization
	Message string // Human-readable error message
}

// AudioFormat describes audio format parameters
type AudioFormat struct {
	SampleRate    int    // e.g., 16000 or 24000
	Channels      int    // e.g., 1 (mono) or 2 (stereo)
	BitsPerSample int    // e.g., 16
	Encoding      string // e.g., "pcm_s16le", "opus"
}

// SessionMetadata contains metadata about a backend session
type SessionMetadata struct {
	DeviceID  string
	UserID    string
	CreatedAt time.Time
	Model     string // e.g., "gemini-2.0-flash-exp"
	Backend   string // e.g., "gemini", "openai"
}

// NewEvent creates a new backend event
func NewEvent(eventType EventType, sessionID string, data any) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now(),
		SessionID: sessionID,
		Data:      data,
	}
}

// NewAudioStartEvent creates an audio start event
func NewAudioStartEvent(sessionID string) Event {
	return NewEvent(EventAudioStart, sessionID, nil)
}

// NewAudioEndEvent creates an audio end event
func NewAudioEndEvent(sessionID string) Event {
	return NewEvent(EventAudioEnd, sessionID, nil)
}

// NewAudioInterruptedEvent creates an audio interrupted event
func NewAudioInterruptedEvent(sessionID string) Event {
	return NewEvent(EventAudioInterrupted, sessionID, nil)
}

// NewTranscriptDeltaEvent creates a transcript delta event
func NewTranscriptDeltaEvent(sessionID string, delta TranscriptDelta) Event {
	return NewEvent(EventTranscriptDelta, sessionID, delta)
}

// NewTranscriptDoneEvent creates a transcript done event
func NewTranscriptDoneEvent(sessionID string, transcript Transcript) Event {
	return NewEvent(EventTranscriptDone, sessionID, transcript)
}

// NewToolCallEvent creates a tool call event
func NewToolCallEvent(sessionID string, toolCall ToolCall) Event {
	return NewEvent(EventToolCall, sessionID, toolCall)
}

// NewErrorEvent creates an error event
func NewErrorEvent(sessionID string, errEvent ErrorEvent) Event {
	return NewEvent(EventError, sessionID, errEvent)
}

// NewSessionEndEvent creates a session end event
func NewSessionEndEvent(sessionID string) Event {
	return NewEvent(EventSessionEnd, sessionID, nil)
}
