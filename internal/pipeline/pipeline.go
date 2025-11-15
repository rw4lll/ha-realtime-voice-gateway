package pipeline

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/protocol/wyoming"
	"go.uber.org/zap"
)

const (
	// Audio buffering and timing constants
	defaultAudioStartBufferMs = 500 // Default audio buffer duration before starting playback

	// Session monitoring constants
	backendSilenceThreshold = 2 * time.Second        // Duration of backend silence before considering response complete
	audioTickInterval       = 500 * time.Millisecond // How often to check audio timing

	// VAD (Voice Activity Detection) constants
	vadSilencePaddingMs = 500 // Milliseconds of silence to send for VAD end-of-speech detection
	vadSilenceFrameMs   = 10  // Duration per silence frame
	vadSilenceFrameSize = 640 // Bytes per silence frame (320 samples * 2 bytes at 16kHz)
)

// Pipeline connects Wyoming sessions to LLM backends and handles bidirectional data flow.
type Pipeline struct {
	backend      backend.Backend
	logger       *zap.Logger
	toolExecutor ToolExecutor

	// Session tracking
	sessions sync.Map // sessionID -> *bridgeSession

	// Configuration
	systemPrompt           string
	audioStartBufferMs     int // Milliseconds to buffer audio before sending (prevents premature audio-start)
	enableMetrics          bool
	metricsCollector       *MetricsCollector
	sessionMode            string // "turn_based" or "continuous"
	autoCloseAfterResponse bool
	sessionResponseTimeout time.Duration
}

// Config holds pipeline configuration.
type Config struct {
	Backend                backend.Backend
	Logger                 *zap.Logger
	ToolExecutor           ToolExecutor
	SystemPrompt           string
	AudioStartBufferMs     int           // Default: 500ms, set to 0 to disable buffering
	EnableMetrics          bool          // Enable metrics collection
	SessionMode            string        // "turn_based" or "continuous"
	AutoCloseAfterResponse bool          // Auto-close session after response
	SessionResponseTimeout time.Duration // Max time to wait for response
}

// ToolExecutor handles Home Assistant tool calls.
type ToolExecutor interface {
	Execute(ctx context.Context, call *backend.ToolCall) (*backend.ToolResult, error)
}

