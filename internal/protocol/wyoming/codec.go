package wyoming

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

const (
	// MaxPayloadSize is the maximum allowed payload size (8KB)
	MaxPayloadSize = 8192
)

// Codec handles encoding and decoding of Wyoming protocol messages
type Codec struct {
	reader *bufio.Reader
	writer *bufio.Writer
}

// NewCodec creates a new Wyoming protocol codec
func NewCodec(rw io.ReadWriter) *Codec {
	return &Codec{
		reader: bufio.NewReader(rw),
		writer: bufio.NewWriter(rw),
	}
}

// ReadEvent reads a Wyoming event from the connection
// Format: JSON header\n [optional binary payload]
func (c *Codec) ReadEvent() (*Event, error) {
	// Read JSON header (terminated by \n)
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		if err == io.EOF {
			return nil, ErrConnectionClosed
		}
		return nil, fmt.Errorf("read header: %w", err)
	}
	
	// Parse JSON header
	var event Event
	if err := json.Unmarshal(line, &event); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	
	// Read binary payload if present
	if event.PayloadLength > 0 {
		// Validate payload size
		if event.PayloadLength > MaxPayloadSize {
			return nil, ErrPayloadTooLarge
		}
		
		// Read exact payload_length bytes
		payload := make([]byte, event.PayloadLength)
		n, err := io.ReadFull(c.reader, payload)
		if err != nil {
			return nil, fmt.Errorf("read payload: %w", err)
		}
		
		if n != event.PayloadLength {
			return nil, ErrPayloadLengthMismatch
		}
		
		event.Payload = payload
	}
	
	// Validate event
	if err := event.Validate(); err != nil {
		return nil, err
	}
	
	return &event, nil
}

// WriteEvent writes a Wyoming event to the connection
// Format: JSON header\n [optional binary payload]
func (c *Codec) WriteEvent(event *Event) error {
	// Validate event before writing
	if err := event.Validate(); err != nil {
		return err
	}
	
	// Marshal JSON header
	header, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	
	// Write JSON header + newline
	if _, err := c.writer.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if err := c.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	
	// Write binary payload if present
	if event.PayloadLength > 0 && len(event.Payload) > 0 {
		n, err := c.writer.Write(event.Payload)
		if err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
		if n != event.PayloadLength {
			return ErrPayloadLengthMismatch
		}
	}
	
	// Flush buffer
	if err := c.writer.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	
	return nil
}

// Reset resets the codec with a new ReadWriter
func (c *Codec) Reset(rw io.ReadWriter) {
	c.reader.Reset(rw)
	c.writer.Reset(rw)
}

