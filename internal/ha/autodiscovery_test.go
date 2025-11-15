package ha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestAutoDiscovery_DiscoverTools(t *testing.T) {
	// Create mock HA server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/services" {
			services := []ServiceInfo{
				{
					Domain: "light",
					Services: map[string]ServiceDetail{
						"turn_on": {
							Name:        "Turn On",
							Description: "Turn on light(s)",
							Fields: map[string]FieldDetail{
								"brightness": {
									Name:        "brightness",
									Description: "Brightness value",
									Required:    false,
								},
							},
						},
						"turn_off": {
							Name:        "Turn Off",
							Description: "Turn off light(s)",
							Fields:      map[string]FieldDetail{},
						},
					},
				},
				{
					Domain: "switch",
					Services: map[string]ServiceDetail{
						"turn_on": {
							Name:        "Turn On",
							Description: "Turn on switch",
							Fields:      map[string]FieldDetail{},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(services)
		}
	}))
	defer server.Close()

	// Create client
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test_token",
		Logger:  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create autodiscovery
	autoDisc, err := NewAutoDiscovery(AutoDiscoveryConfig{
		Client: client,
		Logger: zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create autodiscovery: %v", err)
	}

	// Discover tools
	tools, err := autoDisc.DiscoverTools(context.Background())
	if err != nil {
		t.Fatalf("DiscoverTools failed: %v", err)
	}

	// Verify tools
	if len(tools) != 3 {
		t.Errorf("Expected 3 tools, got %d", len(tools))
	}

	// Check that light.turn_on exists
	foundLightOn := false
	for _, tool := range tools {
		if tool.Name == "light.turn_on" {
			foundLightOn = true
			if tool.Description != "Turn on light(s)" {
				t.Errorf("Wrong description: %s", tool.Description)
			}
			// Check parameters
			props, ok := tool.Parameters["properties"].(map[string]interface{})
			if !ok {
				t.Error("Properties not found in parameters")
			}
			if _, hasEntityID := props["entity_id"]; !hasEntityID {
				t.Error("entity_id parameter missing")
			}
			if _, hasBrightness := props["brightness"]; !hasBrightness {
				t.Error("brightness parameter missing")
			}
		}
	}
	if !foundLightOn {
		t.Error("light.turn_on tool not found")
	}
}

func TestAutoDiscovery_DiscoverTools_WithDomainFilter(t *testing.T) {
	// Create mock HA server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/services" {
			services := []ServiceInfo{
				{
					Domain: "light",
					Services: map[string]ServiceDetail{
						"turn_on": {
							Name:        "Turn On",
							Description: "Turn on light(s)",
						},
					},
				},
				{
					Domain: "switch",
					Services: map[string]ServiceDetail{
						"turn_on": {
							Name:        "Turn On",
							Description: "Turn on switch",
						},
					},
				},
				{
					Domain: "automation",
					Services: map[string]ServiceDetail{
						"trigger": {
							Name:        "Trigger",
							Description: "Trigger automation",
						},
					},
				},
			}
			json.NewEncoder(w).Encode(services)
		}
	}))
	defer server.Close()

	// Create client
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test_token",
		Logger:  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create autodiscovery with domain filter
	autoDisc, err := NewAutoDiscovery(AutoDiscoveryConfig{
		Client:         client,
		AllowedDomains: []string{"light", "switch"},
		Logger:         zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create autodiscovery: %v", err)
	}

	// Discover tools
	tools, err := autoDisc.DiscoverTools(context.Background())
	if err != nil {
		t.Fatalf("DiscoverTools failed: %v", err)
	}

	// Should only have light and switch, not automation
	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}

	for _, tool := range tools {
		if strings.HasPrefix(tool.Name, "automation.") {
			t.Error("automation domain should be filtered out")
		}
	}
}

func TestAutoDiscovery_DiscoverTools_WithDeniedDomains(t *testing.T) {
	// Create mock HA server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/services" {
			services := []ServiceInfo{
				{
					Domain: "light",
					Services: map[string]ServiceDetail{
						"turn_on": {Name: "Turn On"},
					},
				},
				{
					Domain: "script",
					Services: map[string]ServiceDetail{
						"turn_on": {Name: "Run Script"},
					},
				},
			}
			json.NewEncoder(w).Encode(services)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test_token",
		Logger:  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Deny script domain
	autoDisc, err := NewAutoDiscovery(AutoDiscoveryConfig{
		Client:        client,
		DeniedDomains: []string{"script"},
		Logger:        zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create autodiscovery: %v", err)
	}

	tools, err := autoDisc.DiscoverTools(context.Background())
	if err != nil {
		t.Fatalf("DiscoverTools failed: %v", err)
	}

	// Should only have light
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}

	if tools[0].Name != "light.turn_on" {
		t.Errorf("Expected light.turn_on, got %s", tools[0].Name)
	}
}

