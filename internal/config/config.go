package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Config holds all application configuration.
type Config struct {
	// Home Assistant
	HomeAssistant HomeAssistantConfig

	// LLM Backend
	Backend BackendConfig

	// WebSocket Server
	WebSocket WebSocketConfig

	// Audio Settings
	Audio AudioConfig

	// Session Settings
	Session SessionConfig

	// Logging
	Logging LoggingConfig

	// Security
	Security SecurityConfig

	// Performance
	Performance PerformanceConfig

	// Observability
	Observability ObservabilityConfig
}

// HomeAssistantConfig holds HA-specific configuration.
type HomeAssistantConfig struct {
	URL            string        // Base URL (e.g., "http://homeassistant.local:8123")
	Token          string        // Long-lived access token
	Timeout        time.Duration // HTTP request timeout
	AllowList      []string      // Allowed services (e.g., ["light.*", "switch.turn_on"])
	RequireConfirm bool          // Require confirmation for dangerous operations

	// Autodiscovery settings
	AutoDiscovery           bool     // Enable automatic service/entity discovery
	AutoDiscoveryDomains    []string // Allowed domains for autodiscovery (empty = all)
	AutoDiscoveryDenied     []string // Domains to exclude from autodiscovery
	AutoDiscoveryPrompt     bool     // Include entity list in system prompt
	AutoDiscoveryAttributes bool     // Include entity attributes in prompt
}

// BackendConfig holds LLM backend configuration.
type BackendConfig struct {
	Type string // "mock", "gemini", "openai"

	// Gemini-specific
	GeminiAPIKey               string
	GeminiModel                string
	GeminiConnectTimeout       time.Duration
	GeminiReceiveTimeout       time.Duration
	GeminiSendTimeout          time.Duration
	GeminiMaxRetries           int
	GeminiRetryBackoff         time.Duration
	GeminiMaxSessions          int
	GeminiVADDisabled          bool
	GeminiVADStartSensitivity  string
	GeminiVADEndSensitivity    string
	GeminiVADPrefixPaddingMs   int
	GeminiVADSilenceDurationMs int
	GeminiActivityHandling     string
	GeminiTurnCoverage         string

	// OpenAI-specific
	OpenAIAPIKey string
	OpenAIModel  string

	// Common settings
	Temperature float64
	MaxTokens   int
}

// WebSocketConfig holds WebSocket server configuration.
type WebSocketConfig struct {
	Address       string // Listen address (e.g., "0.0.0.0:8080")
	Path          string // WebSocket path (e.g., "/voice-stream")
	MaxBufferSize int    // Max WebSocket frame size in bytes
	ReadTimeout   int    // Read timeout in seconds
	WriteTimeout  int    // Write timeout in seconds
}

// AudioConfig holds audio processing configuration.
type AudioConfig struct {
	BufferSize int // Number of audio frames to buffer
	// Note: Audio formats (sample rate, channels, bits) are determined by:
	//   - Input: WebSocket client (16kHz, 16-bit, mono PCM)
	//   - Output: Backend's native format (from Capabilities())
	// No configuration needed - formats auto-negotiate!
}

// SessionConfig holds session management configuration.
type SessionConfig struct {
	SystemPrompt   string        // Default system prompt for LLM
	SafetyTimeout  time.Duration // Max session duration (safety limit)
	SilenceTimeout time.Duration // Silence duration before ending conversation
	AudioBufferMs  int           // Audio buffering before playback (ms)
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string // "debug", "info", "warn", "error"
	Format string // "console", "json"
}

// SecurityConfig holds security settings.
type SecurityConfig struct {
	// Future: API keys, rate limiting, etc.
}

// PerformanceConfig holds performance tuning settings.
type PerformanceConfig struct {
	AudioBufferSize    int  // Audio channel buffer size
	EventBufferSize    int  // Event channel buffer size
	AudioStartBufferMs int  // Milliseconds to buffer audio before sending (prevents premature audio-start)
	EnableMetrics      bool // Enable metrics collection
}

// ObservabilityConfig holds monitoring and metrics configuration.
type ObservabilityConfig struct {
	MetricsEnabled     bool
	MetricsAddr        string
	HealthCheckEnabled bool
	HealthCheckAddr    string
}

