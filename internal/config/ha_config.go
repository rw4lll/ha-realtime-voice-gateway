package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// HAConfig represents the YAML configuration structure.
type HAConfig struct {
	HomeAssistant HomeAssistantYAML `yaml:"home_assistant"`
	Backend       BackendYAML       `yaml:"backend"`
	Wyoming       WyomingYAML       `yaml:"wyoming"`
	Audio         AudioYAML         `yaml:"audio"`
	Session       SessionYAML       `yaml:"session"`
	Logging       LoggingYAML       `yaml:"logging"`
	Performance   PerformanceYAML   `yaml:"performance"`
	Observability ObservabilityYAML `yaml:"observability"`
}

// HomeAssistantYAML holds HA configuration from YAML.
type HomeAssistantYAML struct {
	URL       string        `yaml:"url"`
	Token     string        `yaml:"token"`
	Timeout   string        `yaml:"timeout"`
	Discovery DiscoveryYAML `yaml:"discovery"`
}

// DiscoveryYAML holds autodiscovery configuration.
type DiscoveryYAML struct {
	Domains  DomainFilterYAML  `yaml:"domains"`
	Services ServiceFilterYAML `yaml:"services"`
	Prompt   PromptYAML        `yaml:"prompt"`
}

// DomainFilterYAML holds domain filtering rules.
type DomainFilterYAML struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// ServiceFilterYAML holds service filtering rules.
type ServiceFilterYAML struct {
	Mode     string   `yaml:"mode"`     // "allow" or "deny"
	Patterns []string `yaml:"patterns"` // Service patterns with wildcards
}

// PromptYAML holds system prompt generation settings.
type PromptYAML struct {
	IncludeEntities   bool   `yaml:"include_entities"`
	IncludeAttributes bool   `yaml:"include_attributes"`
	GroupBy           string `yaml:"group_by"` // "domain", "area", "floor", "none"
	Template          string `yaml:"template"`
}

// BackendYAML holds backend configuration.
type BackendYAML struct {
	Type   string     `yaml:"type"`
	Gemini GeminiYAML `yaml:"gemini"`
	OpenAI OpenAIYAML `yaml:"openai"`
}

// GeminiYAML holds Gemini-specific configuration.
type GeminiYAML struct {
	APIKey               string `yaml:"api_key"`
	Model                string `yaml:"model"`
	ConnectTimeout       string `yaml:"connect_timeout"`
	ReceiveTimeout       string `yaml:"receive_timeout"`
	SendTimeout          string `yaml:"send_timeout"`
	MaxRetries           int    `yaml:"max_retries"`
	RetryBackoff         string `yaml:"retry_backoff"`
	MaxSessions          int    `yaml:"max_sessions"`
	VADDisabled          *bool  `yaml:"vad_disabled"`
	VADStartSensitivity  string `yaml:"vad_start_sensitivity"`
	VADEndSensitivity    string `yaml:"vad_end_sensitivity"`
	VADPrefixPaddingMs   *int   `yaml:"vad_prefix_padding_ms"`
	VADSilenceDurationMs *int   `yaml:"vad_silence_duration_ms"`
	ActivityHandling     string `yaml:"activity_handling"`
	TurnCoverage         string `yaml:"turn_coverage"`
}

// OpenAIYAML holds OpenAI-specific configuration.
type OpenAIYAML struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
}

// WyomingYAML holds Wyoming server configuration.
type WyomingYAML struct {
	Address string `yaml:"address"`
}

// AudioYAML holds audio configuration.
type AudioYAML struct {
	BufferSize int `yaml:"buffer_size"`
}

// SessionYAML holds session configuration.
type SessionYAML struct {
	SystemPrompt string `yaml:"system_prompt"`
}

// LoggingYAML holds logging configuration.
type LoggingYAML struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// PerformanceYAML holds performance configuration.
type PerformanceYAML struct {
	AudioBufferSize int `yaml:"audio_buffer_size"`
	EventBufferSize int `yaml:"event_buffer_size"`
}

// ObservabilityYAML holds observability configuration.
type ObservabilityYAML struct {
	Metrics     MetricsYAML     `yaml:"metrics"`
	HealthCheck HealthCheckYAML `yaml:"health_check"`
}

// MetricsYAML holds metrics configuration.
type MetricsYAML struct {
	Enabled bool   `yaml:"enabled"`
	Address string `yaml:"address"`
}

// HealthCheckYAML holds health check configuration.
type HealthCheckYAML struct {
	Enabled bool   `yaml:"enabled"`
	Address string `yaml:"address"`
}

// LoadYAMLConfig loads configuration from a YAML file.
func LoadYAMLConfig(path string) (*HAConfig, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg HAConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return &cfg, nil
}

