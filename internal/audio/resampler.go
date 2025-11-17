package audio

import (
	"fmt"
	"math"
)

// Resampler handles audio sample rate conversion
type Resampler struct {
	inputRate  int
	outputRate int
	channels   int
	ratio      float64
}

// NewResampler creates a new audio resampler
func NewResampler(inputRate, outputRate, channels int) *Resampler {
	return &Resampler{
		inputRate:  inputRate,
		outputRate: outputRate,
		channels:   channels,
		ratio:      float64(outputRate) / float64(inputRate),
	}
}

// Resample converts audio from input sample rate to output sample rate
// Uses linear interpolation for simplicity and low latency
func (r *Resampler) Resample(input []byte) ([]byte, error) {
	if len(input)%2 != 0 {
		return nil, fmt.Errorf("input length must be even (16-bit samples)")
	}

	inputSamples := len(input) / 2
	outputSamples := int(float64(inputSamples) * r.ratio)
	output := make([]byte, outputSamples*2)

	for i := 0; i < outputSamples; i++ {
		// Calculate position in input
		srcPos := float64(i) / r.ratio
		srcIdx := int(srcPos)
		frac := srcPos - float64(srcIdx)

		if srcIdx >= inputSamples-1 {
			srcIdx = inputSamples - 2
			frac = 1.0
		}

		// Read two 16-bit samples (little-endian)
		sample1 := int16(input[srcIdx*2]) | int16(input[srcIdx*2+1])<<8
		sample2 := int16(input[(srcIdx+1)*2]) | int16(input[(srcIdx+1)*2+1])<<8

		// Linear interpolation
		interpolated := float64(sample1)*(1.0-frac) + float64(sample2)*frac
		result := int16(math.Round(interpolated))

		// Write result (little-endian)
		output[i*2] = byte(result)
		output[i*2+1] = byte(result >> 8)
	}

	return output, nil
}

// ResampleDown24to16 is a specialized fast resampler for 24kHz → 16kHz
// This is the exact ratio (2:3), so we can use a simple algorithm
func ResampleDown24to16(input []byte) ([]byte, error) {
	if len(input)%2 != 0 {
		return nil, fmt.Errorf("input length must be even (16-bit samples)")
	}

	inputSamples := len(input) / 2
	// Output will be 2/3 the size (16kHz is 2/3 of 24kHz)
	outputSamples := (inputSamples * 2) / 3
	output := make([]byte, outputSamples*2)

	// Process in groups of 3 input samples → 2 output samples
	for i := 0; i < outputSamples/2; i++ {
		srcIdx := i * 3
		if srcIdx+2 >= inputSamples {
			break
		}

		// Read 3 samples
		s1 := int16(input[srcIdx*2]) | int16(input[srcIdx*2+1])<<8
		s2 := int16(input[(srcIdx+1)*2]) | int16(input[(srcIdx+1)*2+1])<<8
		s3 := int16(input[(srcIdx+2)*2]) | int16(input[(srcIdx+2)*2+1])<<8

		// Average to create 2 output samples
		out1 := (int32(s1) + int32(s2)) / 2
		out2 := (int32(s2) + int32(s3)) / 2

		outIdx := i * 2
		// Write first output sample
		output[outIdx*2] = byte(out1)
		output[outIdx*2+1] = byte(out1 >> 8)
		// Write second output sample
		output[(outIdx+1)*2] = byte(out2)
		output[(outIdx+1)*2+1] = byte(out2 >> 8)
	}

	return output[:outputSamples*2], nil
}

