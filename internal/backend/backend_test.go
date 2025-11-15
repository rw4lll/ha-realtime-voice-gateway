package backend

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMockBackend_InitAndClose(t *testing.T) {
	backend := NewMockBackend()
	
	// Test init
	ctx := context.Background()
	cfg := Config{
		APIKey: "test-key",
		Model:  "test-model",
	}
	
	err := backend.Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	
	// Verify capabilities
	caps := backend.Capabilities()
	if !caps.SupportsStreaming {
		t.Error("Expected streaming support")
	}
	if !caps.SupportsToolCalling {
		t.Error("Expected tool calling support")
	}
	if caps.Provider != "mock" {
		t.Errorf("Expected provider 'mock', got '%s'", caps.Provider)
	}
	
	// Test close
	err = backend.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestMockBackend_StartSession(t *testing.T) {
	backend := NewMockBackend()
	ctx := context.Background()
	
	// Init backend
	err := backend.Init(ctx, Config{})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer backend.Close()
	
	// Start session
	req := SessionConfig{
		SessionID:    "test-session-1",
		DeviceID:     "device-1",
		UserID:       "user-1",
		SystemPrompt: "You are a helpful assistant",
		AudioFormat: AudioFormat{
			SampleRate:    16000,
			Channels:      1,
			BitsPerSample: 16,
			Encoding:      "pcm_s16le",
		},
	}
	
	session, err := backend.StartSession(ctx, req)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	defer session.Close()
	
	// Verify session
	if session.ID != req.SessionID {
		t.Errorf("Expected session ID %s, got %s", req.SessionID, session.ID)
	}
	
	if session.Metadata.DeviceID != req.DeviceID {
		t.Errorf("Expected device ID %s, got %s", req.DeviceID, session.Metadata.DeviceID)
	}
	
	if session.Metadata.Backend != "mock" {
		t.Errorf("Expected backend 'mock', got '%s'", session.Metadata.Backend)
	}
	
	// Verify channels exist
	if session.AudioIn == nil {
		t.Error("AudioIn channel is nil")
	}
	if session.AudioOut == nil {
		t.Error("AudioOut channel is nil")
	}
	if session.Events == nil {
		t.Error("Events channel is nil")
	}
	if session.ToolResults == nil {
		t.Error("ToolResults channel is nil")
	}
}

func TestMockBackend_AudioFlow(t *testing.T) {
	backend := NewMockBackend()
	ctx := context.Background()
	
	backend.Init(ctx, Config{})
	defer backend.Close()
	
	session, err := backend.StartSession(ctx, SessionConfig{
		SessionID: "audio-test",
		DeviceID:  "device-1",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	defer session.Close()
	
	// Get mock session for verification
	mockSession, ok := backend.GetSession("audio-test")
	if !ok {
		t.Fatal("Session not found")
	}
	
	// Send audio
	testAudio := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	err = session.SendAudio(testAudio)
	if err != nil {
		t.Fatalf("SendAudio failed: %v", err)
	}
	
	// Wait for processing
	time.Sleep(50 * time.Millisecond)
	
	// Verify audio was received
	receivedAudio := mockSession.GetReceivedAudio()
	if len(receivedAudio) != 1 {
		t.Fatalf("Expected 1 audio frame, got %d", len(receivedAudio))
	}
	
	if len(receivedAudio[0].Data) != len(testAudio) {
		t.Errorf("Audio size mismatch: expected %d, got %d", len(testAudio), len(receivedAudio[0].Data))
	}
	
	// Mock backend echoes audio back, verify we receive it
	select {
	case frame := <-session.AudioOut:
		if len(frame.Data) != len(testAudio) {
			t.Errorf("Echo audio size mismatch: expected %d, got %d", len(testAudio), len(frame.Data))
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for echo audio")
	}
}

func TestMockBackend_EventFlow(t *testing.T) {
	backend := NewMockBackend()
	ctx := context.Background()
	
	backend.Init(ctx, Config{})
	defer backend.Close()
	
	session, err := backend.StartSession(ctx, SessionConfig{
		SessionID: "event-test",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	defer session.Close()
	
	mockSession, ok := backend.GetSession("event-test")
	if !ok {
		t.Fatal("Session not found")
	}
	
	// Simulate transcript
	err = mockSession.SimulateTranscript("Hello world", false)
	if err != nil {
		t.Fatalf("SimulateTranscript failed: %v", err)
	}
	
	// Receive event
	select {
	case event := <-session.Events:
		if event.Type != EventTranscriptDelta {
			t.Errorf("Expected transcript_delta event, got %s", event.Type)
		}
		
		delta, ok := event.Data.(TranscriptDelta)
		if !ok {
			t.Fatal("Event data is not TranscriptDelta")
		}
		
		if delta.Text != "Hello world" {
			t.Errorf("Expected text 'Hello world', got '%s'", delta.Text)
		}
		
		if delta.IsFinal {
			t.Error("Expected non-final transcript")
		}
		
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for transcript event")
	}
}

func TestMockBackend_ToolCallFlow(t *testing.T) {
	backend := NewMockBackend()
	ctx := context.Background()
	
	backend.Init(ctx, Config{})
	defer backend.Close()
	
	// Register tools
	tools := []Tool{
		{
			Name:        "homeassistant.turn_on",
			Description: "Turn on a device",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	}
	
	err := backend.RegisterTools(tools)
	if err != nil {
		t.Fatalf("RegisterTools failed: %v", err)
	}
	
	session, err := backend.StartSession(ctx, SessionConfig{
		SessionID: "tool-test",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	defer session.Close()
	
	mockSession, ok := backend.GetSession("tool-test")
	if !ok {
		t.Fatal("Session not found")
	}
	
	// Simulate tool call
	args := map[string]interface{}{
		"entity_id": "light.living_room",
	}
	
	err = mockSession.SimulateToolCall("homeassistant.turn_on", args)
	if err != nil {
		t.Fatalf("SimulateToolCall failed: %v", err)
	}
	
	// Receive tool call event
	var toolCallID string
	select {
	case event := <-session.Events:
		if event.Type != EventToolCall {
			t.Errorf("Expected tool_call event, got %s", event.Type)
		}
		
		toolCall, ok := event.Data.(ToolCall)
		if !ok {
			t.Fatal("Event data is not ToolCall")
		}
		
		if toolCall.Name != "homeassistant.turn_on" {
			t.Errorf("Expected tool 'homeassistant.turn_on', got '%s'", toolCall.Name)
		}
		
		entityID, ok := toolCall.Arguments["entity_id"].(string)
		if !ok || entityID != "light.living_room" {
			t.Errorf("Expected entity_id 'light.living_room', got '%v'", toolCall.Arguments["entity_id"])
		}
		
		toolCallID = toolCall.ID
		
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for tool call event")
	}
	
	// Send tool result back
	result := ToolResult{
		CallID:    toolCallID,
		Result:    map[string]interface{}{"success": true},
		Timestamp: time.Now(),
	}
	
	err = session.SendToolResult(result)
	if err != nil {
		t.Fatalf("SendToolResult failed: %v", err)
	}
	
	// Wait for processing
	time.Sleep(50 * time.Millisecond)
	
	// Verify tool result was received
	receivedTools := mockSession.GetReceivedTools()
	if len(receivedTools) != 1 {
		t.Fatalf("Expected 1 tool result, got %d", len(receivedTools))
	}
	
	if receivedTools[0].CallID != toolCallID {
		t.Errorf("Tool call ID mismatch: expected %s, got %s", toolCallID, receivedTools[0].CallID)
	}
}

func TestMockBackend_MultipleSessions(t *testing.T) {
	backend := NewMockBackend()
	ctx := context.Background()
	
	backend.Init(ctx, Config{})
	defer backend.Close()
	
	// Start multiple sessions
	numSessions := 5
	sessions := make([]*Session, numSessions)
	
	for i := 0; i < numSessions; i++ {
		session, err := backend.StartSession(ctx, SessionConfig{
			SessionID: fmt.Sprintf("session-%d", i),
			DeviceID:  fmt.Sprintf("device-%d", i),
		})
		if err != nil {
			t.Fatalf("StartSession %d failed: %v", i, err)
		}
		sessions[i] = session
	}
	
	// Verify all sessions exist
	for i := 0; i < numSessions; i++ {
		_, ok := backend.GetSession(fmt.Sprintf("session-%d", i))
		if !ok {
			t.Errorf("Session %d not found", i)
		}
	}
	
	// Close all sessions
	for _, session := range sessions {
		session.Close()
	}
}

func TestMockBackend_InterruptSignal(t *testing.T) {
	backend := NewMockBackend()
	ctx := context.Background()
	
	backend.Init(ctx, Config{})
	defer backend.Close()
	
	session, err := backend.StartSession(ctx, SessionConfig{
		SessionID: "interrupt-test",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	defer session.Close()
	
	// Send interrupt signal (non-blocking)
	err = session.TriggerInterrupt()
	if err != nil {
		t.Fatalf("TriggerInterrupt failed: %v", err)
	}
	
	// The interrupt channel is send-only from session perspective
	// In real implementation, backend would receive and handle it
	// For testing, just verify the call succeeded without error
	t.Log("Interrupt signal sent successfully")
}

func TestSessionHelpers(t *testing.T) {
	backend := NewMockBackend()
	ctx := context.Background()
	
	backend.Init(ctx, Config{})
	defer backend.Close()
	
	session, err := backend.StartSession(ctx, SessionConfig{
		SessionID: "helpers-test",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	defer session.Close()
	
	// Test SendAudio helper
	testData := []byte{1, 2, 3}
	err = session.SendAudio(testData)
	if err != nil {
		t.Errorf("SendAudio failed: %v", err)
	}
	
	// Test SendToolResult helper
	result := ToolResult{
		CallID: "test-call",
		Result: "success",
	}
	err = session.SendToolResult(result)
	if err != nil {
		t.Errorf("SendToolResult failed: %v", err)
	}
	
	// Test TriggerInterrupt helper
	err = session.TriggerInterrupt()
	if err != nil {
		t.Errorf("TriggerInterrupt failed: %v", err)
	}
	
	// Test Context helper
	if session.Context() != session.ctx {
		t.Error("Context() returned wrong context")
	}
}

func TestEventConstructors(t *testing.T) {
	sessionID := "test-session"
	
	// Test NewAudioStartEvent
	event := NewAudioStartEvent(sessionID)
	if event.Type != EventAudioStart {
		t.Errorf("Expected EventAudioStart, got %s", event.Type)
	}
	if event.SessionID != sessionID {
		t.Errorf("Session ID mismatch")
	}
	
	// Test NewTranscriptDeltaEvent
	delta := TranscriptDelta{Text: "test", IsFinal: false}
	event = NewTranscriptDeltaEvent(sessionID, delta)
	if event.Type != EventTranscriptDelta {
		t.Errorf("Expected EventTranscriptDelta, got %s", event.Type)
	}
	
	// Test NewToolCallEvent
	toolCall := ToolCall{ID: "call-1", Name: "test.tool"}
	event = NewToolCallEvent(sessionID, toolCall)
	if event.Type != EventToolCall {
		t.Errorf("Expected EventToolCall, got %s", event.Type)
	}
	
	// Test NewErrorEvent
	errEvent := ErrorEvent{Error: fmt.Errorf("test error")}
	event = NewErrorEvent(sessionID, errEvent)
	if event.Type != EventError {
		t.Errorf("Expected EventError, got %s", event.Type)
	}
}

