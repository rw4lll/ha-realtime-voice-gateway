package wyoming

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// Session represents a Wyoming protocol session with a connected device
type Session struct {
	ID     string
	conn   net.Conn
	codec  *Codec
	logger *zap.Logger

	// Service information
	serviceInfo *ServiceInfo

	// Channels for audio streaming
	AudioIn  chan []byte // Audio from device → gateway
	AudioOut chan []byte // Audio from gateway → device
	Events   chan *Event // Non-audio control events from device

	// Audio format tracking
	audioFormat       *AudioFormat // Input format (device → gateway)
	outputAudioFormat *AudioFormat // Output format (gateway → device)
	formatMu          sync.RWMutex

	// Session state
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	closed  bool
	closeMu sync.Mutex
}

// SessionConfig holds configuration for a Wyoming session
type SessionConfig struct {
	ID              string
	Conn            net.Conn
	Logger          *zap.Logger
	AudioBufferSize int
	EventBufferSize int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ServiceInfo     *ServiceInfo // Service capabilities for describe response
}

// NewSession creates a new Wyoming session
func NewSession(cfg SessionConfig) *Session {
	ctx, cancel := context.WithCancel(context.Background())

	if cfg.AudioBufferSize == 0 {
		cfg.AudioBufferSize = 100 // Default from CLAUDE.md
	}
	if cfg.EventBufferSize == 0 {
		cfg.EventBufferSize = 50 // Default from CLAUDE.md
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	sess := &Session{
		ID:          cfg.ID,
		conn:        cfg.Conn,
		codec:       NewCodec(cfg.Conn),
		logger:      cfg.Logger,
		serviceInfo: cfg.ServiceInfo,
		AudioIn:     make(chan []byte, cfg.AudioBufferSize),
		AudioOut:    make(chan []byte, cfg.AudioBufferSize),
		Events:      make(chan *Event, cfg.EventBufferSize),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start goroutines to handle I/O
	sess.wg.Add(2)
	go sess.readLoop(cfg.ReadTimeout)
	go sess.writeLoop(cfg.WriteTimeout)

	return sess
}

// sendDescribeResponse sends service capabilities in response to a describe request
func (s *Session) sendDescribeResponse() error {
	// Use configured service info or fallback to defaults
	info := s.serviceInfo
	if info == nil {
		// Fallback to minimal default info if not configured
		info = &ServiceInfo{
			Name:        "wyoming-gateway",
			Version:     "1.0.0",
			Description: "Wyoming Protocol Gateway",
		}
		info.Attribution.Name = "Unknown"
		info.Attribution.URL = "https://github.com"
		info.Model.Name = "default-model"
		info.Model.Description = "Default Model"
		info.Model.Languages = []string{"en"}
		info.Model.Version = "1.0"
		info.Model.Attribution.Name = "Unknown"
		info.Model.Attribution.URL = "https://github.com"
	}

	// Describe our capabilities following Wyoming protocol specification
	// IMPORTANT: Wyoming protocol expects AsrProgram structure with "models" array
	// Each AsrProgram represents a service (like a Wyoming server)
	// Each AsrModel represents a model that service can use
	describeEvent := DescribeResponseEvent(map[string]any{
		"asr": []map[string]any{
			{
				// AsrProgram fields
				"name":        info.Name,
				"description": info.Description,
				"attribution": map[string]string{
					"name": info.Attribution.Name,
					"url":  info.Attribution.URL,
				},
				"installed": true,
				"version":   info.Version,
				"models": []map[string]any{
					{
						// AsrModel fields
						"name":        info.Model.Name,
						"description": info.Model.Description,
						"attribution": map[string]string{
							"name": info.Model.Attribution.Name,
							"url":  info.Model.Attribution.URL,
						},
						"installed": true,
						"languages": info.Model.Languages,
						"version":   info.Model.Version,
					},
				},
				"supports_transcript_streaming": false,
			},
		},
		"tts":    []map[string]any{},
		"wake":   []map[string]any{},
		"intent": []map[string]any{},
		"handle": []map[string]any{},
	})

	if err := s.codec.WriteEvent(&describeEvent); err != nil {
		return fmt.Errorf("write describe response: %w", err)
	}

	return nil
}

// readLoop continuously reads events from the device
func (s *Session) readLoop(timeout time.Duration) {
	defer s.wg.Done()
	defer s.Close()

	s.logger.Debug("read loop started", zap.String("session_id", s.ID))

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Debug("read loop stopping (context done)", zap.String("session_id", s.ID))
			return
		default:
			// Set read timeout if specified
			if timeout > 0 {
				s.conn.SetReadDeadline(time.Now().Add(timeout))
			}

			// Read event
			event, err := s.codec.ReadEvent()
			if err != nil {
				if s.ctx.Err() != nil {
					// Context cancelled, normal shutdown
					return
				}

				// Check if this is a graceful disconnect (connection reset, EOF, etc.)
				if isGracefulDisconnect(err) {
					s.logger.Debug("client disconnected gracefully",
						zap.String("session_id", s.ID),
						zap.String("reason", err.Error()))
					return
				}

				// Unexpected error - log at error level
				s.logger.Error("failed to read event",
					zap.String("session_id", s.ID),
					zap.Error(err))
				return
			}

			// Handle describe requests by sending our capabilities
			if event.Type == EventDescribe {
				if err := s.sendDescribeResponse(); err != nil {
					s.logger.Error("failed to send describe response",
						zap.String("session_id", s.ID),
						zap.Error(err))
				}
				// Don't forward describe events to the application
				continue
			}

			// Handle audio events
			if event.Type == EventAudioChunk {
				// Update audio format if needed
				if format, err := event.GetAudioFormat(); err == nil {
					s.setAudioFormat(&format)
				}

				// Send audio to AudioIn channel (non-blocking)
				select {
				case s.AudioIn <- event.Payload:
				case <-s.ctx.Done():
					return
				default:
					s.logger.Warn("audio input buffer full, dropping chunk",
						zap.String("session_id", s.ID))
				}
			}

			// Track audio format from audio-start
			if event.Type == EventAudioStart {
				if format, err := event.GetAudioFormat(); err == nil {
					s.setAudioFormat(&format)
					s.logger.Info("audio stream started",
						zap.String("session_id", s.ID),
						zap.Int("rate", format.Rate),
						zap.Int("channels", format.Channels))
				}
			}

			// Send non-audio events to Events channel
			// (audio-chunk events are already handled via AudioIn channel)
			if event.Type != EventAudioChunk {
				select {
				case s.Events <- event:
				case <-s.ctx.Done():
					return
				default:
					s.logger.Warn("event buffer full, dropping event",
						zap.String("session_id", s.ID),
						zap.String("event_type", string(event.Type)))
				}
			}
		}
	}
}

