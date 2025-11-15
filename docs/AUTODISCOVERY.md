# Home Assistant Autodiscovery Feature

## Overview

The autodiscovery feature **automatically discovers** all available Home Assistant services and entities, eliminating the need to manually define tools for new devices. **Autodiscovery is always enabled** - you simply configure what to discover and what to allow.

This makes your voice gateway instantly compatible with any device you add to Home Assistant - no code changes needed!

## Configuration

### Quick Start

1. **Copy the example config:**
   ```bash
   cp ha-config.yaml.example ha-config.yaml
   ```

2. **Edit `ha-config.yaml`:**
   ```yaml
   home_assistant:
     url: "http://homeassistant.local:8123"
     token: "your_token_here"
     
     discovery:
       domains:
         include: []  # Empty = discover all (except excluded)
         exclude:     # Dangerous/internal domains
           - automation
           - script
           - scene
       
       services:
         mode: "allow"
         patterns:
           - "light.*"
           - "switch.*"
           - "climate.*"
       
       prompt:
         include_entities: true
         include_attributes: false
   ```

3. **Override with environment variables (optional):**
   ```bash
   # .env file
   HA_URL=http://192.168.1.100:8123
   HA_TOKEN=supersecret
   ```

### Configuration Priority

**Environment variables > YAML > Defaults**

This allows you to:
- Store base configuration in YAML (version controlled)
- Override sensitive data with environment variables
- Use different configs per environment (dev/staging/prod)

## Configuration Options

### Domain Filtering

```yaml
discovery:
  domains:
    # Include specific domains (empty = all except excluded)
    include:
      - light
      - switch
      - climate
    
    # Exclude domains (takes precedence)
    exclude:
      - automation
      - script
```

**How it works:**
- If `include` is empty: Discover ALL domains (except those in `exclude`)
- If `include` has items: Only discover listed domains (still respecting `exclude`)
- `exclude` always takes precedence

### Service Filtering

```yaml
discovery:
  services:
    mode: "allow"  # or "deny"
    patterns:
      - "light.*"           # All light services
      - "switch.turn_*"     # Only turn_on/turn_off
      - "*.toggle"          # Toggle across all domains
```

**Wildcard support:**
- `domain.*` - All services in a domain
- `*.service` - Specific service across all domains
- `domain.service_*` - Pattern matching service names

### System Prompt Enhancement

```yaml
discovery:
  prompt:
    include_entities: true      # List your devices in prompt
    include_attributes: false   # Include brightness, temperature, etc.
    group_by: "domain"          # "domain", "area", "floor", "none"
```

## How It Works

### 1. **Service Discovery**
At startup, the gateway queries Home Assistant's `/api/services` endpoint to discover:
- All available domains (light, switch, climate, etc.)
- All services within each domain (turn_on, turn_off, set_temperature, etc.)
- Service parameters and descriptions

### 2. **Entity Discovery**
The gateway queries `/api/states` to get:
- All entity IDs (e.g., `light.living_room`, `switch.garden`)
- Current states (on, off, etc.)
- Friendly names and attributes
- Organized by domain

### 3. **Dynamic Tool Generation**
For each discovered service, the gateway automatically:
- Creates a tool definition with proper JSON schema
- Includes service descriptions from Home Assistant
- Adds entity_id and service-specific parameters
- Registers the tool with the LLM backend

### 4. **System Prompt Enhancement**
The gateway can generate a system prompt containing:
- Complete list of your devices with friendly names
- Current states
- Optional attributes (brightness, temperature, etc.)

This helps the LLM understand exactly which devices exist in your home.

## Example: What the LLM Sees

### With Entity List (include_entities: true)
```
You are a helpful voice assistant for Home Assistant.

Available Home Assistant Entities:
==================================================

Light (5 entities):
  - light.living_room (Living Room Light) [on]
  - light.bedroom (Bedroom Light) [off]
  - light.kitchen (Kitchen Light) [on]

Switch (3 entities):
  - switch.garden (Garden Switch) [on]
  - switch.christmas_tree (Christmas Tree) [off]

When users refer to rooms or devices, use these exact entity IDs.
```

### With Attributes (include_attributes: true)
```
Light (5 entities):
  - light.living_room (Living Room Light) [on] - brightness: 200, color_temp: 370
  - light.bedroom (Bedroom Light) [off]
  - light.kitchen (Kitchen Light) [on] - brightness: 255
```

## Benefits

### ✅ Automatic New Device Support
Add a new light to Home Assistant? It works with voice immediately—no code changes needed!

### ✅ Accurate Entity IDs
The LLM sees your exact entity IDs and friendly names, reducing errors.