// bridgeSession represents a connection between a Wyoming session and a backend session.
// It handles bidirectional audio streaming, event processing, and multi-turn conversation management.
//
// Audio Buffering Strategy:
// The session buffers outgoing audio (backend → client) to prevent premature audio-start events.
// This is necessary because LLMs may send audio frames and then immediately interrupt themselves.
// By buffering the first few frames, we can avoid starting playback for very short responses
// that get cancelled. The buffer is flushed when:
//   - Enough frames accumulate (configurable via audioStartBufferMs)
//   - An audio-end event is received (explicit end signal)
//   - The audio output channel is closed
//
// Multi-Turn Conversation Flow:
// In turn-based mode, the session monitors for follow-up questions by tracking:
//   - lastAudioTime: When the backend last sent audio
//   - lastUserAudioTime: When the user last spoke
//
// If the user speaks after the backend stops, it's detected as a follow-up question,
// and the session is reset to prepare for the next response instead of closing.
type bridgeSession struct {
	id                     string
	wyomingConn            *wyoming.Session
	backendConn            *backend.Session
	logger                 *zap.Logger
	ctx                    context.Context
	cancel                 context.CancelFunc
	toolExecutor           ToolExecutor
	audioSuppressed        atomic.Bool
	audioStartBufferMs     int
	metrics                *MetricsCollector
	sessionMode            string
	autoCloseAfterResponse bool
	responseTimeout        time.Duration
	lastAudioTime          atomic.Value  // time.Time - last audio frame received from backend
	audioStartSent         atomic.Bool   // track if we've started sending audio
	lastUserAudioTime      atomic.Value  // time.Time - last audio frame received from user
	audioEndSignal         chan struct{} // signal to flush buffer when AudioEnd event received
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

	// Set default audio buffer duration if not specified
	audioStartBufferMs := cfg.AudioStartBufferMs
	if audioStartBufferMs == 0 {
		audioStartBufferMs = defaultAudioStartBufferMs
	}

	// Initialize metrics if enabled
	var metricsCollector *MetricsCollector
	if cfg.EnableMetrics {
		metricsCollector = NewMetricsCollector(logger)
		logger.Info("metrics collection enabled")
	}

	// Set defaults
	sessionMode := cfg.SessionMode
	if sessionMode == "" {
		sessionMode = "turn_based"
	}

	responseTimeout := cfg.SessionResponseTimeout
	if responseTimeout == 0 {
		responseTimeout = 30 * time.Second
	}

	p := &Pipeline{
		backend:                cfg.Backend,
		logger:                 logger,
		toolExecutor:           cfg.ToolExecutor,
		systemPrompt:           cfg.SystemPrompt,
		audioStartBufferMs:     audioStartBufferMs,
		enableMetrics:          cfg.EnableMetrics,
		metricsCollector:       metricsCollector,
		sessionMode:            sessionMode,
		autoCloseAfterResponse: cfg.AutoCloseAfterResponse,
		sessionResponseTimeout: responseTimeout,
	}

	// Initialize backend if not already initialized
	// (idempotent - backends skip if already initialized)
	backendCfg := backend.Config{
		AudioBufferSize: 100,
		EventBufferSize: 50,
	}
	if err := p.backend.Init(context.Background(), backendCfg); err != nil {
		return nil, fmt.Errorf("failed to initialize backend: %w", err)
	}

	logger.Info("pipeline initialized",
		zap.Int("audio_buffer_ms", audioStartBufferMs),
		zap.Bool("metrics_enabled", cfg.EnableMetrics),
		zap.String("session_mode", sessionMode),
		zap.Bool("auto_close", cfg.AutoCloseAfterResponse))
	return p, nil
}