// writeLoop continuously writes audio to the device
func (s *Session) writeLoop(timeout time.Duration) {
	defer s.wg.Done()
	defer s.Close()

	s.logger.Debug("write loop started", zap.String("session_id", s.ID))

	// Send audio-start when we have audio format
	var startSent bool

	for {
		select {
		case <-s.ctx.Done():
			// Send audio-stop before closing
			if startSent {
				stopEvent := AudioStopEvent(nil)
				if err := s.codec.WriteEvent(&stopEvent); err != nil {
					s.logger.Error("failed to write audio-stop",
						zap.String("session_id", s.ID),
						zap.Error(err))
				}
			}
			s.logger.Debug("write loop stopping", zap.String("session_id", s.ID))
			return

		case audioData := <-s.AudioOut:
			// Get output audio format (for audio going TO the device)
			// If no output format is set, fall back to input format
			format := s.getOutputAudioFormat()
			if format == nil {
				format = s.getAudioFormat()
			}
			if format == nil {
				s.logger.Warn("no audio format set, dropping audio",
					zap.String("session_id", s.ID))
				continue
			}

			// Send audio-start if not sent yet
			if !startSent {
				s.logger.Info("sending audio-start event",
					zap.String("session_id", s.ID),
					zap.Int("rate", format.Rate),
					zap.Int("width", format.Width),
					zap.Int("channels", format.Channels))
				startEvent := AudioStartEvent(*format, nil)
				if err := s.codec.WriteEvent(&startEvent); err != nil {
					s.logger.Error("failed to write audio-start",
						zap.String("session_id", s.ID),
						zap.Error(err))
					return
				}
				startSent = true
				s.logger.Info("audio output started",
					zap.String("session_id", s.ID))
			}

			// Set write timeout if specified
			if timeout > 0 {
				s.conn.SetWriteDeadline(time.Now().Add(timeout))
			}

			// Split large audio chunks to fit Wyoming's max payload size (8KB)
			// Gemini sends 9.6KB chunks at 24kHz, we need to split them
			maxChunkSize := MaxPayloadSize // 8192 bytes
			for offset := 0; offset < len(audioData); offset += maxChunkSize {
				end := offset + maxChunkSize
				if end > len(audioData) {
					end = len(audioData)
				}
				chunk := audioData[offset:end]

				// Send audio chunk
				chunkEvent := AudioChunkEvent(*format, chunk, nil)
				if err := s.codec.WriteEvent(&chunkEvent); err != nil {
					s.logger.Error("failed to write audio chunk",
						zap.String("session_id", s.ID),
						zap.Int("offset", offset),
						zap.Int("chunk_size", len(chunk)),
						zap.Error(err))
					return
				}
			}
		}
	}
}