func TestAutoDiscovery_DiscoverEntities(t *testing.T) {
	// Create mock HA server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/states" {
			states := []State{
				{
					EntityID: "light.living_room",
					State:    "on",
					Attributes: map[string]interface{}{
						"friendly_name": "Living Room Light",
						"brightness":    128,
					},
				},
				{
					EntityID: "light.bedroom",
					State:    "off",
					Attributes: map[string]interface{}{
						"friendly_name": "Bedroom Light",
					},
				},
				{
					EntityID: "switch.garden",
					State:    "on",
					Attributes: map[string]interface{}{
						"friendly_name": "Garden Switch",
					},
				},
			}
			json.NewEncoder(w).Encode(states)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test_token",
		Logger:  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	autoDisc, err := NewAutoDiscovery(AutoDiscoveryConfig{
		Client: client,
		Logger: zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create autodiscovery: %v", err)
	}

	entities, err := autoDisc.DiscoverEntities(context.Background())
	if err != nil {
		t.Fatalf("DiscoverEntities failed: %v", err)
	}

	// Should have 2 domains
	if len(entities) != 2 {
		t.Errorf("Expected 2 domains, got %d", len(entities))
	}

	// Check light domain
	lights, ok := entities["light"]
	if !ok {
		t.Error("light domain not found")
	}
	if len(lights) != 2 {
		t.Errorf("Expected 2 lights, got %d", len(lights))
	}

	// Check switch domain
	switches, ok := entities["switch"]
	if !ok {
		t.Error("switch domain not found")
	}
	if len(switches) != 1 {
		t.Errorf("Expected 1 switch, got %d", len(switches))
	}
}

func TestAutoDiscovery_GenerateSystemPrompt(t *testing.T) {
	// Create mock HA server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/states" {
			states := []State{
				{
					EntityID: "light.living_room",
					State:    "on",
					Attributes: map[string]interface{}{
						"friendly_name": "Living Room Light",
						"brightness":    200,
					},
				},
				{
					EntityID: "switch.garden",
					State:    "off",
					Attributes: map[string]interface{}{
						"friendly_name": "Garden Switch",
					},
				},
			}
			json.NewEncoder(w).Encode(states)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test_token",
		Logger:  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	autoDisc, err := NewAutoDiscovery(AutoDiscoveryConfig{
		Client:            client,
		IncludeAttributes: true,
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create autodiscovery: %v", err)
	}

	basePrompt := "You are a smart home assistant."
	prompt, err := autoDisc.GenerateSystemPrompt(context.Background(), basePrompt)
	if err != nil {
		t.Fatalf("GenerateSystemPrompt failed: %v", err)
	}

	// Check that prompt contains expected elements
	if !strings.Contains(prompt, basePrompt) {
		t.Error("Base prompt not included")
	}
	if !strings.Contains(prompt, "light.living_room") {
		t.Error("Entity ID not included")
	}
	if !strings.Contains(prompt, "Living Room Light") {
		t.Error("Friendly name not included")
	}
	if !strings.Contains(prompt, "switch.garden") {
		t.Error("Switch entity not included")
	}
	if !strings.Contains(prompt, "brightness") {
		t.Error("Attributes not included")
	}
	if !strings.Contains(prompt, "[on]") {
		t.Error("State not included")
	}

	t.Logf("Generated prompt length: %d characters", len(prompt))
}

func TestAutoDiscovery_GenerateSystemPrompt_WithoutAttributes(t *testing.T) {
	// Create mock HA server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/states" {
			states := []State{
				{
					EntityID: "light.living_room",
					State:    "on",
					Attributes: map[string]interface{}{
						"friendly_name": "Living Room Light",
						"brightness":    200,
					},
				},
			}
			json.NewEncoder(w).Encode(states)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test_token",
		Logger:  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	autoDisc, err := NewAutoDiscovery(AutoDiscoveryConfig{
		Client:            client,
		IncludeAttributes: false,
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create autodiscovery: %v", err)
	}

	prompt, err := autoDisc.GenerateSystemPrompt(context.Background(), "")
	if err != nil {
		t.Fatalf("GenerateSystemPrompt failed: %v", err)
	}

	// Should not contain brightness details when attributes disabled
	if strings.Contains(prompt, "brightness: 200") {
		t.Error("Attributes should not be included when disabled")
	}
}

func TestGetDomainFromEntityID(t *testing.T) {
	tests := []struct {
		entityID string
		expected string
	}{
		{"light.living_room", "light"},
		{"switch.garden", "switch"},
		{"climate.upstairs", "climate"},
		{"sensor.temperature_1", "sensor"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		result := getDomainFromEntityID(tt.entityID)
		if result != tt.expected {
			t.Errorf("getDomainFromEntityID(%s) = %s, expected %s", tt.entityID, result, tt.expected)
		}
	}
}

func TestIsGlobalService(t *testing.T) {
	tests := []struct {
		service  string
		expected bool
	}{
		{"reload", true},
		{"reload_all", true},
		{"turn_all_on", true},
		{"turn_on", false},
		{"turn_off", false},
	}

	for _, tt := range tests {
		result := isGlobalService(tt.service)
		if result != tt.expected {
			t.Errorf("isGlobalService(%s) = %v, expected %v", tt.service, result, tt.expected)
		}
	}
}

