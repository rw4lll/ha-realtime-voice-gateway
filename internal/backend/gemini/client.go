package gemini

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

const (
	// Default audio format for Gemini Live (24kHz, 16-bit, mono PCM)
	// Gemini sends audio at 24kHz (audio/pcm;rate=24000)
	defaultSampleRate    = 24000
	defaultSampleWidth   = 2
	defaultChannels      = 1
	defaultAudioMIMEType = "audio/pcm"

	// Session management
	// Increased from 1 minute to 5 minutes to reduce CPU overhead when idle
	// Sessions are still cleaned up immediately when context is cancelled
	sessionCleanupInterval = 5 * time.Minute  // How often to check for orphaned sessions
	maxBackoffDuration     = 30 * time.Second // Maximum backoff duration for retries

	// Audio buffering
	audioInputBufferSize  = 1000      // Increased for burst audio handling
	audioOutputBufferSize = 100       // Standard output buffer
	maxAudioChunkSize     = 10 * 1024 // 10KB chunks from Gemini
)

// GeminiBackend implements the backend interface for Google Gemini Live API using the official SDK
type GeminiBackend struct {
	client      *genai.Client
	logger      *zap.Logger
	apiKey      string
	model       string
	initialized bool

	// Timeout and retry configuration
	connectTimeout time.Duration
	receiveTimeout time.Duration
	sendTimeout    time.Duration
	maxRetries     int
	retryBackoff   time.Duration
	maxSessions    int

	realtimeInputConfig *genai.RealtimeInputConfig

	// Tool registry
	tools   []backend.Tool
	toolsMu sync.RWMutex

	// Session management
	sessions      map[string]*geminiSession
	sessionsMu    sync.Mutex
	cleanupTicker *time.Ticker
	cleanupStop   chan struct{}
	closeOnce     sync.Once
	closed        atomic.Bool
}

// NewGeminiBackend creates a new Gemini Live backend using the official SDK
func NewGeminiBackend(logger *zap.Logger) *GeminiBackend {
	return &GeminiBackend{
		logger:      logger,
		sessions:    make(map[string]*geminiSession),
		cleanupStop: make(chan struct{}),
	}
}

