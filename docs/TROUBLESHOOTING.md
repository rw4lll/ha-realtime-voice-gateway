# Troubleshooting Guide

This guide helps you diagnose and resolve common issues with the Home Assistant Realtime Voice Gateway.

## Quick Diagnostics

Before diving into specific issues, run these quick checks:

```bash
# 1. Check if gateway is running
docker-compose ps
# or
ps aux | grep gateway

# 2. Check gateway logs
docker-compose logs -f gateway
# or
tail -f /var/log/gateway/gateway.log

# 3. Check network connectivity
curl http://localhost:8080/voice-stream  # WebSocket endpoint (should return "Upgrade Required")
curl http://localhost:9090/health  # Health endpoint (if metrics enabled)

# 4. Verify HA connectivity
curl -H "Authorization: Bearer $HA_TOKEN" http://homeassistant.local:8123/api/

# 5. Test Gemini API
curl -H "Authorization: Bearer $GEMINI_API_KEY" \
  https://generativelanguage.googleapis.com/v1/models
```

## Common Issues

### High Latency (>500ms)

**Symptoms:**
- Slow response time from LLM
- Long pause after speaking before LLM responds
- Noticeable delay between request and action

**Possible Causes:**
1. Network latency to LLM API
2. HA state queries are slow
3. Audio buffer size too large
4. Tool calls blocking audio generation
5. System resource constraints

**Solutions:**

1. **Enable state caching:**
   ```bash
   # .env
   ENABLE_STATE_CACHE=true
   CACHE_TTL=30s
   ```

2. **Use async tool calls:**
   ```bash
   # .env
   ASYNC_TOOL_CALLS=true
   ```

3. **Reduce audio buffer size:**
   ```bash
   # .env
   AUDIO_BUFFER_SIZE=2048  # Default is 4096
   ```

4. **Check LLM API status:**
   ```bash
   # For Gemini
   curl https://status.cloud.google.com/
   
   # For OpenAI
   curl https://status.openai.com/
   ```

5. **Use Gemini (faster for low latency):**
   ```bash
   # .env
   BACKEND_TYPE=gemini
   ```

6. **Check network latency:**
   ```bash
   # Ping Gemini API
   ping generativelanguage.googleapis.com
   
   # Check response time
   curl -w "@curl-format.txt" -o /dev/null -s \
     https://generativelanguage.googleapis.com/
   ```

7. **Monitor system resources:**
   ```bash
   # Check CPU/memory
   docker stats gateway
   
   # Check disk I/O
   iostat -x 1
   ```

### Audio Quality Issues

**Symptoms:**
- Choppy, garbled, or distorted audio
- Audio cuts in and out
- Robotic or metallic sound
- Missing words

**Possible Causes:**
1. Sample rate mismatch
2. Buffer overruns/underruns
3. Network packet loss
4. VAD cutting off speech
5. CPU overload

**Solutions:**

1. **Check logs for buffer issues:**
   ```bash
   grep "audio_buffer" /var/log/gateway/gateway.log
   grep "overrun\|underrun" /var/log/gateway/gateway.log
   ```

2. **Increase audio buffer size:**
   ```bash
   # .env
   AUDIO_BUFFER_SIZE=8192  # Default is 4096
   ```

3. **Adjust VAD settings:**
   ```bash
   # .env
   VAD_SENSITIVITY=0.3  # Lower = less sensitive
   VAD_SPEECH_PAD_MS=500  # More padding
   ```
   
   See [VAD_TUNING_GUIDE.md](VAD_TUNING_GUIDE.md) for detailed tuning.

4. **Verify network stability:**
   ```bash
   # Check packet loss
   ping -c 100 your-gateway-ip
   
   # Monitor network stats
   netstat -s | grep -i "packet\|error"
   ```

5. **Check audio format compatibility:**
   ```bash
   # ESP32 device should stream in 16kHz, 16-bit, mono PCM
   # Verify in device logs or gateway logs
   ```