// HandleWyomingSession handles a new Wyoming device connection.
// This is called by the Wyoming server when a device connects.
func (p *Pipeline) HandleWyomingSession(ws *wyoming.Session) {
	sessionLogger := p.logger.With(zap.String("session_id", ws.ID))
	sessionLogger.Info("new wyoming session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start backend session
	sessionLogger.Debug("attempting to start backend session",
		zap.String("session_id", ws.ID),
		zap.String("device_id", "wyoming-client"))
	backendSession, err := p.backend.StartSession(ctx, backend.SessionConfig{
		SessionID:    ws.ID,
		DeviceID:     "wyoming-client", // Default device ID for Wyoming protocol clients
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
		return
	}
	sessionLogger.Info("backend session started successfully")

	// Get backend's output audio format (for responses)
	backendCaps := p.backend.Capabilities()
	outputFormat := backendCaps.SupportedAudioFormats[0] // Use backend's primary format

	// Set Wyoming session's output format to match backend's output
	// This ensures audio from Gemini (24kHz) is correctly labeled
	ws.SetOutputAudioFormat(&wyoming.AudioFormat{
		Rate:     outputFormat.SampleRate,
		Width:    outputFormat.BitsPerSample / 8,
		Channels: outputFormat.Channels,
	})

	// Create bridge session
	bridge := &bridgeSession{
		id:                     ws.ID,
		wyomingConn:            ws,
		backendConn:            backendSession,
		logger:                 sessionLogger,
		ctx:                    ctx,
		cancel:                 cancel,
		toolExecutor:           p.toolExecutor,
		audioStartBufferMs:     p.audioStartBufferMs,
		metrics:                p.metricsCollector,
		sessionMode:            p.sessionMode,
		autoCloseAfterResponse: p.autoCloseAfterResponse,
		responseTimeout:        p.sessionResponseTimeout,
		audioEndSignal:         make(chan struct{}, 1), // buffered channel for AudioEnd signal
	}
	// Don't initialize lastAudioTime - let it be nil until we actually receive audio

	// Track session
	p.sessions.Store(ws.ID, bridge)
	defer p.sessions.Delete(ws.ID)

	// Record session start
	if p.metricsCollector != nil {
		p.metricsCollector.RecordSessionStart()
		defer p.metricsCollector.RecordSessionEnd()
	}

	sessionLogger.Info("bridge session created, starting data flow",
		zap.String("mode", bridge.sessionMode),
		zap.Bool("auto_close", bridge.autoCloseAfterResponse))

	// Start bidirectional data flow
	var wg sync.WaitGroup

	wg.Add(6) // Updated count for Wyoming event handler
	go bridge.forwardWyomingToBackend(&wg)
	go bridge.forwardBackendToWyoming(&wg)
	go bridge.handleBackendEvents(&wg)
	go bridge.handleWyomingEvents(&wg)
	go bridge.monitorShutdown(&wg)

	// Add response completion monitor for turn-based mode
	if bridge.sessionMode == "turn_based" && bridge.autoCloseAfterResponse {
		go bridge.monitorResponseCompletion(&wg)
	} else {
		wg.Done() // Don't start monitor, but keep wg count correct
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Clean up Wyoming session first
	if bridge.wyomingConn != nil {
		bridge.wyomingConn.Close()
	}

	// Clean up backend session
	if backendSession.Close != nil {
		backendSession.Close()
	}

	sessionLogger.Info("bridge session closed")
}

// forwardWyomingToBackend forwards audio from Wyoming device to backend LLM.
func (b *bridgeSession) forwardWyomingToBackend(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			b.logger.Debug("wyoming->backend: context canceled")
			return

		case audioData, ok := <-b.wyomingConn.AudioIn:
			if !ok {
				b.logger.Debug("wyoming->backend: audio channel closed")
				b.cancel() // Trigger shutdown
				return
			}

			// Track user audio for follow-up detection
			b.lastUserAudioTime.Store(time.Now())

			// Record metric
			if b.metrics != nil {
				b.metrics.RecordAudioFrameSent()
			}

			// Forward to backend (non-blocking)
			select {
			case b.backendConn.AudioIn <- backend.AudioFrame{
				Data:      audioData,
				Timestamp: time.Now(),
			}:
				// Sent successfully
			case <-b.ctx.Done():
				return
			default:
				b.logger.Warn("backend audio buffer full, dropping frame")
				if b.metrics != nil {
					b.metrics.RecordAudioFrameDropped()
				}
			}
		}
	}
}

// forwardBackendToWyoming forwards audio from backend LLM to Wyoming device.
//
// Audio Buffering Behavior:
// This method implements smart buffering to prevent premature audio-start events.
// The first N frames (configurable via audioStartBufferMs) are buffered before
// sending audio-start to the client. This prevents starting playback for responses
// that get immediately interrupted or cancelled by the LLM.
//
// The buffer is flushed (and audio-start is sent) when:
//  1. Buffer reaches capacity (normal flow)
//  2. AudioEnd event is received via audioEndSignal (explicit end)
//  3. Audio channel is closed (session ending)
//
// This ensures clients only receive audio that the LLM intends to deliver.
func (b *bridgeSession) forwardBackendToWyoming(wg *sync.WaitGroup) {
	defer wg.Done()

	frameCount := 0
	// Calculate buffer size based on configuration (default 500ms = 50 frames at 10ms each)
	bufferFrames := b.audioStartBufferMs / 10
	if bufferFrames < 1 {
		bufferFrames = 1 // Minimum 1 frame
	}
	audioBuffer := make([][]byte, 0, bufferFrames)
	audioStartSent := false

	for {
		select {
		case <-b.ctx.Done():
			// Don't flush on context cancel - the session is ending
			b.logger.Debug("backend->wyoming: context canceled", zap.Int("total_frames", frameCount))
			return

		case <-b.audioEndSignal:
			// AudioEnd event received - flush any buffered audio immediately
			if !audioStartSent && len(audioBuffer) > 0 {
				b.logger.Info("flushing audio buffer on AudioEnd event",
					zap.Int("buffered_frames", len(audioBuffer)))

				// Record metric
				if b.metrics != nil {
					b.metrics.RecordAudioBufferFlush()
				}

				// Flush all buffered frames
				for _, bufferedFrame := range audioBuffer {
					select {
					case b.wyomingConn.AudioOut <- bufferedFrame:
					case <-b.ctx.Done():
						return
					}
				}
				audioBuffer = audioBuffer[:0] // Clear buffer
				audioStartSent = true
				b.audioStartSent.Store(true)
			}
			// Continue processing - don't return

		case audioFrame, ok := <-b.backendConn.AudioOut:
			if !ok {
				// Channel closed - flush any remaining buffered audio
				if !audioStartSent && len(audioBuffer) > 0 {
					b.logger.Debug("flushing remaining audio buffer on channel close",
						zap.Int("buffered_frames", len(audioBuffer)))

					// Record metric
					if b.metrics != nil {
						b.metrics.RecordAudioBufferFlush()
					}

					for _, bufferedFrame := range audioBuffer {
						select {
						case b.wyomingConn.AudioOut <- bufferedFrame:
						case <-b.ctx.Done():
							return
						}
					}
					audioStartSent = true
					b.audioStartSent.Store(true)
				}
				b.logger.Debug("backend->wyoming: audio channel closed")
				b.cancel() // Trigger shutdown
				return
			}

			frameCount++

			// Track last audio time for turn-based mode
			b.lastAudioTime.Store(time.Now())

			// Record metric
			if b.metrics != nil {
				b.metrics.RecordAudioFrameReceived()
			}

			if frameCount == 1 || frameCount%100 == 0 {
				b.logger.Debug("forwarding audio to Wyoming",
					zap.Int("frame_count", frameCount),
					zap.Int("bytes", len(audioFrame.Data)))
			}

			if b.audioSuppressed.Load() {
				if frameCount == 1 || frameCount%50 == 0 {
					b.logger.Debug("dropping audio frame due to suppression",
						zap.Int("bytes", len(audioFrame.Data)))
				}
				// Record dropped frame
				if b.metrics != nil {
					b.metrics.RecordAudioFrameDropped()
				}
				// Clear buffer and reset flag when suppressed
				audioBuffer = audioBuffer[:0]
				audioStartSent = false
				b.audioStartSent.Store(false)
				continue
			}

			// Buffer audio until we have enough confidence
			if !audioStartSent {
				audioBuffer = append(audioBuffer, audioFrame.Data)
				if len(audioBuffer) >= bufferFrames {
					// Send buffered audio
					for _, bufferedFrame := range audioBuffer {
						select {
						case b.wyomingConn.AudioOut <- bufferedFrame:
						case <-b.ctx.Done():
							return
						}
					}
					audioBuffer = audioBuffer[:0]
					audioStartSent = true
					b.audioStartSent.Store(true)
					b.logger.Debug("audio buffer flushed",
						zap.Int("buffer_ms", b.audioStartBufferMs))
				}
				continue
			}

			// Forward to Wyoming (after audio-start sent)
			select {
			case b.wyomingConn.AudioOut <- audioFrame.Data:
				// Sent successfully
			case <-b.ctx.Done():
				return
			}
		}
	}
}

// handleBackendEvents processes events from the backend (transcripts, tool calls, errors).
func (b *bridgeSession) handleBackendEvents(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			b.logger.Debug("event handler: context canceled")
			return

		case event, ok := <-b.backendConn.Events:
			if !ok {
				b.logger.Debug("event handler: events channel closed")
				b.cancel() // Trigger shutdown
				return
			}

			b.handleEvent(&event)
		}
	}
}

