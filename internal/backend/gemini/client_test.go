package gemini

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"go.uber.org/zap"
)

func TestGeminiBackend_IdempotentInit(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	cfg := backend.Config{
		APIKey:       "test-api-key",
		Model:        "gemini-2.0-flash-exp",
		SystemPrompt: "Test prompt",
	}

	// First initialization should succeed
	err := geminiBackend.Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first Init() failed: %v", err)
	}

	// Second initialization should also succeed (idempotent)
	// This time with empty config (like pipeline does)
	emptyConfig := backend.Config{
		AudioBufferSize: 100,
		EventBufferSize: 50,
	}
	err = geminiBackend.Init(context.Background(), emptyConfig)
	if err != nil {
		t.Fatalf("second Init() with empty config failed: %v", err)
	}

	// Verify the backend is still initialized with original config
	if geminiBackend.apiKey != "test-api-key" {
		t.Errorf("API key was overwritten, got: %s", geminiBackend.apiKey)
	}
	if geminiBackend.model != "gemini-2.0-flash-exp" {
		t.Errorf("Model was overwritten, got: %s", geminiBackend.model)
	}

	// Clean up
	geminiBackend.Close()
}

func TestGeminiBackend_InitWithoutAPIKey(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	// Should fail without API key
	cfg := backend.Config{
		Model: "gemini-2.0-flash-exp",
	}

	err := geminiBackend.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when initializing without API key, got nil")
	}

	expectedMsg := "gemini API key is required"
	if err.Error() != expectedMsg {
		t.Errorf("unexpected error message: got %q, want %q", err.Error(), expectedMsg)
	}
}

func TestGeminiBackend_InitDefaultModel(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	cfg := backend.Config{
		APIKey: "test-api-key",
		// Model not specified
	}

	err := geminiBackend.Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	// Should use default model
	expectedModel := "gemini-2.0-flash-exp"
	if geminiBackend.model != expectedModel {
		t.Errorf("expected default model %q, got %q", expectedModel, geminiBackend.model)
	}

	geminiBackend.Close()
}

func TestGeminiBackend_StartSession_InputValidation(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)
	geminiBackend.initialized = true // Skip actual initialization

	tests := []struct {
		name        string
		ctx         context.Context
		req         backend.SessionConfig
		expectError string
	}{
		{
			name:        "nil context",
			ctx:         nil,
			req:         backend.SessionConfig{SessionID: "test", DeviceID: "device"},
			expectError: "context is required",
		},
		{
			name:        "empty session ID",
			ctx:         context.Background(),
			req:         backend.SessionConfig{SessionID: "", DeviceID: "device"},
			expectError: "session_id is required",
		},
		{
			name:        "empty device ID",
			ctx:         context.Background(),
			req:         backend.SessionConfig{SessionID: "test", DeviceID: ""},
			expectError: "device_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := geminiBackend.StartSession(tt.ctx, tt.req)
			if err == nil {
				t.Errorf("expected error %q, got nil", tt.expectError)
			} else if err.Error() != tt.expectError {
				t.Errorf("expected error %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestGeminiBackend_StartSession_NotInitialized(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	req := backend.SessionConfig{
		SessionID: "test-session",
		DeviceID:  "test-device",
	}

	_, err := geminiBackend.StartSession(context.Background(), req)
	if err != backend.ErrBackendNotInitialized {
		t.Errorf("expected ErrBackendNotInitialized, got: %v", err)
	}
}

func TestGeminiBackend_RegisterTools(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	tools := []backend.Tool{
		{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"param1": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
		{
			Name:        "another_tool",
			Description: "Another test tool",
			Parameters:  map[string]interface{}{},
		},
	}

	err := geminiBackend.RegisterTools(tools)
	if err != nil {
		t.Errorf("RegisterTools() failed: %v", err)
	}

	geminiBackend.toolsMu.RLock()
	if len(geminiBackend.tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(geminiBackend.tools))
	}
	geminiBackend.toolsMu.RUnlock()
}

func TestGeminiBackend_RegisterTools_Concurrent(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tools := []backend.Tool{
				{
					Name:        "tool",
					Description: "test",
					Parameters:  map[string]interface{}{},
				},
			}
			geminiBackend.RegisterTools(tools)
		}(i)
	}

	wg.Wait()

	// Should not panic and should have tools registered
	geminiBackend.toolsMu.RLock()
	hasTools := len(geminiBackend.tools) > 0
	geminiBackend.toolsMu.RUnlock()

	if !hasTools {
		t.Error("expected tools to be registered after concurrent calls")
	}
}

func TestGeminiBackend_Capabilities(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	caps := geminiBackend.Capabilities()

	if !caps.SupportsStreaming {
		t.Error("expected SupportsStreaming to be true")
	}
	if !caps.SupportsToolCalling {
		t.Error("expected SupportsToolCalling to be true")
	}
	if !caps.SupportsBargeIn {
		t.Error("expected SupportsBargeIn to be true")
	}
	if caps.Provider != "gemini" {
		t.Errorf("expected provider 'gemini', got %q", caps.Provider)
	}
	if len(caps.SupportedAudioFormats) == 0 {
		t.Error("expected at least one supported audio format")
	}

	// Verify audio format details
	audioFmt := caps.SupportedAudioFormats[0]
	if audioFmt.SampleRate != 24000 {
		t.Errorf("expected sample rate 24000, got %d", audioFmt.SampleRate)
	}
	if audioFmt.Channels != 1 {
		t.Errorf("expected 1 channel, got %d", audioFmt.Channels)
	}
	if audioFmt.BitsPerSample != 16 {
		t.Errorf("expected 16 bits per sample, got %d", audioFmt.BitsPerSample)
	}
}

func TestGeminiBackend_Close(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	// Initialize to start cleanup goroutine
	cfg := backend.Config{
		APIKey: "test-api-key",
		Model:  "test-model",
	}
	geminiBackend.Init(context.Background(), cfg)

	// Add mock sessions
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	geminiBackend.sessions["session1"] = &geminiSession{
		id:     "session1",
		ctx:    ctx1,
		cancel: cancel1,
		logger: logger,
	}

	// Close should not panic
	err := geminiBackend.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Verify cleanup goroutine stopped
	time.Sleep(100 * time.Millisecond)
}

func TestGeminiBackend_Close_Idempotent(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	// Close multiple times should not panic
	geminiBackend.Close()
	geminiBackend.Close()
	geminiBackend.Close()
}

func TestGeminiBackend_ConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)
	geminiBackend.initialized = true
	geminiBackend.model = "test-model"

	var wg sync.WaitGroup

	// Concurrent RegisterTools calls
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tools := []backend.Tool{{Name: "test", Description: "test"}}
			geminiBackend.RegisterTools(tools)
		}()
	}

	// Concurrent Capabilities calls
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			geminiBackend.Capabilities()
		}()
	}

	// Concurrent session access
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			geminiBackend.sessionsMu.Lock()
			_ = len(geminiBackend.sessions)
			geminiBackend.sessionsMu.Unlock()
		}()
	}

	wg.Wait()
	// Should not panic or deadlock
}
