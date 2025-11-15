package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Clear environment
	clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Check defaults
	if cfg.Backend.Type != "mock" {
		t.Errorf("Expected backend type 'mock', got '%s'", cfg.Backend.Type)
	}
	if cfg.Wyoming.Address != "0.0.0.0:10200" {
		t.Errorf("Expected Wyoming address '0.0.0.0:10200', got '%s'", cfg.Wyoming.Address)
	}
	if cfg.Audio.BufferSize != 100 {
		t.Errorf("Expected buffer size 100, got %d", cfg.Audio.BufferSize)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected log level 'info', got '%s'", cfg.Logging.Level)
	}
}

func TestLoad_WithEnvironment(t *testing.T) {
	// Clear and set environment
	clearEnv()
	os.Setenv("BACKEND_TYPE", "gemini")
	os.Setenv("GEMINI_API_KEY", "test-api-key")
	os.Setenv("HA_URL", "http://homeassistant:8123")
	os.Setenv("HA_TOKEN", "test-token")
	os.Setenv("WYOMING_ADDR", "0.0.0.0:10201")
	os.Setenv("LOG_LEVEL", "debug")
	defer clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Backend.Type != "gemini" {
		t.Errorf("Expected backend type 'gemini', got '%s'", cfg.Backend.Type)
	}
	if cfg.Backend.GeminiAPIKey != "test-api-key" {
		t.Errorf("Expected Gemini API key 'test-api-key', got '%s'", cfg.Backend.GeminiAPIKey)
	}
	if cfg.HomeAssistant.URL != "http://homeassistant:8123" {
		t.Errorf("Expected HA URL 'http://homeassistant:8123', got '%s'", cfg.HomeAssistant.URL)
	}
	if cfg.Wyoming.Address != "0.0.0.0:10201" {
		t.Errorf("Expected Wyoming address '0.0.0.0:10201', got '%s'", cfg.Wyoming.Address)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level 'debug', got '%s'", cfg.Logging.Level)
	}
}

func TestLoad_IntegerParsing(t *testing.T) {
	clearEnv()
	os.Setenv("AUDIO_SAMPLE_RATE", "48000")
	os.Setenv("AUDIO_CHANNELS", "2")
	os.Setenv("AUDIO_BUFFER_SIZE", "200")
	defer clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Audio.BufferSize != 200 {
		t.Errorf("Expected buffer size 200, got %d", cfg.Audio.BufferSize)
	}
}

func TestLoad_BooleanParsing(t *testing.T) {
	clearEnv()
	os.Setenv("HA_REQUIRE_CONFIRMATION", "true")
	os.Setenv("METRICS_ENABLED", "true")
	defer clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if !cfg.HomeAssistant.RequireConfirm {
		t.Error("Expected HA_REQUIRE_CONFIRMATION to be true")
	}
	if !cfg.Observability.MetricsEnabled {
		t.Error("Expected METRICS_ENABLED to be true")
	}
}

