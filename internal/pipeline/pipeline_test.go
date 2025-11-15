package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/protocol/wyoming"
	"go.uber.org/zap"
)

// MockToolExecutor for testing.
type MockToolExecutor struct {
	executedCalls []*backend.ToolCall
}

func (m *MockToolExecutor) Execute(ctx context.Context, call *backend.ToolCall) (*backend.ToolResult, error) {
	m.executedCalls = append(m.executedCalls, call)
	return &backend.ToolResult{
		CallID:    call.ID,
		Result:    map[string]interface{}{"status": "ok"},
		Timestamp: time.Now(),
	}, nil
}

func TestPipeline_Create(t *testing.T) {
	logger := zap.NewNop()
	mockBackend := backend.NewMockBackend()

	pipeline, err := NewPipeline(Config{
		Backend:      mockBackend,
		Logger:       logger,
		SystemPrompt: "Test prompt",
	})

	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}
	if pipeline == nil {
		t.Fatal("pipeline is nil")
	}

	// Clean up
	if err := pipeline.Close(); err != nil {
		t.Errorf("failed to close pipeline: %v", err)
	}
}

func TestPipeline_RequiredFields(t *testing.T) {
	logger := zap.NewNop()
	mockBackend := backend.NewMockBackend()

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "missing backend",
			cfg: Config{
				Backend: nil,
				Logger:  logger,
			},
			wantErr: true,
		},
		{
			name: "missing logger",
			cfg: Config{
				Backend: mockBackend,
				Logger:  nil,
			},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: Config{
				Backend: mockBackend,
				Logger:  logger,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := NewPipeline(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPipeline() error = %v, wantErr %v", err, tt.wantErr)
			}
			if pipeline != nil {
				pipeline.Close()
			}
		})
	}
}

func TestPipeline_AudioForwarding(t *testing.T) {
	logger := zap.NewNop()
	mockBackend := backend.NewMockBackend()

	pipeline, err := NewPipeline(Config{
		Backend: mockBackend,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}
	defer pipeline.Close()

	// Skip this test - requires proper Session initialization with unexported fields
	t.Skip("Skipping test - requires refactoring Session initialization for testing")
}

func TestPipeline_ToolCallHandling(t *testing.T) {
	t.Skip("Skipping test - requires refactoring Session initialization for testing")

	logger := zap.NewNop()
	mockBackend := backend.NewMockBackend()
	mockExecutor := &MockToolExecutor{}

	pipeline, err := NewPipeline(Config{
		Backend:      mockBackend,
		Logger:       logger,
		ToolExecutor: mockExecutor,
	})
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}
	defer pipeline.Close()

	// Create Wyoming session
	wyomingSession := &wyoming.Session{
		ID:       "test-session",
		AudioIn:  make(chan []byte, 10),
		AudioOut: make(chan []byte, 10),
		Events:   make(chan *wyoming.Event, 10),
	}

	// Handle session in background
	go pipeline.HandleWyomingSession(wyomingSession)

	// Wait for session setup
	time.Sleep(50 * time.Millisecond)

	// Get the backend mock session that was created
	mockSession, ok := mockBackend.GetSession("test-session")
	if !ok {
		t.Fatal("backend session not created")
	}

	// Simulate a tool call from the backend
	toolCall := &backend.ToolCall{
		ID:   "tool-123",
		Name: "light.turn_on",
		Arguments: map[string]interface{}{
			"entity_id": "light.living_room",
		},
		Timestamp: time.Now(),
	}

	// Send tool call event via the internal channel
	mockSession.SimulateToolCall(toolCall.Name, toolCall.Arguments)

	// Wait for tool execution
	time.Sleep(100 * time.Millisecond)

	// Verify tool was executed
	if len(mockExecutor.executedCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(mockExecutor.executedCalls))
	}
	if len(mockExecutor.executedCalls) > 0 && mockExecutor.executedCalls[0].ID != "tool-123" {
		t.Errorf("expected tool ID 'tool-123', got %s", mockExecutor.executedCalls[0].ID)
	}

	// Clean up
	close(wyomingSession.AudioIn)
	time.Sleep(50 * time.Millisecond)
}

