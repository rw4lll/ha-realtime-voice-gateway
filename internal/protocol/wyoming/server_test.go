package wyoming

import (
	"bytes"
	"net"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestServer_StartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:0", // Random port
		Logger:  logger,
	})
	
	// Start server
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()
	
	// Verify server is running
	addr := server.Addr()
	if addr == "" {
		t.Fatal("Server address is empty")
	}
	
	t.Logf("Server running on %s", addr)
	
	// Try to connect
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	conn.Close()
	
	// Stop server
	err = server.Close()
	if err != nil {
		t.Fatalf("Failed to close server: %v", err)
	}
}

func TestServer_SessionCreation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	sessionCreated := make(chan *Session, 1)
	
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:0",
		Logger:  logger,
		OnSession: func(s *Session) {
			sessionCreated <- s
		},
	})
	
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()
	
	// Connect to server
	conn, err := net.Dial("tcp", server.Addr())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	
	// Wait for session creation
	select {
	case session := <-sessionCreated:
		t.Logf("Session created: %s", session.ID)
		
		// Verify session exists in server
		retrieved, ok := server.GetSession(session.ID)
		if !ok {
			t.Fatal("Session not found in server")
		}
		
		if retrieved.ID != session.ID {
			t.Errorf("Session ID mismatch: expected %s, got %s", session.ID, retrieved.ID)
		}
		
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for session creation")
	}
	
	// Check session count
	if count := server.SessionCount(); count != 1 {
		t.Errorf("Expected 1 session, got %d", count)
	}
}

func TestServer_MultipleClients(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:0",
		Logger:  logger,
	})
	
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()
	
	// Connect multiple clients
	numClients := 5
	connections := []net.Conn{}
	
	for i := 0; i < numClients; i++ {
		conn, err := net.Dial("tcp", server.Addr())
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		connections = append(connections, conn)
		defer conn.Close()
	}
	
	// Wait for sessions to be created
	time.Sleep(100 * time.Millisecond)
	
	// Check session count
	if count := server.SessionCount(); count != numClients {
		t.Errorf("Expected %d sessions, got %d", numClients, count)
	}
	
	// Get all sessions
	sessions := server.GetAllSessions()
	if len(sessions) != numClients {
		t.Errorf("Expected %d sessions from GetAllSessions, got %d", numClients, len(sessions))
	}
}

func TestSession_AudioStreamingToDevice(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	sessionCreated := make(chan *Session, 1)
	
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:0",
		Logger:  logger,
		OnSession: func(s *Session) {
			sessionCreated <- s
		},
	})
	
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()
	
	// Connect to server
	conn, err := net.Dial("tcp", server.Addr())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	
	codec := NewCodec(conn)
	
	// Wait for session
	var session *Session
	select {
	case session = <-sessionCreated:
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for session")
	}
	
	// Set audio format first by sending audio-start from device
	format := AudioFormat{Rate: 16000, Width: 2, Channels: 1}
	startEvent := AudioStartEvent(format, nil)
	if err := codec.WriteEvent(&startEvent); err != nil {
		t.Fatalf("Failed to send audio-start: %v", err)
	}
	
	// Give session time to process the format
	time.Sleep(50 * time.Millisecond)
	
	// Send audio data to session
	testAudio := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	session.AudioOut <- testAudio
	
	// Read audio-start from device
	event, err := codec.ReadEvent()
	if err != nil {
		t.Fatalf("Failed to read audio-start: %v", err)
	}
	
	if event.Type != EventAudioStart {
		t.Errorf("Expected audio-start, got %s", event.Type)
	}
	
	// Read audio chunk from device
	event, err = codec.ReadEvent()
	if err != nil {
		t.Fatalf("Failed to read audio chunk: %v", err)
	}
	
	if event.Type != EventAudioChunk {
		t.Errorf("Expected audio-chunk, got %s", event.Type)
	}
	
	if !bytes.Equal(event.Payload, testAudio) {
		t.Errorf("Audio data mismatch: expected %v, got %v", testAudio, event.Payload)
	}
}

