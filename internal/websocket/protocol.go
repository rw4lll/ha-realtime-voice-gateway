package websocket

import (
	"encoding/json"
)

// DeviceState represents the conversation state sent to the device
type DeviceState string

const (
	StateListening DeviceState = "listening" // Gateway is listening for user speech
	StateThinking  DeviceState = "thinking"  // AI is processing user input
	StateSpeaking  DeviceState = "speaking"  // AI is delivering audio response
	StateDone      DeviceState = "done"      // Conversation complete, device should close
)

// ControlMessage represents a JSON control message sent to the device
type ControlMessage struct {
	State   DeviceState `json:"state,omitempty"`   // State update
	Command string      `json:"command,omitempty"` // Command (e.g., "stop")
	Error   string      `json:"error,omitempty"`   // Error message
}

// NewStateMessage creates a state update message
func NewStateMessage(state DeviceState) []byte {
	msg := ControlMessage{State: state}
	data, _ := json.Marshal(msg)
	return data
}

// NewStopMessage creates a stop command message
func NewStopMessage() []byte {
	msg := ControlMessage{Command: "stop"}
	data, _ := json.Marshal(msg)
	return data
}

// NewErrorMessage creates an error message
func NewErrorMessage(err string) []byte {
	msg := ControlMessage{Error: err}
	data, _ := json.Marshal(msg)
	return data
}
