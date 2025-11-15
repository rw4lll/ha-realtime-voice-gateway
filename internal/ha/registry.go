package ha

import (
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
)

// GetDefaultTools returns the default set of Home Assistant tools for LLMs.
func GetDefaultTools() []backend.Tool {
	return []backend.Tool{
		// Lights
		{
			Name:        "light.turn_on",
			Description: "Turn on one or more lights. Can optionally set brightness (0-255), color, or temperature.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the light (e.g., 'light.living_room')",
					},
					"brightness": map[string]interface{}{
						"type":        "integer",
						"description": "Brightness value from 0 to 255",
						"minimum":     0,
						"maximum":     255,
					},
					"rgb_color": map[string]interface{}{
						"type":        "array",
						"description": "RGB color as [red, green, blue] array (each 0-255)",
						"items": map[string]interface{}{
							"type": "integer",
						},
					},
					"color_temp": map[string]interface{}{
						"type":        "integer",
						"description": "Color temperature in mireds",
					},
				},
				"required": []string{"entity_id"},
			},
		},
		{
			Name:        "light.turn_off",
			Description: "Turn off one or more lights",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the light",
					},
				},
				"required": []string{"entity_id"},
			},
		},
		{
			Name:        "light.toggle",
			Description: "Toggle one or more lights (on if off, off if on)",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the light",
					},
				},
				"required": []string{"entity_id"},
			},
		},

		// Switches
		{
			Name:        "switch.turn_on",
			Description: "Turn on a switch",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the switch",
					},
				},
				"required": []string{"entity_id"},
			},
		},
		{
			Name:        "switch.turn_off",
			Description: "Turn off a switch",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the switch",
					},
				},
				"required": []string{"entity_id"},
			},
		},

		// Climate
		{
			Name:        "climate.set_temperature",
			Description: "Set the target temperature for a climate device (thermostat, AC, etc.)",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the climate device",
					},
					"temperature": map[string]interface{}{
						"type":        "number",
						"description": "Target temperature",
					},
					"hvac_mode": map[string]interface{}{
						"type":        "string",
						"description": "HVAC mode (heat, cool, auto, off, etc.)",
						"enum":        []string{"heat", "cool", "heat_cool", "auto", "off", "fan_only", "dry"},
					},
				},
				"required": []string{"entity_id", "temperature"},
			},
		},

		// Covers (blinds, shades, garage doors)
		{
			Name:        "cover.open_cover",
			Description: "Open a cover (blinds, shades, garage door)",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the cover",
					},
				},
				"required": []string{"entity_id"},
			},
		},
		{
			Name:        "cover.close_cover",
			Description: "Close a cover (blinds, shades, garage door)",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the cover",
					},
				},
				"required": []string{"entity_id"},
			},
		},

		// Media Player
		{
			Name:        "media_player.media_play",
			Description: "Play media on a media player",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the media player",
					},
				},
				"required": []string{"entity_id"},
			},
		},
		{
			Name:        "media_player.media_pause",
			Description: "Pause media on a media player",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the media player",
					},
				},
				"required": []string{"entity_id"},
			},
		},
		{
			Name:        "media_player.volume_set",
			Description: "Set the volume of a media player",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID of the media player",
					},
					"volume_level": map[string]interface{}{
						"type":        "number",
						"description": "Volume level from 0.0 to 1.0",
						"minimum":     0.0,
						"maximum":     1.0,
					},
				},
				"required": []string{"entity_id", "volume_level"},
			},
		},

		// State queries
		{
			Name:        "homeassistant.get_state",
			Description: "Get the current state and attributes of an entity",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID to query (e.g., 'light.living_room', 'sensor.temperature')",
					},
				},
				"required": []string{"entity_id"},
			},
		},
	}
}

// GetToolsForServices creates tools for a specific list of services.
func GetToolsForServices(services []string) []backend.Tool {
	allTools := GetDefaultTools()
	toolMap := make(map[string]backend.Tool)
	
	for _, tool := range allTools {
		toolMap[tool.Name] = tool
	}

	tools := make([]backend.Tool, 0, len(services))
	for _, service := range services {
		if tool, ok := toolMap[service]; ok {
			tools = append(tools, tool)
		}
	}

	return tools
}

