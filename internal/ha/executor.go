package ha

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"go.uber.org/zap"
)

// cacheEntry stores a cached tool call result with expiration
type cacheEntry struct {
	result    *backend.ToolResult
	expiresAt time.Time
}

// Executor implements the pipeline.ToolExecutor interface for Home Assistant.
type Executor struct {
	client    *Client
	allowList map[string]bool // Map of allowed service calls (e.g., "light.turn_on" -> true)
	logger    *zap.Logger

	// Result caching to prevent duplicate calls
	cache     map[string]*cacheEntry
	cacheMu   sync.RWMutex
	cacheTTL  time.Duration
	cacheHits int64
	cacheMiss int64
}

// ExecutorConfig holds configuration for the HA executor.
type ExecutorConfig struct {
	Client    *Client
	AllowList []string // List of allowed services (e.g., ["light.*", "switch.turn_on"])
	Logger    *zap.Logger
}

// NewExecutor creates a new Home Assistant tool executor.
func NewExecutor(cfg ExecutorConfig) (*Executor, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("HA client is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Build allow list map
	allowList := make(map[string]bool)
	for _, service := range cfg.AllowList {
		allowList[service] = true
	}

	// If no allow list provided, allow common safe services by default
	if len(allowList) == 0 {
		defaultServices := []string{
			"light.turn_on",
			"light.turn_off",
			"light.toggle",
			"switch.turn_on",
			"switch.turn_off",
			"switch.toggle",
			"cover.open_cover",
			"cover.close_cover",
			"cover.stop_cover",
			"climate.set_temperature",
			"climate.set_hvac_mode",
			"fan.turn_on",
			"fan.turn_off",
			"media_player.media_play",
			"media_player.media_pause",
			"media_player.volume_set",
			"homeassistant.get_state",
		}
		for _, service := range defaultServices {
			allowList[service] = true
		}
		logger.Info("using default allow list", zap.Int("services", len(allowList)))
	}

	executor := &Executor{
		client:    cfg.Client,
		allowList: allowList,
		logger:    logger.With(zap.String("component", "ha_executor")),
		cache:     make(map[string]*cacheEntry),
		cacheTTL:  5 * time.Second, // 5-second cache window to prevent duplicate calls
	}

	// Start cache cleanup goroutine
	go executor.cleanupCache()

	// Debug: Log the list of available actions (allow list)
	if logger.Core().Enabled(zap.DebugLevel) {
		allowedServices := make([]string, 0, len(allowList))
		for service := range allowList {
			allowedServices = append(allowedServices, service)
		}
		executor.logger.Debug("executor allow list configured",
			zap.Strings("allowed_services", allowedServices),
			zap.Int("total_allowed", len(allowedServices)))
	}

	return executor, nil
}

// Execute executes a tool call from the LLM.
func (e *Executor) Execute(ctx context.Context, call *backend.ToolCall) (*backend.ToolResult, error) {
	e.logger.Info("executing tool call",
		zap.String("tool_id", call.ID),
		zap.String("tool_name", call.Name),
		zap.Any("arguments", call.Arguments))

	// Check cache for duplicate calls within time window
	cacheKey := e.generateCacheKey(call)
	if cachedResult := e.getFromCache(cacheKey); cachedResult != nil {
		e.logger.Info("returning cached result",
			zap.String("tool_id", call.ID),
			zap.String("tool_name", call.Name),
			zap.String("cache_key", cacheKey))

		// Create new result with updated call ID
		result := *cachedResult
		result.CallID = call.ID
		return &result, nil
	}

	// Parse tool name (format: "domain.service" or just service name)
	domain, service, err := parseToolName(call.Name)
	if err != nil {
		return nil, fmt.Errorf("invalid tool name: %w", err)
	}

	// Check allow list
	fullServiceName := fmt.Sprintf("%s.%s", domain, service)
	if !e.isAllowed(fullServiceName) {
		e.logger.Warn("service call denied by allow list",
			zap.String("service", fullServiceName))
		return &backend.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("service %s is not allowed", fullServiceName),
		}, nil
	}

	// Determine if this is a state query or service call
	var result *backend.ToolResult
	if service == "get_state" {
		result, err = e.executeStateQuery(ctx, call)
	} else {
		result, err = e.executeServiceCall(ctx, call, domain, service)
	}

	// Cache successful results (but not errors)
	if err == nil && result != nil && result.Error == "" {
		e.storeInCache(cacheKey, result)
	}

	return result, err
}

