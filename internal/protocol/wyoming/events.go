package wyoming

import "time"

// EventType represents the type of Wyoming protocol event
type EventType string

// Wyoming protocol event types
const (
	EventAudioChunk       EventType = "audio-chunk"
	EventAudioStart       EventType = "audio-start"
	EventAudioStop        EventType = "audio-stop"
	EventDescribe         EventType = "describe"
	EventInfo             EventType = "info"
	EventRunPipeline      EventType = "run-pipeline"
	EventStreamingStarted EventType = "streaming-started"
	EventStreamingStopped EventType = "streaming-stopped"
)

// Event represents a Wyoming protocol event
type Event struct {
	Type          EventType      `json:"type"`
	Data          map[string]any `json:"data,omitempty"`
	DataLength    int            `json:"data_length,omitempty"`
	PayloadLength int            `json:"payload_length,omitempty"`

	// Payload is the binary data (not part of JSON)
	Payload []byte `json:"-"`
}

// AudioFormat represents the audio format parameters
type AudioFormat struct {
	Rate     int `json:"rate"`     // Sample rate in Hz (e.g., 16000)
	Width    int `json:"width"`    // Sample width in bytes (e.g., 2 for 16-bit)
	Channels int `json:"channels"` // Number of channels (e.g., 1 for mono)
}

// Timestamp in milliseconds
type Timestamp int64

// AudioChunkEvent creates an audio chunk event
func AudioChunkEvent(format AudioFormat, payload []byte, timestamp *time.Time) Event {
	data := map[string]any{
		"rate":     format.Rate,
		"width":    format.Width,
		"channels": format.Channels,
	}

	if timestamp != nil {
		data["timestamp"] = timestamp.UnixMilli()
	}

	return Event{
		Type:          EventAudioChunk,
		Data:          data,
		PayloadLength: len(payload),
		Payload:       payload,
	}
}

// AudioStartEvent creates an audio start event
func AudioStartEvent(format AudioFormat, timestamp *time.Time) Event {
	data := map[string]any{
		"rate":     format.Rate,
		"width":    format.Width,
		"channels": format.Channels,
	}

	if timestamp != nil {
		data["timestamp"] = timestamp.UnixMilli()
	}

	return Event{
		Type: EventAudioStart,
		Data: data,
	}
}

// AudioStopEvent creates an audio stop event
func AudioStopEvent(timestamp *time.Time) Event {
	event := Event{
		Type: EventAudioStop,
	}

	if timestamp != nil {
		event.Data = map[string]any{
			"timestamp": timestamp.UnixMilli(),
		}
	}

	return event
}

// DescribeEvent creates a describe event (request for service info)
func DescribeEvent() Event {
	return Event{
		Type: EventDescribe,
	}
}

// RunPipelineEvent creates a run-pipeline event
func RunPipelineEvent(startStage, endStage string, wakeWordName string, restartOnEnd bool) Event {
	data := map[string]any{
		"start_stage": startStage,
		"end_stage":   endStage,
	}

	if wakeWordName != "" {
		data["wake_word_name"] = wakeWordName
	}

	if restartOnEnd {
		data["restart_on_end"] = true
	}

	return Event{
		Type: EventRunPipeline,
		Data: data,
	}
}

// StreamingStartedEvent creates a streaming-started event
func StreamingStartedEvent() Event {
	return Event{
		Type: EventStreamingStarted,
	}
}

// StreamingStoppedEvent creates a streaming-stopped event
func StreamingStoppedEvent() Event {
	return Event{
		Type: EventStreamingStopped,
	}
}

// DescribeResponseEvent creates a describe response event with service capabilities
// This describes what services the Wyoming server provides (STT, TTS, Intent, etc.)
// According to Wyoming protocol: client sends "describe", server responds with "info"
func DescribeResponseEvent(services map[string]any) Event {
	return Event{
		Type: EventInfo,
		Data: services,
	}
}

// Validate checks if the event is valid
func (e *Event) Validate() error {
	if e.Type == "" {
		return ErrInvalidEventType
	}

	// Audio chunk must have payload
	if e.Type == EventAudioChunk && e.PayloadLength == 0 {
		return ErrMissingPayload
	}

	// Payload length must match actual payload
	if e.PayloadLength > 0 && len(e.Payload) != e.PayloadLength {
		return ErrPayloadLengthMismatch
	}

	// Check payload size limit (8KB max)
	if e.PayloadLength > 8192 {
		return ErrPayloadTooLarge
	}

	// Audio events must have format data
	if e.Type == EventAudioChunk || e.Type == EventAudioStart {
		if e.Data == nil {
			return ErrMissingAudioFormat
		}

		// Check required fields
		if _, ok := e.Data["rate"]; !ok {
			return ErrMissingAudioFormat
		}
		if _, ok := e.Data["width"]; !ok {
			return ErrMissingAudioFormat
		}
		if _, ok := e.Data["channels"]; !ok {
			return ErrMissingAudioFormat
		}
	}

	return nil
}

// GetAudioFormat extracts audio format from event data
func (e *Event) GetAudioFormat() (AudioFormat, error) {
	if e.Data == nil {
		return AudioFormat{}, ErrMissingAudioFormat
	}

	rate, ok := e.Data["rate"].(float64)
	if !ok {
		return AudioFormat{}, ErrInvalidAudioFormat
	}

	width, ok := e.Data["width"].(float64)
	if !ok {
		return AudioFormat{}, ErrInvalidAudioFormat
	}

	channels, ok := e.Data["channels"].(float64)
	if !ok {
		return AudioFormat{}, ErrInvalidAudioFormat
	}

	return AudioFormat{
		Rate:     int(rate),
		Width:    int(width),
		Channels: int(channels),
	}, nil
}

// IsAudioEvent returns true if this is an audio-related event
func (e *Event) IsAudioEvent() bool {
	return e.Type == EventAudioChunk || e.Type == EventAudioStart || e.Type == EventAudioStop
}

// ValidateAudioFormat checks if audio format matches expected values
func (format AudioFormat) ValidateFor16kHzMono() error {
	if format.Rate != 16000 {
		return ErrInvalidSampleRate
	}
	if format.Width != 2 {
		return ErrInvalidSampleWidth
	}
	if format.Channels != 1 {
		return ErrInvalidChannels
	}
	return nil
}
