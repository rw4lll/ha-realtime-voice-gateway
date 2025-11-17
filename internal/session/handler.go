package session

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/audio"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/websocket"
	"go.uber.org/zap"
)

// Handler manages a session between a WebSocket connection and a backend LLM
// This preserves the core logic from the old Wyoming implementation
type Handler struct {
	id             string
	wsSession      *websocket.Session
	backendSession *backend.Session
	logger         *zap.Logger
	ctx            context.Context
	cancel         context.CancelFunc

	// Tool execution
	toolExecutor ToolExecutor

	// Audio processing
	audioProcessor     *audio.Processor
	audioStartBufferMs int
	audioStartSent     atomic.Bool
	audioSuppressed    atomic.Bool

	// Timing for session monitoring
	lastAudioTime     atomic.Value // time.Time
	lastUserAudioTime atomic.Value // time.Time

	// Configuration
	safetyTimeout time.Duration // Max session duration

	// Synchronization
	wg sync.WaitGroup
}

// ToolExecutor handles Home Assistant tool calls
type ToolExecutor interface {
	Execute(ctx context.Context, call *backend.ToolCall) (*backend.ToolResult, error)
}

// Config holds session handler configuration
type Config struct {
	WebSocketSession *websocket.Session
	BackendSession   *backend.Session
	ToolExecutor     ToolExecutor
	AudioProcessor   *audio.Processor // Optional audio processor for resampling
	Logger           *zap.Logger
	AudioBufferMs    int           // Audio start buffer duration
	SafetyTimeout    time.Duration // Max session duration (default: 5 min)
}

// NewHandler creates a new session handler
func NewHandler(cfg Config) *Handler {
	ctx, cancel := context.WithCancel(context.Background())

	if cfg.AudioBufferMs == 0 {
		cfg.AudioBufferMs = 100 // Default 100ms
	}

	if cfg.SafetyTimeout == 0 {
		cfg.SafetyTimeout = 5 * time.Minute // Default 5 min safety timeout
	}

	h := &Handler{
		id:                 cfg.WebSocketSession.ID,
		wsSession:          cfg.WebSocketSession,
		backendSession:     cfg.BackendSession,
		logger:             cfg.Logger.With(zap.String("handler_id", cfg.WebSocketSession.ID)),
		ctx:                ctx,
		cancel:             cancel,
		toolExecutor:       cfg.ToolExecutor,
		audioProcessor:     cfg.AudioProcessor,
		audioStartBufferMs: cfg.AudioBufferMs,
		safetyTimeout:      cfg.SafetyTimeout,
	}

	// Log audio processing configuration
	if h.audioProcessor != nil {
		stats := h.audioProcessor.GetStats()
		if stats.Enabled {
			h.logger.Info("audio resampling enabled for session",
				zap.String("algorithm", stats.Algorithm),
				zap.Int("source_rate", stats.SourceRate),
				zap.Int("target_rate", stats.TargetRate),
				zap.Float64("ratio", stats.Ratio))
		} else {
			h.logger.Debug("audio resampling disabled for session")
		}
	}

	return h
}

// Start starts the session handler
func (h *Handler) Start() {
	h.logger.Info("starting session handler",
		zap.Int("audio_buffer_ms", h.audioStartBufferMs),
		zap.Duration("safety_timeout", h.safetyTimeout))

	// Send initial listening state
	h.wsSession.SendState(websocket.StateListening)

	// Start goroutines
	h.wg.Add(4)
	go h.forwardAudioToBackend()
	go h.forwardAudioToDevice()
	go h.handleBackendEvents()
	go h.monitorSafetyTimeout()

	h.logger.Info("session handler started")
}

// forwardAudioToBackend forwards audio from device to backend LLM
func (h *Handler) forwardAudioToBackend() {
	defer h.wg.Done()

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Debug("audio→backend: context canceled")
			return

		case audioData, ok := <-h.wsSession.AudioIn:
			if !ok {
				h.logger.Debug("audio→backend: channel closed")
				h.cancel()
				return
			}

			// Track user audio time
			h.lastUserAudioTime.Store(time.Now())

			// Forward to backend
			select {
			case h.backendSession.AudioIn <- backend.AudioFrame{
				Data:      audioData,
				Timestamp: time.Now(),
			}:
				// Sent successfully
			case <-h.ctx.Done():
				return
			default:
				h.logger.Warn("backend audio buffer full, dropping frame")
			}
		}
	}
}

