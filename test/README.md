# Gateway Testing Guide

This directory contains test scripts for the Home Assistant Realtime Voice Gateway.

## 📁 Test Script

### `audio_bridge.py` - Full Audio Testing with Laptop

**Purpose**: Test the complete voice pipeline using your laptop's microphone and speakers.

**Use Cases**:
- Test LLM backend (Gemini, OpenAI, etc.) with real audio
- Verify bidirectional audio streaming
- Test voice interactions before getting physical hardware
- Debug audio quality issues
- Test Home Assistant tool execution

**Requirements**:
```bash
# macOS
brew install portaudio
pip install -r requirements.txt

# Linux
sudo apt-get install portaudio19-dev
pip install -r requirements.txt
```

**Usage**:
```bash
# Basic test (30 seconds)
python3 audio_bridge.py

# Custom duration
python3 audio_bridge.py --duration 60

# Run indefinitely (Ctrl+C to stop)
python3 audio_bridge.py --duration 0

# Connect to remote gateway
python3 audio_bridge.py --host 192.168.1.100 --port 8080

# Custom WebSocket path
python3 audio_bridge.py --path /custom-path

# Enable debug logging
python3 audio_bridge.py --verbose
```

**What It Does**:
1. Connects to WebSocket gateway at `ws://host:8080/voice-stream`
2. Captures audio from your microphone (16kHz, 16-bit, mono)
3. Sends raw PCM audio to gateway via WebSocket (binary frames)
4. Gateway forwards to LLM backend (Gemini, OpenAI, etc.)
5. LLM processes and responds
6. Response audio comes back through gateway
7. Plays through your laptop speakers
8. Displays gateway state transitions (listening → thinking → speaking → done)

**Try Saying**:
- "Hello, how are you?"
- "Tell me a joke"
- "What's 25 times 34?"
- "Turn on the living room lights" (if HA is configured)

## 🚀 Quick Start Guide

### Step 0: Set Up Python Virtual Environment (Recommended)

```bash
# Navigate to test directory
cd test/

# Create virtual environment
python3 -m venv venv

# Activate it
source venv/bin/activate  # On Windows: venv\Scripts\activate

# Install system dependency (macOS only, one-time)
brew install portaudio

# Install Python packages
pip install -r requirements.txt
```

**Daily usage:**
```bash
cd test/
source venv/bin/activate
python3 audio_bridge.py
deactivate  # When done
```

### Step 1: Get an LLM API Key

**For Gemini (Free tier available):**
1. Go to https://aistudio.google.com/apikey
2. Click "Create API Key"
3. Copy the key

**For OpenAI:**
1. Go to https://platform.openai.com/api-keys
2. Create a new API key
3. Add credits to your account

### Step 2: Configure Gateway

Create/update `.env` in project root:

```bash
# Backend Configuration (choose one)
BACKEND_TYPE=gemini  # or "openai" or "mock"

# Gemini Configuration
GEMINI_API_KEY=your_gemini_api_key_here
GEMINI_MODEL=gemini-2.0-flash-exp

# OR OpenAI Configuration
OPENAI_API_KEY=your_openai_api_key_here
OPENAI_MODEL=gpt-4o-realtime-preview

# WebSocket Server (defaults shown)
WEBSOCKET_ADDR=0.0.0.0:8080
WEBSOCKET_PATH=/voice-stream

# Logging
LOG_LEVEL=info
LOG_FORMAT=console

# System Prompt (optional)
SYSTEM_PROMPT="You are a helpful voice assistant. Be concise and friendly."

# Home Assistant (optional - for tool execution)
HA_URL=http://localhost:8123
HA_TOKEN=your_ha_token_here
```

### Step 3: Build and Run Gateway

```bash
# Navigate to project root
cd /Users/rw4lll/Projects/ha-realtime-voice-gateway

# Option 1: Run directly (recommended for development)
go run ./cmd/gateway/main.go

# Option 2: Build and run binary
go build -o gateway ./cmd/gateway/
./gateway
```

Expected output:
```
INFO    Starting Home Assistant Realtime Voice Gateway
INFO    Backend initialized successfully
INFO    Pipeline initialized
INFO    WebSocket server starting    address=0.0.0.0:8080    path=/voice-stream
```

### Step 4: Test with Audio Bridge

In a new terminal:

```bash
# Navigate to test directory
cd test/

# Activate virtual environment (if you set it up in Step 0)
source venv/bin/activate

# Run audio bridge
python3 audio_bridge.py
```