### ✅ Service Completeness
Automatically includes all services, even custom ones from integrations.

### ✅ Reduced Maintenance
No need to update tool definitions when adding new devices or integrations.

### ✅ Better Organization
YAML provides better structure, comments, and version control than env vars.

## Performance Considerations

### Startup Time
- Service discovery: ~100-500ms
- Entity discovery: ~200-800ms
- Total overhead: ~1-2 seconds on startup

This happens once at gateway startup, not per voice session.

### System Prompt Size

| Configuration | Approximate Size |
|--------------|------------------|
| No entities | ~200 chars |
| Entities without attributes | ~50-100 chars per entity |
| Entities with attributes | ~100-200 chars per entity |

**Example:** 50 entities without attributes ≈ 4KB prompt

Modern LLMs handle this easily. Consider disabling attributes for 100+ entities.

## Advanced Examples

### Example 1: Living Room Only
```yaml
discovery:
  domains:
    include: [light, media_player, climate]
  services:
    patterns:
      - "light.living_room*"  # Only living room lights
      - "media_player.living_room*"
      - "climate.*"
```

### Example 2: Safe Operations Only
```yaml
discovery:
  services:
    mode: "allow"
    patterns:
      - "*.turn_on"
      - "*.turn_off"
      - "*.toggle"
      - "climate.set_temperature"
      # No wildcards - explicit is safer
```

### Example 3: Everything Except Dangerous
```yaml
discovery:
  domains:
    include: []  # Everything
    exclude: [automation, script, scene, input_boolean]
  services:
    mode: "deny"
    patterns:
      - "*.reload"
      - "*.restart"
```

## Troubleshooting

### "Config file not found" error
The gateway looks for `ha-config.yaml` by default. Either:
1. Create the file: `cp ha-config.yaml.example ha-config.yaml`
2. Use a different path: `HA_CONFIG_FILE=my-config.yaml`
3. Disable YAML: `HA_CONFIG_FILE=` (uses only env vars)

### "Service not allowed" errors
Services must pass BOTH filters:
1. Domain must be in `domains.include` (or not in `exclude`)
2. Service must match a pattern in `services.patterns`

**Example:**
```yaml
domains:
  include: [light]  # ✅ light domain allowed
services:
  patterns: [switch.*]  # ❌ but only switch services allowed!
```

Fix: Add `light.*` to service patterns.

### Large homes (100+ entities)
For very large homes:
1. Disable attributes: `include_attributes: false`
2. Filter domains to essentials
3. Consider disabling entity prompt if < 1000 tokens is critical

### Custom integrations not working
1. Check it returns valid JSON from `/api/services`
2. Ensure domain isn't in `exclude` list
3. Check logs: `LOG_LEVEL=debug`

### Configuration Best Practices
- **Use YAML for structure**: Define domains, services, and filtering rules in `ha-config.yaml`
- **Use environment variables for secrets**: Keep `HA_TOKEN` and `GEMINI_API_KEY` in `.env` (gitignored)
- **Override when needed**: Environment variables take precedence over YAML settings
- **Version control YAML**: Commit `ha-config.yaml` to track configuration changes
- **Multiple environments**: Use different YAML files (dev/staging/prod) via `HA_CONFIG_FILE` env var

## Best Practices

1. **Start simple:** Use default config with just URL/token overrides
2. **Use domain filtering:** Start broad, narrow down if needed
3. **Enable entity prompt:** Helps LLM accuracy significantly
4. **Disable attributes initially:** Add only if you need current state info
5. **Version control your YAML:** Track config changes in git
6. **Keep secrets in .env:** Never commit tokens to version control
7. **Test changes:** Use `LOG_LEVEL=debug` to see what's discovered

## Example Logs

```
INFO    Loading configuration from YAML file   path=ha-config.yaml
INFO    Starting Home Assistant autodiscovery  allowed_domains=0  denied_domains=6
DEBUG   getting all services
DEBUG   got all services   domains=42
INFO    autodiscovery completed   tools_generated=127
DEBUG   getting all entity states
DEBUG   got all states   count=83
INFO    entity discovery completed   total_entities=83   domains=15
INFO    Generated system prompt with entity list   prompt_length=6234
INFO    Home Assistant integration ready   tools_available=127
```

## Future Enhancements

Planned features:
- [ ] Area/floor-based entity grouping in prompts
- [ ] Entity filtering (not just domains)
- [ ] Hot-reload on config changes (webhook from HA)
- [ ] Custom tool descriptions from HA device_class
- [ ] Template-based system prompts
- [ ] Config validation CLI tool

