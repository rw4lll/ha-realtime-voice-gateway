package audio

import (
	"math"
	"testing"
)

func TestResampleDown24to16(t *testing.T) {
	// Generate 24kHz test tone (1 second, 440Hz)
	sampleRate := 24000
	duration := 1.0
	frequency := 440.0
	samples := int(float64(sampleRate) * duration)
	
	input := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		// Generate sine wave
		value := math.Sin(2 * math.Pi * frequency * float64(i) / float64(sampleRate))
		sample := int16(value * 32767.0 * 0.5) // 50% amplitude
		
		input[i*2] = byte(sample)
		input[i*2+1] = byte(sample >> 8)
	}
	
	// Resample to 16kHz
	output, err := ResampleDown24to16(input)
	if err != nil {
		t.Fatalf("ResampleDown24to16 failed: %v", err)
	}
	
	// Check output size (should be 2/3 of input)
	expectedSamples := (samples * 2) / 3
	actualSamples := len(output) / 2
	
	if actualSamples != expectedSamples {
		t.Errorf("Expected %d samples, got %d", expectedSamples, actualSamples)
	}
	
	// Verify output is valid PCM
	if len(output)%2 != 0 {
		t.Error("Output length should be even")
	}
}

func TestResampler(t *testing.T) {
	tests := []struct {
		name       string
		inputRate  int
		outputRate int
		channels   int
	}{
		{"24kHz to 16kHz", 24000, 16000, 1},
		{"16kHz to 24kHz", 16000, 24000, 1},
		{"48kHz to 16kHz", 48000, 16000, 1},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resampler := NewResampler(tt.inputRate, tt.outputRate, tt.channels)
			
			// Generate test input (100ms)
			samples := tt.inputRate / 10
			input := make([]byte, samples*2)
			
			for i := 0; i < samples; i++ {
				sample := int16(i % 1000)
				input[i*2] = byte(sample)
				input[i*2+1] = byte(sample >> 8)
			}
			
			output, err := resampler.Resample(input)
			if err != nil {
				t.Fatalf("Resample failed: %v", err)
			}
			
			expectedSamples := int(float64(samples) * resampler.ratio)
			actualSamples := len(output) / 2
			
			// Allow 1% tolerance
			tolerance := int(float64(expectedSamples) * 0.01)
			if math.Abs(float64(actualSamples-expectedSamples)) > float64(tolerance) {
				t.Errorf("Expected ~%d samples, got %d", expectedSamples, actualSamples)
			}
		})
	}
}

func BenchmarkResampleDown24to16(b *testing.B) {
	// 20ms of audio at 24kHz
	samples := 480
	input := make([]byte, samples*2)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ResampleDown24to16(input)
	}
}

func BenchmarkResampler(b *testing.B) {
	resampler := NewResampler(24000, 16000, 1)
	samples := 480
	input := make([]byte, samples*2)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resampler.Resample(input)
	}
}