You'll see:
```
============================================================
🎙️  READY TO TEST!
============================================================
Speak into your microphone and listen for responses...

Try saying:
  • 'Hello, how are you?'
  • 'Tell me a joke'
  • 'What's 25 times 34?'

Recording for 30 seconds...
Press Ctrl+C to stop early
============================================================

✅ Connected to gateway!
🎤 Microphone ready - speak now!
🎤 Gateway is listening...
🤔 Gateway is thinking...
🔊 Gateway is speaking...
✅ Session complete
```

**Speak into your microphone!** You should hear the LLM's response through your speakers.

## 🧪 Testing Scenarios

### Scenario 1: Basic Conversation Test

**Goal**: Verify LLM responds to simple queries.

**Terminal 1 - Gateway:**
```bash
cd /path/to/ha-realtime-voice-gateway
go run ./cmd/gateway/main.go
```

**Terminal 2 - Audio Bridge:**
```bash
cd /path/to/ha-realtime-voice-gateway/test
source venv/bin/activate  # If using virtual environment
python3 audio_bridge.py
```

**Say**: "Hello, how are you?"

**Expected**:
- Gateway logs show: "New WebSocket session connected"
- Gateway logs show: "session handler starting"
- You see state transitions: listening → thinking → speaking
- You hear LLM's voice response

### Scenario 2: Home Assistant Control Test

**Prerequisites**:
- Home Assistant running
- `HA_URL` and `HA_TOKEN` configured in `.env`
- Entities autodiscovered (check gateway startup logs)

**Say**: "Turn on the living room lights"

**Expected**:
- Gateway logs show tool execution
- Home Assistant service is called
- Lights turn on
- LLM confirms the action

### Scenario 3: Long Conversation Test

**Goal**: Test extended interaction.

```bash
cd test/
source venv/bin/activate
python3 audio_bridge.py --duration 0  # Run indefinitely
```

**Try**:
- Multiple back-and-forth exchanges
- Interrupting LLM mid-response
- Long questions
- Quick successive questions

**Monitor**:
- Gateway logs for errors
- Audio quality
- Response latency
- Memory usage

### Scenario 4: Remote Gateway Test

**Goal**: Test connecting to gateway on another machine.

```bash
# Connect to gateway at 192.168.1.100
python3 audio_bridge.py --host 192.168.1.100 --port 8080
```

**Prerequisites**:
- Gateway running on remote host
- Firewall allows port 8080
- Network connectivity

## 📊 What to Check

### Gateway Logs

**Good signs**:
```
INFO    New WebSocket session connected    session_id=...
INFO    session handler starting
DEBUG   forwardDeviceToBackend: forwarding audio
DEBUG   Audio chunk sent to backend
DEBUG   Received audio from backend
INFO    executing tool call    tool_name=light.turn_on
```

**Warning signs**:
```
ERROR   Failed to start WebSocket server
ERROR   Backend error
WARN    backend audio buffer full
ERROR   Failed to execute tool
```

### Audio Bridge Output

**Good signs**:
```
✅ Connected to gateway!
✅ Microphone ready - speak now!
🎤 Gateway is listening...
🤔 Gateway is thinking...
🔊 Gateway is speaking...
✅ Session complete
```

**Issues**:
```
❌ Error: Connection refused  → Gateway not running
❌ Failed to open microphone  → Mic permissions or in use
❌ Error from gateway  → Check backend API key
```

## 🐛 Troubleshooting

### No Audio Capture

**Problem**: Microphone not working

**Solutions**:
1. Check microphone permissions (System Preferences → Security & Privacy → Microphone)
2. Ensure no other app is using the microphone
3. Test mic with: `python3 -c "import pyaudio; p = pyaudio.PyAudio(); print('Mics:', p.get_device_count())"`
4. Try different microphone if multiple available

### No Audio Playback

**Problem**: Can't hear LLM responses

**Solutions**:
1. Check volume level
2. Ensure speakers are selected as output device
3. Test speakers with: `python3 -c "import pyaudio; p = pyaudio.PyAudio(); print('Speakers:', p.get_default_output_device_info())"`
4. Check gateway logs for "Gateway is speaking"
5. Check if you see state transition to "speaking" in audio bridge

### Gateway Connection Refused

**Problem**: `Connection refused to localhost:8080`

