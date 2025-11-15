package ha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"go.uber.org/zap"
)

func TestExecutor_normalizeParameters(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		BaseURL: "http://localhost:8123",
		Token:   "test",
		Logger:  zap.NewNop(),
	})

	executor, _ := NewExecutor(ExecutorConfig{
		Client: client,
		Logger: zap.NewNop(),
	})

	tests := []struct {
		name     string
		domain   string
		args     map[string]any
		expected map[string]any
	}{
		{
			name:   "todo domain with list_id should be converted to entity_id",
			domain: "todo",
			args: map[string]any{
				"list_id": "todo.shopping_list",
				"item":    "bread",
			},
			expected: map[string]any{
				"entity_id": "todo.shopping_list",
				"item":      "bread",
			},
		},
		{
			name:   "todo domain with entity_id should not change",
			domain: "todo",
			args: map[string]any{
				"entity_id": "todo.shopping_list",
				"item":      "bread",
			},
			expected: map[string]any{
				"entity_id": "todo.shopping_list",
				"item":      "bread",
			},
		},
		{
			name:   "non-todo domain should not change",
			domain: "light",
			args: map[string]any{
				"entity_id":  "light.bedroom",
				"brightness": 255,
			},
			expected: map[string]any{
				"entity_id":  "light.bedroom",
				"brightness": 255,
			},
		},
		{
			name:     "nil args should return nil",
			domain:   "todo",
			args:     nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.normalizeParameters(tt.args, tt.domain)

			if (result == nil) != (tt.expected == nil) {
				t.Errorf("normalizeParameters() nil mismatch: got %v, want %v", result, tt.expected)
				return
			}

			if result == nil {
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("normalizeParameters() length mismatch: got %d, want %d", len(result), len(tt.expected))
			}

			for k, expectedV := range tt.expected {
				if gotV, ok := result[k]; !ok {
					t.Errorf("normalizeParameters() missing key %q", k)
				} else if gotV != expectedV {
					t.Errorf("normalizeParameters() key %q: got %v, want %v", k, gotV, expectedV)
				}
			}

			// Check that list_id was removed for todo domain
			if tt.domain == "todo" {
				if _, hasListID := result["list_id"]; hasListID {
					t.Error("normalizeParameters() should have removed 'list_id' key")
				}
			}
		})
	}
}

func TestNewExecutor(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		BaseURL: "http://localhost:8123",
		Token:   "test",
		Logger:  zap.NewNop(),
	})

	tests := []struct {
		name      string
		cfg       ExecutorConfig
		wantErr   bool
		wantCount int // Expected number of allowed services
	}{
		{
			name: "with custom allow list",
			cfg: ExecutorConfig{
				Client:    client,
				AllowList: []string{"light.turn_on", "switch.turn_on"},
				Logger:    zap.NewNop(),
			},
			wantErr:   false,
			wantCount: 2,
		},
		{
			name: "with default allow list",
			cfg: ExecutorConfig{
				Client: client,
				Logger: zap.NewNop(),
			},
			wantErr:   false,
			wantCount: 17, // Default services count (including homeassistant.get_state)
		},
		{
			name: "missing client",
			cfg: ExecutorConfig{
				Logger: zap.NewNop(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, err := NewExecutor(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewExecutor() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if executor == nil {
					t.Error("NewExecutor() returned nil executor")
				}
				if executor != nil && len(executor.GetAllowList()) != tt.wantCount {
					t.Errorf("Expected %d allowed services, got %d", tt.wantCount, len(executor.GetAllowList()))
				}
			}
		})
	}
}

func TestExecutor_Execute_ServiceCall(t *testing.T) {
	// Create mock HA server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := []ServiceCallResponse{
			{
				Context: struct {
					ID       string `json:"id"`
					ParentID string `json:"parent_id"`
					UserID   string `json:"user_id"`
				}{
					ID: "test-context-id",
				},
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test-token",
		Logger:  zap.NewNop(),
	})

	executor, _ := NewExecutor(ExecutorConfig{
		Client:    client,
		AllowList: []string{"light.turn_on"},
		Logger:    zap.NewNop(),
	})

	// Test service call
	toolCall := &backend.ToolCall{
		ID:   "call-123",
		Name: "light.turn_on",
		Arguments: map[string]interface{}{
			"entity_id": "light.living_room",
		},
		Timestamp: time.Now(),
	}

	result, err := executor.Execute(context.Background(), toolCall)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil result")
	}
	if result.CallID != "call-123" {
		t.Errorf("Expected CallID 'call-123', got '%s'", result.CallID)
	}
	if result.Error != "" {
		t.Errorf("Unexpected error in result: %s", result.Error)
	}
}