// Init initializes the Gemini backend
func (g *GeminiBackend) Init(ctx context.Context, cfg backend.Config) error {
	g.sessionsMu.Lock()
	defer g.sessionsMu.Unlock()

	// If already initialized, skip re-initialization
	if g.initialized {
		g.logger.Debug("Gemini backend already initialized, skipping")
		return nil
	}

	if cfg.APIKey == "" {
		return fmt.Errorf("gemini API key is required")
	}

	g.apiKey = cfg.APIKey
	g.model = cfg.Model
	if g.model == "" {
		g.model = "gemini-2.0-flash-exp" // Default model
	}

	// Set timeout and retry configuration with defaults
	g.connectTimeout = 30 * time.Second
	g.receiveTimeout = 60 * time.Second
	g.sendTimeout = 10 * time.Second
	g.maxRetries = 3
	g.retryBackoff = 1 * time.Second
	g.maxSessions = 5

	// Override with config values if provided
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["connect_timeout"].(time.Duration); ok && v > 0 {
			g.connectTimeout = v
		}
		if v, ok := cfg.Extra["receive_timeout"].(time.Duration); ok && v > 0 {
			g.receiveTimeout = v
		}
		if v, ok := cfg.Extra["send_timeout"].(time.Duration); ok && v > 0 {
			g.sendTimeout = v
		}
		if v, ok := cfg.Extra["max_retries"].(int); ok && v > 0 {
			g.maxRetries = v
		}
		if v, ok := cfg.Extra["retry_backoff"].(time.Duration); ok && v > 0 {
			g.retryBackoff = v
		}
		if v, ok := cfg.Extra["max_sessions"].(int); ok && v > 0 {
			g.maxSessions = v
		}

		ensureRealtime := func() *genai.RealtimeInputConfig {
			if g.realtimeInputConfig == nil {
				g.realtimeInputConfig = &genai.RealtimeInputConfig{}
			}
			return g.realtimeInputConfig
		}

		ensureDetection := func() *genai.AutomaticActivityDetection {
			realtime := ensureRealtime()
			if realtime.AutomaticActivityDetection == nil {
				realtime.AutomaticActivityDetection = &genai.AutomaticActivityDetection{}
			}
			return realtime.AutomaticActivityDetection
		}

		if val, ok := cfg.Extra["vad_disabled"].(bool); ok {
			ensureDetection().Disabled = val
		}

		if val, ok := cfg.Extra["vad_start_sensitivity"].(string); ok {
			switch strings.ToLower(val) {
			case "high":
				ensureDetection().StartOfSpeechSensitivity = genai.StartSensitivityHigh
			case "low":
				ensureDetection().StartOfSpeechSensitivity = genai.StartSensitivityLow
			}
		}

		if val, ok := cfg.Extra["vad_end_sensitivity"].(string); ok {
			switch strings.ToLower(val) {
			case "high":
				ensureDetection().EndOfSpeechSensitivity = genai.EndSensitivityHigh
			case "low":
				ensureDetection().EndOfSpeechSensitivity = genai.EndSensitivityLow
			}
		}

		toInt := func(v any) (int, bool) {
			switch value := v.(type) {
			case int:
				return value, true
			case int32:
				return int(value), true
			case int64:
				return int(value), true
			case float64:
				return int(value), true
			default:
				return 0, false
			}
		}

		if val, ok := cfg.Extra["vad_prefix_padding_ms"]; ok {
			if intVal, ok := toInt(val); ok && intVal > 0 {
				detection := ensureDetection()
				converted := int32(intVal)
				detection.PrefixPaddingMs = &converted
			}
		}

		if val, ok := cfg.Extra["vad_silence_duration_ms"]; ok {
			if intVal, ok := toInt(val); ok && intVal > 0 {
				detection := ensureDetection()
				converted := int32(intVal)
				detection.SilenceDurationMs = &converted
			}
		}

		if val, ok := cfg.Extra["activity_handling"].(string); ok {
			switch strings.ToLower(val) {
			case "start_interrupts":
				ensureRealtime().ActivityHandling = genai.ActivityHandlingStartOfActivityInterrupts
			case "no_interruption":
				ensureRealtime().ActivityHandling = genai.ActivityHandlingNoInterruption
			}
		}

		if val, ok := cfg.Extra["turn_coverage"].(string); ok {
			switch strings.ToLower(val) {
			case "only_activity":
				ensureRealtime().TurnCoverage = genai.TurnCoverageTurnIncludesOnlyActivity
			case "all_input":
				ensureRealtime().TurnCoverage = genai.TurnCoverageTurnIncludesAllInput
			}
		}
	}

	g.logger.Debug("Gemini configuration loaded",
		zap.Duration("connect_timeout", g.connectTimeout),
		zap.Duration("receive_timeout", g.receiveTimeout),
		zap.Duration("send_timeout", g.sendTimeout),
		zap.Int("max_retries", g.maxRetries),
		zap.Duration("retry_backoff", g.retryBackoff),
		zap.Int("max_sessions", g.maxSessions))

	// Create the official SDK client
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  g.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return fmt.Errorf("failed to create Gemini client: %w", err)
	}

	g.client = client
	g.initialized = true

	// Start session cleanup goroutine
	g.startSessionCleanup()

	g.logger.Info("Gemini backend initialized using official SDK",
		zap.String("model", g.model))

	return nil
}

// RegisterTools registers available Home Assistant tools
func (g *GeminiBackend) RegisterTools(tools []backend.Tool) error {
	g.toolsMu.Lock()
	defer g.toolsMu.Unlock()

	g.tools = tools
	g.logger.Info("registered tools", zap.Int("count", len(tools)))
	return nil
}

