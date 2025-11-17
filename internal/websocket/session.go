package websocket

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Session represents a WebSocket connection to an ESP32 device
type Session struct {
	ID     string
	conn   *websocket.Conn
	logger *zap.Logger

	// Channels for bidirectional data flow
	AudioIn  chan []byte      // Audio from device → backend
	AudioOut chan []byte      // Audio from backend → device
	StateOut chan DeviceState // State updates to device

	// Context for lifecycle management
	ctx    context.Context
	cancel context.CancelFunc

	// Synchronization
	wg     sync.WaitGroup
	closed bool
	mu     sync.Mutex

	// Configuration
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// SessionConfig holds configuration for a WebSocket session
type SessionConfig struct {
	Conn         *websocket.Conn
	Logger       *zap.Logger
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// NewSession creates a new WebSocket session
func NewSession(cfg SessionConfig) *Session {
	ctx, cancel := context.WithCancel(context.Background())

	sessionID := uuid.New().String()
	logger := cfg.Logger.With(zap.String("session_id", sessionID))

	s := &Session{
		ID:           sessionID,
		conn:         cfg.Conn,
		logger:       logger,
		AudioIn:      make(chan []byte, 100),
		AudioOut:     make(chan []byte, 100),
		StateOut:     make(chan DeviceState, 10),
		ctx:          ctx,
		cancel:       cancel,
		readTimeout:  cfg.ReadTimeout,
		writeTimeout: cfg.WriteTimeout,
	}

	// Start read/write loops
	s.wg.Add(2)
	go s.readLoop()
	go s.writeLoop()

	logger.Info("websocket session created")
	return s
}

// readLoop continuously reads messages from the device
func (s *Session) readLoop() {
	defer s.wg.Done()
	defer s.Close()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Debug("read loop: context canceled")
			return
		default:
		}

		// Set read deadline
		if s.readTimeout > 0 {
			s.conn.SetReadDeadline(time.Now().Add(s.readTimeout))
		}

		messageType, data, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.logger.Error("websocket read error", zap.Error(err))
			} else {
				s.logger.Debug("websocket closed", zap.Error(err))
			}
			return
		}

		// Handle binary audio frames
		if messageType == websocket.BinaryMessage {
			select {
			case s.AudioIn <- data:
				// Audio frame queued
			case <-s.ctx.Done():
				return
			default:
				s.logger.Warn("audio input buffer full, dropping frame",
					zap.Int("bytes", len(data)))
			}
		} else if messageType == websocket.TextMessage {
			// Device shouldn't send text messages in this protocol
			s.logger.Warn("unexpected text message from device",
				zap.String("message", string(data)))
		}
	}
}

// writeLoop continuously writes messages to the device
func (s *Session) writeLoop() {
	defer s.wg.Done()
	defer s.Close()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Debug("write loop: context canceled")
			return

		case state := <-s.StateOut:
			// Send state update
			if err := s.writeJSON(NewStateMessage(state)); err != nil {
				s.logger.Error("failed to send state message",
					zap.String("state", string(state)),
					zap.Error(err))
				return
			}
			s.logger.Debug("sent state message", zap.String("state", string(state)))

		case audioData := <-s.AudioOut:
			// Send binary audio
			if err := s.writeBinary(audioData); err != nil {
				s.logger.Error("failed to send audio",
					zap.Int("bytes", len(audioData)),
					zap.Error(err))
				return
			}
		}
	}
}

// writeJSON writes a JSON message to the device
func (s *Session) writeJSON(data []byte) error {
	if s.writeTimeout > 0 {
		s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

// writeBinary writes binary audio data to the device
func (s *Session) writeBinary(data []byte) error {
	if s.writeTimeout > 0 {
		s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, data)
}

// SendState sends a state update to the device
func (s *Session) SendState(state DeviceState) error {
	select {
	case s.StateOut <- state:
		return nil
	case <-s.ctx.Done():
		return fmt.Errorf("session closed")
	}
}

// SendError sends an error message to the device
func (s *Session) SendError(errMsg string) error {
	s.logger.Warn("sending error to device", zap.String("error", errMsg))
	return s.writeJSON(NewErrorMessage(errMsg))
}

// Close closes the WebSocket session
func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	s.logger.Info("closing websocket session")

	// Cancel context
	s.cancel()

	// Close channels
	close(s.AudioIn)
	close(s.AudioOut)
	close(s.StateOut)

	// Close WebSocket connection
	if err := s.conn.Close(); err != nil {
		s.logger.Warn("error closing websocket connection", zap.Error(err))
	}

	s.logger.Info("websocket session closed")
	return nil
}

// Wait waits for the session to complete
func (s *Session) Wait() {
	s.wg.Wait()
}

// Context returns the session context
func (s *Session) Context() context.Context {
	return s.ctx
}
