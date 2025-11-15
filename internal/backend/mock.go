package backend

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockBackend implements Backend interface for testing
type MockBackend struct {
	initialized bool
	closed      bool
	mu          sync.RWMutex

	sessions   map[string]*mockSession
	sessionsMu sync.RWMutex

	tools        []Tool
	capabilities Capabilities

	// Test hooks
	OnInit          func(ctx context.Context, cfg Config) error
	OnStartSession  func(ctx context.Context, req SessionConfig) error
	OnRegisterTools func(tools []Tool) error
	OnClose         func() error
}

type mockSession struct {
	session     *Session
	audioIn     chan AudioFrame
	audioOut    chan AudioFrame
	events      chan Event
	toolResults chan ToolResult
	interrupt   chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc

	// For testing
	receivedAudio []AudioFrame
	sentAudio     []AudioFrame
	receivedTools []ToolResult
	mu            sync.RWMutex
}

// NewMockBackend creates a new mock backend
func NewMockBackend() *MockBackend {
	return &MockBackend{
		sessions: make(map[string]*mockSession),
		capabilities: Capabilities{
			SupportsStreaming:   true,
			SupportsToolCalling: true,
			SupportsBargeIn:     true,
			SupportedAudioFormats: []AudioFormat{
				{SampleRate: 16000, Channels: 1, BitsPerSample: 16, Encoding: "pcm_s16le"},
			},
			MaxAudioChunkSize: 8192,
			Provider:          "mock",
		},
	}
}

// Init initializes the mock backend
func (m *MockBackend) Init(ctx context.Context, cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If already initialized, skip re-initialization
	if m.initialized {
		return nil
	}

	if m.OnInit != nil {
		if err := m.OnInit(ctx, cfg); err != nil {
			return err
		}
	}

	m.initialized = true
	return nil
}

// StartSession starts a mock session
func (m *MockBackend) StartSession(ctx context.Context, req SessionConfig) (*Session, error) {
	if m.OnStartSession != nil {
		if err := m.OnStartSession(ctx, req); err != nil {
			return nil, err
		}
	}

	// Create channels
	audioIn := make(chan AudioFrame, 100)
	audioOut := make(chan AudioFrame, 100)
	events := make(chan Event, 50)
	toolResults := make(chan ToolResult, 10)
	interrupt := make(chan struct{}, 1)

	sessionCtx, cancel := context.WithCancel(ctx)

	mock := &mockSession{
		audioIn:       audioIn,
		audioOut:      audioOut,
		events:        events,
		toolResults:   toolResults,
		interrupt:     interrupt,
		ctx:           sessionCtx,
		cancel:        cancel,
		receivedAudio: []AudioFrame{},
		sentAudio:     []AudioFrame{},
		receivedTools: []ToolResult{},
	}

	session := &Session{
		ID: req.SessionID,
		Metadata: SessionMetadata{
			DeviceID:  req.DeviceID,
			UserID:    req.UserID,
			CreatedAt: time.Now(),
			Model:     "mock-model",
			Backend:   "mock",
		},
		AudioIn:     audioIn,
		AudioOut:    audioOut,
		Events:      events,
		ToolResults: toolResults,
		Interrupt:   interrupt,
		Close: func() error {
			cancel()
			return nil
		},
		ctx: sessionCtx,
	}

	mock.session = session

	// Store session
	m.sessionsMu.Lock()
	m.sessions[req.SessionID] = mock
	m.sessionsMu.Unlock()

	// Start processing goroutines
	go mock.processAudioIn()
	go mock.processToolResults()

	return session, nil
}

// RegisterTools registers tools
func (m *MockBackend) RegisterTools(tools []Tool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tools = tools
	return nil
}

// Capabilities returns mock capabilities
func (m *MockBackend) Capabilities() Capabilities {
	return m.capabilities
}

// Close closes the mock backend
func (m *MockBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	if m.OnClose != nil {
		if err := m.OnClose(); err != nil {
			return err
		}
	}

	// Close all sessions
	m.sessionsMu.Lock()
	for _, sess := range m.sessions {
		sess.cancel()
	}
	m.sessions = make(map[string]*mockSession)
	m.sessionsMu.Unlock()

	m.closed = true
	return nil
}

// GetSession returns a mock session (for testing)
func (m *MockBackend) GetSession(sessionID string) (*mockSession, bool) {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	sess, ok := m.sessions[sessionID]
	return sess, ok
}

// RangeSessions iterates over all sessions (for testing)
func (m *MockBackend) RangeSessions(fn func(id string, session *Session) bool) {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	for id, mock := range m.sessions {
		if !fn(id, mock.session) {
			return
		}
	}
}

// processAudioIn processes incoming audio
func (s *mockSession) processAudioIn() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case frame := <-s.audioIn:
			s.mu.Lock()
			s.receivedAudio = append(s.receivedAudio, frame)
			s.mu.Unlock()

			// Echo audio back (for testing)
			select {
			case s.audioOut <- frame:
			case <-s.ctx.Done():
				return
			}
		}
	}
}

// processToolResults processes tool results
func (s *mockSession) processToolResults() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case result := <-s.toolResults:
			s.mu.Lock()
			s.receivedTools = append(s.receivedTools, result)
			s.mu.Unlock()
		}
	}
}

// SimulateTranscript simulates a transcript event (for testing)
func (s *mockSession) SimulateTranscript(text string, isFinal bool) error {
	event := NewTranscriptDeltaEvent(s.session.ID, TranscriptDelta{
		Text:       text,
		IsFinal:    isFinal,
		Confidence: 0.95,
	})

	select {
	case s.events <- event:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// SimulateToolCall simulates a tool call event (for testing)
func (s *mockSession) SimulateToolCall(name string, args map[string]interface{}) error {
	toolCall := ToolCall{
		ID:        fmt.Sprintf("call_%d", time.Now().UnixNano()),
		Name:      name,
		Arguments: args,
		Timestamp: time.Now(),
	}

	event := NewToolCallEvent(s.session.ID, toolCall)

	select {
	case s.events <- event:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// SimulateAudio simulates audio output (for testing)
func (s *mockSession) SimulateAudio(data []byte) error {
	frame := AudioFrame{
		Data:      data,
		Timestamp: time.Now(),
	}

	s.mu.Lock()
	s.sentAudio = append(s.sentAudio, frame)
	s.mu.Unlock()

	select {
	case s.audioOut <- frame:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// GetReceivedAudio returns all received audio frames (for testing)
func (s *mockSession) GetReceivedAudio() []AudioFrame {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return append([]AudioFrame{}, s.receivedAudio...)
}

// GetReceivedTools returns all received tool results (for testing)
func (s *mockSession) GetReceivedTools() []ToolResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return append([]ToolResult{}, s.receivedTools...)
}

// GetSentAudio returns all sent audio frames (for testing)
func (s *mockSession) GetSentAudio() []AudioFrame {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return append([]AudioFrame{}, s.sentAudio...)
}