// forwardAudioToDevice forwards audio from backend to device with buffering
func (h *Handler) forwardAudioToDevice() {
	defer h.wg.Done()

	frameCount := 0
	// Calculate buffer based on frame count, not timing assumptions
	// With variable frame sizes from LLM, we buffer a fixed small number of frames
	bufferFrames := 3 // Buffer just 3 frames before starting playback
	if h.audioStartBufferMs > 0 {
		// If specific buffer time is set, use it but be conservative
		// Assume 20ms per frame (typical for Gemini at 24kHz)
		bufferFrames = h.audioStartBufferMs / 20
		if bufferFrames < 1 {
			bufferFrames = 1
		}
		// Cap at reasonable max to avoid long delays
		if bufferFrames > 10 {
			bufferFrames = 10
		}
	}
	audioBuffer := make([][]byte, 0, bufferFrames)
	audioStartSent := false

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Debug("audio→device: context canceled", zap.Int("total_frames", frameCount))
			return

		case audioFrame, ok := <-h.backendSession.AudioOut:
			if !ok {
				// Channel closed - flush remaining buffer
				if !audioStartSent && len(audioBuffer) > 0 {
					h.logger.Debug("flushing remaining audio buffer",
						zap.Int("buffered_frames", len(audioBuffer)))
					h.flushAudioBuffer(audioBuffer)
				}
				h.logger.Debug("audio→device: channel closed")
				h.cancel()
				return
			}

			frameCount++

			// Track last audio time
			h.lastAudioTime.Store(time.Now())

			// Skip if audio suppressed (after interruption)
			if h.audioSuppressed.Load() {
				if frameCount%50 == 0 {
					h.logger.Debug("dropping audio frame (suppressed)",
						zap.Int("bytes", len(audioFrame.Data)))
				}
				audioBuffer = audioBuffer[:0]
				audioStartSent = false
				h.audioStartSent.Store(false)
				continue
			}

			// Process audio (resampling if needed)
			processedData := audioFrame.Data
			if h.audioProcessor != nil {
				var err error
				processedData, err = h.audioProcessor.Process(audioFrame.Data)
				if err != nil {
					h.logger.Error("audio processing failed",
						zap.Error(err),
						zap.Int("input_size", len(audioFrame.Data)))
					continue
				}

				// Log size changes on first frame or periodically
				if frameCount == 1 || (frameCount%100 == 0 && len(processedData) != len(audioFrame.Data)) {
					h.logger.Debug("audio resampling applied",
						zap.Int("input_bytes", len(audioFrame.Data)),
						zap.Int("output_bytes", len(processedData)),
						zap.Int("frame_count", frameCount))
				}
			}

			// Buffer audio until we have confidence
			if !audioStartSent {
				audioBuffer = append(audioBuffer, processedData)
				if len(audioBuffer) >= bufferFrames {
					// Flush buffer
					h.flushAudioBuffer(audioBuffer)
					audioBuffer = audioBuffer[:0]
					audioStartSent = true
					h.audioStartSent.Store(true)
					h.logger.Debug("audio buffer flushed", zap.Int("buffer_ms", h.audioStartBufferMs))
				}
				continue
			}

			// Forward directly after buffer flushed
			select {
			case h.wsSession.AudioOut <- processedData:
				// Sent successfully
			case <-h.ctx.Done():
				return
			}
		}
	}
}

// flushAudioBuffer sends all buffered audio frames
func (h *Handler) flushAudioBuffer(buffer [][]byte) {
	for _, frame := range buffer {
		select {
		case h.wsSession.AudioOut <- frame:
		case <-h.ctx.Done():
			return
		}
	}
}

// handleBackendEvents processes events from the backend and maps to device states
func (h *Handler) handleBackendEvents() {
	defer h.wg.Done()

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Debug("event handler: context canceled")
			return

		case event, ok := <-h.backendSession.Events:
			if !ok {
				h.logger.Debug("event handler: channel closed")
				h.cancel()
				return
			}

			h.handleEvent(&event)
		}
	}
}

// handleEvent processes a single backend event
func (h *Handler) handleEvent(event *backend.Event) {
	switch event.Type {
	case backend.EventTranscriptDelta:
		// Progressive transcript (for logging)
		if text, ok := event.Data.(string); ok {
			h.logger.Debug("transcript delta", zap.String("text", text))
		}

	case backend.EventTranscriptDone:
		// User finished speaking - transition to thinking
		if text, ok := event.Data.(string); ok {
			h.logger.Info("user transcript complete", zap.String("text", text))
		}
		h.wsSession.SendState(websocket.StateThinking)

	case backend.EventAudioStart:
		// Backend starts speaking - transition to speaking
		h.logger.Debug("backend audio start")
		h.wsSession.SendState(websocket.StateSpeaking)
		if h.audioSuppressed.Swap(false) {
			h.logger.Debug("audio suppression cleared")
		}

	case backend.EventAudioEnd:
		// Backend finished speaking - monitor for silence, then send "done"
		h.logger.Debug("backend audio end")
		// After audio ends, we'll wait for silence before sending "done"
		// This is handled by monitorSafetyTimeout checking lastAudioTime

	case backend.EventAudioInterrupted:
		// User interrupted - suppress audio playback
		h.logger.Info("audio interrupted by backend")
		h.audioSuppressed.Store(true)
		h.drainDeviceAudioBuffer()

	case backend.EventToolCall:
		// Execute tool call
		if toolCall, ok := event.Data.(backend.ToolCall); ok {
			h.executeToolCall(&toolCall)
		} else {
			h.logger.Error("invalid tool call event data type",
				zap.String("expected", "backend.ToolCall"),
				zap.String("got", fmt.Sprintf("%T", event.Data)))
		}

	case backend.EventToolCallDone:
		h.logger.Debug("tool call done")

	case backend.EventError:
		// Backend error - notify device
		if err, ok := event.Data.(error); ok {
			h.logger.Error("backend error", zap.Error(err))
			h.wsSession.SendError(err.Error())
		} else if errStr, ok := event.Data.(string); ok {
			h.logger.Error("backend error", zap.String("error", errStr))
			h.wsSession.SendError(errStr)
		}

	case backend.EventSessionEnd:
		// Backend ended session
		h.logger.Info("backend session ended")
		h.wsSession.SendState(websocket.StateDone)
		h.cancel()

	default:
		h.logger.Warn("unknown event type", zap.String("type", string(event.Type)))
	}
}

