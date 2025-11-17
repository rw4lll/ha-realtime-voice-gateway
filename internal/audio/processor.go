package audio

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// Processor handles audio processing including optional resampling
type Processor struct {
	logger    *zap.Logger
	resampler *Resampler

	// Configuration
	enabled    bool
	algorithm  string
	sourceRate int
	targetRate int
}

// ProcessorConfig holds audio processor configuration
type ProcessorConfig struct {
	Enabled    string // "auto", "true", "false"
	Algorithm  string // "fast", "linear", "cubic"
	SourceRate int    // Backend's output sample rate (e.g., 24000)
	TargetRate int    // Device's expected sample rate (e.g., 16000)
	Logger     *zap.Logger
}

// NewProcessor creates a new audio processor
func NewProcessor(cfg ProcessorConfig) (*Processor, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	p := &Processor{
		logger:     cfg.Logger,
		algorithm:  strings.ToLower(cfg.Algorithm),
		sourceRate: cfg.SourceRate,
		targetRate: cfg.TargetRate,
	}

	// Determine if resampling is needed
	enabledLower := strings.ToLower(cfg.Enabled)
	needsResampling := cfg.SourceRate != cfg.TargetRate && cfg.SourceRate > 0 && cfg.TargetRate > 0

	switch enabledLower {
	case "true":
		p.enabled = true
		if !needsResampling {
			p.logger.Warn("resampling enabled but source and target rates are the same",
				zap.Int("source_rate", cfg.SourceRate),
				zap.Int("target_rate", cfg.TargetRate))
		}
	case "false":
		p.enabled = false
		if needsResampling {
			p.logger.Warn("resampling disabled but sample rates differ - audio may be distorted!",
				zap.Int("source_rate", cfg.SourceRate),
				zap.Int("target_rate", cfg.TargetRate))
		}
	case "auto":
		p.enabled = needsResampling
		if p.enabled {
			p.logger.Info("auto-enabling resampling (sample rate mismatch detected)",
				zap.Int("source_rate", cfg.SourceRate),
				zap.Int("target_rate", cfg.TargetRate))
		}
	default:
		return nil, fmt.Errorf("invalid enabled value: %s (must be 'auto', 'true', or 'false')", cfg.Enabled)
	}

	// Create resampler if enabled
	if p.enabled {
		if err := p.initResampler(); err != nil {
			return nil, fmt.Errorf("failed to initialize resampler: %w", err)
		}

		p.logger.Info("audio processor initialized with resampling",
			zap.String("algorithm", p.algorithm),
			zap.Int("source_rate", p.sourceRate),
			zap.Int("target_rate", p.targetRate),
			zap.Float64("ratio", float64(p.targetRate)/float64(p.sourceRate)))
	} else {
		p.logger.Info("audio processor initialized without resampling",
			zap.Int("rate", cfg.SourceRate))
	}

	return p, nil
}

// initResampler initializes the appropriate resampler
func (p *Processor) initResampler() error {
	// For now, we only support downsampling
	if p.sourceRate < p.targetRate {
		return fmt.Errorf("upsampling not yet supported (source: %d, target: %d)", p.sourceRate, p.targetRate)
	}

	// Validate algorithm
	validAlgorithms := map[string]bool{
		"fast":   true,
		"linear": true,
		"cubic":  true, // TODO: Implement cubic
	}
	if !validAlgorithms[p.algorithm] {
		return fmt.Errorf("invalid algorithm: %s (must be 'fast', 'linear', or 'cubic')", p.algorithm)
	}

	// For cubic, fall back to linear for now
	if p.algorithm == "cubic" {
		p.logger.Warn("cubic interpolation not yet implemented, falling back to linear")
		p.algorithm = "linear"
	}

	// Create resampler
	p.resampler = NewResampler(p.sourceRate, p.targetRate, 1) // 1 channel (mono)

	return nil
}

// Process processes an audio frame, applying resampling if enabled
func (p *Processor) Process(input []byte) ([]byte, error) {
	if !p.enabled || p.resampler == nil {
		// Pass through without modification
		return input, nil
	}

	// Choose algorithm
	var output []byte
	var err error

	// Use optimized fast path for 24kHz→16kHz
	if p.algorithm == "fast" && p.sourceRate == 24000 && p.targetRate == 16000 {
		output, err = ResampleDown24to16(input)
	} else {
		// Use generic linear interpolation
		output, err = p.resampler.Resample(input)
	}

	if err != nil {
		return nil, fmt.Errorf("resampling failed: %w", err)
	}

	return output, nil
}

// IsEnabled returns true if resampling is enabled
func (p *Processor) IsEnabled() bool {
	return p.enabled
}

// GetSourceRate returns the source sample rate
func (p *Processor) GetSourceRate() int {
	return p.sourceRate
}

// GetTargetRate returns the target sample rate
func (p *Processor) GetTargetRate() int {
	return p.targetRate
}

// GetAlgorithm returns the resampling algorithm name
func (p *Processor) GetAlgorithm() string {
	return p.algorithm
}

// Stats returns resampling statistics
type Stats struct {
	Enabled    bool
	Algorithm  string
	SourceRate int
	TargetRate int
	Ratio      float64
}

// GetStats returns processor statistics
func (p *Processor) GetStats() Stats {
	var ratio float64
	if p.sourceRate > 0 && p.targetRate > 0 {
		ratio = float64(p.targetRate) / float64(p.sourceRate)
	}

	return Stats{
		Enabled:    p.enabled,
		Algorithm:  p.algorithm,
		SourceRate: p.sourceRate,
		TargetRate: p.targetRate,
		Ratio:      ratio,
	}
}
