package ha

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"go.uber.org/zap"
)

// AutoDiscoveryConfig holds configuration for autodiscovery.
type AutoDiscoveryConfig struct {
	Client            *Client
	AllowedDomains    []string // If empty, allows all domains
	DeniedDomains     []string // Domains to explicitly exclude
	AllowedServices   []string // Specific services to allow (e.g., "light.turn_on")
	IncludeAttributes bool     // Include entity attributes in system prompt
	Logger            *zap.Logger
}

// AutoDiscovery handles automatic discovery of HA services and entities.
type AutoDiscovery struct {
	client            *Client
	allowedDomains    map[string]bool
	deniedDomains     map[string]bool
	allowedServices   map[string]bool
	includeAttributes bool
	logger            *zap.Logger
}

// NewAutoDiscovery creates a new autodiscovery instance.
func NewAutoDiscovery(cfg AutoDiscoveryConfig) (*AutoDiscovery, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("HA client is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Build allowed domains map
	allowedDomains := make(map[string]bool)
	for _, domain := range cfg.AllowedDomains {
		allowedDomains[domain] = true
	}

	// Build denied domains map
	deniedDomains := make(map[string]bool)
	for _, domain := range cfg.DeniedDomains {
		deniedDomains[domain] = true
	}

	// Build allowed services map
	allowedServices := make(map[string]bool)
	for _, service := range cfg.AllowedServices {
		allowedServices[service] = true
	}

	return &AutoDiscovery{
		client:            cfg.Client,
		allowedDomains:    allowedDomains,
		deniedDomains:     deniedDomains,
		allowedServices:   allowedServices,
		includeAttributes: cfg.IncludeAttributes,
		logger:            logger.With(zap.String("component", "ha_autodiscovery")),
	}, nil
}

// DiscoverTools discovers and generates tool definitions from Home Assistant services.
func (a *AutoDiscovery) DiscoverTools(ctx context.Context) ([]backend.Tool, error) {
	a.logger.Info("discovering Home Assistant services")

	// Fetch all services from HA
	services, err := a.client.GetServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %w", err)
	}

	var tools []backend.Tool
	serviceCount := 0

	for _, domainInfo := range services {
		domain := domainInfo.Domain

		// Check if domain is allowed
		if !a.isDomainAllowed(domain) {
			a.logger.Debug("skipping domain",
				zap.String("domain", domain),
				zap.String("reason", "not in allowed list or in denied list"))
			continue
		}

		// Generate tools for each service in the domain
		for serviceName, serviceDetail := range domainInfo.Services {
			fullServiceName := fmt.Sprintf("%s.%s", domain, serviceName)

			// Check specific service allow list (with wildcard support)
			if len(a.allowedServices) > 0 && !a.isServiceAllowed(fullServiceName) {
				a.logger.Debug("skipping service",
					zap.String("service", fullServiceName),
					zap.String("reason", "not in allowed services list"))
				continue
			}

			tool := a.generateToolFromService(domain, serviceName, serviceDetail)
			tools = append(tools, tool)
			serviceCount++
		}
	}

	a.logger.Info("autodiscovery completed",
		zap.Int("domains_found", len(services)),
		zap.Int("tools_generated", serviceCount))

	// Debug: Log the list of discovered tools
	if a.logger.Core().Enabled(zap.DebugLevel) {
		toolNames := make([]string, 0, len(tools))
		for _, tool := range tools {
			toolNames = append(toolNames, tool.Name)
		}
		a.logger.Debug("autodiscovered tools list",
			zap.Strings("tools", toolNames))
	}

	return tools, nil
}

// DiscoverEntities discovers all entities and returns them organized by domain.
func (a *AutoDiscovery) DiscoverEntities(ctx context.Context) (map[string][]State, error) {
	a.logger.Info("discovering Home Assistant entities")

	// Fetch all entity states
	states, err := a.client.GetStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get states: %w", err)
	}

	// Organize by domain
	entitiesByDomain := make(map[string][]State)
	for _, state := range states {
		domain := getDomainFromEntityID(state.EntityID)

		// Check if domain is allowed
		if !a.isDomainAllowed(domain) {
			continue
		}

		entitiesByDomain[domain] = append(entitiesByDomain[domain], state)
	}

	a.logger.Info("entity discovery completed",
		zap.Int("total_entities", len(states)),
		zap.Int("domains", len(entitiesByDomain)))

	return entitiesByDomain, nil
}

