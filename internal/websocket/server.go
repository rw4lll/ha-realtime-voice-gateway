package websocket

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Server is an HTTP server that handles WebSocket upgrades
type Server struct {
	addr     string
	path     string
	logger   *zap.Logger
	upgrader websocket.Upgrader

	// Session management
	sessions  sync.Map // sessionID -> *Session
	onSession func(*Session)

	// HTTP server
	httpServer *http.Server
	ctx        context.Context
	cancel     context.CancelFunc

	// Configuration
	config ServerConfig
}

// ServerConfig holds WebSocket server configuration
type ServerConfig struct {
	Address       string        // Listen address (e.g., "0.0.0.0:8080")
	Path          string        // WebSocket path (e.g., "/voice-stream")
	ReadTimeout   time.Duration // WebSocket read timeout
	WriteTimeout  time.Duration // WebSocket write timeout
	MaxBufferSize int           // Max message size in bytes
	Logger        *zap.Logger
	OnSession     func(*Session) // Callback for new sessions
}

// NewServer creates a new WebSocket server
func NewServer(cfg ServerConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	if cfg.Path == "" {
		cfg.Path = "/voice-stream"
	}

	if cfg.MaxBufferSize == 0 {
		cfg.MaxBufferSize = 32768 // 32KB default
	}

	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 60 * time.Second
	}

	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 10 * time.Second
	}

	s := &Server{
		addr:      cfg.Address,
		path:      cfg.Path,
		logger:    cfg.Logger,
		onSession: cfg.OnSession,
		ctx:       ctx,
		cancel:    cancel,
		config:    cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  cfg.MaxBufferSize,
			WriteBufferSize: cfg.MaxBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				// Accept all origins for now
				// TODO: Add origin validation for production
				return true
			},
		},
	}

	return s
}

// Start starts the WebSocket server
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.path, s.handleWebSocket)

	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	s.logger.Info("starting websocket server",
		zap.String("address", s.addr),
		zap.String("path", s.path),
		zap.Int("max_buffer_size", s.config.MaxBufferSize))

	// Start server in background
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("websocket server error", zap.Error(err))
		}
	}()

	return nil
}

// handleWebSocket handles WebSocket upgrade and creates a session
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("new websocket connection",
		zap.String("remote_addr", r.RemoteAddr),
		zap.String("user_agent", r.UserAgent()))

	// Upgrade HTTP connection to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("failed to upgrade connection", zap.Error(err))
		return
	}

	// Create session
	session := NewSession(SessionConfig{
		Conn:         conn,
		Logger:       s.logger,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	})

	// Store session
	s.sessions.Store(session.ID, session)

	s.logger.Info("websocket session created",
		zap.String("session_id", session.ID),
		zap.String("remote_addr", r.RemoteAddr))

	// Call session callback
	if s.onSession != nil {
		go s.onSession(session)
	}

	// Monitor session and clean up when done
	go func() {
		session.Wait()
		s.sessions.Delete(session.ID)
		s.logger.Info("websocket session ended",
			zap.String("session_id", session.ID))
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
	s.logger.Info("shutting down websocket server")

	// Cancel context
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

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("error shutting down http server", zap.Error(err))
		return err
	}

	s.logger.Info("websocket server shut down")
	return nil
}

// Addr returns the server's listen address
func (s *Server) Addr() string {
	return s.addr
}

// Context returns the server's context
func (s *Server) Context() context.Context {
	return s.ctx
}