// MergeWithEnv merges YAML config with environment variable overrides.
func (c *Config) MergeWithEnv(yamlCfg *HAConfig) error {
	// Home Assistant - override with env vars if present
	if url := getEnv("HA_URL", ""); url != "" {
		c.HomeAssistant.URL = url
	} else if yamlCfg.HomeAssistant.URL != "" {
		c.HomeAssistant.URL = yamlCfg.HomeAssistant.URL
	}

	if token := getEnv("HA_TOKEN", ""); token != "" {
		c.HomeAssistant.Token = token
	} else if yamlCfg.HomeAssistant.Token != "" {
		c.HomeAssistant.Token = yamlCfg.HomeAssistant.Token
	}

	// Discovery domains
	if domains := getSliceEnv("HA_DISCOVERY_INCLUDE", []string{}); len(domains) > 0 {
		c.HomeAssistant.AutoDiscoveryDomains = domains
	} else if len(yamlCfg.HomeAssistant.Discovery.Domains.Include) > 0 {
		c.HomeAssistant.AutoDiscoveryDomains = yamlCfg.HomeAssistant.Discovery.Domains.Include
	}

	if excluded := getSliceEnv("HA_DISCOVERY_EXCLUDE", []string{}); len(excluded) > 0 {
		c.HomeAssistant.AutoDiscoveryDenied = excluded
	} else if len(yamlCfg.HomeAssistant.Discovery.Domains.Exclude) > 0 {
		c.HomeAssistant.AutoDiscoveryDenied = yamlCfg.HomeAssistant.Discovery.Domains.Exclude
	}

	// Service patterns
	if patterns := getSliceEnv("HA_SERVICE_PATTERNS", []string{}); len(patterns) > 0 {
		c.HomeAssistant.AllowList = patterns
	} else if len(yamlCfg.HomeAssistant.Discovery.Services.Patterns) > 0 {
		c.HomeAssistant.AllowList = yamlCfg.HomeAssistant.Discovery.Services.Patterns
	}

	// Prompt settings
	if includeEntities := os.Getenv("HA_PROMPT_ENTITIES"); includeEntities != "" {
		c.HomeAssistant.AutoDiscoveryPrompt = getBoolEnv("HA_PROMPT_ENTITIES", true)
	} else {
		c.HomeAssistant.AutoDiscoveryPrompt = yamlCfg.HomeAssistant.Discovery.Prompt.IncludeEntities
	}

	if includeAttrs := os.Getenv("HA_PROMPT_ATTRIBUTES"); includeAttrs != "" {
		c.HomeAssistant.AutoDiscoveryAttributes = getBoolEnv("HA_PROMPT_ATTRIBUTES", false)
	} else {
		c.HomeAssistant.AutoDiscoveryAttributes = yamlCfg.HomeAssistant.Discovery.Prompt.IncludeAttributes
	}

	// Backend - override with env vars
	if backendType := getEnv("BACKEND_TYPE", ""); backendType != "" {
		c.Backend.Type = backendType
	} else if yamlCfg.Backend.Type != "" {
		c.Backend.Type = yamlCfg.Backend.Type
	}

	// Gemini
	if apiKey := getEnv("GEMINI_API_KEY", ""); apiKey != "" {
		c.Backend.GeminiAPIKey = apiKey
	} else if yamlCfg.Backend.Gemini.APIKey != "" {
		c.Backend.GeminiAPIKey = yamlCfg.Backend.Gemini.APIKey
	}

	if model := getEnv("GEMINI_MODEL", ""); model != "" {
		c.Backend.GeminiModel = model
	} else if yamlCfg.Backend.Gemini.Model != "" {
		c.Backend.GeminiModel = yamlCfg.Backend.Gemini.Model
	}

	if val := os.Getenv("GEMINI_VAD_DISABLED"); val != "" {
		c.Backend.GeminiVADDisabled = getBoolEnv("GEMINI_VAD_DISABLED", false)
	} else if yamlCfg.Backend.Gemini.VADDisabled != nil {
		c.Backend.GeminiVADDisabled = *yamlCfg.Backend.Gemini.VADDisabled
	}

	if val := strings.TrimSpace(os.Getenv("GEMINI_VAD_START_SENSITIVITY")); val != "" {
		c.Backend.GeminiVADStartSensitivity = val
	} else if yamlVal := strings.TrimSpace(yamlCfg.Backend.Gemini.VADStartSensitivity); yamlVal != "" {
		c.Backend.GeminiVADStartSensitivity = yamlVal
	}

	if val := strings.TrimSpace(os.Getenv("GEMINI_VAD_END_SENSITIVITY")); val != "" {
		c.Backend.GeminiVADEndSensitivity = val
	} else if yamlVal := strings.TrimSpace(yamlCfg.Backend.Gemini.VADEndSensitivity); yamlVal != "" {
		c.Backend.GeminiVADEndSensitivity = yamlVal
	}

	if val := os.Getenv("GEMINI_VAD_PREFIX_PADDING_MS"); val != "" {
		c.Backend.GeminiVADPrefixPaddingMs = getIntEnv("GEMINI_VAD_PREFIX_PADDING_MS", c.Backend.GeminiVADPrefixPaddingMs)
	} else if yamlCfg.Backend.Gemini.VADPrefixPaddingMs != nil {
		c.Backend.GeminiVADPrefixPaddingMs = *yamlCfg.Backend.Gemini.VADPrefixPaddingMs
	}

	if val := os.Getenv("GEMINI_VAD_SILENCE_DURATION_MS"); val != "" {
		c.Backend.GeminiVADSilenceDurationMs = getIntEnv("GEMINI_VAD_SILENCE_DURATION_MS", c.Backend.GeminiVADSilenceDurationMs)
	} else if yamlCfg.Backend.Gemini.VADSilenceDurationMs != nil {
		c.Backend.GeminiVADSilenceDurationMs = *yamlCfg.Backend.Gemini.VADSilenceDurationMs
	}

	if val := strings.TrimSpace(os.Getenv("GEMINI_ACTIVITY_HANDLING")); val != "" {
		c.Backend.GeminiActivityHandling = val
	} else if yamlVal := strings.TrimSpace(yamlCfg.Backend.Gemini.ActivityHandling); yamlVal != "" {
		c.Backend.GeminiActivityHandling = yamlVal
	}

	if val := strings.TrimSpace(os.Getenv("GEMINI_TURN_COVERAGE")); val != "" {
		c.Backend.GeminiTurnCoverage = val
	} else if yamlVal := strings.TrimSpace(yamlCfg.Backend.Gemini.TurnCoverage); yamlVal != "" {
		c.Backend.GeminiTurnCoverage = yamlVal
	}

	// Wyoming
	if addr := getEnv("WYOMING_ADDR", ""); addr != "" {
		c.Wyoming.Address = addr
	} else if yamlCfg.Wyoming.Address != "" {
		c.Wyoming.Address = yamlCfg.Wyoming.Address
	}

	// Session
	if prompt := getEnv("SYSTEM_PROMPT", ""); prompt != "" {
		c.Session.SystemPrompt = prompt
	} else if yamlCfg.Session.SystemPrompt != "" {
		c.Session.SystemPrompt = yamlCfg.Session.SystemPrompt
	}

	// Logging
	if level := getEnv("LOG_LEVEL", ""); level != "" {
		c.Logging.Level = level
	} else if yamlCfg.Logging.Level != "" {
		c.Logging.Level = yamlCfg.Logging.Level
	}

	if format := getEnv("LOG_FORMAT", ""); format != "" {
		c.Logging.Format = format
	} else if yamlCfg.Logging.Format != "" {
		c.Logging.Format = yamlCfg.Logging.Format
	}

	// Performance
	if audioBufferSize := getIntEnv("AUDIO_BUFFER_SIZE", 0); audioBufferSize > 0 {
		c.Performance.AudioBufferSize = audioBufferSize
		c.Performance.EnableMetrics = true // Enable metrics when set via env
	} else if yamlCfg.Performance.AudioBufferSize > 0 {
		c.Performance.AudioBufferSize = yamlCfg.Performance.AudioBufferSize
	}

	if eventBufferSize := getIntEnv("EVENT_BUFFER_SIZE", 0); eventBufferSize > 0 {
		c.Performance.EventBufferSize = eventBufferSize
	} else if yamlCfg.Performance.EventBufferSize > 0 {
		c.Performance.EventBufferSize = yamlCfg.Performance.EventBufferSize
	}

	// Observability - Metrics
	if metricsEnabled := os.Getenv("METRICS_ENABLED"); metricsEnabled != "" {
		c.Performance.EnableMetrics = getBoolEnv("METRICS_ENABLED", false)
		c.Observability.MetricsEnabled = c.Performance.EnableMetrics
	} else {
		// Use YAML value for both fields (they should be in sync)
		c.Performance.EnableMetrics = yamlCfg.Observability.Metrics.Enabled
		c.Observability.MetricsEnabled = yamlCfg.Observability.Metrics.Enabled
	}

	if metricsAddr := getEnv("METRICS_ADDR", ""); metricsAddr != "" {
		c.Observability.MetricsAddr = metricsAddr
	} else if yamlCfg.Observability.Metrics.Address != "" {
		c.Observability.MetricsAddr = yamlCfg.Observability.Metrics.Address
	}

	// Observability - Health Check
	if healthEnabled := os.Getenv("HEALTH_CHECK_ENABLED"); healthEnabled != "" {
		c.Observability.HealthCheckEnabled = getBoolEnv("HEALTH_CHECK_ENABLED", true)
	} else {
		c.Observability.HealthCheckEnabled = yamlCfg.Observability.HealthCheck.Enabled
	}

	if healthAddr := getEnv("HEALTH_CHECK_ADDR", ""); healthAddr != "" {
		c.Observability.HealthCheckAddr = healthAddr
	} else if yamlCfg.Observability.HealthCheck.Address != "" {
		c.Observability.HealthCheckAddr = yamlCfg.Observability.HealthCheck.Address
	}

	return nil
}

// ParseDuration parses a duration string with fallback.
func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