// StartSession starts a new Gemini Live session
func (g *GeminiBackend) StartSession(ctx context.Context, req backend.SessionConfig) (*backend.Session, error) {
	// Input validation
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.DeviceID == "" {
		return nil, fmt.Errorf("device_id is required")
	}

	g.logger.Debug("StartSession called",
		zap.String("session_id", req.SessionID),
		zap.String("device_id", req.DeviceID))

	if !g.initialized {
		return nil, backend.ErrBackendNotInitialized
	}

	// Check session limit
	g.sessionsMu.Lock()
	if len(g.sessions) >= g.maxSessions {
		g.sessionsMu.Unlock()
		return nil, fmt.Errorf("max concurrent sessions reached (%d)", g.maxSessions)
	}
	g.sessionsMu.Unlock()

	// Log context deadline if present
	if deadline, ok := ctx.Deadline(); ok {
		g.logger.Debug("session context has deadline",
			zap.Time("deadline", deadline),
			zap.Duration("time_remaining", time.Until(deadline)))
	}

	g.logger.Info("starting Gemini Live session using official SDK",
		zap.String("session_id", req.SessionID),
		zap.String("model", g.model))

	// Prepare system instruction
	var systemInstruction *genai.Content
	if req.SystemPrompt != "" {
		systemInstruction = genai.NewContentFromText(req.SystemPrompt, genai.RoleUser)
	}

	// Convert tools to SDK format
	g.toolsMu.RLock()
	sdkTools := make([]*genai.Tool, 0, len(g.tools))
	for _, tool := range g.tools {
		funcDecl := &genai.FunctionDeclaration{
			Name:                 tool.Name,
			Description:          tool.Description,
			ParametersJsonSchema: tool.Parameters, // Pass JSON Schema as-is
		}

		// Translate generic tool characteristics to Gemini-specific behavior
		if tool.Metadata != nil {
			// Check for data_returning characteristic
			if dataReturning, ok := tool.Metadata["data_returning"].(bool); ok && dataReturning {
				// Tools that return data must be blocking
				funcDecl.Behavior = genai.BehaviorBlocking
			} else {
				// Control operations can be non-blocking
				funcDecl.Behavior = genai.BehaviorNonBlocking
			}
		}

		sdkTool := &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{funcDecl},
		}
		sdkTools = append(sdkTools, sdkTool)
	}
	g.toolsMu.RUnlock()

	// Connect to Gemini Live with retry logic
	session, err := g.connectWithRetry(ctx, g.model, &genai.LiveConnectConfig{
		ResponseModalities:  []genai.Modality{genai.ModalityAudio},
		SystemInstruction:   systemInstruction,
		Tools:               sdkTools,
		RealtimeInputConfig: g.realtimeInputConfig,
		// Enable input transcription to help Gemini understand tool-calling intent better
		InputAudioTranscription: &genai.AudioTranscriptionConfig{},
		SpeechConfig: &genai.SpeechConfig{
			VoiceConfig: &genai.VoiceConfig{
				PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{
					VoiceName: "Puck", // Default voice
				},
			},
		},
	})
	if err != nil {
		g.logger.Error("failed to connect to Gemini Live after retries", zap.Error(err))
		return nil, err // Error already wrapped by connectWithRetry
	}

	// Create channels
	audioIn := make(chan backend.AudioFrame, audioInputBufferSize) // Increased for burst audio handling
	audioOut := make(chan backend.AudioFrame, audioOutputBufferSize)
	events := make(chan backend.Event, 50)
	toolResults := make(chan backend.ToolResult, 10)
	interrupt := make(chan struct{}, 1)
	control := make(chan backend.ControlMessage, 10)

	sessionCtx, cancel := context.WithCancel(ctx)

	gemSession := &geminiSession{
		id:              req.SessionID,
		deviceID:        req.DeviceID,
		userID:          req.UserID,
		session:         session,
		logger:          g.logger.With(zap.String("session_id", req.SessionID)),
		sendTimeout:     g.sendTimeout,
		receiveTimeout:  g.receiveTimeout,
		audioIn:         audioIn,
		audioOut:        audioOut,
		events:          events,
		toolResults:     toolResults,
		interrupt:       interrupt,
		control:         control,
		ctx:             sessionCtx,
		cancel:          cancel,
		audioHashes:     make(map[string]time.Time),
		lastHashCleanup: time.Now(),
	}

	// Store session
	g.sessionsMu.Lock()
	g.sessions[req.SessionID] = gemSession
	g.sessionsMu.Unlock()

	// Start processing goroutines
	go gemSession.readLoop()
	go gemSession.writeLoop()

	g.logger.Info("Gemini Live session started successfully using official SDK",
		zap.String("session_id", req.SessionID))

	backendSession := &backend.Session{
		ID: req.SessionID,
		Metadata: backend.SessionMetadata{
			DeviceID:  req.DeviceID,
			UserID:    req.UserID,
			CreatedAt: time.Now(),
			Model:     g.model,
			Backend:   "gemini",
		},
		AudioIn:     audioIn,
		AudioOut:    audioOut,
		Events:      events,
		ToolResults: toolResults,
		Interrupt:   interrupt,
		Control:     control,
		Close: func() error {
			gemSession.close()
			return nil
		},
	}

	// Set the context using reflection-free approach via the internal field
	// The Session struct has an unexported ctx field that's set via Context() method
	// But we need to pass sessionCtx somehow - let's use a helper if available
	// For now, the session will work without explicit ctx access from outside

	return backendSession, nil
}