func TestExecutor_Execute_NotAllowed(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		BaseURL: "http://localhost:8123",
		Token:   "test",
		Logger:  zap.NewNop(),
	})

	executor, _ := NewExecutor(ExecutorConfig{
		Client:    client,
		AllowList: []string{"light.turn_on"},
		Logger:    zap.NewNop(),
	})

	// Try to call a service that's not in the allow list
	toolCall := &backend.ToolCall{
		ID:   "call-123",
		Name: "switch.turn_on", // Not in allow list
		Arguments: map[string]interface{}{
			"entity_id": "switch.dangerous",
		},
		Timestamp: time.Now(),
	}

	result, err := executor.Execute(context.Background(), toolCall)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil result")
	}
	if result.Error == "" {
		t.Error("Expected error for disallowed service")
	}
}

func TestExecutor_Execute_StateQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := State{
			EntityID: "light.living_room",
			State:    "on",
			Attributes: map[string]interface{}{
				"brightness": 255,
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(state)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test-token",
		Logger:  zap.NewNop(),
	})

	executor, _ := NewExecutor(ExecutorConfig{
		Client: client,
		Logger: zap.NewNop(),
	})

	toolCall := &backend.ToolCall{
		ID:   "call-123",
		Name: "homeassistant.get_state",
		Arguments: map[string]interface{}{
			"entity_id": "light.living_room",
		},
		Timestamp: time.Now(),
	}

	result, err := executor.Execute(context.Background(), toolCall)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil result")
	}
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}

	// Check result contains state information
	resultMap, ok := result.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Result is not a map")
	}
	if resultMap["state"] != "on" {
		t.Errorf("Expected state 'on', got '%v'", resultMap["state"])
	}
}

func TestExecutor_IsAllowed(t *testing.T) {
	executor := &Executor{
		allowList: map[string]bool{
			"light.turn_on":           true,
			"switch.*":                true,
			"*.toggle":                true,
			"climate.set_temperature": true,
		},
		logger: zap.NewNop(),
	}

	tests := []struct {
		name        string
		serviceName string
		want        bool
	}{
		{"exact match", "light.turn_on", true},
		{"domain wildcard", "switch.turn_on", true},
		{"domain wildcard 2", "switch.turn_off", true},
		{"service wildcard", "light.toggle", true},
		{"service wildcard 2", "switch.toggle", true},
		{"not allowed", "light.turn_off", false},
		{"not allowed 2", "cover.open_cover", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executor.isAllowed(tt.serviceName)
			if got != tt.want {
				t.Errorf("isAllowed(%s) = %v, want %v", tt.serviceName, got, tt.want)
			}
		})
	}
}

func TestParseToolName(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		wantDomain  string
		wantService string
		wantErr     bool
	}{
		{
			name:        "domain.service format",
			toolName:    "light.turn_on",
			wantDomain:  "light",
			wantService: "turn_on",
			wantErr:     false,
		},
		{
			name:        "just service name",
			toolName:    "get_state",
			wantDomain:  "homeassistant",
			wantService: "get_state",
			wantErr:     false,
		},
		{
			name:     "invalid format",
			toolName: "light.turn.on.extra",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain, service, err := parseToolName(tt.toolName)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseToolName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if domain != tt.wantDomain {
					t.Errorf("Expected domain '%s', got '%s'", tt.wantDomain, domain)
				}
				if service != tt.wantService {
					t.Errorf("Expected service '%s', got '%s'", tt.wantService, service)
				}
			}
		})
	}
}

func TestExecutor_AllowListManagement(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		BaseURL: "http://localhost:8123",
		Token:   "test",
		Logger:  zap.NewNop(),
	})

	executor, _ := NewExecutor(ExecutorConfig{
		Client:    client,
		AllowList: []string{"light.turn_on"},
		Logger:    zap.NewNop(),
	})

	// Test initial state
	if !executor.isAllowed("light.turn_on") {
		t.Error("Expected light.turn_on to be allowed")
	}

	// Add new service
	executor.AddToAllowList("switch.turn_on")
	if !executor.isAllowed("switch.turn_on") {
		t.Error("Expected switch.turn_on to be allowed after adding")
	}

	// Remove service
	executor.RemoveFromAllowList("light.turn_on")
	if executor.isAllowed("light.turn_on") {
		t.Error("Expected light.turn_on to be disallowed after removal")
	}
}

func TestGetDefaultTools(t *testing.T) {
	tools := GetDefaultTools()

	if len(tools) == 0 {
		t.Error("GetDefaultTools() returned empty list")
	}

	// Check that common tools are present
	foundLight := false
	foundSwitch := false
	foundState := false

	for _, tool := range tools {
		switch tool.Name {
		case "light.turn_on":
			foundLight = true
		case "switch.turn_on":
			foundSwitch = true
		case "homeassistant.get_state":
			foundState = true
		}
	}

	if !foundLight {
		t.Error("light.turn_on not found in default tools")
	}
	if !foundSwitch {
		t.Error("switch.turn_on not found in default tools")
	}
	if !foundState {
		t.Error("homeassistant.get_state not found in default tools")
	}
}

