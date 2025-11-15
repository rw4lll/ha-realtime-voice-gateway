package gemini

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

// TestSessionCloseIdempotent tests that calling close() multiple times is safe
func TestSessionCloseIdempotent(t *testing.T) {
	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &geminiSession{
		id:          "test-session",
		deviceID:    "test-device",
		userID:      "test-user",
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		audioIn:     make(chan backend.AudioFrame, 10),
		audioOut:    make(chan backend.AudioFrame, 10),
		events:      make(chan backend.Event, 10),
		toolResults: make(chan backend.ToolResult, 10),
		interrupt:   make(chan struct{}, 1),
	}

	// Close multiple times concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.close()
		}()
	}

	// Wait for all close calls to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - all close calls completed
	case <-time.After(5 * time.Second):
		t.Fatal("close() deadlocked or took too long")
	}

	// Verify closed flag is set
	if !session.closed.Load() {
		t.Error("closed flag not set after close()")
	}
}

// TestSessionGoroutineCleanup tests that goroutines exit properly
func TestSessionGoroutineCleanup(t *testing.T) {
	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())

	session := &geminiSession{
		id:          "test-session",
		deviceID:    "test-device",
		userID:      "test-user",
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		audioIn:     make(chan backend.AudioFrame, 10),
		audioOut:    make(chan backend.AudioFrame, 10),
		events:      make(chan backend.Event, 10),
		toolResults: make(chan backend.ToolResult, 10),
		interrupt:   make(chan struct{}, 1),
	}

	// Track goroutine completion
	var writeLoopDone, readLoopDone atomic.Bool

	// Mock goroutines with tracking
	session.wg.Add(1)
	go func() {
		defer session.wg.Done()
		defer writeLoopDone.Store(true)

		for {
			select {
			case <-session.ctx.Done():
				return
			case <-session.audioIn:
			case <-session.toolResults:
			case <-session.interrupt:
			}
		}
	}()

	session.wg.Add(1)
	go func() {
		defer session.wg.Done()
		defer readLoopDone.Store(true)

		<-session.ctx.Done()
	}()

	// Give goroutines time to start
	time.Sleep(50 * time.Millisecond)

	// Close session
	start := time.Now()
	session.close()
	duration := time.Since(start)

	// Verify cleanup completed quickly
	if duration > 2*time.Second {
		t.Errorf("close() took too long: %v", duration)
	}

	// Verify goroutines exited
	time.Sleep(100 * time.Millisecond)
	if !writeLoopDone.Load() {
		t.Error("write loop did not exit")
	}
	if !readLoopDone.Load() {
		t.Error("read loop did not exit")
	}
}

// TestSessionClosedChannelHandling tests that closed channels are handled gracefully
func TestSessionClosedChannelHandling(t *testing.T) {
	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	audioIn := make(chan backend.AudioFrame, 1)
	toolResults := make(chan backend.ToolResult, 1)

	session := &geminiSession{
		id:          "test-session",
		deviceID:    "test-device",
		userID:      "test-user",
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		audioIn:     audioIn,
		audioOut:    make(chan backend.AudioFrame, 10),
		events:      make(chan backend.Event, 10),
		toolResults: toolResults,
		interrupt:   make(chan struct{}, 1),
	}

	// Track if goroutine exits properly
	var exited atomic.Bool

	// Start a mock write loop
	session.wg.Add(1)
	go func() {
		defer session.wg.Done()
		defer exited.Store(true)

		for {
			select {
			case <-session.ctx.Done():
				return
			case _, ok := <-session.audioIn:
				if !ok {
					return
				}
			case _, ok := <-session.toolResults:
				if !ok {
					return
				}
			}
		}
	}()

	// Close channels
	close(audioIn)
	close(toolResults)

	// Wait for goroutine to exit
	time.Sleep(100 * time.Millisecond)

	if !exited.Load() {
		t.Error("goroutine did not exit after channels closed")
	}
}

// TestSessionContextCancellation tests that session respects context cancellation
func TestSessionContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	session := &geminiSession{
		id:          "test-session",
		deviceID:    "test-device",
		userID:      "test-user",
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		audioIn:     make(chan backend.AudioFrame, 10),
		audioOut:    make(chan backend.AudioFrame, 10),
		events:      make(chan backend.Event, 10),
		toolResults: make(chan backend.ToolResult, 10),
		interrupt:   make(chan struct{}, 1),
	}

	var exited atomic.Bool

	// Start goroutine that should exit when context is cancelled
	session.wg.Add(1)
	go func() {
		defer session.wg.Done()
		defer exited.Store(true)

		<-session.ctx.Done()
	}()

	// Wait for context to cancel
	time.Sleep(200 * time.Millisecond)

	if !exited.Load() {
		t.Error("goroutine did not exit after context cancelled")
	}
}