// GenerateSystemPrompt generates a system prompt with discovered entities.
func (a *AutoDiscovery) GenerateSystemPrompt(ctx context.Context, basePrompt string) (string, error) {
	a.logger.Info("generating system prompt with entity list")

	entities, err := a.DiscoverEntities(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to discover entities: %w", err)
	}

	var builder strings.Builder

	// IMPORTANT: Put critical tool schema information FIRST (LLMs pay more attention to beginning)
	builder.WriteString("CRITICAL TOOL CALLING RULES:\n")
	builder.WriteString("=" + strings.Repeat("=", 50) + "\n")
	builder.WriteString("1. Use EXACT parameter names from tool schemas - do NOT invent variations\n")
	builder.WriteString("2. All domain services use 'entity_id' parameter (NOT 'todo_list_entity_id', 'target_id', etc.)\n")
	builder.WriteString("3. Validate parameter names match the schema before calling\n\n")

	builder.WriteString("SENSOR ENTITIES (READ-ONLY):\n")
	builder.WriteString("=" + strings.Repeat("=", 50) + "\n")
	builder.WriteString("- Sensor entities (sensor.*) are READ-ONLY - they do NOT have callable services\n")
	builder.WriteString("- When users ask about sensor values (temperature, humidity, air quality, etc.), describe the current state from the entity list\n")
	builder.WriteString("- DO NOT try to call services on sensor entities (e.g., sensor.temperature, sensor.pm2_5, sensor.air_quality)\n")
	builder.WriteString("- Sensors only provide data - you can read their state but cannot control them\n\n")

	builder.WriteString("WEATHER SERVICE LIMITATIONS:\n")
	builder.WriteString("=" + strings.Repeat("=", 50) + "\n")
	builder.WriteString("- Weather services ONLY provide forecasts for configured local weather entities\n")
	builder.WriteString("- You CANNOT get weather for arbitrary cities (e.g., Berlin, Tokyo) unless there's a weather entity for that location\n")
	builder.WriteString("- When users ask for weather in other cities, explain that you can only provide local weather from available entities\n")
	builder.WriteString("- weather.get_forecasts REQUIRES TWO parameters:\n")
	builder.WriteString("  1. 'type': MUST be 'daily', 'hourly', or 'twice_daily' (use 'daily' by default)\n")
	builder.WriteString("  2. 'entity_id': array of weather entity IDs (e.g., [\"weather.home\"])\n")
	builder.WriteString("  Example call: {\"type\": \"daily\", \"entity_id\": [\"weather.home\"]}\n\n")

	// Add base prompt
	if basePrompt != "" {
		builder.WriteString(basePrompt)
		builder.WriteString("\n\n")
	}

	// Add entity information
	builder.WriteString("Available Home Assistant Entities:\n")
	builder.WriteString("=" + strings.Repeat("=", 50) + "\n\n")

	// Sort domains for consistent output
	domains := make([]string, 0, len(entities))
	for domain := range entities {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	for _, domain := range domains {
		entityList := entities[domain]
		builder.WriteString(fmt.Sprintf("%s (%d entities):\n", strings.Title(domain), len(entityList)))

		// Sort entities within domain
		sort.Slice(entityList, func(i, j int) bool {
			return entityList[i].EntityID < entityList[j].EntityID
		})

		for _, entity := range entityList {
			// Get friendly name from attributes
			friendlyName := entity.EntityID
			if name, ok := entity.Attributes["friendly_name"].(string); ok && name != "" {
				friendlyName = name
			}

			builder.WriteString(fmt.Sprintf("  - %s", entity.EntityID))
			if friendlyName != entity.EntityID {
				builder.WriteString(fmt.Sprintf(" (%s)", friendlyName))
			}
			builder.WriteString(fmt.Sprintf(" [%s]", entity.State))

			// Optionally include relevant attributes
			if a.includeAttributes {
				if attrs := a.formatRelevantAttributes(entity.Attributes); attrs != "" {
					builder.WriteString(fmt.Sprintf(" - %s", attrs))
				}
			}

			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("\nWhen users refer to rooms or devices, use these exact entity IDs.\n")
	builder.WriteString("Always verify the entity exists before calling a service.\n")

	// Add schema guidance for common service parameters
	a.logger.Info("adding schema guidance to system prompt")
	if err := a.addSchemaGuidance(ctx, &builder); err != nil {
		a.logger.Warn("failed to add schema guidance", zap.Error(err))
		// Continue without schema guidance - it's not critical
	} else {
		a.logger.Info("schema guidance added successfully")
	}

	prompt := builder.String()
	a.logger.Info("system prompt generated",
		zap.Int("prompt_length", len(prompt)),
		zap.Int("domains", len(domains)))

	return prompt, nil
}

// addSchemaGuidance adds parameter guidance for commonly used services to help LLMs use correct parameters.
func (a *AutoDiscovery) addSchemaGuidance(ctx context.Context, builder *strings.Builder) error {
	// Fetch services to get their schemas
	servicesList, err := a.client.GetServices(ctx)
	if err != nil {
		return fmt.Errorf("failed to get services: %w", err)
	}

	// Convert to map for easier lookup
	servicesMap := make(map[string]map[string]ServiceDetail)
	for _, info := range servicesList {
		servicesMap[info.Domain] = info.Services
	}

	builder.WriteString("\n")
	builder.WriteString("Service Parameter Reference:\n")
	builder.WriteString("=" + strings.Repeat("=", 50) + "\n\n")

	// Define which services to include detailed schemas for
	guidanceServices := map[string][]string{
		"fan":          {"turn_on", "turn_off", "set_percentage", "set_preset_mode"},
		"light":        {"turn_on", "turn_off"},
		"climate":      {"set_temperature", "set_hvac_mode"},
		"media_player": {"volume_set", "media_play_pause"},
		"cover":        {"set_cover_position"},
		"todo":         {"add_item", "update_item", "remove_item"},
	}

	// Track if we found any services to document
	foundAny := false

	a.logger.Info("checking guidance services", zap.Int("domains", len(guidanceServices)))

	for domain, serviceNames := range guidanceServices {
		domainServices, ok := servicesMap[domain]
		if !ok {
			a.logger.Info("domain not found in services", zap.String("domain", domain))
			continue
		}

		a.logger.Info("found domain services", zap.String("domain", domain), zap.Int("service_count", len(domainServices)))

		for _, serviceName := range serviceNames {
			detail, ok := domainServices[serviceName]
			if !ok {
				a.logger.Info("service not found", zap.String("service", fmt.Sprintf("%s.%s", domain, serviceName)))
				continue
			}

			a.logger.Info("found service", zap.String("service", fmt.Sprintf("%s.%s", domain, serviceName)), zap.Int("fields", len(detail.Fields)))

			// Only include if there are actual fields to document
			if len(detail.Fields) == 0 {
				continue
			}

			if !foundAny {
				foundAny = true
			}

			fullName := fmt.Sprintf("%s.%s", domain, serviceName)
			builder.WriteString(fmt.Sprintf("Service: %s\n", fullName))
			if detail.Description != "" {
				builder.WriteString(fmt.Sprintf("  Description: %s\n", detail.Description))
			}
			builder.WriteString("  Parameters:\n")

			// Sort fields for consistent output
			fieldNames := make([]string, 0, len(detail.Fields))
			for name := range detail.Fields {
				fieldNames = append(fieldNames, name)
			}
			sort.Strings(fieldNames)

			for _, fieldName := range fieldNames {
				field := detail.Fields[fieldName]

				builder.WriteString(fmt.Sprintf("    - %s", fieldName))

				// Add type information
				if field.Selector != nil {
					// Type assert selector to map
					if selectorMap, ok := field.Selector.(map[string]any); ok {
						// Infer type from selector
						if selector, ok := selectorMap["number"]; ok {
							builder.WriteString(" (number")
							if numberMap, ok := selector.(map[string]any); ok {
								if min, ok := numberMap["min"]; ok {
									if max, ok := numberMap["max"]; ok {
										builder.WriteString(fmt.Sprintf(", range: %.0f-%.0f", min, max))
									}
								}
							}
							builder.WriteString(")")
						} else if _, ok := selectorMap["text"]; ok {
							builder.WriteString(" (string)")
						} else if _, ok := selectorMap["boolean"]; ok {
							builder.WriteString(" (boolean)")
						}
					}
				}

				// Add description
				if field.Description != "" {
					builder.WriteString(fmt.Sprintf(": %s", field.Description))
				}

				// Add example if available
				if field.Example != nil {
					builder.WriteString(fmt.Sprintf(" Example: %v", field.Example))
				}

				builder.WriteString("\n")
			}
			builder.WriteString("\n")
		}
	}

	if foundAny {
		builder.WriteString("Remember: Use only the parameters listed above for each service.\n")
		builder.WriteString("If a parameter is not listed, do not include it in your function call.\n\n")
	}

	return nil
}

// generateToolFromService creates a tool definition from a HA service.
func (a *AutoDiscovery) generateToolFromService(domain, serviceName string, detail ServiceDetail) backend.Tool {
	fullName := fmt.Sprintf("%s.%s", domain, serviceName)

	// Use HA's description or generate a natural action-oriented one
	description := detail.Description
	if description == "" {
		// Generate natural descriptions based on service patterns
		description = a.generateNaturalDescription(domain, serviceName)
	}

	// Special override for weather.get_forecasts to emphasize type parameter
	if fullName == "weather.get_forecasts" {
		description = "Get weather forecast. REQUIRES 'type' parameter ('daily', 'hourly', or 'twice_daily') AND 'entity_id' array. Example: {\"type\": \"daily\", \"entity_id\": [\"weather.home\"]}"
	}

	// Build parameters from service fields
	parameters := map[string]any{
		"type":                 "object",
		"properties":           make(map[string]any),
		"required":             []string{},
		"additionalProperties": false, // Reject any parameters not in the schema
	}

	properties := parameters["properties"].(map[string]any)
	required := []string{}

	// Add entity_id as default parameter (most services need it)
	// Special case: weather.get_forecasts accepts array of entity IDs
	if fullName == "weather.get_forecasts" {
		properties["entity_id"] = map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "List of weather entity IDs. MUST use entity IDs from the weather entities in your available entities list. Cannot get weather for arbitrary cities.",
		}
	} else if fullName == "todo.get_items" {
		properties["entity_id"] = map[string]any{
			"type":        "string",
			"description": fmt.Sprintf("REQUIRED: Use parameter name 'entity_id' (NOT 'list_id'). The todo entity ID (e.g., 'todo.shopping_list'). Use exact entity ID from available %s entities.", domain),
		}
	} else if domain == "todo" {
		// For other todo services (add_item, update_item, remove_item)
		properties["entity_id"] = map[string]any{
			"type":        "string",
			"description": fmt.Sprintf("REQUIRED: Use parameter name 'entity_id' (NOT 'list_id'). The todo entity ID (e.g., 'todo.shopping_list'). Use exact entity ID from available %s entities.", domain),
		}
	} else {
		properties["entity_id"] = map[string]any{
			"type":        "string",
			"description": fmt.Sprintf("The %s entity ID to control. Must match an entity from the available %s entities list (e.g., '%s.bedroom_light').", domain, domain, domain),
		}
	}

	// Add service-specific fields
	for fieldName, fieldDetail := range detail.Fields {
		fieldSchema := a.convertFieldToSchema(fieldName, fieldDetail)

		// Special enhancement for weather.get_forecasts type parameter
		if fullName == "weather.get_forecasts" && fieldName == "type" {
			fieldSchema["description"] = "REQUIRED: Forecast type. Must be 'daily', 'hourly', or 'twice_daily'. Use 'daily' for general weather requests."
			fieldSchema["enum"] = []string{"daily", "hourly", "twice_daily"}
		}

		properties[fieldName] = fieldSchema

		if fieldDetail.Required {
			required = append(required, fieldName)
		}
	}

	// Most services require entity_id unless it's a domain-wide service
	if !isGlobalService(serviceName) {
		required = append(required, "entity_id")
	}

	parameters["required"] = required

	// Add generic tool characteristics that backends can interpret
	// These are provider-agnostic hints about the tool's behavior
	metadata := a.determineToolCharacteristics(domain, serviceName)

	tool := backend.Tool{
		Name:        fullName,
		Description: description,
		Parameters:  parameters,
		Metadata:    metadata,
	}

	// Debug log for fan and todo services to see what parameters are included
	if (domain == "fan" || domain == "todo" || domain == "weather") && a.logger.Core().Enabled(zap.InfoLevel) {
		a.logger.Info("generated tool schema",
			zap.String("service", fullName),
			zap.Int("field_count", len(detail.Fields)),
			zap.Any("parameters", parameters),
			zap.Any("metadata", metadata))
	}

	return tool
}

// convertFieldToSchema converts a FieldDetail to a JSON Schema property.
func (a *AutoDiscovery) convertFieldToSchema(fieldName string, field FieldDetail) map[string]any {
	schema := map[string]any{
		"description": field.Description,
	}

	// Infer type from selector or field name
	fieldType := "string" // default

	// Check selector for type hints
	if field.Selector != nil {
		if selectorMap, ok := field.Selector.(map[string]any); ok {
			if _, hasNumber := selectorMap["number"]; hasNumber {
				fieldType = "number"
			} else if _, hasBool := selectorMap["boolean"]; hasBool {
				fieldType = "boolean"
			} else if _, hasObject := selectorMap["object"]; hasObject {
				fieldType = "object"
			}
		}
	}

	// Common field name patterns
	if strings.Contains(strings.ToLower(fieldName), "brightness") ||
		strings.Contains(strings.ToLower(fieldName), "temperature") ||
		strings.Contains(strings.ToLower(fieldName), "volume") {
		fieldType = "number"
	}

	schema["type"] = fieldType

	// Add example if available
	if field.Example != nil {
		schema["example"] = field.Example
	}

	return schema
}

// isDomainAllowed checks if a domain is allowed based on allow/deny lists.
func (a *AutoDiscovery) isDomainAllowed(domain string) bool {
	// If in denied list, reject immediately
	if a.deniedDomains[domain] {
		return false
	}

	// If allow list is empty, allow all (except denied)
	if len(a.allowedDomains) == 0 {
		return true
	}

	// Otherwise, must be in allow list
	return a.allowedDomains[domain]
}

// isServiceAllowed checks if a service is allowed based on the allow list (with wildcard support).
func (a *AutoDiscovery) isServiceAllowed(serviceName string) bool {
	// If no allow list, allow all
	if len(a.allowedServices) == 0 {
		return true
	}

	// Exact match
	if a.allowedServices[serviceName] {
		return true
	}

	// Wildcard match (e.g., "light.*" matches "light.turn_on")
	parts := strings.Split(serviceName, ".")
	if len(parts) == 2 {
		// Check "domain.*" pattern
		wildcard := parts[0] + ".*"
		if a.allowedServices[wildcard] {
			return true
		}

		// Check "*.*" (allow all) pattern
		if a.allowedServices["*.*"] || a.allowedServices["*"] {
			return true
		}

		// Check "*.service" pattern (e.g., "*.turn_on")
		wildcard = "*." + parts[1]
		if a.allowedServices[wildcard] {
			return true
		}
	}

	return false
}

// formatRelevantAttributes formats important attributes for display.
func (a *AutoDiscovery) formatRelevantAttributes(attributes map[string]any) string {
	var parts []string

	// Common useful attributes
	relevantKeys := []string{"brightness", "temperature", "humidity", "battery", "volume_level"}

	for _, key := range relevantKeys {
		if val, ok := attributes[key]; ok {
			parts = append(parts, fmt.Sprintf("%s: %v", key, val))
		}
	}

	return strings.Join(parts, ", ")
}

// generateNaturalDescription creates natural, action-oriented descriptions for tools.
func (a *AutoDiscovery) generateNaturalDescription(domain, serviceName string) string {
	// Create action-oriented descriptions that sound natural
	actions := map[string]map[string]string{
		"light": {
			"turn_on":  "Turn on lights with optional brightness, color, or color temperature",
			"turn_off": "Turn off lights with optional transition",
			"toggle":   "Toggle lights on or off",
		},
		"fan": {
			"turn_on":        "Turn on fans with optional speed or preset mode",
			"turn_off":       "Turn off fans",
			"toggle":         "Toggle fans on or off",
			"set_percentage": "Set fan speed by percentage (0-100)",
			"set_direction":  "Set fan rotation direction (forward/reverse)",
			"oscillate":      "Enable or disable fan oscillation",
		},
		"media_player": {
			"turn_on":          "Turn on media player",
			"turn_off":         "Turn off media player",
			"media_play":       "Start playing media",
			"media_pause":      "Pause media playback",
			"media_stop":       "Stop media playback",
			"media_play_pause": "Toggle between play and pause",
			"volume_set":       "Set media player volume level (0.0-1.0)",
			"volume_up":        "Increase media player volume",
			"volume_down":      "Decrease media player volume",
			"volume_mute":      "Mute or unmute media player",
		},
		"todo": {
			"add_item":               "Add a new item to a todo list",
			"update_item":            "Update an existing todo list item",
			"remove_item":            "Remove an item from a todo list",
			"get_items":              "Get all items from a todo list",
			"remove_completed_items": "Remove all completed items from a todo list",
		},
		"weather": {
			"get_forecasts": "Get weather forecasts for local weather entities. Only provides data for configured local locations.",
		},
		"vacuum": {
			"start":          "Start vacuum cleaning",
			"pause":          "Pause vacuum cleaning",
			"stop":           "Stop vacuum cleaning",
			"return_to_base": "Send vacuum back to charging dock",
			"clean_spot":     "Start spot cleaning",
			"locate":         "Make vacuum play a sound to locate it",
			"set_fan_speed":  "Set vacuum suction power/fan speed",
		},
	}

	// Check if we have a specific description
	if domainActions, ok := actions[domain]; ok {
		if desc, ok := domainActions[serviceName]; ok {
			return desc
		}
	}

	// Fallback to generic description
	friendlyName := strings.ReplaceAll(serviceName, "_", " ")
	return fmt.Sprintf("%s for %s devices", strings.Title(friendlyName), domain)
}

// determineToolCharacteristics returns provider-agnostic metadata about the tool's behavior.
// Backends can interpret these characteristics according to their specific requirements.
func (a *AutoDiscovery) determineToolCharacteristics(domain, serviceName string) map[string]any {
	characteristics := make(map[string]any)

	// Determine if tool returns data (blocks until response available)
	dataReturning := false
	if strings.Contains(serviceName, "get_") ||
		strings.Contains(serviceName, "list_") ||
		strings.Contains(serviceName, "browse_") ||
		strings.Contains(serviceName, "search_") {
		dataReturning = true
	}

	// Specific services that always return data
	dataReturningServices := map[string]map[string]bool{
		"weather": {"get_forecasts": true},
		"todo":    {"get_items": true},
		"media_player": {
			"browse_media": true,
			"search_media": true,
		},
	}
	if domainServices, ok := dataReturningServices[domain]; ok {
		if domainServices[serviceName] {
			dataReturning = true
		}
	}

	characteristics["data_returning"] = dataReturning

	// Determine if this is a critical operation that might need immediate feedback
	critical := false
	criticalServices := map[string]map[string]bool{
		"alarm_control_panel": {
			"alarm_arm_away": true,
			"alarm_arm_home": true,
			"alarm_disarm":   true,
		},
		"lock": {
			"lock":   true,
			"unlock": true,
		},
	}
	if domainServices, ok := criticalServices[domain]; ok {
		if domainServices[serviceName] {
			critical = true
		}
	}

	if critical {
		characteristics["critical"] = true
	}

	// Control operations are generally safe to run asynchronously
	characteristics["async_capable"] = !dataReturning

	return characteristics
}

// getDomainFromEntityID extracts the domain from an entity ID.
func getDomainFromEntityID(entityID string) string {
	parts := strings.Split(entityID, ".")
	if len(parts) >= 2 {
		return parts[0]
	}
	return ""
}

// isGlobalService checks if a service is domain-wide (doesn't need entity_id).
func isGlobalService(serviceName string) bool {
	globalServices := map[string]bool{
		"reload":       true,
		"reload_all":   true,
		"turn_all_on":  true,
		"turn_all_off": true,
	}
	return globalServices[serviceName]
}