func TestGetToolsForServices(t *testing.T) {
	services := []string{"light.turn_on", "switch.turn_on"}
	tools := GetToolsForServices(services)

	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}

	for _, tool := range tools {
		if tool.Name != "light.turn_on" && tool.Name != "switch.turn_on" {
			t.Errorf("Unexpected tool: %s", tool.Name)
		}
	}
}

func TestExecutor_CallServiceWithResponse(t *testing.T) {
	// Create mock HA server that returns data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for return_response parameter
		if r.URL.Query().Get("return_response") != "true" {
			t.Errorf("Expected return_response=true in query params")
		}

		// Return mock weather data
		resp := map[string]interface{}{
			"weather.home": map[string]interface{}{
				"forecast": []map[string]interface{}{
					{
						"datetime":    "2024-01-01T12:00:00",
						"temperature": 20.5,
						"condition":   "sunny",
					},
				},
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test-token",
		Logger:  zap.NewNop(),
	})

	executor, _ := NewExecutor(ExecutorConfig{
		Client:    client,
		AllowList: []string{"weather.*"},
		Logger:    zap.NewNop(),
	})

	// Test weather.get_forecasts service (response-returning)
	toolCall := &backend.ToolCall{
		ID:   "call-123",
		Name: "weather.get_forecasts",
		Arguments: map[string]interface{}{
			"type":      "daily",
			"entity_id": []string{"weather.home"},
		},
		Timestamp: time.Now(),
	}

	result, err := executor.Execute(context.Background(), toolCall)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil result")
	}
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}

	// Check result contains forecast data
	resultMap, ok := result.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Result is not a map")
	}
	if _, exists := resultMap["weather.home"]; !exists {
		t.Error("Expected weather.home in result")
	}
}

func TestExecutor_DetermineResponseTiming(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		BaseURL: "http://localhost:8123",
		Token:   "test",
		Logger:  zap.NewNop(),
	})

	executor, _ := NewExecutor(ExecutorConfig{
		Client: client,
		Logger: zap.NewNop(),
	})

	tests := []struct {
		name           string
		domain         string
		service        string
		success        bool
		wantInterrupt  bool
		wantDeferrable bool
		wantNil        bool
	}{
		{
			name:          "error response should interrupt",
			domain:        "light",
			service:       "turn_on",
			success:       false,
			wantInterrupt: true,
		},
		{
			name:    "blocking service returns nil",
			domain:  "weather",
			service: "get_forecasts",
			success: true,
			wantNil: true,
		},
		{
			name:          "critical service should interrupt",
			domain:        "lock",
			service:       "unlock",
			success:       true,
			wantInterrupt: true,
		},
		{
			name:           "normal control is deferrable",
			domain:         "light",
			service:        "turn_on",
			success:        true,
			wantDeferrable: true,
		},
		{
			name:           "fan control is deferrable",
			domain:         "fan",
			service:        "turn_on",
			success:        true,
			wantDeferrable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executor.determineResponseTiming(tt.domain, tt.service, tt.success)

			if tt.wantNil {
				if got != nil {
					t.Errorf("determineResponseTiming(%s, %s, %v) = %v, want nil",
						tt.domain, tt.service, tt.success, got)
				}
				return
			}

			if got == nil {
				if tt.wantInterrupt || tt.wantDeferrable {
					t.Errorf("determineResponseTiming(%s, %s, %v) returned nil, but expected timing metadata",
						tt.domain, tt.service, tt.success)
				}
				return
			}

			hasInterrupt, _ := got["interrupt"].(bool)
			hasDeferrable, _ := got["deferrable"].(bool)

			if hasInterrupt != tt.wantInterrupt {
				t.Errorf("determineResponseTiming(%s, %s, %v) interrupt = %v, want %v",
					tt.domain, tt.service, tt.success, hasInterrupt, tt.wantInterrupt)
			}

			if hasDeferrable != tt.wantDeferrable {
				t.Errorf("determineResponseTiming(%s, %s, %v) deferrable = %v, want %v",
					tt.domain, tt.service, tt.success, hasDeferrable, tt.wantDeferrable)
			}
		})
	}
}

func TestExecutor_IsResponseReturningService(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		BaseURL: "http://localhost:8123",
		Token:   "test",
		Logger:  zap.NewNop(),
	})

	executor, _ := NewExecutor(ExecutorConfig{
		Client: client,
		Logger: zap.NewNop(),
	})

	tests := []struct {
		name        string
		serviceName string
		want        bool
	}{
		{"weather forecast returns data", "weather.get_forecasts", true},
		{"todo get items returns data", "todo.get_items", true},
		{"calendar events returns data", "calendar.get_events", true},
		{"light turn on doesn't return data", "light.turn_on", false},
		{"fan turn on doesn't return data", "fan.turn_on", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executor.isResponseReturningService(tt.serviceName)
			if got != tt.want {
				t.Errorf("isResponseReturningService(%s) = %v, want %v",
					tt.serviceName, got, tt.want)
			}
		})
	}
}
