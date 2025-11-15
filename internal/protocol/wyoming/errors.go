package wyoming

import "errors"

// Wyoming protocol errors
var (
	ErrInvalidEventType       = errors.New("invalid event type")
	ErrMissingPayload         = errors.New("missing payload for audio chunk")
	ErrPayloadLengthMismatch  = errors.New("payload length mismatch")
	ErrMissingAudioFormat     = errors.New("missing audio format data")
	ErrInvalidAudioFormat     = errors.New("invalid audio format")
	ErrInvalidSampleRate      = errors.New("invalid sample rate (expected 16000)")
	ErrInvalidSampleWidth     = errors.New("invalid sample width (expected 2)")
	ErrInvalidChannels        = errors.New("invalid channels (expected 1)")
	ErrInvalidJSON            = errors.New("invalid JSON")
	ErrPayloadTooLarge        = errors.New("payload too large (max 8192 bytes)")
	ErrConnectionClosed       = errors.New("connection closed")
	ErrReadTimeout            = errors.New("read timeout")
	ErrWriteTimeout           = errors.New("write timeout")
)

