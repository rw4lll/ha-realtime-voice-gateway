package wyoming

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// mockReadWriter implements io.ReadWriter for testing
type mockReadWriter struct {
	*bytes.Buffer
}

func newMockReadWriter() *mockReadWriter {
	return &mockReadWriter{Buffer: new(bytes.Buffer)}
}

func TestCodec_WriteRead_AudioChunk(t *testing.T) {
	// Create codec with mock connection
	rw := newMockReadWriter()
	codec := NewCodec(rw)
	
	// Create test audio chunk
	format := AudioFormat{
		Rate:     16000,
		Width:    2,
		Channels: 1,
	}
	payload := make([]byte, 4096)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	
	now := time.Now()
	originalEvent := AudioChunkEvent(format, payload, &now)
	
	// Write event
	err := codec.WriteEvent(&originalEvent)
	if err != nil {
		t.Fatalf("WriteEvent failed: %v", err)
	}
	
	// Reset codec to read from same buffer
	codec.Reset(rw)
	
	// Read event back
	readEvent, err := codec.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}
	
	// Verify event type
	if readEvent.Type != EventAudioChunk {
		t.Errorf("Expected type %s, got %s", EventAudioChunk, readEvent.Type)
	}
	
	// Verify audio format
	readFormat, err := readEvent.GetAudioFormat()
	if err != nil {
		t.Fatalf("GetAudioFormat failed: %v", err)
	}
	
	if readFormat.Rate != format.Rate {
		t.Errorf("Expected rate %d, got %d", format.Rate, readFormat.Rate)
	}
	if readFormat.Width != format.Width {
		t.Errorf("Expected width %d, got %d", format.Width, readFormat.Width)
	}
	if readFormat.Channels != format.Channels {
		t.Errorf("Expected channels %d, got %d", format.Channels, readFormat.Channels)
	}
	
	// Verify payload
	if len(readEvent.Payload) != len(payload) {
		t.Errorf("Expected payload length %d, got %d", len(payload), len(readEvent.Payload))
	}
	
	if !bytes.Equal(readEvent.Payload, payload) {
		t.Error("Payload data mismatch")
	}
}

func TestCodec_WriteRead_AudioStart(t *testing.T) {
	rw := newMockReadWriter()
	codec := NewCodec(rw)
	
	format := AudioFormat{Rate: 16000, Width: 2, Channels: 1}
	now := time.Now()
	originalEvent := AudioStartEvent(format, &now)
	
	// Write and read
	if err := codec.WriteEvent(&originalEvent); err != nil {
		t.Fatalf("WriteEvent failed: %v", err)
	}
	
	codec.Reset(rw)
	readEvent, err := codec.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}
	
	// Verify
	if readEvent.Type != EventAudioStart {
		t.Errorf("Expected type %s, got %s", EventAudioStart, readEvent.Type)
	}
	
	if readEvent.PayloadLength != 0 {
		t.Errorf("Expected no payload, got length %d", readEvent.PayloadLength)
	}
	
	// Verify format
	readFormat, err := readEvent.GetAudioFormat()
	if err != nil {
		t.Fatalf("GetAudioFormat failed: %v", err)
	}
	
	if readFormat.Rate != format.Rate {
		t.Errorf("Rate mismatch: expected %d, got %d", format.Rate, readFormat.Rate)
	}
}

func TestCodec_WriteRead_AudioStop(t *testing.T) {
	rw := newMockReadWriter()
	codec := NewCodec(rw)
	
	now := time.Now()
	originalEvent := AudioStopEvent(&now)
	
	// Write and read
	if err := codec.WriteEvent(&originalEvent); err != nil {
		t.Fatalf("WriteEvent failed: %v", err)
	}
	
	codec.Reset(rw)
	readEvent, err := codec.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}
	
	// Verify
	if readEvent.Type != EventAudioStop {
		t.Errorf("Expected type %s, got %s", EventAudioStop, readEvent.Type)
	}
}

func TestCodec_MultipleEvents(t *testing.T) {
	rw := newMockReadWriter()
	codec := NewCodec(rw)
	
	format := AudioFormat{Rate: 16000, Width: 2, Channels: 1}
	
	// Write multiple events
	events := []Event{
		AudioStartEvent(format, nil),
		AudioChunkEvent(format, make([]byte, 1024), nil),
		AudioChunkEvent(format, make([]byte, 2048), nil),
		AudioStopEvent(nil),
	}
	
	for i, event := range events {
		if err := codec.WriteEvent(&event); err != nil {
			t.Fatalf("WriteEvent %d failed: %v", i, err)
		}
	}
	
	// Read them back
	codec.Reset(rw)
	for i, expectedEvent := range events {
		readEvent, err := codec.ReadEvent()
		if err != nil {
			t.Fatalf("ReadEvent %d failed: %v", i, err)
		}
		
		if readEvent.Type != expectedEvent.Type {
			t.Errorf("Event %d: expected type %s, got %s", i, expectedEvent.Type, readEvent.Type)
		}
		
		if readEvent.PayloadLength != expectedEvent.PayloadLength {
			t.Errorf("Event %d: expected payload length %d, got %d", i, expectedEvent.PayloadLength, readEvent.PayloadLength)
		}
	}
}

