package pipeline

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestMetricsCollector(t *testing.T) {
	logger := zap.NewNop()
	metrics := NewMetricsCollector(logger)

	// Test tool call metrics
	metrics.RecordToolCall(true)
	metrics.RecordToolCall(true)
	metrics.RecordToolCall(false)

	if metrics.ToolCallsTotal.Load() != 3 {
		t.Errorf("Expected 3 total tool calls, got %d", metrics.ToolCallsTotal.Load())
	}
	if metrics.ToolCallsSuccess.Load() != 2 {
		t.Errorf("Expected 2 successful tool calls, got %d", metrics.ToolCallsSuccess.Load())
	}
	if metrics.ToolCallsError.Load() != 1 {
		t.Errorf("Expected 1 failed tool call, got %d", metrics.ToolCallsError.Load())
	}

	// Test audio metrics
	for i := 0; i < 100; i++ {
		metrics.RecordAudioFrameSent()
	}
	for i := 0; i < 95; i++ {
		metrics.RecordAudioFrameReceived()
	}
	for i := 0; i < 5; i++ {
		metrics.RecordAudioFrameDropped()
	}
	for i := 0; i < 2; i++ {
		metrics.RecordAudioFrameDuplicated()
	}

	if metrics.AudioFramesSent.Load() != 100 {
		t.Errorf("Expected 100 audio frames sent, got %d", metrics.AudioFramesSent.Load())
	}
	if metrics.AudioFramesReceived.Load() != 95 {
		t.Errorf("Expected 95 audio frames received, got %d", metrics.AudioFramesReceived.Load())
	}
	if metrics.AudioFramesDropped.Load() != 5 {
		t.Errorf("Expected 5 audio frames dropped, got %d", metrics.AudioFramesDropped.Load())
	}
	if metrics.AudioFramesDuplicated.Load() != 2 {
		t.Errorf("Expected 2 audio frames duplicated, got %d", metrics.AudioFramesDuplicated.Load())
	}

	// Test session metrics
	metrics.RecordSessionStart()
	metrics.RecordSessionStart()
	metrics.RecordSessionEnd()

	if metrics.SessionsTotal.Load() != 2 {
		t.Errorf("Expected 2 total sessions, got %d", metrics.SessionsTotal.Load())
	}
	if metrics.SessionsActive.Load() != 1 {
		t.Errorf("Expected 1 active session, got %d", metrics.SessionsActive.Load())
	}

	// Test latency recording
	testLatency := 50 * time.Millisecond
	metrics.RecordToolCallLatency(testLatency)
	if metrics.lastToolCallLatency != testLatency {
		t.Errorf("Expected latency %v, got %v", testLatency, metrics.lastToolCallLatency)
	}
}

func TestMetricsSnapshot(t *testing.T) {
	logger := zap.NewNop()
	metrics := NewMetricsCollector(logger)

	// Add some metrics
	metrics.RecordToolCall(true)
	metrics.RecordAudioFrameSent()
	metrics.RecordSessionStart()

	// Get snapshot
	snapshot := metrics.GetSnapshot()

	if snapshot.ToolCallsTotal != 1 {
		t.Errorf("Expected 1 tool call in snapshot, got %d", snapshot.ToolCallsTotal)
	}
	if snapshot.AudioFramesSent != 1 {
		t.Errorf("Expected 1 audio frame in snapshot, got %d", snapshot.AudioFramesSent)
	}
	if snapshot.SessionsTotal != 1 {
		t.Errorf("Expected 1 session in snapshot, got %d", snapshot.SessionsTotal)
	}

	// Verify snapshot is independent (doesn't change if metrics change)
	metrics.RecordToolCall(true)
	if snapshot.ToolCallsTotal != 1 {
		t.Error("Snapshot should be independent of later changes")
	}
}

func TestMetricsCollector_LogMetrics(t *testing.T) {
	logger := zap.NewNop()
	metrics := NewMetricsCollector(logger)

	// Add some test data
	metrics.RecordToolCall(true)
	metrics.RecordToolCall(true)
	metrics.RecordToolCall(false)
	metrics.RecordAudioFrameSent()
	metrics.RecordAudioFrameReceived()

	// This should not panic
	metrics.LogMetrics()
}

func TestMetricsCollector_RateCalculations(t *testing.T) {
	logger := zap.NewNop()
	metrics := NewMetricsCollector(logger)

	// Setup test data: 80% success rate
	for i := 0; i < 8; i++ {
		metrics.RecordToolCall(true)
	}
	for i := 0; i < 2; i++ {
		metrics.RecordToolCall(false)
	}

	// Setup audio: 10% drop rate
	for i := 0; i < 90; i++ {
		metrics.RecordAudioFrameReceived()
	}
	for i := 0; i < 10; i++ {
		metrics.RecordAudioFrameDropped()
	}

	// Setup deduplication: 5% dedup rate
	for i := 0; i < 5; i++ {
		metrics.RecordAudioFrameDuplicated()
	}

	snapshot := metrics.GetSnapshot()

	// Calculate rates manually
	successRate := float64(snapshot.ToolCallsSuccess) / float64(snapshot.ToolCallsTotal) * 100
	if successRate != 80.0 {
		t.Errorf("Expected 80%% success rate, got %.2f%%", successRate)
	}

	dropRate := float64(snapshot.AudioFramesDropped) / float64(snapshot.AudioFramesReceived) * 100
	if dropRate < 11.0 || dropRate > 11.2 { // Allow small floating point variance
		t.Errorf("Expected ~11.11%% drop rate, got %.2f%%", dropRate)
	}
}
