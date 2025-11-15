# Security & Safety Guide

This document describes the security features and best practices for the Home Assistant Realtime Voice Gateway.

## Overview

The gateway implements multiple layers of security to ensure safe operation of your smart home devices through voice commands.

## Tool Execution Safety

The gateway implements **7 security layers** for Home Assistant service calls:

### 1. Domain Allow-list

Only explicitly allowed domains can be controlled by the LLM.

**Configuration:**
```bash
# .env
ALLOWED_DOMAINS=light,switch,media_player,cover,climate,fan
```

**Example:**
- ✅ `light.living_room` → Allowed (light domain)
- ✅ `switch.bedroom` → Allowed (switch domain)
- ❌ `automation.security` → Denied (not in allow-list)

### 2. Entity Deny-list

Block specific sensitive entities even if their domain is allowed.

**Configuration:**
```bash
# .env
ALLOWED_DOMAINS=lock,cover
DENIED_ENTITIES=lock.front_door,cover.garage_door
```

**Example:**
- ✅ `lock.bedroom` → Allowed
- ❌ `lock.front_door` → Denied (in deny-list)

### 3. Confirmation Required

Certain domains require voice confirmation before execution.

**Configuration:**
```bash
# .env
REQUIRE_CONFIRMATION=lock,cover
```

**Flow:**
```
User: "Unlock the front door"
LLM: "Are you sure you want to unlock the front door?"
User: "Yes"
LLM: → Executes unlock command
```

### 4. Read-only Mode

Optionally disable all state changes (queries only).

**Configuration:**
```bash
# .env
READ_ONLY_MODE=true
```

**Effect:**
- ✅ Query states: "What's the temperature?"
- ❌ Control devices: "Turn on the lights"

### 5. Timeouts

All tool calls have configurable timeouts to prevent hanging operations.

**Configuration:**
```bash
# .env
TOOL_TIMEOUT=5s
TOOL_RETRY_ATTEMPTS=2
```

**Timeout Hierarchy:**
```
Session Timeout (5min)
  └─> Tool Call Timeout (5s)
        └─> HA API Timeout (3s)
              └─> Network Timeout (1s)
```

### 6. Rate Limiting

Prevent abuse via rate limits per session.

**Configuration:**
```yaml
# ha-config.yaml
session:
  max_tool_calls_per_minute: 30
  max_tool_calls_per_hour: 200
```

**Effect:**
- Normal use: No impact
- Rapid-fire commands: Throttled after limits
- Malicious loops: Blocked automatically

### 7. Audit Logging

All tool calls are logged with full context for security auditing.

**Log Example:**
```json
{
  "timestamp": "2025-11-15T12:34:56Z",
  "level": "info",
  "msg": "tool_call_executed",
  "session_id": "abc123",
  "device_id": "living_room_voice",
  "user_id": "alice",
  "tool": "homeassistant.turn_on",
  "entity_id": "light.living_room",
  "duration_ms": 85,
  "result": "success"
}
```

## Implementation Example

```go
// Tool execution with comprehensive safety checks
type ToolPolicy struct {
    AllowedDomains   []string          // e.g., ["light", "switch"]
    DeniedEntities   []string          // e.g., ["lock.front_door"]
    RequireConfirm   []string          // e.g., ["lock", "cover"]
    ReadOnlyMode     bool
    Timeout          time.Duration
    RateLimit        rate.Limit
}

func (e *Executor) ExecuteTool(call ToolCall, policy ToolPolicy) (interface{}, error) {
    // 1. Check domain allow-list
    if !contains(policy.AllowedDomains, call.Domain) {
        return nil, ErrDomainNotAllowed
    }
    
    // 2. Check entity deny-list
    if contains(policy.DeniedEntities, call.EntityID) {
        return nil, ErrEntityDenied
    }
    
    // 3. Check if confirmation required
    if contains(policy.RequireConfirm, call.Domain) && !call.Confirmed {
        return nil, ErrConfirmationRequired
    }
    
    // 4. Rate limiting
    if !e.limiter.Allow() {
        return nil, ErrRateLimited
    }
    
    // 5. Execute with timeout
    ctx, cancel := context.WithTimeout(context.Background(), policy.Timeout)
    defer cancel()
    
    result, err := e.haClient.CallService(ctx, call)
    
    // 6. Audit log
    e.auditLog.Record(call, result, err)
    
    return result, err
}
```

