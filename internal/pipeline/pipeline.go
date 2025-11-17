package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/session"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/websocket"
	"go.uber.org/zap"
)

// Pipeline connects device sessions to LLM backends and handles bidirectional data flow.
type Pipeline struct {
	backend      backend.Backend
	logger       *zap.Logger
	toolExecutor ToolExecutor

	// Session tracking
	sessions sync.Map // sessionID -> *session.Handler

	// Configuration
	systemPrompt     string
	enableMetrics    bool
	metricsCollector *MetricsCollector
	audioBufferMs    int
	safetyTimeout    time.Duration
	silenceTimeout   time.Duration
}

// Config holds pipeline configuration.
type Config struct {
	Backend        backend.Backend
	Logger         *zap.Logger
	ToolExecutor   ToolExecutor
	SystemPrompt   string
	AudioBufferMs  int           // Audio buffering before playback (ms)
	EnableMetrics  bool          // Enable metrics collection
	SafetyTimeout  time.Duration // Max session duration
	SilenceTimeout time.Duration // Silence before ending conversation
}

// ToolExecutor handles Home Assistant tool calls.
type ToolExecutor interface {
	Execute(ctx context.Context, call *backend.ToolCall) (*backend.ToolResult, error)
}

// NewPipeline creates a new pipeline instance.
func NewPipeline(cfg Config) (*Pipeline, error) {
	if cfg.Backend == nil {
		return nil, fmt.Errorf("backend is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	logger := cfg.Logger.With(zap.String("component", "pipeline"))

	// Initialize metrics if enabled
	var metricsCollector *MetricsCollector
	if cfg.EnableMetrics {
		metricsCollector = NewMetricsCollector(logger)
		logger.Info("metrics collection enabled")
	}

	// Set defaults
	audioBufferMs := cfg.AudioBufferMs
	if audioBufferMs == 0 {
		audioBufferMs = 500
	}

	safetyTimeout := cfg.SafetyTimeout
	if safetyTimeout == 0 {
		safetyTimeout = 5 * time.Minute
	}

	silenceTimeout := cfg.SilenceTimeout
	if silenceTimeout == 0 {
		silenceTimeout = 3 * time.Second
	}

	p := &Pipeline{
		backend:          cfg.Backend,
		logger:           logger,
		toolExecutor:     cfg.ToolExecutor,
		systemPrompt:     cfg.SystemPrompt,
		enableMetrics:    cfg.EnableMetrics,
		metricsCollector: metricsCollector,
		audioBufferMs:    audioBufferMs,
		safetyTimeout:    safetyTimeout,
		silenceTimeout:   silenceTimeout,
	}

	// Initialize backend if not already initialized
	backendCfg := backend.Config{
		AudioBufferSize: 100,
		EventBufferSize: 50,
	}
	if err := p.backend.Init(context.Background(), backendCfg); err != nil {
		return nil, fmt.Errorf("failed to initialize backend: %w", err)
	}

	logger.Info("pipeline initialized",
		zap.Bool("metrics_enabled", cfg.EnableMetrics),
		zap.Int("audio_buffer_ms", audioBufferMs),
		zap.Duration("safety_timeout", safetyTimeout),
		zap.Duration("silence_timeout", silenceTimeout))
	return p, nil
}

// HandleWebSocketSession handles a new WebSocket device connection
func (p *Pipeline) HandleWebSocketSession(ws *websocket.Session) {
	sessionLogger := p.logger.With(zap.String("session_id", ws.ID))
	sessionLogger.Info("new websocket session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start backend session
	sessionLogger.Debug("starting backend session")
	backendSession, err := p.backend.StartSession(ctx, backend.SessionConfig{
		SessionID:    ws.ID,
		DeviceID:     "esp32-voice-device",
		SystemPrompt: p.systemPrompt,
		AudioFormat: backend.AudioFormat{
			SampleRate:    16000,
			Channels:      1,
			BitsPerSample: 16,
			Encoding:      "pcm",
		},
	})
	if err != nil {
		sessionLogger.Error("failed to start backend session", zap.Error(err))
		ws.SendError("Failed to initialize AI backend")
		ws.Close()
		return
	}
	sessionLogger.Info("backend session started successfully")

	// Create session handler (preserves old Wyoming logic)
	handler := session.NewHandler(session.Config{
		WebSocketSession: ws,
		BackendSession:   backendSession,
		ToolExecutor:     p.toolExecutor,
		Logger:           sessionLogger,
		AudioBufferMs:    p.audioBufferMs,
		SafetyTimeout:    p.safetyTimeout,
	})

	// Track session
	p.sessions.Store(ws.ID, handler)
	defer p.sessions.Delete(ws.ID)

	// Record session start metric
	if p.metricsCollector != nil {
		p.metricsCollector.RecordSessionStart()
		defer p.metricsCollector.RecordSessionEnd()
	}

	sessionLogger.Info("session handler created, starting data flow")

	// Start session handler
	handler.Start()

	// Wait for completion
	handler.Wait()

	// Clean up backend session
	if backendSession.Close != nil {
		backendSession.Close()
	}

	// Clean up WebSocket session
	ws.Close()

	sessionLogger.Info("session complete")
}

// GetSessionCount returns the number of active sessions
func (p *Pipeline) GetSessionCount() int {
	count := 0
	p.sessions.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Close shuts down the pipeline and all active sessions.
func (p *Pipeline) Close() error {
	p.logger.Info("closing pipeline")

	// Log final metrics
	if p.metricsCollector != nil {
		p.metricsCollector.LogMetrics()
	}

	// Close backend
	if err := p.backend.Close(); err != nil {
		p.logger.Error("failed to close backend", zap.Error(err))
		return err
	}

	p.logger.Info("pipeline closed")
	return nil
}

// GetMetrics returns current metrics snapshot (returns nil if metrics disabled)
func (p *Pipeline) GetMetrics() *MetricsSnapshot {
	if p.metricsCollector == nil {
		return nil
	}
	snapshot := p.metricsCollector.GetSnapshot()
	return &snapshot
}

// LogMetrics logs current metrics (no-op if metrics disabled)
func (p *Pipeline) LogMetrics() {
	if p.metricsCollector != nil {
		p.metricsCollector.LogMetrics()
	}
}