// handleEvent processes a single backend event.
func (b *bridgeSession) handleEvent(event *backend.Event) {
	switch event.Type {
	case backend.EventTranscriptDelta:
		// Progressive transcript (not sent to device, for logging/debugging)
		if text, ok := event.Data.(string); ok {
			b.logger.Debug("transcript delta", zap.String("text", text))
		}

	case backend.EventTranscriptDone:
		// Final transcript (for logging)
		if text, ok := event.Data.(string); ok {
			b.logger.Info("transcript done", zap.String("text", text))
		}

	case backend.EventAudioStart:
		b.logger.Debug("audio start event from backend")
		if b.audioSuppressed.Swap(false) {
			b.logger.Debug("audio suppression cleared due to start event")
		}

	case backend.EventAudioEnd:
		b.logger.Debug("audio end event from backend - signaling buffer flush")
		// Signal forwardBackendToWyoming to flush any buffered audio immediately
		select {
		case b.audioEndSignal <- struct{}{}:
			b.logger.Debug("audioEndSignal sent successfully")
		default:
			// Channel full (signal already pending) - that's fine
			b.logger.Debug("audioEndSignal already pending")
		}
		// Don't clear suppression on end - only clear on next start
		// This prevents replaying audio after an interruption

	case backend.EventAudioInterrupted:
		b.handleAudioInterrupted()

	case backend.EventToolCall:
		// Handle tool call request from LLM
		if toolCall, ok := event.Data.(backend.ToolCall); ok {
			b.handleToolCall(&toolCall)
		} else {
			b.logger.Error("invalid tool call event data type",
				zap.String("expected", "backend.ToolCall"),
				zap.String("got", fmt.Sprintf("%T", event.Data)))
		}

	case backend.EventToolCallDone:
		b.logger.Debug("tool call done")

	case backend.EventError:
		if err, ok := event.Data.(error); ok {
			b.logger.Error("backend error", zap.Error(err))
		} else if errStr, ok := event.Data.(string); ok {
			b.logger.Error("backend error", zap.String("error", errStr))
		}

	case backend.EventSessionEnd:
		b.logger.Info("backend session ended")
		b.cancel() // Trigger shutdown

	default:
		b.logger.Warn("unknown event type", zap.String("type", string(event.Type)))
	}
}

