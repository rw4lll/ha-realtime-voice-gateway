package audio

import (
	"testing"

	"go.uber.org/zap"
)

func TestNewProcessor(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name        string
		config      ProcessorConfig
		wantEnabled bool
		wantError   bool
	}{
		{
			name: "auto enable with different rates",
			config: ProcessorConfig{
				Enabled:    "auto",
				Algorithm:  "fast",
				SourceRate: 24000,
				TargetRate: 16000,
				Logger:     logger,
			},
			wantEnabled: true,
			wantError:   false,
		},
		{
			name: "auto disable with same rates",
			config: ProcessorConfig{
				Enabled:    "auto",
				Algorithm:  "fast",
				SourceRate: 16000,
				TargetRate: 16000,
				Logger:     logger,
			},
			wantEnabled: false,
			wantError:   false,
		},
		{
			name: "force enable",
			config: ProcessorConfig{
				Enabled:    "true",
				Algorithm:  "fast",
				SourceRate: 24000,
				TargetRate: 16000,
				Logger:     logger,
			},
			wantEnabled: true,
			wantError:   false,
		},
		{
			name: "force disable",
			config: ProcessorConfig{
				Enabled:    "false",
				Algorithm:  "fast",
				SourceRate: 24000,
				TargetRate: 16000,
				Logger:     logger,
			},
			wantEnabled: false,
			wantError:   false,
		},
		{
			name: "linear algorithm",
			config: ProcessorConfig{
				Enabled:    "true",
				Algorithm:  "linear",
				SourceRate: 48000,
				TargetRate: 16000,
				Logger:     logger,
			},
			wantEnabled: true,
			wantError:   false,
		},
		{
			name: "missing logger",
			config: ProcessorConfig{
				Enabled:    "auto",
				Algorithm:  "fast",
				SourceRate: 24000,
				TargetRate: 16000,
				Logger:     nil,
			},
			wantEnabled: false,
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProcessor(tt.config)
			if (err != nil) != tt.wantError {
				t.Errorf("NewProcessor() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if err == nil && p.IsEnabled() != tt.wantEnabled {
				t.Errorf("NewProcessor() enabled = %v, want %v", p.IsEnabled(), tt.wantEnabled)
			}
		})
	}
}

func TestProcessor_Process(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name       string
		config     ProcessorConfig
		inputSize  int
		wantChange bool
	}{
		{
			name: "resample 24kHz to 16kHz",
			config: ProcessorConfig{
				Enabled:    "true",
				Algorithm:  "fast",
				SourceRate: 24000,
				TargetRate: 16000,
				Logger:     logger,
			},
			inputSize:  960, // 20ms at 24kHz
			wantChange: true,
		},
		{
			name: "pass through disabled",
			config: ProcessorConfig{
				Enabled:    "false",
				Algorithm:  "fast",
				SourceRate: 24000,
				TargetRate: 16000,
				Logger:     logger,
			},
			inputSize:  960,
			wantChange: false,
		},
		{
			name: "pass through same rate",
			config: ProcessorConfig{
				Enabled:    "auto",
				Algorithm:  "fast",
				SourceRate: 16000,
				TargetRate: 16000,
				Logger:     logger,
			},
			inputSize:  640,
			wantChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProcessor(tt.config)
			if err != nil {
				t.Fatalf("NewProcessor() error = %v", err)
			}

			input := make([]byte, tt.inputSize)
			// Fill with test data
			for i := 0; i < tt.inputSize; i++ {
				input[i] = byte(i % 256)
			}

			output, err := p.Process(input)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantChange {
				if len(output) == len(input) {
					t.Errorf("Process() output size = %d, expected different from input size %d", len(output), len(input))
				}
			} else {
				if len(output) != len(input) {
					t.Errorf("Process() output size = %d, want %d", len(output), len(input))
				}
			}
		})
	}
}

func TestProcessor_Stats(t *testing.T) {
	logger := zap.NewNop()

	p, err := NewProcessor(ProcessorConfig{
		Enabled:    "true",
		Algorithm:  "fast",
		SourceRate: 24000,
		TargetRate: 16000,
		Logger:     logger,
	})
	if err != nil {
		t.Fatalf("NewProcessor() error = %v", err)
	}

	stats := p.GetStats()

	if !stats.Enabled {
		t.Error("Stats.Enabled = false, want true")
	}
	if stats.Algorithm != "fast" {
		t.Errorf("Stats.Algorithm = %s, want fast", stats.Algorithm)
	}
	if stats.SourceRate != 24000 {
		t.Errorf("Stats.SourceRate = %d, want 24000", stats.SourceRate)
	}
	if stats.TargetRate != 16000 {
		t.Errorf("Stats.TargetRate = %d, want 16000", stats.TargetRate)
	}

	expectedRatio := float64(16000) / float64(24000)
	if stats.Ratio != expectedRatio {
		t.Errorf("Stats.Ratio = %f, want %f", stats.Ratio, expectedRatio)
	}
}

func BenchmarkProcessor_Process_Enabled(b *testing.B) {
	logger := zap.NewNop()
	p, _ := NewProcessor(ProcessorConfig{
		Enabled:    "true",
		Algorithm:  "fast",
		SourceRate: 24000,
		TargetRate: 16000,
		Logger:     logger,
	})

	input := make([]byte, 960) // 20ms at 24kHz

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.Process(input)
	}
}

func BenchmarkProcessor_Process_Disabled(b *testing.B) {
	logger := zap.NewNop()
	p, _ := NewProcessor(ProcessorConfig{
		Enabled:    "false",
		Algorithm:  "fast",
		SourceRate: 24000,
		TargetRate: 16000,
		Logger:     logger,
	})

	input := make([]byte, 960)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.Process(input)
	}
}