// executeServiceCall executes a Home Assistant service call.
func (e *Executor) executeServiceCall(ctx context.Context, call *backend.ToolCall, domain, service string) (*backend.ToolResult, error) {
	// Check if this service returns data (requires return_response=true)
	fullServiceName := fmt.Sprintf("%s.%s", domain, service)
	if e.isResponseReturningService(fullServiceName) {
		return e.executeServiceCallWithResponse(ctx, call, domain, service)
	}

	// Normalize parameters (fix common LLM mistakes)
	normalizedArgs := e.normalizeParameters(call.Arguments, domain)

	// Build service call request
	req := ServiceCallRequest{
		Domain:  domain,
		Service: service,
		Data:    normalizedArgs,
	}

	// Call Home Assistant
	resp, err := e.client.CallService(ctx, req)
	if err != nil {
		e.logger.Error("service call failed",
			zap.String("tool_id", call.ID),
			zap.String("service", fmt.Sprintf("%s.%s", domain, service)),
			zap.Error(err))
		return &backend.ToolResult{
			CallID:   call.ID,
			Error:    err.Error(),
			Metadata: e.createToolResultMetadata(domain, service, false),
		}, nil
	}

	e.logger.Info("service call succeeded",
		zap.String("tool_id", call.ID),
		zap.String("service", fmt.Sprintf("%s.%s", domain, service)))

	// Create clear success message for the LLM
	resultMessage := fmt.Sprintf("Successfully executed %s.%s", domain, service)
	if entityID, ok := call.Arguments["entity_id"].(string); ok {
		resultMessage = fmt.Sprintf("Successfully executed %s.%s on %s", domain, service, entityID)
	}

	return &backend.ToolResult{
		CallID: call.ID,
		Result: map[string]any{
			"success":    true,
			"message":    resultMessage,
			"context_id": resp.Context.ID,
		},
		Metadata: e.createToolResultMetadata(domain, service, true),
	}, nil
}

// executeServiceCallWithResponse executes services that return data.
func (e *Executor) executeServiceCallWithResponse(ctx context.Context, call *backend.ToolCall, domain, service string) (*backend.ToolResult, error) {
	// Normalize parameters (fix common LLM mistakes)
	normalizedArgs := e.normalizeParameters(call.Arguments, domain)

	// Build service call request
	req := ServiceCallRequest{
		Domain:  domain,
		Service: service,
		Data:    normalizedArgs,
	}

	// Call Home Assistant with response
	responseData, err := e.client.CallServiceWithResponse(ctx, req)
	if err != nil {
		e.logger.Error("service call with response failed",
			zap.String("tool_id", call.ID),
			zap.String("service", fmt.Sprintf("%s.%s", domain, service)),
			zap.Error(err))
		return &backend.ToolResult{
			CallID:   call.ID,
			Error:    err.Error(),
			Metadata: e.createToolResultMetadata(domain, service, false),
		}, nil
	}

	e.logger.Info("service call with response succeeded",
		zap.String("tool_id", call.ID),
		zap.String("service", fmt.Sprintf("%s.%s", domain, service)))

	return &backend.ToolResult{
		CallID:   call.ID,
		Result:   responseData,
		Metadata: e.createToolResultMetadata(domain, service, true),
	}, nil
}

// isResponseReturningService checks if a service returns data.
func (e *Executor) isResponseReturningService(serviceName string) bool {
	// Services that return data instead of just executing actions
	responseServices := map[string]bool{
		"weather.get_forecasts":    true,
		"todo.get_items":           true,
		"calendar.get_events":      true,
		"logbook.get_events":       true,
		"conversation.process":     true,
		"image_processing.scan":    true,
		"cloud.remote_connect":     true,
		"camera.get_image":         true,
		"homeassistant.get_config": true,
	}
	return responseServices[serviceName]
}