// handleToolCall executes a Home Assistant tool call and sends the result back to the backend.
func (b *bridgeSession) handleToolCall(toolCall *backend.ToolCall) {
	b.logger.Info("executing tool call",
		zap.String("tool_id", toolCall.ID),
		zap.String("tool_name", toolCall.Name),
	)

	if b.toolExecutor == nil {
		b.logger.Warn("no tool executor configured, skipping tool call")
		// Record failed metric
		if b.metrics != nil {
			b.metrics.RecordToolCall(false)
		}
		// Send error result back
		result := backend.ToolResult{
			CallID:    toolCall.ID,
			Error:     "tool executor not configured",
			Timestamp: time.Now(),
		}
		select {
		case b.backendConn.ToolResults <- result:
		case <-b.ctx.Done():
		}
		return
	}

	// Execute tool call asynchronously
	go func() {
		startTime := time.Now()
		result, err := b.toolExecutor.Execute(b.ctx, toolCall)
		executionTime := time.Since(startTime)

		success := err == nil && (result != nil && result.Error == "")

		// Record metrics
		if b.metrics != nil {
			b.metrics.RecordToolCall(success)
			b.metrics.RecordToolCallLatency(executionTime)
		}

		if err != nil {
			b.logger.Error("tool execution failed",
				zap.String("tool_id", toolCall.ID),
				zap.Error(err),
				zap.Duration("duration", executionTime),
			)
			errorResult := backend.ToolResult{
				CallID:    toolCall.ID,
				Error:     err.Error(),
				Timestamp: time.Now(),
			}
			result = &errorResult
		}

		// Send result back to backend
		select {
		case b.backendConn.ToolResults <- *result:
			b.logger.Info("tool result sent",
				zap.String("tool_id", toolCall.ID),
				zap.Bool("success", result.Error == ""),
				zap.Duration("duration", executionTime),
			)
		case <-b.ctx.Done():
			b.logger.Debug("context canceled while sending tool result")
		}
	}()
}