func TestLoad_SliceParsing(t *testing.T) {
	clearEnv()
	os.Setenv("HA_ALLOW_LIST", "light.*, switch.turn_on, climate.set_temperature")
	defer clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	expected := []string{"light.*", "switch.turn_on", "climate.set_temperature"}
	if len(cfg.HomeAssistant.AllowList) != len(expected) {
		t.Errorf("Expected %d allow list items, got %d", len(expected), len(cfg.HomeAssistant.AllowList))
	}
	for i, item := range expected {
		if i >= len(cfg.HomeAssistant.AllowList) || cfg.HomeAssistant.AllowList[i] != item {
			t.Errorf("Expected allow list item %d to be '%s', got '%s'", i, item, cfg.HomeAssistant.AllowList[i])
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid mock backend",
			cfg: Config{
				Backend: BackendConfig{Type: "mock"},
				Audio:   AudioConfig{BufferSize: 100},
				Session: SessionConfig{Mode: "turn_based"},
				Logging: LoggingConfig{Level: "info", Format: "console"},
			},
			wantErr: false,
		},
		{
			name: "gemini without API key",
			cfg: Config{
				Backend: BackendConfig{Type: "gemini"},
				Audio:   AudioConfig{BufferSize: 100},
				Session: SessionConfig{Mode: "turn_based"},
				Logging: LoggingConfig{Level: "info", Format: "console"},
			},
			wantErr: true,
			errMsg:  "GEMINI_API_KEY",
		},
		{
			name: "openai without API key",
			cfg: Config{
				Backend: BackendConfig{Type: "openai"},
				Audio:   AudioConfig{BufferSize: 100},
				Session: SessionConfig{Mode: "turn_based"},
				Logging: LoggingConfig{Level: "info", Format: "console"},
			},
			wantErr: true,
			errMsg:  "OPENAI_API_KEY",
		},
		{
			name: "invalid backend type",
			cfg: Config{
				Backend: BackendConfig{Type: "invalid"},
				Audio:   AudioConfig{BufferSize: 100},
				Session: SessionConfig{Mode: "turn_based"},
				Logging: LoggingConfig{Level: "info", Format: "console"},
			},
			wantErr: true,
			errMsg:  "invalid BACKEND_TYPE",
		},
		{
			name: "invalid log level",
			cfg: Config{
				Backend: BackendConfig{Type: "mock"},
				Audio:   AudioConfig{BufferSize: 100},
				Session: SessionConfig{Mode: "turn_based"},
				Logging: LoggingConfig{Level: "invalid", Format: "console"},
			},
			wantErr: true,
			errMsg:  "invalid LOG_LEVEL",
		},
		{
			name: "invalid log format",
			cfg: Config{
				Backend: BackendConfig{Type: "mock"},
				Audio:   AudioConfig{BufferSize: 100},
				Session: SessionConfig{Mode: "turn_based"},
				Logging: LoggingConfig{Level: "info", Format: "invalid"},
			},
			wantErr: true,
			errMsg:  "invalid LOG_FORMAT",
		},
		{
			name: "invalid gemini sensitivity",
			cfg: Config{
				Backend: BackendConfig{
					Type:                      "gemini",
					GeminiAPIKey:              "key",
					GeminiVADStartSensitivity: "medium",
				},
				Audio:   AudioConfig{BufferSize: 100},
				Session: SessionConfig{Mode: "turn_based"},
				Logging: LoggingConfig{Level: "info", Format: "console"},
			},
			wantErr: true,
			errMsg:  "GEMINI_VAD_START_SENSITIVITY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errMsg, err.Error())
				}
			}
		})
	}
}