**Solutions**:
1. Ensure gateway is running: `go run ./cmd/gateway/main.go`
2. Check port isn't in use: `lsof -i :8080` (macOS/Linux) or `netstat -ano | findstr :8080` (Windows)
3. Verify WebSocket address in `.env`: `WEBSOCKET_ADDR=0.0.0.0:8080`
4. Check gateway logs for startup errors
5. Ensure firewall allows port 8080

### Backend API Errors

**Problem**: `Error from gateway: Backend error`

**Solutions**:
1. Verify API key is correct in `.env`
2. Check API key hasn't expired
3. Ensure you have API quota remaining (especially for OpenAI)
4. Try creating a new API key
5. Check gateway logs for detailed error messages

### High Latency

**Problem**: Slow responses (>2 seconds)

**Check**:
1. Internet connection speed
2. Gateway logs for delays
3. Audio buffer sizes in config
4. System resource usage

**Solutions**:
- Use faster internet connection
- Check CPU/memory usage
- Switch to `gemini-2.0-flash-exp` (faster than other models)
- Adjust `SESSION_AUDIO_BUFFER_MS` in `.env`

### Audio Quality Issues

**Problem**: Distorted or choppy audio

**Solutions**:
1. Check microphone input level (not too high)
2. Reduce background noise
3. Adjust buffer sizes in config
4. Check system audio settings
5. Try different audio device
6. Check gateway logs for "audio buffer full" warnings

### WebSocket Disconnects

**Problem**: Connection drops unexpectedly

**Solutions**:
1. Check `WEBSOCKET_READ_TIMEOUT` and `WEBSOCKET_WRITE_TIMEOUT` in `.env`
2. Increase timeouts if on slow network
3. Check for network instability
4. Review gateway logs for disconnect reason

## 💡 Tips & Best Practices

### For Best Results

1. **Use a good microphone**: Built-in laptop mics work, but external mics are better
2. **Quiet environment**: Reduce background noise for better recognition
3. **Clear speech**: Speak clearly and at a normal pace
4. **Check logs**: Always monitor gateway logs during testing
5. **Start simple**: Test basic queries before complex HA commands

### Performance Optimization

- **Debug logging**: Use `LOG_LEVEL=info` in production, `debug` only for testing
- **Buffer sizes**: Adjust `SESSION_AUDIO_BUFFER_MS` if experiencing glitches (default: 500ms)
- **System resources**: Close unnecessary applications during testing
- **Backend choice**: Gemini 2.0 Flash is fast and free; OpenAI has more quota limits

### Security Considerations

- **Never commit** `.env` files with real API keys
- **Use allow-lists** for HA service calls in production (`HA_ALLOW_LIST` in `.env`)
- **Test in isolated environment** before production deployment
- **Monitor API usage** to avoid unexpected charges
- **Use WSS (TLS)** in production for encrypted WebSocket connections

## 📈 Test Metrics

Track these metrics during testing:

- **Latency**: Time from speaking to hearing response
  - <500ms is excellent
  - <1s is good
  - <2s is acceptable
- **Audio Quality**: Clear, understandable speech from LLM
- **Success Rate**: % of commands understood correctly
- **Tool Execution**: % of HA commands executed successfully
- **Uptime**: Gateway runs without crashes
- **Memory Usage**: Should remain stable over time

## 🎯 Next Steps

After successful testing:

1. **Deploy with Docker**:
   ```bash
   docker-compose up -d
   ```

2. **Configure ESP32 Device**:
   - Flash your ESP32 with the modified firmware
   - Configure WebSocket URL: `ws://gateway-ip:8080/voice-stream`
   - Test with 4-button click to enter direct mode

3. **Set up Home Assistant integration**:
   - Configure autodiscovery (enabled by default)
   - Set up allow-lists for safety
   - Test HA tool execution

4. **Monitor in production**:
   - Enable metrics (`METRICS_ENABLED=true`)
   - Set up logging aggregation
   - Configure alerts for errors

## 📚 Additional Resources

- [WebSocket Protocol](https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API)
- [Gemini Live API Docs](https://ai.google.dev/gemini-api/docs/live)
- [OpenAI Realtime API](https://platform.openai.com/docs/guides/realtime)
- [Home Assistant Voice](https://www.home-assistant.io/voice_control/)
- [Project README](../Readme.md)
- [Docker Deployment Guide](../docs/DOCKER.md)
- [Autodiscovery Guide](../docs/AUTODISCOVERY.md)
- [Configuration Guide](../docs/CONFIGURATION.md)

## 🤝 Contributing

Found an issue or have an improvement? Please open an issue or PR on GitHub!

---

**Happy Testing!** 🎉