// handleAudioInterrupted suppresses outgoing audio and flushes pending buffers.
func (b *bridgeSession) handleAudioInterrupted() {
	b.logger.Info("audio interrupted by backend; suppressing playback")
	b.audioSuppressed.Store(true)

	drained := b.drainWyomingAudioBuffer()
	if drained > 0 {
		b.logger.Debug("cleared pending Wyoming audio frames", zap.Int("frames", drained))
	}
}

// handleWyomingEvents listens to Wyoming protocol events (audio-stop, etc.).
func (b *bridgeSession) handleWyomingEvents(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			b.logger.Debug("wyoming event handler: context canceled")
			return

		case event, ok := <-b.wyomingConn.Events:
			if !ok {
				b.logger.Debug("wyoming events channel closed")
				return
			}

			// Handle audio-stop event
			if event.Type == wyoming.EventAudioStop {
				b.logger.Debug("audio-stop received from Wyoming client")

				// NOTE: For realtime audio (SendRealtimeInput), Gemini's built-in VAD
				// automatically detects silence and triggers model response.
				// However, VAD needs trailing silence to detect end-of-speech.
				//
				// Send silence padding to give VAD time to trigger response.
				silenceDuration := time.Duration(vadSilencePaddingMs) * time.Millisecond
				silenceFrames := vadSilencePaddingMs / vadSilenceFrameMs

				// Create silence frame (320 samples * 2 bytes per sample = 640 bytes of zeros)
				silenceFrame := make([]byte, vadSilenceFrameSize)

				b.logger.Debug("sending silence padding for VAD detection",
					zap.Int("frames", silenceFrames),
					zap.Duration("duration", silenceDuration))

				for i := 0; i < silenceFrames; i++ {
					select {
					case b.backendConn.AudioIn <- backend.AudioFrame{
						Data:      silenceFrame,
						Timestamp: time.Now(),
					}:
						// Sent successfully
					case <-b.ctx.Done():
						return
					default:
						b.logger.Warn("backend audio buffer full during silence padding")
						goto silenceDone
					}
				}
			silenceDone:

				b.logger.Debug("silence padding sent, VAD should detect end-of-speech")
			}
		}
	}
}

// drainWyomingAudioBuffer removes any queued audio frames awaiting device playback.
func (b *bridgeSession) drainWyomingAudioBuffer() int {
	drained := 0
	for {
		select {
		case <-b.wyomingConn.AudioOut:
			drained++
		default:
			return drained
		}
	}
}

// monitorShutdown monitors for Wyoming connection closure.
func (b *bridgeSession) monitorShutdown(wg *sync.WaitGroup) {
	defer wg.Done()

	// Just wait for context cancellation
	// The Wyoming session will close its channels when the connection closes,
	// which will trigger cancellation via the audio forwarding loops
	<-b.ctx.Done()
	b.logger.Debug("shutdown monitor: context canceled")
}