func TestHasHomeAssistant(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "both URL and token set",
			cfg: Config{
				HomeAssistant: HomeAssistantConfig{
					URL:   "http://homeassistant:8123",
					Token: "test-token",
				},
			},
			want: true,
		},
		{
			name: "only URL set",
			cfg: Config{
				HomeAssistant: HomeAssistantConfig{
					URL: "http://homeassistant:8123",
				},
			},
			want: false,
		},
		{
			name: "only token set",
			cfg: Config{
				HomeAssistant: HomeAssistantConfig{
					Token: "test-token",
				},
			},
			want: false,
		},
		{
			name: "neither set",
			cfg: Config{
				HomeAssistant: HomeAssistantConfig{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.HasHomeAssistant(); got != tt.want {
				t.Errorf("HasHomeAssistant() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary .env file
	tmpFile := ".env.test"
	content := `# Test config
BACKEND_TYPE=gemini
GEMINI_API_KEY=test-key-from-file
HA_URL=http://localhost:8123
HA_TOKEN=test-token-from-file
LOG_LEVEL=debug
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(tmpFile)

	clearEnv()
	cfg, err := LoadFromFile(tmpFile)
	if err != nil {
		t.Fatalf("LoadFromFile() failed: %v", err)
	}

	if cfg.Backend.Type != "gemini" {
		t.Errorf("Expected backend type 'gemini', got '%s'", cfg.Backend.Type)
	}
	if cfg.Backend.GeminiAPIKey != "test-key-from-file" {
		t.Errorf("Expected API key 'test-key-from-file', got '%s'", cfg.Backend.GeminiAPIKey)
	}
	if cfg.HomeAssistant.URL != "http://localhost:8123" {
		t.Errorf("Expected HA URL 'http://localhost:8123', got '%s'", cfg.HomeAssistant.URL)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level 'debug', got '%s'", cfg.Logging.Level)
	}
}

func TestLoadFromFile_EnvironmentTakesPrecedence(t *testing.T) {
	// Create a temporary .env file
	tmpFile := ".env.test2"
	content := `BACKEND_TYPE=mock
LOG_LEVEL=info
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(tmpFile)

	clearEnv()
	// Set environment variable (should take precedence)
	os.Setenv("LOG_LEVEL", "debug")
	defer clearEnv()

	cfg, err := LoadFromFile(tmpFile)
	if err != nil {
		t.Fatalf("LoadFromFile() failed: %v", err)
	}

	// Environment variable should take precedence
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level 'debug' from env, got '%s'", cfg.Logging.Level)
	}
}

func TestLoadFromFile_NonExistent(t *testing.T) {
	clearEnv()
	cfg, err := LoadFromFile(".env.nonexistent")
	if err != nil {
		t.Fatalf("LoadFromFile() should not fail for nonexistent file: %v", err)
	}
	// Should still return valid config with defaults
	if cfg == nil {
		t.Error("Expected non-nil config")
	}
}

func TestCreateLogger(t *testing.T) {
	tests := []struct {
		name   string
		cfg    LoggingConfig
		wantOk bool
	}{
		{
			name:   "console debug",
			cfg:    LoggingConfig{Level: "debug", Format: "console"},
			wantOk: true,
		},
		{
			name:   "json info",
			cfg:    LoggingConfig{Level: "info", Format: "json"},
			wantOk: true,
		},
		{
			name:   "console error",
			cfg:    LoggingConfig{Level: "error", Format: "console"},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Logging: tt.cfg}
			logger, err := cfg.CreateLogger()
			if (err == nil) != tt.wantOk {
				t.Errorf("CreateLogger() error = %v, wantOk %v", err, tt.wantOk)
			}
			if tt.wantOk && logger == nil {
				t.Error("Expected non-nil logger")
			}
		})
	}
}

func TestGetDurationEnv(t *testing.T) {
	clearEnv()
	os.Setenv("TEST_DURATION", "5s")
	defer clearEnv()

	duration := getDurationEnv("TEST_DURATION", 10*time.Second)
	if duration != 5*time.Second {
		t.Errorf("Expected 5s, got %v", duration)
	}

	// Test default
	duration = getDurationEnv("NONEXISTENT", 10*time.Second)
	if duration != 10*time.Second {
		t.Errorf("Expected 10s default, got %v", duration)
	}
}

// Helper functions

func clearEnv() {
	// Clear all relevant environment variables
	vars := []string{
		"HA_URL", "HA_TOKEN", "HA_TIMEOUT", "HA_ALLOW_LIST", "HA_REQUIRE_CONFIRMATION",
		"BACKEND_TYPE", "GEMINI_API_KEY", "GEMINI_MODEL", "OPENAI_API_KEY", "OPENAI_MODEL",
		"GEMINI_VAD_DISABLED", "GEMINI_VAD_START_SENSITIVITY", "GEMINI_VAD_END_SENSITIVITY",
		"GEMINI_VAD_PREFIX_PADDING_MS", "GEMINI_VAD_SILENCE_DURATION_MS",
		"GEMINI_ACTIVITY_HANDLING", "GEMINI_TURN_COVERAGE",
		"BACKEND_TEMPERATURE", "BACKEND_MAX_TOKENS",
		"WYOMING_ADDR",
		"AUDIO_SAMPLE_RATE", "AUDIO_CHANNELS", "AUDIO_BITS_PER_SAMPLE", "AUDIO_BUFFER_SIZE",
		"SYSTEM_PROMPT",
		"LOG_LEVEL", "LOG_FORMAT",
		"EVENT_BUFFER_SIZE",
		"METRICS_ENABLED", "METRICS_ADDR", "HEALTH_CHECK_ENABLED", "HEALTH_CHECK_ADDR",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findInString(s, substr))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
