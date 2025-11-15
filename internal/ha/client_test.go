package ha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ClientConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: ClientConfig{
				BaseURL: "http://localhost:8123",
				Token:   "test-token",
			},
			wantErr: false,
		},
		{
			name: "missing base URL",
			cfg: ClientConfig{
				Token: "test-token",
			},
			wantErr: true,
		},
		{
			name: "missing token",
			cfg: ClientConfig{
				BaseURL: "http://localhost:8123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && client == nil {
				t.Error("NewClient() returned nil client")
			}
		})
	}
}

func TestClient_CallService(t *testing.T) {
	// Create mock HA server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("Missing or incorrect Authorization header")
		}
		if !contains(r.URL.Path, "/api/services/") {
			t.Errorf("Unexpected URL path: %s", r.URL.Path)
		}

		// Return success response
		resp := []ServiceCallResponse{
			{
				Context: struct {
					ID       string `json:"id"`
					ParentID string `json:"parent_id"`
					UserID   string `json:"user_id"`
				}{
					ID:     "test-context-id",
					UserID: "test-user",
				},
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test-token",
		Logger:  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test service call
	resp, err := client.CallService(context.Background(), ServiceCallRequest{
		Domain:  "light",
		Service: "turn_on",
		Data: map[string]interface{}{
			"entity_id": "light.living_room",
		},
	})

	if err != nil {
		t.Errorf("CallService() error = %v", err)
	}
	if resp == nil {
		t.Error("CallService() returned nil response")
	}
	if resp != nil && resp.Context.ID != "test-context-id" {
		t.Errorf("Expected context ID 'test-context-id', got '%s'", resp.Context.ID)
	}
}

func TestClient_CallService_Errors(t *testing.T) {
	tests := []struct {
		name       string
		req        ServiceCallRequest
		statusCode int
		response   interface{}
		wantErr    bool
	}{
		{
			name: "missing domain",
			req: ServiceCallRequest{
				Service: "turn_on",
			},
			wantErr: true,
		},
		{
			name: "missing service",
			req: ServiceCallRequest{
				Domain: "light",
			},
			wantErr: true,
		},
		{
			name: "API error",
			req: ServiceCallRequest{
				Domain:  "light",
				Service: "turn_on",
			},
			statusCode: http.StatusBadRequest,
			response: ErrorResponse{
				Message: "Invalid service call",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			client, _ := NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  zap.NewNop(),
			})

			_, err := client.CallService(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("CallService() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_GetState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Return state
		state := State{
			EntityID: "light.living_room",
			State:    "on",
			Attributes: map[string]interface{}{
				"brightness": 255,
			},
			LastChanged: time.Now(),
			LastUpdated: time.Now(),
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

	state, err := client.GetState(context.Background(), "light.living_room")
	if err != nil {
		t.Errorf("GetState() error = %v", err)
	}
	if state == nil {
		t.Error("GetState() returned nil state")
	}
	if state != nil && state.State != "on" {
		t.Errorf("Expected state 'on', got '%s'", state.State)
	}
}

func TestClient_GetState_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test-token",
		Logger:  zap.NewNop(),
	})

	_, err := client.GetState(context.Background(), "light.nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent entity")
	}
}

func TestClient_GetStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		states := []State{
			{EntityID: "light.living_room", State: "on"},
			{EntityID: "light.bedroom", State: "off"},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(states)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test-token",
		Logger:  zap.NewNop(),
	})

	states, err := client.GetStates(context.Background())
	if err != nil {
		t.Errorf("GetStates() error = %v", err)
	}
	if len(states) != 2 {
		t.Errorf("Expected 2 states, got %d", len(states))
	}
}

func TestClient_Ping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "API running"})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test-token",
		Logger:  zap.NewNop(),
	})

	err := client.Ping(context.Background())
	if err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestClient_Ping_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "bad-token",
		Logger:  zap.NewNop(),
	})

	err := client.Ping(context.Background())
	if err == nil {
		t.Error("Expected error from failed ping")
	}
}

func TestClient_CallServiceWithResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify return_response parameter
		if r.URL.Query().Get("return_response") != "true" {
			t.Errorf("Expected return_response=true query parameter")
		}

		// Verify it's a POST request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Return mock weather forecast data
		resp := map[string]interface{}{
			"weather.home": map[string]interface{}{
				"forecast": []map[string]interface{}{
					{
						"datetime":    "2024-01-01T12:00:00",
						"temperature": 20.5,
						"condition":   "sunny",
					},
					{
						"datetime":    "2024-01-02T12:00:00",
						"temperature": 18.0,
						"condition":   "cloudy",
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

	// Test weather.get_forecasts
	result, err := client.CallServiceWithResponse(context.Background(), ServiceCallRequest{
		Domain:  "weather",
		Service: "get_forecasts",
		Data: map[string]interface{}{
			"type":      "daily",
			"entity_id": []string{"weather.home"},
		},
	})

	if err != nil {
		t.Errorf("CallServiceWithResponse() error = %v", err)
	}
	if result == nil {
		t.Fatal("CallServiceWithResponse() returned nil result")
	}

	// Verify result contains forecast data
	weatherData, ok := result["weather.home"].(map[string]interface{})
	if !ok {
		t.Fatal("Result doesn't contain weather.home data")
	}

	forecast, ok := weatherData["forecast"].([]interface{})
	if !ok {
		t.Fatal("Result doesn't contain forecast array")
	}

	if len(forecast) != 2 {
		t.Errorf("Expected 2 forecast items, got %d", len(forecast))
	}
}

func TestClient_CallServiceWithResponse_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Message: "Invalid entity_id",
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test-token",
		Logger:  zap.NewNop(),
	})

	result, err := client.CallServiceWithResponse(context.Background(), ServiceCallRequest{
		Domain:  "weather",
		Service: "get_forecasts",
		Data:    map[string]interface{}{},
	})

	if err == nil {
		t.Error("Expected error from failed service call")
	}
	if result != nil {
		t.Error("Expected nil result on error")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && s[len(s)-len(substr):] == substr || findInString(s, substr)
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