## Network Security

### TLS Support

Optional TLS for Wyoming protocol connections:

```yaml
# ha-config.yaml
wyoming:
  tls:
    enabled: true
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
```

### Authentication

- **Home Assistant**: Token-based authentication via long-lived access token
- **LLM APIs**: API key authentication (Gemini, OpenAI)

**Best Practices:**
- Use long-lived tokens with restricted scopes
- Rotate tokens regularly
- Never commit tokens to version control
- Use environment variables for secrets

### Firewall Configuration

**Recommended firewall rules:**

```bash
# Allow Wyoming protocol (from HA devices only)
sudo ufw allow from 192.168.1.0/24 to any port 10200 proto tcp
sudo ufw allow from 192.168.1.0/24 to any port 10201 proto tcp

# Allow metrics (from monitoring server only)
sudo ufw allow from 192.168.1.10 to any port 9090 proto tcp

# Allow health checks (internal network only)
sudo ufw allow from 192.168.1.0/24 to any port 8080 proto tcp

# Deny all other incoming
sudo ufw default deny incoming
```

### Network Segmentation

**Recommended network architecture:**

```
Internet
    │
    ▼
[Firewall]
    │
    ├──> [IoT VLAN 192.168.1.0/24]
    │      ├── Home Assistant
    │      ├── Voice Gateway
    │      └── Voice Preview Devices
    │
    └──> [Main Network 192.168.0.0/24]
         └── User devices
```

**Benefits:**
- Isolate smart home devices from main network
- Limit attack surface
- Easier to apply firewall rules
- Better traffic monitoring

### No Internet Exposure

The gateway only needs outbound connections:

- ✅ Outbound to Gemini/OpenAI APIs
- ✅ Outbound to Home Assistant
- ❌ No inbound from internet required
- ❌ No port forwarding needed

## Error Handling & Resilience

### Circuit Breakers

Prevent cascading failures when HA or LLM services are degraded:

```go
// Automatically open circuit after N failures
type CircuitBreaker struct {
    maxFailures   int
    resetTimeout  time.Duration
    state         State // Closed, Open, HalfOpen
}

// Usage: Protect HA API calls
result, err := e.circuitBreaker.Execute(func() (interface{}, error) {
    return e.haClient.CallService(ctx, call)
})
```

**States:**
- **Closed**: Normal operation, requests go through
- **Open**: Too many failures, requests blocked immediately
- **Half-Open**: Testing if service recovered

### Automatic Reconnection

- **WebSocket Reconnection**: Auto-reconnect to HA WebSocket on disconnect
- **Backend Reconnection**: Retry LLM connections with exponential backoff
- **Session Recovery**: Optionally persist session state for recovery

**Configuration:**
```bash
# .env
GEMINI_MAX_RETRIES=3
GEMINI_RETRY_BACKOFF=1s  # Exponential backoff: 1s, 2s, 4s
HA_WEBSOCKET_RECONNECT=true
```

### Graceful Degradation

When services fail, the gateway degrades gracefully:

| Failure | Fallback Behavior |
|---------|-------------------|
| LLM unavailable | Fall back to HA Conversation API (text-only) |
| Audio pipeline failure | Switch to traditional STT→TTS pipeline |
| Tool execution failure | LLM receives error and informs user |
| HA unavailable | Queue commands, retry when available |

### Panic Recovery

The gateway includes panic recovery to prevent crashes:

```go
// Panic recovery in Gemini backend
defer func() {
    if r := recover(); r != nil {
        log.Error("panic in session", 
            zap.Any("panic", r),
            zap.Stack("stack"))
        // Close session gracefully
        session.Close()
    }
}()
```

## Security Best Practices

### 1. Least Privilege

Only allow domains that users actually need:

```bash
# Don't do this (too permissive)
ALLOWED_DOMAINS=*

# Do this (explicit allow-list)
ALLOWED_DOMAINS=light,switch,climate,media_player
```

### 2. Deny Sensitive Entities

Even if a domain is allowed, deny critical entities:

