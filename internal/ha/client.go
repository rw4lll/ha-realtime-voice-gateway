package ha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Client handles communication with Home Assistant REST API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	logger     *zap.Logger
}

// ClientConfig holds configuration for the HA client.
type ClientConfig struct {
	BaseURL string        // Home Assistant URL (e.g., "http://homeassistant.local:8123")
	Token   string        // Long-lived access token
	Timeout time.Duration // HTTP request timeout
	Logger  *zap.Logger
}

// ServiceCallRequest represents a Home Assistant service call.
type ServiceCallRequest struct {
	Domain  string                 `json:"-"`                   // e.g., "light"
	Service string                 `json:"-"`                   // e.g., "turn_on"
	Data    map[string]interface{} `json:"entity_id,omitempty"` // Service data (entity_id, brightness, etc.)
}

// ServiceCallResponse represents the response from a service call.
type ServiceCallResponse struct {
	Context struct {
		ID       string `json:"id"`
		ParentID string `json:"parent_id"`
		UserID   string `json:"user_id"`
	} `json:"context"`
}

// State represents an entity's current state.
type State struct {
	EntityID    string                 `json:"entity_id"`
	State       string                 `json:"state"`
	Attributes  map[string]interface{} `json:"attributes"`
	LastChanged time.Time              `json:"last_changed"`
	LastUpdated time.Time              `json:"last_updated"`
}

// ErrorResponse represents an HA API error.
type ErrorResponse struct {
	Message string `json:"message"`
}

// NewClient creates a new Home Assistant client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("access token is required")
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Client{
		baseURL: cfg.BaseURL,
		token:   cfg.Token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger.With(zap.String("component", "ha_client")),
	}, nil
}

// CallService calls a Home Assistant service.
func (c *Client) CallService(ctx context.Context, req ServiceCallRequest) (*ServiceCallResponse, error) {
	if req.Domain == "" {
		return nil, fmt.Errorf("service domain is required")
	}
	if req.Service == "" {
		return nil, fmt.Errorf("service name is required")
	}

	url := fmt.Sprintf("%s/api/services/%s/%s", c.baseURL, req.Domain, req.Service)

	c.logger.Debug("calling HA service",
		zap.String("domain", req.Domain),
		zap.String("service", req.Service),
		zap.Any("data", req.Data))

	jsonData, err := json.Marshal(req.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != "" {
			return nil, fmt.Errorf("HA API error: %s", errResp.Message)
		}
		return nil, fmt.Errorf("HA API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var serviceResp []ServiceCallResponse
	if err := json.Unmarshal(body, &serviceResp); err != nil {
		// Some services return empty response
		if len(body) == 0 || string(body) == "null" {
			c.logger.Debug("service call succeeded with empty response")
			return &ServiceCallResponse{}, nil
		}
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(serviceResp) == 0 {
		return &ServiceCallResponse{}, nil
	}

	c.logger.Debug("service call succeeded",
		zap.String("context_id", serviceResp[0].Context.ID))

	return &serviceResp[0], nil
}

// CallServiceWithResponse calls a Home Assistant service that returns data.
// This is used for services like weather.get_forecasts, todo.get_items, etc.
func (c *Client) CallServiceWithResponse(ctx context.Context, req ServiceCallRequest) (map[string]interface{}, error) {
	if req.Domain == "" {
		return nil, fmt.Errorf("service domain is required")
	}
	if req.Service == "" {
		return nil, fmt.Errorf("service name is required")
	}

	// Add return_response=true query parameter
	url := fmt.Sprintf("%s/api/services/%s/%s?return_response=true", c.baseURL, req.Domain, req.Service)

	c.logger.Debug("calling HA service with response",
		zap.String("domain", req.Domain),
		zap.String("service", req.Service),
		zap.Any("data", req.Data))

	jsonData, err := json.Marshal(req.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != "" {
			return nil, fmt.Errorf("HA API error: %s", errResp.Message)
		}
		return nil, fmt.Errorf("HA API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response data
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Debug("service call with response succeeded",
		zap.Int("response_size", len(body)))

	return result, nil
}

// GetState retrieves the current state of an entity.
func (c *Client) GetState(ctx context.Context, entityID string) (*State, error) {
	if entityID == "" {
		return nil, fmt.Errorf("entity ID is required")
	}

	url := fmt.Sprintf("%s/api/states/%s", c.baseURL, entityID)

	c.logger.Debug("getting entity state", zap.String("entity_id", entityID))

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("entity not found: %s", entityID)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != "" {
			return nil, fmt.Errorf("HA API error: %s", errResp.Message)
		}
		return nil, fmt.Errorf("HA API error: status %d", resp.StatusCode)
	}

	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	c.logger.Debug("got entity state",
		zap.String("entity_id", entityID),
		zap.String("state", state.State))

	return &state, nil
}

// GetStates retrieves all entity states.
func (c *Client) GetStates(ctx context.Context) ([]State, error) {
	url := fmt.Sprintf("%s/api/states", c.baseURL)

	c.logger.Debug("getting all entity states")

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HA API error: status %d", resp.StatusCode)
	}

	var states []State
	if err := json.Unmarshal(body, &states); err != nil {
		return nil, fmt.Errorf("failed to unmarshal states: %w", err)
	}

	c.logger.Debug("got all states", zap.Int("count", len(states)))

	return states, nil
}

// ServiceInfo represents a Home Assistant service.
type ServiceInfo struct {
	Domain   string                   `json:"domain"`
	Services map[string]ServiceDetail `json:"services"`
}

// ServiceDetail contains details about a specific service.
type ServiceDetail struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Fields      map[string]FieldDetail `json:"fields"`
	Target      interface{}            `json:"target"`
}

// FieldDetail contains details about a service field/parameter.
type FieldDetail struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Example     interface{} `json:"example"`
	Required    bool        `json:"required"`
	Selector    interface{} `json:"selector"`
}

// GetServices retrieves all available services from Home Assistant.
func (c *Client) GetServices(ctx context.Context) ([]ServiceInfo, error) {
	url := fmt.Sprintf("%s/api/services", c.baseURL)

	c.logger.Debug("getting all services")

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HA API error: status %d", resp.StatusCode)
	}

	var services []ServiceInfo
	if err := json.Unmarshal(body, &services); err != nil {
		return nil, fmt.Errorf("failed to unmarshal services: %w", err)
	}

	c.logger.Debug("got all services", zap.Int("domains", len(services)))

	return services, nil
}

// Ping checks if Home Assistant is reachable.
func (c *Client) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/", c.baseURL)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to ping HA: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HA ping failed: status %d", resp.StatusCode)
	}

	c.logger.Debug("HA ping successful")
	return nil
}