// Capabilities returns Gemini backend capabilities
func (g *GeminiBackend) Capabilities() backend.Capabilities {
	return backend.Capabilities{
		SupportsStreaming:   true,
		SupportsToolCalling: true,
		SupportsBargeIn:     true,
		SupportedAudioFormats: []backend.AudioFormat{
			{
				SampleRate:    defaultSampleRate,
				Channels:      defaultChannels,
				BitsPerSample: defaultSampleWidth * 8, // Convert bytes to bits
				Encoding:      "pcm_s16le",
			},
		},
		MaxAudioChunkSize: maxAudioChunkSize,
		Provider:          "gemini",
	}
}

// Close closes the Gemini backend
func (g *GeminiBackend) Close() error {
	g.closeOnce.Do(func() {
		g.closed.Store(true)
		g.logger.Info("closing Gemini backend")

		// Stop cleanup goroutine
		if g.cleanupTicker != nil {
			g.cleanupTicker.Stop()
		}
		if g.cleanupStop != nil {
			close(g.cleanupStop)
		}

		// Close all sessions
		g.sessionsMu.Lock()
		for _, session := range g.sessions {
			session.close()
		}
		g.sessionsMu.Unlock()

		// Note: genai.Client doesn't have a Close() method
		// Sessions are closed individually above

		g.logger.Info("Gemini backend closed successfully")
	})

	return nil
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Network errors are retryable
	retryablePatterns := []string{
		"timeout",
		"temporary",
		"connection reset",
		"connection refused",
		"broken pipe",
		"no such host",
		"network is unreachable",
		// HTTP status codes that are retryable
		"429", // Rate limit
		"500", // Internal server error
		"502", // Bad gateway
		"503", // Service unavailable
		"504", // Gateway timeout
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	// Context deadline exceeded is retryable
	if strings.Contains(errStr, "context deadline exceeded") {
		return true
	}

	return false
}

// connectWithRetry attempts to connect to Gemini with retry logic
func (g *GeminiBackend) connectWithRetry(ctx context.Context, model string, config *genai.LiveConnectConfig) (*genai.Session, error) {
	backoff := g.retryBackoff
	var lastErr error

	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		if attempt > 0 {
			g.logger.Info("retrying connection to Gemini",
				zap.Int("attempt", attempt),
				zap.Int("max_retries", g.maxRetries),
				zap.Duration("backoff", backoff),
				zap.Error(lastErr))

			// Wait with backoff before retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				// Calculate next backoff (exponential)
				backoff *= 2
				if backoff > maxBackoffDuration {
					backoff = maxBackoffDuration
				}
			}
		}

		// Add timeout to connection attempt
		connectCtx, connectCancel := context.WithTimeout(ctx, g.connectTimeout)

		session, err := g.client.Live.Connect(connectCtx, model, config)
		connectCancel() // Always cancel context

		if err == nil {
			if attempt > 0 {
				g.logger.Info("connection successful after retry",
					zap.Int("attempts", attempt+1))
			}
			return session, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			g.logger.Error("non-retryable error encountered",
				zap.Error(err),
				zap.Int("attempt", attempt+1))
			return nil, fmt.Errorf("failed to connect to Gemini Live: %w", err)
		}

		g.logger.Warn("retryable error encountered",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", g.maxRetries))
	}

	return nil, fmt.Errorf("max retries (%d) exceeded, last error: %w", g.maxRetries, lastErr)
}

// startSessionCleanup starts a goroutine that periodically cleans up orphaned sessions
func (g *GeminiBackend) startSessionCleanup() {
	g.cleanupTicker = time.NewTicker(sessionCleanupInterval)
	go g.sessionCleanupLoop()
}

// sessionCleanupLoop periodically checks for and cleans up orphaned sessions
func (g *GeminiBackend) sessionCleanupLoop() {
	g.logger.Debug("session cleanup loop started")
	defer g.logger.Debug("session cleanup loop stopped")

	for {
		select {
		case <-g.cleanupTicker.C:
			g.cleanupOrphanedSessions()
		case <-g.cleanupStop:
			return
		}
	}
}