// executeToolCall executes a Home Assistant tool call asynchronously
func (h *Handler) executeToolCall(toolCall *backend.ToolCall) {
	h.logger.Info("executing tool call",
		zap.String("tool_id", toolCall.ID),
		zap.String("tool_name", toolCall.Name))

	if h.toolExecutor == nil {
		h.logger.Warn("no tool executor configured")
		result := backend.ToolResult{
			CallID:    toolCall.ID,
			Error:     "tool executor not configured",
			Timestamp: time.Now(),
		}
		select {
		case h.backendSession.ToolResults <- result:
		case <-h.ctx.Done():
		}
		return
	}

	// Execute asynchronously
	go func() {
		startTime := time.Now()
		result, err := h.toolExecutor.Execute(h.ctx, toolCall)
		executionTime := time.Since(startTime)

		if err != nil {
			h.logger.Error("tool execution failed",
				zap.String("tool_id", toolCall.ID),
				zap.Error(err),
				zap.Duration("duration", executionTime))
			errorResult := backend.ToolResult{
				CallID:    toolCall.ID,
				Error:     err.Error(),
				Timestamp: time.Now(),
			}
			result = &errorResult
		}

		// Send result back to backend
		select {
		case h.backendSession.ToolResults <- *result:
			h.logger.Info("tool result sent",
				zap.String("tool_id", toolCall.ID),
				zap.Bool("success", result.Error == ""),
				zap.Duration("duration", executionTime))
		case <-h.ctx.Done():
			h.logger.Debug("context canceled while sending tool result")
		}
	}()
}

// drainDeviceAudioBuffer removes queued audio frames
func (h *Handler) drainDeviceAudioBuffer() {
	drained := 0
	for {
		select {
		case <-h.wsSession.AudioOut:
			drained++
		default:
			if drained > 0 {
				h.logger.Debug("drained audio buffer", zap.Int("frames", drained))
			}
			return
		}
	}
}

// monitorSafetyTimeout monitors for conversation completion and safety timeouts
func (h *Handler) monitorSafetyTimeout() {
	defer h.wg.Done()

	sessionStart := time.Now()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	const silenceThreshold = 3 * time.Second // Silence after backend stops

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Debug("timeout monitor: context canceled")
			return

		case <-ticker.C:
			// Check safety timeout (max session duration)
			if time.Since(sessionStart) > h.safetyTimeout {
				h.logger.Warn("safety timeout exceeded, closing session",
					zap.Duration("timeout", h.safetyTimeout),
					zap.Duration("duration", time.Since(sessionStart)))
				h.wsSession.SendState(websocket.StateDone)
				h.cancel()
				return
			}

			// Check for conversation completion (silence after both user and backend stop)
			// Only end conversation if BOTH backend finished speaking AND user hasn't spoken recently
			if h.audioStartSent.Load() {
				var lastActivity time.Time

				// Get last backend audio time
				lastAudioInterface := h.lastAudioTime.Load()
				if lastAudioInterface != nil {
					lastAudio := lastAudioInterface.(time.Time)
					if !lastAudio.IsZero() {
						lastActivity = lastAudio
					}
				}

				// Get last user audio time
				lastUserInterface := h.lastUserAudioTime.Load()
				if lastUserInterface != nil {
					lastUser := lastUserInterface.(time.Time)
					// Use the most recent activity (either user or backend)
					if !lastUser.IsZero() && lastUser.After(lastActivity) {
						lastActivity = lastUser
					}
				}

				// Only end if both sides have been silent for the threshold
				if !lastActivity.IsZero() {
					timeSinceActivity := time.Since(lastActivity)
					if timeSinceActivity > silenceThreshold {
						h.logger.Info("conversation complete (silence detected)",
							zap.Duration("silence", timeSinceActivity))
						h.wsSession.SendState(websocket.StateDone)
						h.cancel()
						return
					}
				}
			}
		}
	}
}

// Wait waits for the session handler to complete
func (h *Handler) Wait() {
	h.wg.Wait()
}

// Close closes the session handler
func (h *Handler) Close() error {
	h.logger.Info("closing session handler")
	h.cancel()
	h.wg.Wait()
	return nil
}