// executeStateQuery queries the state of an entity.
func (e *Executor) executeStateQuery(ctx context.Context, call *backend.ToolCall) (*backend.ToolResult, error) {
	// Extract entity_id from arguments
	entityID, ok := call.Arguments["entity_id"].(string)
	if !ok {
		return &backend.ToolResult{
			CallID:   call.ID,
			Error:    "entity_id is required for state queries",
			Metadata: e.createToolResultMetadata("state", "query", false),
		}, nil
	}

	// Get state from Home Assistant
	state, err := e.client.GetState(ctx, entityID)
	if err != nil {
		e.logger.Error("state query failed",
			zap.String("tool_id", call.ID),
			zap.String("entity_id", entityID),
			zap.Error(err))
		return &backend.ToolResult{
			CallID:   call.ID,
			Error:    err.Error(),
			Metadata: e.createToolResultMetadata("state", "query", false),
		}, nil
	}

	e.logger.Info("state query succeeded",
		zap.String("tool_id", call.ID),
		zap.String("entity_id", entityID),
		zap.String("state", state.State))

	return &backend.ToolResult{
		CallID: call.ID,
		Result: map[string]any{
			"entity_id":  state.EntityID,
			"state":      state.State,
			"attributes": state.Attributes,
		},
		Metadata: e.createToolResultMetadata("state", "query", true),
	}, nil
}

// isAllowed checks if a service call is allowed based on the allow list.
func (e *Executor) isAllowed(serviceName string) bool {
	// Exact match
	if e.allowList[serviceName] {
		return true
	}

	// Wildcard match (e.g., "light.*" matches "light.turn_on")
	parts := strings.Split(serviceName, ".")
	if len(parts) == 2 {
		wildcard := parts[0] + ".*"
		if e.allowList[wildcard] {
			return true
		}
	}

	// Match "*.service" (e.g., "*.turn_on")
	if len(parts) == 2 {
		wildcard := "*." + parts[1]
		if e.allowList[wildcard] {
			return true
		}
	}

	return false
}

// parseToolName parses a tool name into domain and service.
// Supports formats:
// - "domain.service" (e.g., "light.turn_on")
// - "service" (assumes "homeassistant" domain)
func parseToolName(toolName string) (domain, service string, err error) {
	parts := strings.Split(toolName, ".")

	switch len(parts) {
	case 1:
		// Just service name, use default domain
		return "homeassistant", parts[0], nil
	case 2:
		// Domain.service format
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("invalid tool name format: %s", toolName)
	}
}

// GetAllowList returns the current allow list.
func (e *Executor) GetAllowList() []string {
	services := make([]string, 0, len(e.allowList))
	for service := range e.allowList {
		services = append(services, service)
	}
	return services
}

// AddToAllowList adds a service to the allow list.
func (e *Executor) AddToAllowList(service string) {
	e.allowList[service] = true
	e.logger.Info("added service to allow list", zap.String("service", service))
}

// RemoveFromAllowList removes a service from the allow list.
func (e *Executor) RemoveFromAllowList(service string) {
	delete(e.allowList, service)
	e.logger.Info("removed service from allow list", zap.String("service", service))
}

// determineResponseTiming determines when the backend should provide feedback after tool execution.
// Returns generic timing characteristics that backends can interpret according to their needs.
func (e *Executor) determineResponseTiming(domain, service string, success bool) map[string]any {
	// Error responses need immediate feedback
	if !success {
		return map[string]any{
			"urgent":    true,
			"interrupt": true,
		}
	}

	// Data retrieval services are blocking, so timing doesn't apply
	fullServiceName := fmt.Sprintf("%s.%s", domain, service)
	if e.isResponseReturningService(fullServiceName) {
		return nil // Blocking services don't need timing hints
	}

	// Check if this is a critical operation
	criticalServices := map[string]bool{
		"alarm_control_panel.alarm_arm_away": true,
		"alarm_control_panel.alarm_arm_home": true,
		"alarm_control_panel.alarm_disarm":   true,
		"lock.lock":                          true,
		"lock.unlock":                        true,
	}

	if criticalServices[fullServiceName] {
		return map[string]any{
			"critical":  true,
			"interrupt": true,
		}
	}

	// Most successful operations can wait for natural conversation flow
	return map[string]any{
		"deferrable": true,
	}
}