6. **Reduce CPU load:**
   ```bash
   # Disable audio quality tracking
   TRACK_AUDIO_QUALITY=false
   
   # Reduce log level
   LOG_LEVEL=info
   ```

### Tool Calls Not Working

**Symptoms:**
- LLM says it will do something but nothing happens
- Lights don't turn on/off when requested
- No error messages but no action
- LLM seems confused about available devices

**Possible Causes:**
1. Domain not in `ALLOWED_DOMAINS`
2. Entity in `DENIED_ENTITIES`
3. HA token lacks permissions
4. HA service doesn't exist
5. Autodiscovery not configured
6. Network connectivity issues

**Solutions:**

1. **Check allowed domains:**
   ```bash
   # .env
   ALLOWED_DOMAINS=light,switch,climate  # Add missing domains
   ```

2. **Check logs for tool errors:**
   ```bash
   grep "tool_denied\|tool_error" /var/log/gateway/gateway.log
   ```

3. **Verify domain is allowed:**
   ```bash
   # Example log entry:
   # "tool_denied" domain="automation" reason="not in allow-list"
   ```

4. **Test HA API directly:**
   ```bash
   # List all services
   curl -H "Authorization: Bearer $HA_TOKEN" \
     http://homeassistant.local:8123/api/services
   
   # Test specific service
   curl -X POST \
     -H "Authorization: Bearer $HA_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"entity_id": "light.living_room"}' \
     http://homeassistant.local:8123/api/services/light/turn_on
   ```

5. **Check HA token permissions:**
   - Go to HA → Profile → Long-Lived Access Tokens
   - Verify token exists and hasn't expired
   - Create new token if needed

6. **Verify autodiscovery:**
   ```bash
   # Check gateway startup logs
   grep "autodiscovery\|tools_generated" /var/log/gateway/gateway.log
   
   # Should see: "autodiscovery completed tools_generated=127"
   ```

7. **Check HA logs:**
   ```bash
   # In Home Assistant
   # Settings → System → Logs
   # Look for API errors or permission issues
   ```

### Device Not Connecting

**Symptoms:**
- ESP32 device can't connect to gateway
- WebSocket connection refused errors
- Device shows "connection error" or retries
- Gateway logs show no incoming connections

**Possible Causes:**
1. Firewall blocking ports
2. Wrong IP address in HA config
3. Gateway not listening on correct interface
4. Network segmentation issues
5. Gateway not running

**Solutions:**

1. **Verify gateway is running:**
   ```bash
   # Check process
   docker-compose ps gateway
   
   # Check logs for startup
   docker-compose logs gateway | grep "Gateway ready"
   
   # Should see: "Gateway ready  websocket_addr=0.0.0.0:8080"
   ```

2. **Check port binding:**
   ```bash
   # Verify WebSocket server is listening
   netstat -an | grep 8080
   # Or use ss
   ss -tlnp | grep 8080
   
   # Should see:
   # tcp 0 0 0.0.0.0:8080 0.0.0.0:* LISTEN
   ```

3. **Test WebSocket connection from client:**
   ```bash
   # From ESP32's network (or same network as device)
   curl http://gateway-ip:8080/voice-stream
   
   # Should see: "Upgrade Required" (indicates WebSocket endpoint is working)
   
   # Or test with wscat (install: npm install -g wscat)
   wscat -c ws://gateway-ip:8080/voice-stream
   ```

4. **Check firewall rules:**
   ```bash
   # List rules
   sudo ufw status verbose
   
   # Add WebSocket rule if needed
   sudo ufw allow 8080/tcp
   
   # Or restrict to specific subnet (recommended)
   sudo ufw allow from 192.168.1.0/24 to any port 8080 proto tcp
   ```

5. **Verify listen address:**
   ```bash
   # .env
   WEBSOCKET_ADDR=0.0.0.0:8080  # Listen on all interfaces
   # NOT 127.0.0.1:8080 (localhost only)
   ```

