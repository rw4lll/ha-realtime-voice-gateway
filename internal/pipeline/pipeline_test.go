package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
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
		Backend:       mockBackend,
		Logger:        logger,
		ToolExecutor:  &MockToolExecutor{},
		SystemPrompt:  "Test assistant",
		EnableMetrics: true,
		AudioBufferMs: 500,
		SafetyTimeout: 5 * time.Minute,
	})

	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	if pipeline == nil {
		t.Fatal("Pipeline should not be nil")
	}

	// Test Close
	if err := pipeline.Close(); err != nil {
		t.Errorf("Failed to close pipeline: %v", err)
	}
}

func TestPipeline_CreateWithoutBackend(t *testing.T) {
	logger := zap.NewNop()

	_, err := NewPipeline(Config{
		Backend: nil,
		Logger:  logger,
	})

	if err == nil {
		t.Fatal("Expected error when creating pipeline without backend")
	}
}

func TestPipeline_CreateWithoutLogger(t *testing.T) {
	mockBackend := backend.NewMockBackend()

	_, err := NewPipeline(Config{
		Backend: mockBackend,
		Logger:  nil,
	})

	if err == nil {
		t.Fatal("Expected error when creating pipeline without logger")
	}
}

func TestPipeline_Metrics(t *testing.T) {
	logger := zap.NewNop()
	mockBackend := backend.NewMockBackend()

	// Create pipeline with metrics enabled
	pipeline, err := NewPipeline(Config{
		Backend:       mockBackend,
		Logger:        logger,
		EnableMetrics: true,
	})

	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}
	defer pipeline.Close()

	// Get metrics
	metrics := pipeline.GetMetrics()
	if metrics == nil {
		t.Fatal("Metrics should not be nil when enabled")
	}

	// Test LogMetrics (should not panic)
	pipeline.LogMetrics()
}

func TestPipeline_MetricsDisabled(t *testing.T) {
	logger := zap.NewNop()
	mockBackend := backend.NewMockBackend()

	// Create pipeline with metrics disabled
	pipeline, err := NewPipeline(Config{
		Backend:       mockBackend,
		Logger:        logger,
		EnableMetrics: false,
	})

	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}
	defer pipeline.Close()

	// Get metrics
	metrics := pipeline.GetMetrics()
	if metrics != nil {
		t.Fatal("Metrics should be nil when disabled")
	}

	// Test LogMetrics (should not panic)
	pipeline.LogMetrics()
}

// TODO: Add WebSocket session tests when WebSocket support is implemented