// Load loads configuration from environment variables or YAML file.
// Priority: Environment variables > YAML file > Defaults
func Load() (*Config, error) {
	// Check if YAML config file is specified
	configFile := getEnv("HA_CONFIG_FILE", "ha-config.yaml")

	var yamlCfg *HAConfig
	var err error

	// Try to load YAML config if it exists
	if _, statErr := os.Stat(configFile); statErr == nil {
		yamlCfg, err = LoadYAMLConfig(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load YAML config: %w", err)
		}
	}

	// Create config with defaults
	cfg := &Config{
		HomeAssistant: HomeAssistantConfig{
			URL:            "",
			Token:          "",
			Timeout:        10 * time.Second,
			AllowList:      []string{},
			RequireConfirm: false,

			// Autodiscovery - always enabled, just configure filtering
			AutoDiscovery:           true, // Always enabled
			AutoDiscoveryDomains:    []string{},
			AutoDiscoveryDenied:     []string{"automation", "script", "scene", "group", "zone", "person"},
			AutoDiscoveryPrompt:     true,
			AutoDiscoveryAttributes: false,
		},
		Backend: BackendConfig{
			Type:                       "mock",
			GeminiAPIKey:               "",
			GeminiModel:                "gemini-2.0-flash-exp",
			GeminiConnectTimeout:       30 * time.Second,
			GeminiReceiveTimeout:       60 * time.Second,
			GeminiSendTimeout:          10 * time.Second,
			GeminiMaxRetries:           3,
			GeminiRetryBackoff:         1 * time.Second,
			GeminiMaxSessions:          5,
			GeminiVADDisabled:          false,
			GeminiVADStartSensitivity:  "",
			GeminiVADEndSensitivity:    "",
			GeminiVADPrefixPaddingMs:   0,
			GeminiVADSilenceDurationMs: 0,
			GeminiActivityHandling:     "",
			GeminiTurnCoverage:         "",
			OpenAIAPIKey:               "",
			OpenAIModel:                "gpt-4o-realtime-preview",
			Temperature:                0.7,
			MaxTokens:                  4096,
		},
		WebSocket: WebSocketConfig{
			Address:       "0.0.0.0:8080",
			Path:          "/voice-stream",
			MaxBufferSize: 32768, // 32KB max frame
			ReadTimeout:   60,
			WriteTimeout:  10,
		},
		Audio: AudioConfig{
			BufferSize: 100,
		},
		Session: SessionConfig{
			SystemPrompt:   "You are a helpful voice assistant for Home Assistant. You can control lights, switches, climate, covers, and media players.",
			SafetyTimeout:  5 * time.Minute, // 5 min max session duration
			SilenceTimeout: 3 * time.Second, // 3s silence before ending
			AudioBufferMs:  500,             // 500ms audio buffer
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "console",
		},
		Performance: PerformanceConfig{
			AudioBufferSize:    100,
			EventBufferSize:    50,
			AudioStartBufferMs: 200, // Default: 200ms buffer (reduced from 500ms for faster responses)
			EnableMetrics:      false,
		},
		Observability: ObservabilityConfig{
			MetricsEnabled:     false,
			MetricsAddr:        "0.0.0.0:9090",
			HealthCheckEnabled: true,
			HealthCheckAddr:    "0.0.0.0:8080",
		},
	}

	// Merge with YAML config if loaded
	if yamlCfg != nil {
		if err := cfg.MergeWithEnv(yamlCfg); err != nil {
			return nil, fmt.Errorf("failed to merge config: %w", err)
		}
	} else {
		// Fall back to pure environment variables if no YAML
		cfg.HomeAssistant.URL = getEnv("HA_URL", "")
		cfg.HomeAssistant.Token = getEnv("HA_TOKEN", "")
		cfg.HomeAssistant.Timeout = getDurationEnv("HA_TIMEOUT", 10*time.Second)
		cfg.HomeAssistant.AllowList = getSliceEnv("HA_ALLOW_LIST", []string{})
		cfg.HomeAssistant.RequireConfirm = getBoolEnv("HA_REQUIRE_CONFIRMATION", false)
		cfg.HomeAssistant.AutoDiscoveryDomains = getSliceEnv("HA_AUTODISCOVERY_DOMAINS", []string{})
		cfg.HomeAssistant.AutoDiscoveryDenied = getSliceEnv("HA_AUTODISCOVERY_DENIED", []string{"automation", "script", "scene", "group", "zone", "person"})
		cfg.HomeAssistant.AutoDiscoveryPrompt = getBoolEnv("HA_AUTODISCOVERY_PROMPT", true)
		cfg.HomeAssistant.AutoDiscoveryAttributes = getBoolEnv("HA_AUTODISCOVERY_ATTRIBUTES", false)

		cfg.Backend.Type = getEnv("BACKEND_TYPE", "mock")
		cfg.Backend.GeminiAPIKey = getEnv("GEMINI_API_KEY", "")
		cfg.Backend.GeminiModel = getEnv("GEMINI_MODEL", "gemini-2.0-flash-exp")
		cfg.Backend.GeminiConnectTimeout = getDurationEnv("GEMINI_CONNECT_TIMEOUT", 30*time.Second)
		cfg.Backend.GeminiReceiveTimeout = getDurationEnv("GEMINI_RECEIVE_TIMEOUT", 60*time.Second)
		cfg.Backend.GeminiSendTimeout = getDurationEnv("GEMINI_SEND_TIMEOUT", 10*time.Second)
		cfg.Backend.GeminiMaxRetries = getIntEnv("GEMINI_MAX_RETRIES", 3)
		cfg.Backend.GeminiRetryBackoff = getDurationEnv("GEMINI_RETRY_BACKOFF", 1*time.Second)
		cfg.Backend.GeminiMaxSessions = getIntEnv("GEMINI_MAX_SESSIONS", 5)
		cfg.Backend.GeminiVADDisabled = getBoolEnv("GEMINI_VAD_DISABLED", false)
		cfg.Backend.GeminiVADStartSensitivity = strings.TrimSpace(getEnv("GEMINI_VAD_START_SENSITIVITY", cfg.Backend.GeminiVADStartSensitivity))
		cfg.Backend.GeminiVADEndSensitivity = strings.TrimSpace(getEnv("GEMINI_VAD_END_SENSITIVITY", cfg.Backend.GeminiVADEndSensitivity))
		cfg.Backend.GeminiVADPrefixPaddingMs = getIntEnv("GEMINI_VAD_PREFIX_PADDING_MS", cfg.Backend.GeminiVADPrefixPaddingMs)
		cfg.Backend.GeminiVADSilenceDurationMs = getIntEnv("GEMINI_VAD_SILENCE_DURATION_MS", cfg.Backend.GeminiVADSilenceDurationMs)
		cfg.Backend.GeminiActivityHandling = strings.TrimSpace(getEnv("GEMINI_ACTIVITY_HANDLING", cfg.Backend.GeminiActivityHandling))
		cfg.Backend.GeminiTurnCoverage = strings.TrimSpace(getEnv("GEMINI_TURN_COVERAGE", cfg.Backend.GeminiTurnCoverage))
		cfg.Backend.OpenAIAPIKey = getEnv("OPENAI_API_KEY", "")
		cfg.Backend.OpenAIModel = getEnv("OPENAI_MODEL", "gpt-4o-realtime-preview")
		cfg.Backend.Temperature = getFloatEnv("BACKEND_TEMPERATURE", 0.7)
		cfg.Backend.MaxTokens = getIntEnv("BACKEND_MAX_TOKENS", 4096)

		cfg.WebSocket.Address = getEnv("WEBSOCKET_ADDR", "0.0.0.0:8080")
		cfg.WebSocket.Path = getEnv("WEBSOCKET_PATH", "/voice-stream")
		cfg.WebSocket.MaxBufferSize = getIntEnv("WEBSOCKET_MAX_BUFFER_SIZE", 32768)
		cfg.WebSocket.ReadTimeout = getIntEnv("WEBSOCKET_READ_TIMEOUT", 60)
		cfg.WebSocket.WriteTimeout = getIntEnv("WEBSOCKET_WRITE_TIMEOUT", 10)
		cfg.Audio.BufferSize = getIntEnv("AUDIO_BUFFER_SIZE", 100)
		cfg.Session.SystemPrompt = getEnv("SYSTEM_PROMPT", "You are a helpful voice assistant for Home Assistant. You can control lights, switches, climate, covers, and media players.")
		cfg.Session.SafetyTimeout = getDurationEnv("SESSION_SAFETY_TIMEOUT", 5*time.Minute)
		cfg.Session.SilenceTimeout = getDurationEnv("SESSION_SILENCE_TIMEOUT", 3*time.Second)
		cfg.Session.AudioBufferMs = getIntEnv("SESSION_AUDIO_BUFFER_MS", 500)
		cfg.Logging.Level = getEnv("LOG_LEVEL", "info")
		cfg.Logging.Format = getEnv("LOG_FORMAT", "console")
		cfg.Performance.AudioBufferSize = getIntEnv("AUDIO_BUFFER_SIZE", 100)
		cfg.Performance.EventBufferSize = getIntEnv("EVENT_BUFFER_SIZE", 50)
		cfg.Observability.MetricsEnabled = getBoolEnv("METRICS_ENABLED", false)
		cfg.Observability.MetricsAddr = getEnv("METRICS_ADDR", "0.0.0.0:9090")
		cfg.Observability.HealthCheckEnabled = getBoolEnv("HEALTH_CHECK_ENABLED", true)
		cfg.Observability.HealthCheckAddr = getEnv("HEALTH_CHECK_ADDR", "0.0.0.0:8080")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// LoadFromFile loads configuration from a .env file and environment variables.
// Environment variables take precedence over file values.
func LoadFromFile(path string) (*Config, error) {
	// Load .env file if it exists
	if err := loadEnvFile(path); err != nil {
		return nil, fmt.Errorf("failed to load env file: %w", err)
	}

	// Load from environment (which now includes .env values)
	return Load()
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate backend configuration
	switch c.Backend.Type {
	case "mock":
		// No additional validation needed
	case "gemini":
		if c.Backend.GeminiAPIKey == "" {
			return fmt.Errorf("GEMINI_API_KEY is required when BACKEND_TYPE=gemini")
		}
	case "openai":
		if c.Backend.OpenAIAPIKey == "" {
			return fmt.Errorf("OPENAI_API_KEY is required when BACKEND_TYPE=openai")
		}
	default:
		return fmt.Errorf("invalid BACKEND_TYPE: %s (must be 'mock', 'gemini', or 'openai')", c.Backend.Type)
	}

	// Audio configuration is now auto-negotiated from backend capabilities
	// No validation needed for sample rate, channels, or bits per sample

	// Validate logging configuration
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid LOG_LEVEL: %s (must be 'debug', 'info', 'warn', or 'error')", c.Logging.Level)
	}

	validLogFormats := map[string]bool{"console": true, "json": true}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("invalid LOG_FORMAT: %s (must be 'console' or 'json')", c.Logging.Format)
	}

	// Validate session configuration
	if c.Session.SafetyTimeout < time.Second {
		return fmt.Errorf("SESSION_SAFETY_TIMEOUT must be at least 1 second")
	}
	if c.Session.SilenceTimeout < 0 {
		return fmt.Errorf("SESSION_SILENCE_TIMEOUT must be non-negative")
	}

	if c.Backend.Type == "gemini" {
		if err := validateGeminiRealtimeConfig(c.Backend); err != nil {
			return err
		}
	}

	return nil
}