6. **Check ESP32 device configuration:**
   - Gateway URL should be: `ws://gateway-ip:8080/voice-stream`
   - Use actual IP address, not `localhost` (unless on same host)
   - Make sure to use `ws://` protocol, not `http://`

7. **Test WebSocket handshake manually:**
   ```bash
   # Using curl (HTTP upgrade test)
   curl -i -N \
     -H "Connection: Upgrade" \
     -H "Upgrade: websocket" \
     -H "Sec-WebSocket-Version: 13" \
     -H "Sec-WebSocket-Key: test" \
     http://gateway-ip:8080/voice-stream
   
   # Should see "101 Switching Protocols"
   ```

### Gemini API Errors

**Symptoms:**
- `Failed to initialize Gemini backend`
- `WebSocket connection failed`
- `API quota exceeded`
- `Invalid API key`

**Possible Causes:**
1. Invalid or expired API key
2. API quota exceeded
3. Network connectivity issues
4. Gemini service outage
5. Wrong model name

**Solutions:**

1. **Verify API key:**
   ```bash
   # Test API key
   curl -H "Content-Type: application/json" \
     -d '{"contents":[{"parts":[{"text":"Hello"}]}]}' \
     -X POST \
     "https://generativelanguage.googleapis.com/v1/models/gemini-2.0-flash-exp:generateContent?key=$GEMINI_API_KEY"
   
   # Should return JSON response, not error
   ```

2. **Check API quota:**
   - Go to https://aistudio.google.com/apikey
   - Check quota usage
   - Wait for quota reset or upgrade plan

3. **Try creating new API key:**
   - Delete old key
   - Create new key at https://aistudio.google.com/apikey
   - Update `.env` file

4. **Check Gemini status:**
   ```bash
   curl https://status.cloud.google.com/
   ```

5. **Verify model name:**
   ```bash
   # .env
   GEMINI_MODEL=gemini-2.0-flash-exp  # Check for typos
   ```

6. **Check gateway logs:**
   ```bash
   grep "gemini\|api_key" /var/log/gateway/gateway.log
   ```

7. **Test with increased timeouts:**
   ```bash
   # .env
   GEMINI_CONNECT_TIMEOUT=60s
   GEMINI_RECEIVE_TIMEOUT=120s
   ```

### Gateway Won't Start

**Symptoms:**
- `docker-compose up` fails
- Configuration validation errors
- Port already in use
- Permission denied errors

**Possible Causes:**
1. Configuration errors
2. Port conflicts
3. Permission issues
4. Missing environment variables
5. Docker issues

**Solutions:**

1. **Check configuration validation:**
   ```bash
   docker-compose logs gateway | grep -i "error\|validation"
   ```

2. **Verify required variables:**
   ```bash
   # Check .env file
   cat .env | grep -E "^(HA_URL|HA_TOKEN|GEMINI_API_KEY)"
   
   # All three should have values
   ```

3. **Check for port conflicts:**
   ```bash
   # See what's using port 8080
   lsof -i :8080
   # Or
   ss -tlnp | grep 8080
   
   # Kill conflicting process or change port mapping
   # In docker-compose.yml: ports: ["8081:8080"]
   ```

4. **Check file permissions:**
   ```bash
   # Ensure gateway binary is executable
   chmod +x gateway
   
   # Check log directory
   sudo chown -R $(whoami) /var/log/gateway/
   ```

5. **Validate YAML syntax:**
   ```bash
   # Test ha-config.yaml
   cat ha-config.yaml | yaml-lint
   # or
   python3 -c "import yaml; yaml.safe_load(open('ha-config.yaml'))"
   ```

6. **Check Docker logs:**
   ```bash
   docker-compose logs gateway
   ```

7. **Try running directly:**
   ```bash
   # Test without Docker
   ./gateway
   
   # Check for specific error messages
   ```

### High Memory Usage