// cleanupOrphanedSessions removes sessions whose contexts have been cancelled
func (g *GeminiBackend) cleanupOrphanedSessions() {
	g.sessionsMu.Lock()
	defer g.sessionsMu.Unlock()

	orphanedCount := 0
	for id, session := range g.sessions {
		select {
		case <-session.ctx.Done():
			g.logger.Info("cleaning up orphaned session",
				zap.String("session_id", id),
				zap.String("device_id", session.deviceID))
			session.close()
			delete(g.sessions, id)
			orphanedCount++
		default:
			// Session still active
		}
	}

	if orphanedCount > 0 {
		g.logger.Info("cleaned up orphaned sessions",
			zap.Int("count", orphanedCount),
			zap.Int("active_sessions", len(g.sessions)))
	}
}

// geminiSession represents an active Gemini Live session
type geminiSession struct {
	id       string
	deviceID string
	userID   string

	session *genai.Session // Official SDK session
	logger  *zap.Logger

	// Timeouts
	sendTimeout    time.Duration
	receiveTimeout time.Duration

	// Channels for communication with pipeline
	audioIn     chan backend.AudioFrame
	audioOut    chan backend.AudioFrame
	events      chan backend.Event
	toolResults chan backend.ToolResult
	interrupt   chan struct{}
	control     chan backend.ControlMessage

	ctx    context.Context
	cancel context.CancelFunc

	// Lifecycle management
	wg          sync.WaitGroup
	closeOnce   sync.Once
	closed      atomic.Bool
	audioActive atomic.Bool

	// Audio deduplication - track audio chunks by content hash
	audioHashes     map[string]time.Time // hash -> first seen time
	audioHashesMu   sync.Mutex
	lastHashCleanup time.Time
}

// writeLoop writes audio and control messages to Gemini using the official SDK
func (s *geminiSession) writeLoop() {
	s.wg.Add(1)
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("writeLoop panic recovered",
				zap.Any("panic", r),
				zap.Stack("stacktrace"))
		}
	}()

	s.logger.Debug("write loop started",
		zap.String("session_id", s.id),
		zap.String("device_id", s.deviceID))
	defer s.logger.Debug("write loop stopped", zap.String("session_id", s.id))

	audioFrameCount := 0
	for {
		if s.closed.Load() {
			s.logger.Debug("write loop detected closed flag")
			return
		}

		select {
		case <-s.ctx.Done():
			s.logger.Debug("write loop context done", zap.Int("total_frames_sent", audioFrameCount))
			return

		case frame, ok := <-s.audioIn:
			if !ok {
				s.logger.Debug("audioIn channel closed, stopping write loop")
				return
			}

			audioFrameCount++
			// Log first frame and periodically to track audio streaming
			if audioFrameCount == 1 {
				s.logger.Info("sending first audio frame to Gemini",
					zap.Int("frame_size", len(frame.Data)))
			}
			if audioFrameCount%500 == 0 {
				s.logger.Debug("sending audio to Gemini",
					zap.Int("frames_sent", audioFrameCount),
					zap.Int("buffer_usage", len(s.audioIn)),
					zap.Int("buffer_capacity", cap(s.audioIn)))
			}

			// Send audio with timeout
			err := s.sendAudioWithTimeout(frame.Data)
			if err != nil {
				if s.ctx.Err() == nil {
					s.logger.Error("failed to send audio", zap.Error(err))
				}
				return
			}

		case toolResult, ok := <-s.toolResults:
			if !ok {
				s.logger.Debug("toolResults channel closed, stopping write loop")
				return
			}

			// Translate generic response timing to Gemini scheduling
			scheduling := s.translateResponseTiming(toolResult.Metadata)

			s.logger.Info("sending tool result to Gemini",
				zap.String("call_id", toolResult.CallID),
				zap.Bool("success", toolResult.Error == ""),
				zap.String("scheduling", scheduling))

			// Send tool response using the official SDK
			var response map[string]any
			if toolResult.Error != "" {
				response = map[string]any{
					"error": toolResult.Error,
				}
			} else {
				response = map[string]any{
					"result": toolResult.Result,
				}
			}

			funcResponse := &genai.FunctionResponse{
				ID:       toolResult.CallID,
				Response: response,
			}

			// Set scheduling for NON_BLOCKING functions (if provided)
			if scheduling != "" {
				switch scheduling {
				case "INTERRUPT":
					funcResponse.Scheduling = genai.FunctionResponseSchedulingInterrupt
				case "WHEN_IDLE":
					funcResponse.Scheduling = genai.FunctionResponseSchedulingWhenIdle
				case "SILENT":
					funcResponse.Scheduling = genai.FunctionResponseSchedulingSilent
				}
			}

			err := s.sendToolResponseWithTimeout(genai.LiveToolResponseInput{
				FunctionResponses: []*genai.FunctionResponse{funcResponse},
			})
			if err != nil {
				s.logger.Error("failed to send tool response", zap.Error(err))
			}

		case <-s.interrupt:
			s.logger.Debug("interrupt signal received")
			// The official SDK handles interruptions automatically
			// We just need to stop sending audio temporarily

		case ctrl, ok := <-s.control:
			if !ok {
				s.logger.Debug("control channel closed, stopping write loop")
				return
			}

			s.logger.Debug("control message received", zap.String("type", string(ctrl.Type)))

			// NOTE: Control messages are currently unused for Gemini Live API.
			// For realtime audio via SendRealtimeInput, Gemini's built-in VAD handles
			// turn detection automatically. There's no explicit turn_complete signal.
			//
			// Attempting to send turn_complete via SendClientContent would fail because:
			// "Interleaving SendClientContent and SendRealtimeInput is not recommended
			// and may lead to unexpected behavior" (per Gemini SDK documentation).
			//
			// This control channel is kept for future extensions:
			// TODO: Add support for ControlMute/ControlUnmute when Gemini SDK supports it
			// TODO: Implement explicit turn signaling if Gemini adds a recommended pattern
			// TODO: Consider using control messages for session-level configuration changes
		}
	}
}