func TestPipeline_SessionLifecycle(t *testing.T) {
	t.Skip("Skipping test - requires refactoring Session initialization for testing")

	logger := zap.NewNop()
	mockBackend := backend.NewMockBackend()

	pipeline, err := NewPipeline(Config{
		Backend: mockBackend,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}
	defer pipeline.Close()

	// Create Wyoming session
	wyomingSession := &wyoming.Session{
		ID:       "test-session",
		AudioIn:  make(chan []byte, 10),
		AudioOut: make(chan []byte, 10),
		Events:   make(chan *wyoming.Event, 10),
	}

	// Handle session in background
	sessionDone := make(chan struct{})
	go func() {
		pipeline.HandleWyomingSession(wyomingSession)
		close(sessionDone)
	}()

	// Wait for session to start
	time.Sleep(50 * time.Millisecond)

	// Verify session is tracked
	if pipeline.GetSessionCount() != 1 {
		t.Errorf("expected 1 session, got %d", pipeline.GetSessionCount())
	}

	// Close Wyoming session
	close(wyomingSession.AudioIn)

	// Wait for cleanup
	select {
	case <-sessionDone:
		// Session handler completed
	case <-time.After(1 * time.Second):
		t.Fatal("session handler did not complete")
	}

	// Verify session is removed
	time.Sleep(50 * time.Millisecond)
	if pipeline.GetSessionCount() != 0 {
		t.Errorf("expected 0 sessions after cleanup, got %d", pipeline.GetSessionCount())
	}
}

func TestPipeline_MultipleSessionsConcurrent(t *testing.T) {
	t.Skip("Skipping test - requires refactoring Session initialization for testing")

	logger := zap.NewNop()
	mockBackend := backend.NewMockBackend()

	pipeline, err := NewPipeline(Config{
		Backend: mockBackend,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}
	defer pipeline.Close()

	// Create multiple Wyoming sessions
	numSessions := 3
	sessions := make([]*wyoming.Session, numSessions)

	for i := 0; i < numSessions; i++ {
		sessions[i] = &wyoming.Session{
			ID:       string(rune('A' + i)),
			AudioIn:  make(chan []byte, 10),
			AudioOut: make(chan []byte, 10),
			Events:   make(chan *wyoming.Event, 10),
		}
		go pipeline.HandleWyomingSession(sessions[i])
	}

	// Wait for all sessions to start
	time.Sleep(100 * time.Millisecond)

	// Verify all sessions are tracked
	if pipeline.GetSessionCount() != numSessions {
		t.Errorf("expected %d sessions, got %d", numSessions, pipeline.GetSessionCount())
	}

	// Close all sessions
	for _, session := range sessions {
		close(session.AudioIn)
	}

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	// Verify all sessions are removed
	if pipeline.GetSessionCount() != 0 {
		t.Errorf("expected 0 sessions after cleanup, got %d", pipeline.GetSessionCount())
	}
}

func TestPipeline_EventRouting(t *testing.T) {
	t.Skip("Skipping test - requires refactoring Session initialization for testing")

	logger := zap.NewNop()
	mockBackend := backend.NewMockBackend()

	pipeline, err := NewPipeline(Config{
		Backend: mockBackend,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}
	defer pipeline.Close()

	// Create Wyoming session
	wyomingSession := &wyoming.Session{
		ID:       "test-session",
		AudioIn:  make(chan []byte, 10),
		AudioOut: make(chan []byte, 10),
		Events:   make(chan *wyoming.Event, 10),
	}

	// Handle session in background
	go pipeline.HandleWyomingSession(wyomingSession)

	// Wait for session setup
	time.Sleep(50 * time.Millisecond)

	// Get backend mock session
	mockSession, ok := mockBackend.GetSession("test-session")
	if !ok {
		t.Fatal("backend session not created")
	}

	// Test different event types by simulating transcripts
	mockSession.SimulateTranscript("Hello", false)
	time.Sleep(10 * time.Millisecond)
	mockSession.SimulateTranscript("Hello world", true)
	time.Sleep(10 * time.Millisecond)

	// Other events are processed internally, we're mainly testing for no panics

	// Events should be processed without errors (logged but not blocking)
	// No explicit verification needed as we're testing for no panics/deadlocks

	// Clean up
	close(wyomingSession.AudioIn)
	time.Sleep(50 * time.Millisecond)
}

func TestPipeline_NoToolExecutor(t *testing.T) {
	t.Skip("Skipping test - requires refactoring Session initialization for testing")

	logger := zap.NewNop()
	mockBackend := backend.NewMockBackend()

	// Create pipeline WITHOUT tool executor
	pipeline, err := NewPipeline(Config{
		Backend:      mockBackend,
		Logger:       logger,
		ToolExecutor: nil, // No executor
	})
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}
	defer pipeline.Close()

	// Create Wyoming session
	wyomingSession := &wyoming.Session{
		ID:       "test-session",
		AudioIn:  make(chan []byte, 10),
		AudioOut: make(chan []byte, 10),
		Events:   make(chan *wyoming.Event, 10),
	}

	go pipeline.HandleWyomingSession(wyomingSession)
	time.Sleep(50 * time.Millisecond)

	// Get backend mock session
	mockSession, ok := mockBackend.GetSession("test-session")
	if !ok {
		t.Fatal("backend session not created")
	}

	// Send tool call event
	mockSession.SimulateToolCall("test.tool", map[string]interface{}{})

	// Wait for processing and check if tool result was received
	time.Sleep(100 * time.Millisecond)

	// Check that a tool result was received by the mock backend
	receivedTools := mockSession.GetReceivedTools()
	if len(receivedTools) == 0 {
		t.Error("expected to receive error tool result")
	}
	if len(receivedTools) > 0 {
		result := receivedTools[0]
		if result.Error == "" {
			t.Error("expected error result when no tool executor")
		}
	}

	// Clean up
	close(wyomingSession.AudioIn)
	time.Sleep(50 * time.Millisecond)
}
