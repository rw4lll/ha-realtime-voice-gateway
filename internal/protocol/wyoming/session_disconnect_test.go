package wyoming

import (
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
)

func TestIsGracefulDisconnect(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "io.EOF",
			err:      io.EOF,
			expected: true,
		},
		{
			name:     "net.ErrClosed",
			err:      net.ErrClosed,
			expected: true,
		},
		{
			name:     "wrapped io.EOF",
			err:      errors.New("read: " + io.EOF.Error()),
			expected: false, // String matching removed, need proper wrapping
		},
		{
			name:     "unexpected error",
			err:      errors.New("some unexpected error"),
			expected: false,
		},
		{
			name:     "context cancelled",
			err:      errors.New("context canceled"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGracefulDisconnect(tt.err)
			if result != tt.expected {
				t.Errorf("isGracefulDisconnect(%v) = %v, want %v",
					tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsGracefulDisconnect_SyscallErrors(t *testing.T) {
	tests := []struct {
		name     string
		errno    syscall.Errno
		expected bool
	}{
		{
			name:     "ECONNRESET - connection reset by peer",
			errno:    syscall.ECONNRESET,
			expected: true,
		},
		{
			name:     "EPIPE - broken pipe",
			errno:    syscall.EPIPE,
			expected: true,
		},
		{
			name:     "ECONNREFUSED - connection refused",
			errno:    syscall.ECONNREFUSED,
			expected: true,
		},
		{
			name:     "ENETUNREACH - network unreachable",
			errno:    syscall.ENETUNREACH,
			expected: true,
		},
		{
			name:     "ECONNABORTED - connection aborted",
			errno:    syscall.ECONNABORTED,
			expected: true,
		},
		{
			name:     "EINVAL - invalid argument (not graceful)",
			errno:    syscall.EINVAL,
			expected: false,
		},
		{
			name:     "ETIMEDOUT - timeout (not graceful)",
			errno:    syscall.ETIMEDOUT,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrap in net.OpError as would happen in real network code
			opErr := &net.OpError{
				Op:  "read",
				Net: "tcp",
				Err: tt.errno,
			}

			result := isGracefulDisconnect(opErr)
			if result != tt.expected {
				t.Errorf("isGracefulDisconnect(%v) = %v, want %v",
					tt.errno, result, tt.expected)
			}
		})
	}
}

func TestIsGracefulDisconnect_NetOpError(t *testing.T) {
	t.Run("connection reset via net.OpError", func(t *testing.T) {
		opErr := &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: syscall.ECONNRESET,
		}

		result := isGracefulDisconnect(opErr)
		if !result {
			t.Error("should recognize ECONNRESET in net.OpError")
		}
	})

	t.Run("broken pipe via net.OpError", func(t *testing.T) {
		opErr := &net.OpError{
			Op:  "write",
			Net: "tcp",
			Err: syscall.EPIPE,
		}

		result := isGracefulDisconnect(opErr)
		if !result {
			t.Error("should recognize EPIPE in net.OpError")
		}
	})

	t.Run("nested net.OpError", func(t *testing.T) {
		innerOpErr := &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: syscall.ECONNRESET,
		}

		outerOpErr := &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: innerOpErr,
		}

		result := isGracefulDisconnect(outerOpErr)
		if !result {
			t.Error("should handle nested net.OpError")
		}
	})
}

func TestIsGracefulDisconnect_ErrorsIs(t *testing.T) {
	t.Run("wrapped io.EOF with errors.Is", func(t *testing.T) {
		wrappedErr := fmt.Errorf("connection closed: %w", io.EOF)

		result := isGracefulDisconnect(wrappedErr)
		if !result {
			t.Error("should recognize wrapped io.EOF")
		}
	})

	t.Run("wrapped net.ErrClosed", func(t *testing.T) {
		wrappedErr := fmt.Errorf("connection error: %w", net.ErrClosed)

		result := isGracefulDisconnect(wrappedErr)
		if !result {
			t.Error("should recognize wrapped net.ErrClosed")
		}
	})
}