func validateGeminiRealtimeConfig(cfg BackendConfig) error {
	allowedSensitivity := map[string]struct{}{"": {}, "low": {}, "high": {}}
	if _, ok := allowedSensitivity[strings.ToLower(cfg.GeminiVADStartSensitivity)]; !ok {
		return fmt.Errorf("invalid GEMINI_VAD_START_SENSITIVITY: %s (must be 'low', 'high', or empty)", cfg.GeminiVADStartSensitivity)
	}
	if _, ok := allowedSensitivity[strings.ToLower(cfg.GeminiVADEndSensitivity)]; !ok {
		return fmt.Errorf("invalid GEMINI_VAD_END_SENSITIVITY: %s (must be 'low', 'high', or empty)", cfg.GeminiVADEndSensitivity)
	}

	if cfg.GeminiVADPrefixPaddingMs < 0 {
		return fmt.Errorf("GEMINI_VAD_PREFIX_PADDING_MS must be >= 0")
	}
	if cfg.GeminiVADSilenceDurationMs < 0 {
		return fmt.Errorf("GEMINI_VAD_SILENCE_DURATION_MS must be >= 0")
	}

	allowedActivity := map[string]struct{}{"": {}, "start_interrupts": {}, "no_interruption": {}}
	if _, ok := allowedActivity[strings.ToLower(cfg.GeminiActivityHandling)]; !ok {
		return fmt.Errorf("invalid GEMINI_ACTIVITY_HANDLING: %s (must be 'start_interrupts', 'no_interruption', or empty)", cfg.GeminiActivityHandling)
	}

	allowedCoverage := map[string]struct{}{"": {}, "only_activity": {}, "all_input": {}}
	if _, ok := allowedCoverage[strings.ToLower(cfg.GeminiTurnCoverage)]; !ok {
		return fmt.Errorf("invalid GEMINI_TURN_COVERAGE: %s (must be 'only_activity', 'all_input', or empty)", cfg.GeminiTurnCoverage)
	}

	return nil
}

// HasHomeAssistant returns true if Home Assistant is configured.
func (c *Config) HasHomeAssistant() bool {
	return c.HomeAssistant.URL != "" && c.HomeAssistant.Token != ""
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getFloatEnv(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getSliceEnv(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Split by comma and trim whitespace
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

// loadEnvFile loads environment variables from a .env file.
func loadEnvFile(path string) error {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// File doesn't exist, but that's okay - we'll use environment variables
		return nil
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse line by line
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		// Skip empty lines and comments
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid line %d: %s", i+1, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, `"'`)

		// Set environment variable (only if not already set)
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}

	return nil
}

// CreateLogger creates a zap logger based on the configuration.
func (c *Config) CreateLogger() (*zap.Logger, error) {
	var zapCfg zap.Config

	if c.Logging.Format == "json" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
	}

	// Set log level
	switch c.Logging.Level {
	case "debug":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	}

	return zapCfg.Build()
}