func TestCodec_InvalidEvents(t *testing.T) {
	tests := []struct {
		name      string
		event     Event
		expectErr error
	}{
		{
			name: "audio chunk without payload",
			event: Event{
				Type:          EventAudioChunk,
				Data:          map[string]interface{}{"rate": 16000, "width": 2, "channels": 1},
				PayloadLength: 0,
			},
			expectErr: ErrMissingPayload,
		},
		{
			name: "audio chunk without format",
			event: Event{
				Type:          EventAudioChunk,
				PayloadLength: 100,
				Payload:       make([]byte, 100),
			},
			expectErr: ErrMissingAudioFormat,
		},
		{
			name: "payload length mismatch",
			event: Event{
				Type:          EventAudioChunk,
				Data:          map[string]interface{}{"rate": 16000, "width": 2, "channels": 1},
				PayloadLength: 100,
				Payload:       make([]byte, 50), // Wrong size
			},
			expectErr: ErrPayloadLengthMismatch,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rw := newMockReadWriter()
			codec := NewCodec(rw)
			
			err := codec.WriteEvent(&tt.event)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			
			if err != tt.expectErr {
				t.Errorf("Expected error %v, got %v", tt.expectErr, err)
			}
		})
	}
}

func TestCodec_PayloadTooLarge(t *testing.T) {
	rw := newMockReadWriter()
	codec := NewCodec(rw)
	
	// Create event with oversized payload
	format := AudioFormat{Rate: 16000, Width: 2, Channels: 1}
	oversizedPayload := make([]byte, MaxPayloadSize+1)
	
	event := AudioChunkEvent(format, oversizedPayload, nil)
	
	// Try to write (should fail validation)
	err := codec.WriteEvent(&event)
	if err != ErrPayloadTooLarge {
		t.Errorf("Expected ErrPayloadTooLarge, got %v", err)
	}
}

func TestCodec_EOF(t *testing.T) {
	// Create codec with empty buffer
	rw := newMockReadWriter()
	codec := NewCodec(rw)
	
	// Try to read (should get EOF -> ConnectionClosed)
	_, err := codec.ReadEvent()
	if err != ErrConnectionClosed {
		t.Errorf("Expected ErrConnectionClosed, got %v", err)
	}
}

func TestCodec_PartialPayload(t *testing.T) {
	rw := newMockReadWriter()
	
	// Write malformed event (header says 1000 bytes, but only write 100)
	header := `{"type":"audio-chunk","data":{"rate":16000,"width":2,"channels":1},"payload_length":1000}` + "\n"
	rw.WriteString(header)
	rw.Write(make([]byte, 100)) // Only 100 bytes instead of 1000
	
	codec := NewCodec(rw)
	_, err := codec.ReadEvent()
	if err == nil {
		t.Fatal("Expected error for partial payload")
	}
	
	// Should get io.EOF or similar error when trying to read full payload
	if err != io.ErrUnexpectedEOF && err != ErrConnectionClosed {
		t.Logf("Got error: %v (acceptable)", err)
	}
}

func TestAudioFormat_Validate(t *testing.T) {
	tests := []struct {
		name      string
		format    AudioFormat
		expectErr error
	}{
		{
			name:      "valid 16kHz mono",
			format:    AudioFormat{Rate: 16000, Width: 2, Channels: 1},
			expectErr: nil,
		},
		{
			name:      "invalid sample rate",
			format:    AudioFormat{Rate: 48000, Width: 2, Channels: 1},
			expectErr: ErrInvalidSampleRate,
		},
		{
			name:      "invalid width",
			format:    AudioFormat{Rate: 16000, Width: 1, Channels: 1},
			expectErr: ErrInvalidSampleWidth,
		},
		{
			name:      "invalid channels",
			format:    AudioFormat{Rate: 16000, Width: 2, Channels: 2},
			expectErr: ErrInvalidChannels,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.format.ValidateFor16kHzMono()
			if err != tt.expectErr {
				t.Errorf("Expected error %v, got %v", tt.expectErr, err)
			}
		})
	}
}

func BenchmarkCodec_WriteAudioChunk(b *testing.B) {
	rw := newMockReadWriter()
	codec := NewCodec(rw)
	
	format := AudioFormat{Rate: 16000, Width: 2, Channels: 1}
	payload := make([]byte, 4096)
	event := AudioChunkEvent(format, payload, nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.Reset()
		codec.Reset(rw)
		codec.WriteEvent(&event)
	}
}

func BenchmarkCodec_ReadAudioChunk(b *testing.B) {
	// Pre-write event
	rw := newMockReadWriter()
	codec := NewCodec(rw)
	
	format := AudioFormat{Rate: 16000, Width: 2, Channels: 1}
	payload := make([]byte, 4096)
	event := AudioChunkEvent(format, payload, nil)
	
	// Write once
	codec.WriteEvent(&event)
	eventData := rw.Bytes()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.Reset()
		rw.Write(eventData)
		codec.Reset(rw)
		codec.ReadEvent()
	}
}