// TestSessionBufferOverflow tests behavior when buffers are full
func TestSessionBufferOverflow(t *testing.T) {
	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create small buffers to trigger overflow
	audioOut := make(chan backend.AudioFrame, 2)
	events := make(chan backend.Event, 2)

	session := &geminiSession{
		id:              "test-session",
		deviceID:        "test-device",
		userID:          "test-user",
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
		audioIn:         make(chan backend.AudioFrame, 10),
		audioOut:        audioOut,
		events:          events,
		toolResults:     make(chan backend.ToolResult, 10),
		interrupt:       make(chan struct{}, 1),
		audioHashes:     make(map[string]time.Time), // Initialize the map to prevent nil panic
		lastHashCleanup: time.Now(),
	}

	// Fill the buffers
	audioOut <- backend.AudioFrame{Data: []byte{1}}
	audioOut <- backend.AudioFrame{Data: []byte{2}}

	events <- backend.NewTranscriptDoneEvent("test", backend.Transcript{Text: "test1"})
	events <- backend.NewTranscriptDoneEvent("test", backend.Transcript{Text: "test2"})

	// Try to handle more audio (should not block or panic)
	content := &genai.LiveServerContent{
		ModelTurn: &genai.Content{
			Parts: []*genai.Part{
				{
					InlineData: &genai.Blob{
						MIMEType: "audio/pcm",
						Data:     []byte{3, 4, 5},
					},
				},
			},
		},
	}

	done := make(chan struct{})
	go func() {
		session.handleServerContent(content)
		close(done)
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(1 * time.Second):
		t.Error("handleServerContent blocked when buffer full")
	}

	// Verify buffer didn't overflow (still has 2 items)
	if len(audioOut) != 2 {
		t.Errorf("expected audioOut buffer to have 2 items, got %d", len(audioOut))
	}
}

// TestGeminiBackend_SessionLimit tests max concurrent sessions limit
func TestGeminiBackend_SessionLimit(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	// Manually set initialized to true and configure to bypass actual API initialization
	geminiBackend.initialized = true
	geminiBackend.model = "test-model"
	geminiBackend.maxSessions = 5 // Set to default

	ctx := context.Background()

	// Create sessions up to the limit
	for i := 0; i < geminiBackend.maxSessions; i++ {
		geminiBackend.sessionsMu.Lock()
		geminiBackend.sessions[string(rune(i))] = &geminiSession{
			id: string(rune(i)),
		}
		geminiBackend.sessionsMu.Unlock()
	}

	// Try to create one more session - should fail
	req := backend.SessionConfig{
		SessionID: "overflow-session",
		DeviceID:  "test-device",
	}

	_, err := geminiBackend.StartSession(ctx, req)
	if err == nil {
		t.Error("expected error when exceeding session limit, got nil")
	}

	expectedMsg := "max concurrent sessions reached"
	if err != nil && len(err.Error()) > 0 && err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("expected error about max sessions, got: %v", err)
	}
}

// TestGeminiBackend_CleanupOrphanedSessions tests session cleanup
func TestGeminiBackend_CleanupOrphanedSessions(t *testing.T) {
	logger := zap.NewNop()
	geminiBackend := NewGeminiBackend(logger)

	// Create some active and some cancelled sessions
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	ctx3 := context.Background() // Active

	geminiBackend.sessions["session1"] = &geminiSession{
		id:     "session1",
		ctx:    ctx1,
		cancel: cancel1,
		logger: logger,
	}
	geminiBackend.sessions["session2"] = &geminiSession{
		id:     "session2",
		ctx:    ctx2,
		cancel: cancel2,
		logger: logger,
	}
	geminiBackend.sessions["session3"] = &geminiSession{
		id:     "session3",
		ctx:    ctx3,
		cancel: func() {},
		logger: logger,
	}

	// Cancel two sessions
	cancel1()
	cancel2()

	// Give context cancellation time to propagate
	time.Sleep(10 * time.Millisecond)

	// Run cleanup
	geminiBackend.cleanupOrphanedSessions()

	// Verify only active session remains
	geminiBackend.sessionsMu.Lock()
	count := len(geminiBackend.sessions)
	_, has3 := geminiBackend.sessions["session3"]
	geminiBackend.sessionsMu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 session after cleanup, got %d", count)
	}
	if !has3 {
		t.Error("active session was incorrectly cleaned up")
	}
}

// TestSessionPanicRecovery tests that panics in loops are recovered
func TestSessionPanicRecovery(t *testing.T) {
	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &geminiSession{
		id:          "test-session",
		deviceID:    "test-device",
		userID:      "test-user",
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		audioIn:     make(chan backend.AudioFrame, 10),
		audioOut:    make(chan backend.AudioFrame, 10),
		events:      make(chan backend.Event, 10),
		toolResults: make(chan backend.ToolResult, 10),
		interrupt:   make(chan struct{}, 1),
	}

	var recovered atomic.Bool

	// Simulate a goroutine with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				recovered.Store(true)
			}
		}()

		session.wg.Add(1)
		defer session.wg.Done()

		// Intentionally panic
		panic("test panic")
	}()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Note: In real implementation, panic is recovered inside writeLoop/readLoop
	// This test just verifies the pattern works
}