// SendEvent sends a non-audio event to the device
func (s *Session) SendEvent(event *Event) error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()

	if s.closed {
		return fmt.Errorf("session closed")
	}

	return s.codec.WriteEvent(event)
}

// GetAudioFormat returns the current audio format (thread-safe)
func (s *Session) GetAudioFormat() *AudioFormat {
	return s.getAudioFormat()
}

func (s *Session) getAudioFormat() *AudioFormat {
	s.formatMu.RLock()
	defer s.formatMu.RUnlock()
	return s.audioFormat
}

func (s *Session) setAudioFormat(format *AudioFormat) {
	s.formatMu.Lock()
	defer s.formatMu.Unlock()
	s.audioFormat = format
}

// getOutputAudioFormat returns the output audio format (thread-safe)
func (s *Session) getOutputAudioFormat() *AudioFormat {
	s.formatMu.RLock()
	defer s.formatMu.RUnlock()
	return s.outputAudioFormat
}

// SetOutputAudioFormat sets the audio format for OUTPUT (gateway → device)
// This is used when the backend's output format differs from the input format
func (s *Session) SetOutputAudioFormat(format *AudioFormat) {
	s.formatMu.Lock()
	defer s.formatMu.Unlock()
	s.outputAudioFormat = format
	s.logger.Info("output audio format set",
		zap.String("session_id", s.ID),
		zap.Int("rate", format.Rate),
		zap.Int("width", format.Width),
		zap.Int("channels", format.Channels))
}

// Close closes the session and cleans up resources
func (s *Session) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	s.logger.Info("closing session", zap.String("session_id", s.ID))

	// Cancel context to stop goroutines
	s.cancel()

	// Close connection
	if err := s.conn.Close(); err != nil {
		s.logger.Warn("error closing connection",
			zap.String("session_id", s.ID),
			zap.Error(err))
	}

	// Close channels (after goroutines stop)
	go func() {
		s.wg.Wait()
		close(s.AudioIn)
		close(s.AudioOut)
		close(s.Events)
	}()

	return nil
}

// isGracefulDisconnect checks if an error represents a graceful client disconnect
// rather than an unexpected error. Uses proper error type checking instead of
// string matching for reliability across platforms and Go versions.
func isGracefulDisconnect(err error) bool {
	if err == nil {
		return false
	}

	// Check for io.EOF - standard end of stream
	if errors.Is(err, io.EOF) {
		return true
	}

	// Check for net.ErrClosed - connection was already closed
	if errors.Is(err, net.ErrClosed) {
		return true
	}

	// Unwrap net.OpError to check the underlying syscall error
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		// Check for common graceful disconnect syscall errors
		var errno syscall.Errno
		if errors.As(opErr.Err, &errno) {
			switch errno {
			case syscall.ECONNRESET: // Connection reset by peer
				return true
			case syscall.EPIPE: // Broken pipe
				return true
			case syscall.ECONNREFUSED: // Connection refused
				return true
			case syscall.ENETUNREACH: // Network unreachable
				return true
			case syscall.ECONNABORTED: // Connection aborted
				return true
			}
		}

		// Recursively check the underlying error
		if opErr.Err != nil {
			return isGracefulDisconnect(opErr.Err)
		}
	}

	return false
}

// Wait waits for the session to finish
func (s *Session) Wait() {
	s.wg.Wait()
}

// Context returns the session's context
func (s *Session) Context() context.Context {
	return s.ctx
}