```bash
ALLOWED_DOMAINS=lock,cover
DENIED_ENTITIES=lock.front_door,lock.back_door,cover.garage_door
```

### 3. Require Confirmation

For actions with security implications:

```bash
REQUIRE_CONFIRMATION=lock,cover,alarm_control_panel
```

### 4. Monitor Audit Logs

Regularly review audit logs for suspicious activity:

```bash
# Check for denied operations
grep "tool_denied" /var/log/gateway/gateway.log

# Check for rate limiting
grep "rate_limited" /var/log/gateway/gateway.log

# Check unusual access patterns
grep "tool_call_executed" /var/log/gateway/gateway.log | \
  jq '.entity_id' | sort | uniq -c | sort -nr
```

### 5. Rotate Credentials

Regularly rotate API keys and tokens:

- Home Assistant tokens: Rotate every 90 days
- LLM API keys: Rotate every 180 days
- Document rotation procedures

### 6. Use Read-only Mode for Testing

When testing new configurations:

```bash
READ_ONLY_MODE=true
LOG_LEVEL=debug
```

This allows testing without risking actual device control.

### 7. Network Isolation

- Run gateway in isolated network segment
- Use firewall rules to restrict access
- Monitor network traffic for anomalies

### 8. Update Regularly

Keep the gateway updated:

```bash
# Check for updates
git pull origin main

# Review changelog
cat CHANGELOG.md

# Rebuild and restart
docker-compose build
docker-compose up -d
```

## Common Security Scenarios

### Scenario 1: Children's Access

**Problem**: Kids can trigger voice commands

**Solution**:
```bash
# Allow only safe domains
ALLOWED_DOMAINS=light,media_player

# Deny bedtime overrides
DENIED_ENTITIES=switch.parental_controls

# Require confirmation for TV
REQUIRE_CONFIRMATION=media_player
```

### Scenario 2: Guest Access

**Problem**: Guests might access through voice

**Solution**:
- Use read-only mode during guest visits
- Or create guest-specific allow-list
- Enable session timeouts

```bash
READ_ONLY_MODE=false
ALLOWED_DOMAINS=light,climate
SESSION_TIMEOUT=5m
```

### Scenario 3: High-Security Home

**Problem**: Critical security devices exist

**Solution**:
```bash
# Minimal allow-list
ALLOWED_DOMAINS=light,switch

# Comprehensive deny-list
DENIED_ENTITIES=lock.*,alarm.*,camera.*,gate.*

# Require confirmation for everything
REQUIRE_CONFIRMATION=light,switch

# Strict rate limits
TOOL_TIMEOUT=3s
MAX_CONCURRENT_SESSIONS=2
```

### Scenario 4: Multi-User Home

**Problem**: Need different permissions per user

**Solution**: Use user-specific configurations (future feature)

Currently: Configure for most restrictive user

## Incident Response

### If Security Issue Detected

1. **Immediate Actions:**
   ```bash
   # Stop the gateway
   docker-compose down
   
   # Review audit logs
   cat /var/log/gateway/audit.log | grep -A5 "$(date +%Y-%m-%d)"
   ```

2. **Investigate:**
   - What command was executed?
   - Which session/device initiated it?
   - Was it authorized?

3. **Remediate:**
   - Update allow/deny lists
   - Rotate credentials if compromised
   - Review and improve security policies

4. **Restart with stricter policy:**
   ```bash
   # Temporarily use read-only mode
   READ_ONLY_MODE=true docker-compose up -d
   ```

## Security Checklist

Before deploying to production:

- [ ] Review and configure domain allow-list
- [ ] Set up entity deny-list for sensitive devices
- [ ] Enable confirmation for security-critical domains
- [ ] Configure appropriate timeouts
- [ ] Enable audit logging
- [ ] Set up log monitoring/alerts
- [ ] Configure firewall rules
- [ ] Use network segmentation
- [ ] Rotate default credentials
- [ ] Test read-only mode
- [ ] Document security policies
- [ ] Set up backup/recovery procedures

## See Also

- [CONFIGURATION.md](CONFIGURATION.md) - Complete configuration reference
- [AUTODISCOVERY.md](AUTODISCOVERY.md) - Service and entity discovery
- [ARCHITECTURE.md](ARCHITECTURE.md) - Technical architecture
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Common issues