**Symptoms:**
- Gateway using excessive memory
- Out of memory errors
- System becoming unresponsive
- Docker container being killed

**Possible Causes:**
1. Memory leaks
2. Too many concurrent sessions
3. Large audio buffers
4. Conversation history not being cleared

**Solutions:**

1. **Limit concurrent sessions:**
   ```bash
   # .env
   MAX_CONCURRENT_SESSIONS=5  # Reduce from 10
   GEMINI_MAX_SESSIONS=3
   ```

2. **Reduce conversation history:**
   ```bash
   # .env
   MAX_CONVERSATION_HISTORY=20  # Reduce from 50
   ```

3. **Reduce buffer sizes:**
   ```bash
   # .env
   AUDIO_BUFFER_SIZE=2048
   BUFFER_POOL_SIZE=50
   ```

4. **Enable session cleanup:**
   ```bash
   # .env
   SESSION_TIMEOUT=3m  # Reduce from 5m
   CLEANUP_INTERVAL=30s
   ```

5. **Monitor memory usage:**
   ```bash
   # Watch memory in real-time
   watch docker stats gateway
   
   # Check for memory leaks
   docker exec gateway ps aux
   ```

6. **Set Docker memory limits:**
   ```yaml
   # docker-compose.yml
   services:
     gateway:
       mem_limit: 512m
       memswap_limit: 512m
   ```

7. **Restart gateway regularly:**
   ```bash
   # Add to cron for nightly restart
   0 3 * * * docker-compose restart gateway
   ```

## Debugging Tips

### Enable Debug Logging

```bash
# .env
LOG_LEVEL=debug
LOG_FORMAT=console  # Easier to read than JSON
```

### Monitor Metrics

```bash
# Enable Prometheus metrics
METRICS_ENABLED=true
METRICS_PORT=9090

# View metrics
curl http://localhost:9090/metrics
```

### Check Health Endpoint

```bash
curl http://localhost:8080/health | jq

# Expected output:
# {
#   "status": "healthy",
#   "backend": "gemini",
#   "backend_connected": true,
#   "ha_connected": true,
#   "active_sessions": 2
# }
```

### Trace Network Traffic

```bash
# Monitor WebSocket traffic
tcpdump -i any -A 'tcp port 8080'

# Monitor HA API calls
tcpdump -i any -A 'tcp port 8123'
```

### Test Individual Components

```bash
# Test WebSocket protocol
cd test/
python3 websocket_client.py --host localhost --port 8080

# Test with audio (using laptop mic/speakers)
python3 audio_bridge.py --gateway ws://localhost:8080/voice-stream
```

## Getting Help

If you're still experiencing issues:

1. **Check existing issues:**
   - https://github.com/yourusername/ha-realtime-voice-gateway/issues

2. **Gather diagnostic information:**
   ```bash
   # Create diagnostics bundle
   cat > diagnostics.txt << EOF
   Gateway Version: $(./gateway --version)
   Docker Version: $(docker --version)
   OS: $(uname -a)
   
   Configuration (redacted):
   $(cat .env | sed 's/=.*/=<REDACTED>/')
   
   Recent Logs:
   $(docker-compose logs --tail=100 gateway)
   
   Health Check:
   $(curl -s http://localhost:8080/health)
   EOF
   ```

3. **Create an issue with:**
   - Description of the problem
   - Steps to reproduce
   - Expected vs actual behavior
   - Diagnostic information (above)
   - Relevant log excerpts

4. **Join the community:**
   - Discord: https://discord.gg/home-assistant
   - Forums: https://community.home-assistant.io/

## See Also

- [CONFIGURATION.md](CONFIGURATION.md) - Configuration reference
- [SECURITY.md](SECURITY.md) - Security and safety
- [VAD_TUNING_GUIDE.md](VAD_TUNING_GUIDE.md) - Voice activity detection
- [ARCHITECTURE.md](ARCHITECTURE.md) - Technical architecture
- [test/README.md](../test/README.md) - Testing guide
