package wyoming

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Server represents a Wyoming protocol TCP server
type Server struct {
	addr     string
	listener net.Listener
	logger   *zap.Logger

	// Session management
	sessions  sync.Map       // map[string]*Session
	onSession func(*Session) // Callback for new sessions

	// Configuration
	config ServerConfig

	// Server state
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ServerConfig holds configuration for the Wyoming server
type ServerConfig struct {
	// Network configuration
	Address string // e.g., "0.0.0.0:10200"

	// Session configuration
	AudioBufferSize int
	EventBufferSize int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration

	// Logging
	Logger *zap.Logger

	// Callbacks
	OnSession func(*Session) // Called when a new session is created
}

// NewServer creates a new Wyoming TCP server
func NewServer(cfg ServerConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	if cfg.AudioBufferSize == 0 {
		cfg.AudioBufferSize = 100
	}

	if cfg.EventBufferSize == 0 {
		cfg.EventBufferSize = 50
	}

	return &Server{
		addr:      cfg.Address,
		logger:    cfg.Logger,
		onSession: cfg.OnSession,
		config:    cfg,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start starts the Wyoming server
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}

	s.listener = listener
	s.logger.Info("Wyoming server started", zap.String("address", s.addr))

	// Start accept loop
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// acceptLoop accepts incoming connections and creates sessions
func (s *Server) acceptLoop() {
	defer s.wg.Done()
	defer s.listener.Close()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("accept loop stopping")
			return
		default:
			// Set accept timeout to allow periodic context checks
			// Use longer 5-second timeout to reduce CPU polling when idle
			// This is a balance between responsiveness and resource usage
			s.listener.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))

			conn, err := s.listener.Accept()
			if err != nil {
				// Check if it's a timeout (expected during context checks)
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}

				// Check if server is shutting down
				select {
				case <-s.ctx.Done():
					return
				default:
					s.logger.Error("accept connection", zap.Error(err))
					continue
				}
			}

			// Handle new connection
			s.logger.Info("new connection", zap.String("remote_addr", conn.RemoteAddr().String()))
			s.handleConnection(conn)
		}
	}
}

// handleConnection creates a session for a new connection
func (s *Server) handleConnection(conn net.Conn) {
	sessionID := uuid.New().String()

	session := NewSession(SessionConfig{
		ID:              sessionID,
		Conn:            conn,
		Logger:          s.logger.With(zap.String("session_id", sessionID)),
		AudioBufferSize: s.config.AudioBufferSize,
		EventBufferSize: s.config.EventBufferSize,
		ReadTimeout:     s.config.ReadTimeout,
		WriteTimeout:    s.config.WriteTimeout,
	})

	// Store session
	s.sessions.Store(sessionID, session)

	s.logger.Info("session created",
		zap.String("session_id", sessionID),
		zap.String("remote_addr", conn.RemoteAddr().String()))

	// Call session callback
	if s.onSession != nil {
		go s.onSession(session)
	}

	// Monitor session and clean up when done
	go func() {
		session.Wait()
		s.sessions.Delete(sessionID)
		s.logger.Info("session ended", zap.String("session_id", sessionID))
	}()
}

// GetSession returns a session by ID
func (s *Server) GetSession(sessionID string) (*Session, bool) {
	val, ok := s.sessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	return val.(*Session), true
}

// GetAllSessions returns all active sessions
func (s *Server) GetAllSessions() []*Session {
	sessions := []*Session{}
	s.sessions.Range(func(key, value interface{}) bool {
		sessions = append(sessions, value.(*Session))
		return true
	})
	return sessions
}

// SessionCount returns the number of active sessions
func (s *Server) SessionCount() int {
	count := 0
	s.sessions.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// Close closes the server and all sessions
func (s *Server) Close() error {
	s.logger.Info("shutting down Wyoming server")

	// Cancel context to stop accept loop
	s.cancel()

	// Close all active sessions
	s.sessions.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		if err := session.Close(); err != nil {
			s.logger.Warn("error closing session",
				zap.String("session_id", session.ID),
				zap.Error(err))
		}
		return true
	})

	// Wait for accept loop to finish
	s.wg.Wait()

	s.logger.Info("Wyoming server shut down")
	return nil
}

// Addr returns the server's listen address
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

// Context returns the server's context
func (s *Server) Context() context.Context {
	return s.ctx
}