// readLoop reads messages from Gemini using the official SDK
func (s *geminiSession) readLoop() {
	s.wg.Add(1)
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("readLoop panic recovered",
				zap.Any("panic", r),
				zap.Stack("stacktrace"))
		}
	}()
	defer s.close()   // Executes SECOND: close the session after wg is decremented
	defer s.wg.Done() // Executes FIRST: decrement WaitGroup (defers are LIFO)

	s.logger.Debug("read loop started",
		zap.String("session_id", s.id),
		zap.String("device_id", s.deviceID))
	defer s.logger.Debug("read loop stopped", zap.String("session_id", s.id))

	messageCount := 0
	for {
		if s.closed.Load() {
			s.logger.Debug("read loop detected closed flag")
			return
		}

		select {
		case <-s.ctx.Done():
			s.logger.Debug("read loop context done", zap.Int("total_messages_received", messageCount))
			return
		default:
			// Receive message with timeout
			if messageCount == 0 {
				s.logger.Debug("waiting for first message from Gemini...")
			}
			msg, err := s.receiveWithTimeout()
			if err != nil {
				if s.ctx.Err() == nil {
					s.logger.Error("failed to receive from Gemini", zap.Error(err))
				}
				return
			}

			messageCount++
			s.logger.Debug("received message from Gemini",
				zap.Int("message_number", messageCount),
				zap.Bool("has_server_content", msg.ServerContent != nil),
				zap.Bool("has_tool_call", msg.ToolCall != nil),
				zap.Bool("has_go_away", msg.GoAway != nil))

			// Handle different message types
			if msg.ServerContent != nil {
				s.handleServerContent(msg.ServerContent)
			}
			if msg.ToolCall != nil {
				s.handleToolCall(msg.ToolCall)
			}
			if msg.GoAway != nil {
				s.logger.Warn("received GoAway from Gemini",
					zap.Duration("time_left", msg.GoAway.TimeLeft))
				return
			}
		}
	}
}

// emitEvent forwards backend events to the pipeline with non-blocking semantics.
func (s *geminiSession) emitEvent(event backend.Event) {
	select {
	case s.events <- event:
	case <-s.ctx.Done():
	default:
		s.logger.Warn("events buffer full, dropping event",
			zap.String("event_type", string(event.Type)))
	}
}