// monitorResponseCompletion monitors for conversation turn completion in turn-based mode.
// Auto-closes the session after the LLM finishes responding.
//
// Multi-Turn Conversation Support:
// This monitor detects follow-up questions by comparing user activity with backend silence.
// If the user speaks after the backend stops responding, it's treated as a follow-up,
// and the session continues instead of closing. This enables natural multi-turn conversations
// without requiring the client to reconnect for each question.
//
// Timeout Management:
// - Response timeout: Maximum time to wait for backend audio after user speech
// - Silence threshold: How long backend must be silent before checking for follow-ups
// - Grace period: Enforced by the client, not this monitor
func (b *bridgeSession) monitorResponseCompletion(wg *sync.WaitGroup) {
	defer wg.Done()

	b.logger.Debug("response completion monitor started",
		zap.Duration("timeout", b.responseTimeout))

	sessionStart := time.Now()
	lastSessionResetTime := sessionStart        // Track when we last reset for multi-turn
	ticker := time.NewTicker(audioTickInterval) // Check periodically
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			b.logger.Debug("response monitor: context canceled")
			return

		case <-ticker.C:
			// Only monitor response completion if we've started sending audio (response began)
			audioStarted := b.audioStartSent.Load()

			// Check if we've exceeded the overall timeout since last reset
			// Only enforce this AFTER we get a response, to allow multi-turn
			if audioStarted {
				timeSinceReset := time.Since(lastSessionResetTime)
				if timeSinceReset > b.responseTimeout {
					b.logger.Warn("session timeout after response",
						zap.Duration("timeout", b.responseTimeout),
						zap.Duration("time_since_reset", timeSinceReset))
					b.cancel()
					return
				}
			}

			if !audioStarted {
				continue
			}

			// Check if audio has stopped (no new frames for a while)
			lastAudioInterface := b.lastAudioTime.Load()
			if lastAudioInterface == nil {
				continue
			}

			lastAudio := lastAudioInterface.(time.Time)
			// Skip if lastAudio is zero (not yet set or reset)
			if lastAudio.IsZero() {
				continue
			}

			timeSinceLastAudio := time.Since(lastAudio)

			// If Gemini stopped speaking (silence threshold), monitor for follow-ups
			if timeSinceLastAudio > backendSilenceThreshold {
				// Check if user is sending a follow-up question
				if b.detectFollowUpQuestion(lastAudio) {
					// Reset for next turn
					b.resetForFollowUp()
					lastSessionResetTime = time.Now()
					continue
				}

				// After Gemini response, keep session open indefinitely
				// Let client decide when to disconnect (after their own grace period)
				// Safety timeout will close if needed
				continue
			}

			// Check overall timeout (in case Gemini never responds)
			if timeSinceLastAudio > b.responseTimeout {
				b.logger.Warn("response timeout exceeded, closing session",
					zap.Duration("timeout", b.responseTimeout))
				b.cancel()
				return
			}
		}
	}
}

// detectFollowUpQuestion checks if the user is asking a follow-up question
// after the backend (Gemini) has stopped responding.
func (b *bridgeSession) detectFollowUpQuestion(lastBackendAudio time.Time) bool {
	lastUserAudioInterface := b.lastUserAudioTime.Load()
	if lastUserAudioInterface == nil {
		return false
	}

	lastUserAudio := lastUserAudioInterface.(time.Time)
	// Skip if lastUserAudio is zero (not yet set)
	if lastUserAudio.IsZero() {
		return false
	}

	// If user spoke after Gemini stopped, they're asking a follow-up
	if lastUserAudio.After(lastBackendAudio) {
		b.logger.Info("user follow-up detected - continuing session",
			zap.Time("user_last_audio", lastUserAudio),
			zap.Time("gemini_stopped", lastBackendAudio))

		// Record metric
		if b.metrics != nil {
			b.metrics.RecordFollowUpQuestion()
		}

		return true
	}

	return false
}

// resetForFollowUp resets session state to prepare for the next conversation turn.
func (b *bridgeSession) resetForFollowUp() {
	// Reset audio start sent flag to prepare for next response
	b.audioStartSent.Store(false)
	// Clear lastAudioTime so we wait for Gemini's next response
	// Use zero value instead of nil to maintain atomic.Value type consistency
	b.lastAudioTime.Store(time.Time{})
	b.logger.Debug("session state reset for follow-up question")

	// Record metric
	if b.metrics != nil {
		b.metrics.RecordSessionResetForFollowUp()
	}
}

// Close shuts down the pipeline and all active sessions.
func (p *Pipeline) Close() error {
	p.logger.Info("closing pipeline")

	// Log final metrics
	if p.metricsCollector != nil {
		p.metricsCollector.LogMetrics()
	}

	// Close all active sessions
	p.sessions.Range(func(key, value any) bool {
		if bridge, ok := value.(*bridgeSession); ok {
			bridge.cancel()
		}
		return true
	})

	// Close backend
	if err := p.backend.Close(); err != nil {
		p.logger.Error("failed to close backend", zap.Error(err))
		return err
	}

	p.logger.Info("pipeline closed")
	return nil
}

// GetSessionCount returns the number of active bridge sessions.
func (p *Pipeline) GetSessionCount() int {
	count := 0
	p.sessions.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
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