func TestSession_AudioStreamingFromDevice(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	sessionCreated := make(chan *Session, 1)
	
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:0",
		Logger:  logger,
		OnSession: func(s *Session) {
			sessionCreated <- s
		},
	})
	
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()
	
	// Connect to server
	conn, err := net.Dial("tcp", server.Addr())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	
	codec := NewCodec(conn)
	
	// Wait for session
	var session *Session
	select {
	case session = <-sessionCreated:
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for session")
	}
	
	// Send audio from device
	format := AudioFormat{Rate: 16000, Width: 2, Channels: 1}
	testAudio := []byte{10, 20, 30, 40}
	
	// Send audio-start
	startEvent := AudioStartEvent(format, nil)
	if err := codec.WriteEvent(&startEvent); err != nil {
		t.Fatalf("Failed to send audio-start: %v", err)
	}
	
	// Send audio-chunk
	chunkEvent := AudioChunkEvent(format, testAudio, nil)
	if err := codec.WriteEvent(&chunkEvent); err != nil {
		t.Fatalf("Failed to send audio chunk: %v", err)
	}
	
	// Read from session's AudioIn channel
	select {
	case receivedAudio := <-session.AudioIn:
		if !bytes.Equal(receivedAudio, testAudio) {
			t.Errorf("Audio mismatch: expected %v, got %v", testAudio, receivedAudio)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for audio")
	}
}

func TestSession_BidirectionalAudio(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	sessionCreated := make(chan *Session, 1)
	
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:0",
		Logger:  logger,
		OnSession: func(s *Session) {
			sessionCreated <- s
		},
	})
	
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()
	
	// Connect to server
	conn, err := net.Dial("tcp", server.Addr())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	
	codec := NewCodec(conn)
	
	// Wait for session
	var session *Session
	select {
	case session = <-sessionCreated:
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for session")
	}
	
	format := AudioFormat{Rate: 16000, Width: 2, Channels: 1}
	
	// Device sends audio-start
	startEvent := AudioStartEvent(format, nil)
	if err := codec.WriteEvent(&startEvent); err != nil {
		t.Fatalf("Failed to send audio-start: %v", err)
	}
	
	// Device sends audio chunk
	deviceAudio := []byte{1, 2, 3, 4}
	chunkEvent := AudioChunkEvent(format, deviceAudio, nil)
	if err := codec.WriteEvent(&chunkEvent); err != nil {
		t.Fatalf("Failed to send audio chunk: %v", err)
	}
	
	// Session receives device audio
	select {
	case audio := <-session.AudioIn:
		if !bytes.Equal(audio, deviceAudio) {
			t.Error("Device audio mismatch")
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout receiving device audio")
	}
	
	// Session sends audio to device
	gatewayAudio := []byte{5, 6, 7, 8}
	session.AudioOut <- gatewayAudio
	
	// Device receives audio-start
	event, err := codec.ReadEvent()
	if err != nil {
		t.Fatalf("Failed to read event: %v", err)
	}
	if event.Type != EventAudioStart {
		t.Errorf("Expected audio-start, got %s", event.Type)
	}
	
	// Device receives audio chunk
	event, err = codec.ReadEvent()
	if err != nil {
		t.Fatalf("Failed to read audio chunk: %v", err)
	}
	if event.Type != EventAudioChunk {
		t.Errorf("Expected audio-chunk, got %s", event.Type)
	}
	if !bytes.Equal(event.Payload, gatewayAudio) {
		t.Error("Gateway audio mismatch")
	}
}

func TestSession_AutoCleanup(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	server := NewServer(ServerConfig{
		Address: "127.0.0.1:0",
		Logger:  logger,
	})
	
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()
	
	// Connect and immediately disconnect
	conn, err := net.Dial("tcp", server.Addr())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	
	// Wait for session creation
	time.Sleep(100 * time.Millisecond)
	
	initialCount := server.SessionCount()
	if initialCount != 1 {
		t.Errorf("Expected 1 session, got %d", initialCount)
	}
	
	// Close connection
	conn.Close()
	
	// Wait for session cleanup
	time.Sleep(200 * time.Millisecond)
	
	finalCount := server.SessionCount()
	if finalCount != 0 {
		t.Errorf("Expected 0 sessions after cleanup, got %d", finalCount)
	}
}