// handleServerContent handles server content messages (including audio)
// handleServerContent handles server content messages (including audio)
func (s *geminiSession) handleServerContent(content *genai.LiveServerContent) {
	if content == nil {
		return
	}

	s.logger.Debug("handleServerContent called",
		zap.Bool("has_model_turn", content.ModelTurn != nil),
		zap.Bool("interrupted", content.Interrupted),
		zap.Bool("turn_complete", content.TurnComplete),
		zap.Bool("generation_complete", content.GenerationComplete))

	// Log the full structure for debugging
	if content.ModelTurn == nil {
		// This is normal behavior - Gemini sends ServerContent messages for turn management
		// without audio data (e.g., acknowledgments, state updates, turn completion signals)
		s.logger.Debug("ModelTurn is nil - Gemini sending non-audio message",
			zap.Bool("turn_complete", content.TurnComplete),
			zap.Bool("generation_complete", content.GenerationComplete),
			zap.Bool("interrupted", content.Interrupted))
		return
	}

	if content.Interrupted {
		s.logger.Info("received interruption signal from Gemini")
		s.emitEvent(backend.NewAudioInterruptedEvent(s.id))
	}

	if content.ModelTurn != nil {
		s.logger.Debug("processing ModelTurn",
			zap.Int("parts", len(content.ModelTurn.Parts)))
		for partIdx, part := range content.ModelTurn.Parts {
			s.logger.Debug("processing part",
				zap.Int("part_index", partIdx),
				zap.Bool("has_inline_data", part.InlineData != nil),
				zap.Bool("has_text", part.Text != ""))
			if part.InlineData != nil {
				s.logger.Debug("inline data details",
					zap.String("mime_type", part.InlineData.MIMEType),
					zap.Int("data_size", len(part.InlineData.Data)))
			}
			if part.InlineData != nil && strings.HasPrefix(part.InlineData.MIMEType, "audio/pcm") {
				audioData := part.InlineData.Data

				// Calculate hash of audio data for deduplication
				hash := sha256.Sum256(audioData)
				hashStr := hex.EncodeToString(hash[:])

				// Check if we've seen this exact audio chunk recently (within last 10 seconds)
				s.audioHashesMu.Lock()
				if firstSeen, exists := s.audioHashes[hashStr]; exists {
					timeSinceFirst := time.Since(firstSeen)
					if timeSinceFirst < 10*time.Second {
						s.logger.Debug("skipping duplicate audio chunk",
							zap.String("hash", hashStr[:16]),
							zap.Duration("age", timeSinceFirst))
						s.audioHashesMu.Unlock()
						continue // Skip this duplicate audio
					}
				}

				// Store hash with current timestamp
				s.audioHashes[hashStr] = time.Now()

				// Clean up old hashes every 30 seconds to prevent memory growth
				if time.Since(s.lastHashCleanup) > 30*time.Second {
					now := time.Now()
					for h, t := range s.audioHashes {
						if now.Sub(t) > 15*time.Second {
							delete(s.audioHashes, h)
						}
					}
					s.lastHashCleanup = now
				}
				s.audioHashesMu.Unlock()

				s.logger.Debug("received audio from Gemini",
					zap.Int("bytes", len(audioData)),
					zap.Int("buffer_usage", len(s.audioOut)),
					zap.Int("buffer_capacity", cap(s.audioOut)))

				if s.audioActive.CompareAndSwap(false, true) {
					s.emitEvent(backend.NewAudioStartEvent(s.id))
				}

				frame := backend.AudioFrame{
					Data:      audioData,
					Timestamp: time.Now(),
				}

				select {
				case s.audioOut <- frame:
				case <-s.ctx.Done():
					return
				default:
					s.logger.Warn("audio out buffer full, dropping frame",
						zap.Int("buffer_size", cap(s.audioOut)))
				}
			}

			if part.Text != "" {
				event := backend.NewTranscriptDoneEvent(s.id, backend.Transcript{
					Text:       part.Text,
					Confidence: 1.0,
				})
				s.emitEvent(event)
			}
		}
	}

	if content.GenerationComplete || content.Interrupted {
		if s.audioActive.Swap(false) {
			s.emitEvent(backend.NewAudioEndEvent(s.id))
		}
	}
}