// createToolResultMetadata creates metadata with generic response timing hints
func (e *Executor) createToolResultMetadata(domain, service string, success bool) map[string]any {
	return e.determineResponseTiming(domain, service, success)
}

// normalizeParameters fixes common LLM parameter mistakes by mapping incorrect
// parameter names to correct ones expected by Home Assistant.
func (e *Executor) normalizeParameters(args map[string]any, domain string) map[string]any {
	if args == nil {
		return nil
	}

	normalized := make(map[string]any, len(args))
	maps.Copy(normalized, args)

	// Common todo service mistake: LLMs often use 'list_id' instead of 'entity_id'
	if domain == "todo" {
		if listID, hasListID := normalized["list_id"]; hasListID {
			// Log the correction
			e.logger.Warn("correcting parameter name",
				zap.String("domain", domain),
				zap.String("from", "list_id"),
				zap.String("to", "entity_id"),
				zap.Any("value", listID))

			// Move list_id -> entity_id
			normalized["entity_id"] = listID
			delete(normalized, "list_id")
		}
	}

	return normalized
}

// generateCacheKey creates a unique cache key for a tool call based on name and arguments
func (e *Executor) generateCacheKey(call *backend.ToolCall) string {
	// Serialize arguments to JSON for consistent hashing
	argsJSON, err := json.Marshal(call.Arguments)
	if err != nil {
		// If serialization fails, use a non-cacheable key
		e.logger.Warn("failed to serialize arguments for cache key",
			zap.String("tool_name", call.Name),
			zap.Error(err))
		return fmt.Sprintf("uncacheable_%s_%d", call.Name, time.Now().UnixNano())
	}

	// Create hash from tool name + arguments
	hash := sha256.Sum256([]byte(call.Name + string(argsJSON)))
	return fmt.Sprintf("%x", hash[:16]) // Use first 16 bytes for shorter key
}

// getFromCache retrieves a cached result if it exists and hasn't expired
func (e *Executor) getFromCache(key string) *backend.ToolResult {
	e.cacheMu.RLock()
	defer e.cacheMu.RUnlock()

	entry, exists := e.cache[key]
	if !exists {
		e.cacheMiss++
		return nil
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		e.cacheMiss++
		return nil
	}

	e.cacheHits++
	e.logger.Debug("cache hit",
		zap.String("cache_key", key),
		zap.Int64("total_hits", e.cacheHits),
		zap.Int64("total_misses", e.cacheMiss))

	return entry.result
}

// storeInCache stores a result in the cache with expiration
func (e *Executor) storeInCache(key string, result *backend.ToolResult) {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()

	e.cache[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(e.cacheTTL),
	}

	e.logger.Debug("cached result",
		zap.String("cache_key", key),
		zap.Duration("ttl", e.cacheTTL),
		zap.Int("cache_size", len(e.cache)))
}

// cleanupCache periodically removes expired cache entries
func (e *Executor) cleanupCache() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		e.cacheMu.Lock()
		now := time.Now()
		removed := 0

		for key, entry := range e.cache {
			if now.After(entry.expiresAt) {
				delete(e.cache, key)
				removed++
			}
		}

		if removed > 0 {
			e.logger.Debug("cleaned up expired cache entries",
				zap.Int("removed", removed),
				zap.Int("remaining", len(e.cache)))
		}

		e.cacheMu.Unlock()
	}
}

// GetCacheStats returns cache hit/miss statistics
func (e *Executor) GetCacheStats() (hits int64, misses int64, size int) {
	e.cacheMu.RLock()
	defer e.cacheMu.RUnlock()
	return e.cacheHits, e.cacheMiss, len(e.cache)
}
