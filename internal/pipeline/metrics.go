package pipeline

import (
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// MetricsCollector collects pipeline and backend metrics
type MetricsCollector struct {
	// Tool call metrics
	ToolCallsTotal   atomic.Int64
	ToolCallsSuccess atomic.Int64
	ToolCallsError   atomic.Int64

	// Audio metrics
	AudioFramesSent     atomic.Int64
	AudioFramesReceived atomic.Int64
	AudioFramesDropped  atomic.Int64 // Dropped due to buffer full or suppression

	// Session metrics
	SessionsTotal  atomic.Int64
	SessionsActive atomic.Int64

	// Audio deduplication metrics (for Gemini backend)
	AudioFramesDuplicated atomic.Int64

	// Multi-turn conversation metrics
	FollowUpQuestionsDetected atomic.Int64 // Number of follow-up questions detected
	SessionResetsForFollowUp  atomic.Int64 // Number of times session was reset for follow-up
	AudioBufferFlushes        atomic.Int64 // Number of times audio buffer was flushed

	// Latency tracking
	lastToolCallLatency time.Duration

	logger *zap.Logger
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(logger *zap.Logger) *MetricsCollector {
	return &MetricsCollector{
		logger: logger,
	}
}

// RecordToolCall records a tool call attempt
func (m *MetricsCollector) RecordToolCall(success bool) {
	m.ToolCallsTotal.Add(1)
	if success {
		m.ToolCallsSuccess.Add(1)
	} else {
		m.ToolCallsError.Add(1)
	}
}

// RecordAudioFrameSent records an audio frame sent to backend
func (m *MetricsCollector) RecordAudioFrameSent() {
	m.AudioFramesSent.Add(1)
}

// RecordAudioFrameReceived records an audio frame received from backend
func (m *MetricsCollector) RecordAudioFrameReceived() {
	m.AudioFramesReceived.Add(1)
}

// RecordAudioFrameDropped records a dropped audio frame
func (m *MetricsCollector) RecordAudioFrameDropped() {
	m.AudioFramesDropped.Add(1)
}

// RecordAudioFrameDuplicated records a deduplicated audio frame
func (m *MetricsCollector) RecordAudioFrameDuplicated() {
	m.AudioFramesDuplicated.Add(1)
}

// RecordSessionStart records a new session
func (m *MetricsCollector) RecordSessionStart() {
	m.SessionsTotal.Add(1)
	m.SessionsActive.Add(1)
}

// RecordSessionEnd records a session ending
func (m *MetricsCollector) RecordSessionEnd() {
	m.SessionsActive.Add(-1)
}

// RecordToolCallLatency records the latency of a tool call
func (m *MetricsCollector) RecordToolCallLatency(duration time.Duration) {
	m.lastToolCallLatency = duration
}

// RecordFollowUpQuestion records a follow-up question detection
func (m *MetricsCollector) RecordFollowUpQuestion() {
	m.FollowUpQuestionsDetected.Add(1)
}

// RecordSessionResetForFollowUp records a session reset for follow-up
func (m *MetricsCollector) RecordSessionResetForFollowUp() {
	m.SessionResetsForFollowUp.Add(1)
}

// RecordAudioBufferFlush records an audio buffer flush event
func (m *MetricsCollector) RecordAudioBufferFlush() {
	m.AudioBufferFlushes.Add(1)
}

// LogMetrics logs current metrics (call periodically)
func (m *MetricsCollector) LogMetrics() {
	toolCallsTotal := m.ToolCallsTotal.Load()
	toolCallsSuccess := m.ToolCallsSuccess.Load()
	toolCallsError := m.ToolCallsError.Load()
	audioFramesSent := m.AudioFramesSent.Load()
	audioFramesReceived := m.AudioFramesReceived.Load()
	audioFramesDropped := m.AudioFramesDropped.Load()
	audioFramesDuplicated := m.AudioFramesDuplicated.Load()
	sessionsTotal := m.SessionsTotal.Load()
	sessionsActive := m.SessionsActive.Load()
	followUpQuestions := m.FollowUpQuestionsDetected.Load()
	sessionResets := m.SessionResetsForFollowUp.Load()
	bufferFlushes := m.AudioBufferFlushes.Load()

	var successRate float64
	if toolCallsTotal > 0 {
		successRate = float64(toolCallsSuccess) / float64(toolCallsTotal) * 100
	}

	var dropRate float64
	if audioFramesReceived > 0 {
		dropRate = float64(audioFramesDropped) / float64(audioFramesReceived) * 100
	}

	var dedupRate float64
	if audioFramesReceived > 0 {
		dedupRate = float64(audioFramesDuplicated) / float64(audioFramesReceived) * 100
	}

	m.logger.Info("pipeline metrics",
		zap.Int64("tool_calls_total", toolCallsTotal),
		zap.Int64("tool_calls_success", toolCallsSuccess),
		zap.Int64("tool_calls_error", toolCallsError),
		zap.Float64("tool_success_rate", successRate),
		zap.Int64("audio_frames_sent", audioFramesSent),
		zap.Int64("audio_frames_received", audioFramesReceived),
		zap.Int64("audio_frames_dropped", audioFramesDropped),
		zap.Float64("audio_drop_rate", dropRate),
		zap.Int64("audio_frames_duplicated", audioFramesDuplicated),
		zap.Float64("audio_dedup_rate", dedupRate),
		zap.Int64("audio_buffer_flushes", bufferFlushes),
		zap.Int64("sessions_total", sessionsTotal),
		zap.Int64("sessions_active", sessionsActive),
		zap.Int64("follow_up_questions_detected", followUpQuestions),
		zap.Int64("session_resets_for_follow_up", sessionResets),
		zap.Duration("last_tool_call_latency", m.lastToolCallLatency),
	)
}

// GetSnapshot returns a snapshot of current metrics
func (m *MetricsCollector) GetSnapshot() MetricsSnapshot {
	return MetricsSnapshot{
		ToolCallsTotal:            m.ToolCallsTotal.Load(),
		ToolCallsSuccess:          m.ToolCallsSuccess.Load(),
		ToolCallsError:            m.ToolCallsError.Load(),
		AudioFramesSent:           m.AudioFramesSent.Load(),
		AudioFramesReceived:       m.AudioFramesReceived.Load(),
		AudioFramesDropped:        m.AudioFramesDropped.Load(),
		AudioFramesDuplicated:     m.AudioFramesDuplicated.Load(),
		AudioBufferFlushes:        m.AudioBufferFlushes.Load(),
		SessionsTotal:             m.SessionsTotal.Load(),
		SessionsActive:            m.SessionsActive.Load(),
		FollowUpQuestionsDetected: m.FollowUpQuestionsDetected.Load(),
		SessionResetsForFollowUp:  m.SessionResetsForFollowUp.Load(),
		LastToolCallLatency:       m.lastToolCallLatency,
	}
}

// MetricsSnapshot is a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	ToolCallsTotal            int64
	ToolCallsSuccess          int64
	ToolCallsError            int64
	AudioFramesSent           int64
	AudioFramesReceived       int64
	AudioFramesDropped        int64
	AudioFramesDuplicated     int64
	AudioBufferFlushes        int64
	SessionsTotal             int64
	SessionsActive            int64
	FollowUpQuestionsDetected int64
	SessionResetsForFollowUp  int64
	LastToolCallLatency       time.Duration
}