// handleToolCall handles tool call requests from Gemini
func (s *geminiSession) handleToolCall(toolCall *genai.LiveServerToolCall) {
	for _, funcCall := range toolCall.FunctionCalls {
		s.logger.Info("tool call received from Gemini",
			zap.String("tool_name", funcCall.Name),
			zap.String("call_id", funcCall.ID),
			zap.Any("arguments", funcCall.Args))

		event := backend.NewToolCallEvent(s.id, backend.ToolCall{
			ID:        funcCall.ID,
			Name:      funcCall.Name,
			Arguments: funcCall.Args,
			Timestamp: time.Now(),
		})

		select {
		case s.events <- event:
		case <-s.ctx.Done():
			return
		default:
			s.logger.Warn("events buffer full, dropping tool call",
				zap.String("tool_name", funcCall.Name),
				zap.Int("buffer_size", cap(s.events)))
		}
	}
}

// sendAudioWithTimeout sends audio with a timeout
func (s *geminiSession) sendAudioWithTimeout(data []byte) error {
	type result struct {
		err error
	}

	done := make(chan result, 1)
	go func() {
		err := s.session.SendRealtimeInput(genai.LiveRealtimeInput{
			Audio: &genai.Blob{
				Data:     data,
				MIMEType: defaultAudioMIMEType,
			},
		})
		done <- result{err: err}
	}()

	select {
	case res := <-done:
		return res.err
	case <-time.After(s.sendTimeout):
		return fmt.Errorf("send audio timeout after %v", s.sendTimeout)
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// sendToolResponseWithTimeout sends tool response with a timeout
func (s *geminiSession) sendToolResponseWithTimeout(input genai.LiveToolResponseInput) error {
	type result struct {
		err error
	}

	done := make(chan result, 1)
	go func() {
		err := s.session.SendToolResponse(input)
		done <- result{err: err}
	}()

	select {
	case res := <-done:
		return res.err
	case <-time.After(s.sendTimeout):
		return fmt.Errorf("send tool response timeout after %v", s.sendTimeout)
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// sendClientContentWithTimeout sends client content (e.g., turn_complete) with a timeout
func (s *geminiSession) sendClientContentWithTimeout(input genai.LiveClientContentInput) error {
	type result struct {
		err error
	}

	done := make(chan result, 1)
	go func() {
		err := s.session.SendClientContent(input)
		done <- result{err: err}
	}()

	select {
	case res := <-done:
		return res.err
	case <-time.After(s.sendTimeout):
		return fmt.Errorf("send client content timeout after %v", s.sendTimeout)
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// receiveWithTimeout receives a message with a timeout
func (s *geminiSession) receiveWithTimeout() (*genai.LiveServerMessage, error) {
	type result struct {
		msg *genai.LiveServerMessage
		err error
	}

	done := make(chan result, 1)
	go func() {
		msg, err := s.session.Receive()
		done <- result{msg: msg, err: err}
	}()

	select {
	case res := <-done:
		return res.msg, res.err
	case <-time.After(s.receiveTimeout):
		return nil, fmt.Errorf("receive timeout after %v", s.receiveTimeout)
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

// translateResponseTiming translates generic response timing metadata to Gemini scheduling mode
func (s *geminiSession) translateResponseTiming(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}

	// Check for interrupt/urgent flag (errors or critical operations)
	if interrupt, ok := metadata["interrupt"].(bool); ok && interrupt {
		return "INTERRUPT"
	}

	// Check for deferrable flag (normal successful operations)
	if deferrable, ok := metadata["deferrable"].(bool); ok && deferrable {
		return "WHEN_IDLE"
	}

	// Default to no specific scheduling (let Gemini decide)
	return ""
}

// close closes the session
func (s *geminiSession) close() {
	s.closeOnce.Do(func() {
		s.logger.Debug("closing session", zap.String("session_id", s.id))
		startTime := time.Now()

		// Set closed flag
		s.closed.Store(true)

		// Cancel context to signal goroutines
		s.cancel()

		// Wait for goroutines with timeout
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			s.logger.Debug("all goroutines stopped gracefully",
				zap.String("session_id", s.id),
				zap.Duration("wait_time", time.Since(startTime)))
		case <-time.After(10 * time.Second):
			s.logger.Warn("timeout waiting for goroutines to stop",
				zap.String("session_id", s.id))
		}

		// Close the official SDK session
		if s.session != nil {
			if err := s.session.Close(); err != nil {
				s.logger.Error("failed to close Gemini session", zap.Error(err))
			}
		}

		duration := time.Since(startTime)
		s.logger.Info("session closed successfully",
			zap.String("session_id", s.id),
			zap.Duration("cleanup_duration", duration))
	})
}
