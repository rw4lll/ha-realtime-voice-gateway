# Gateway Testing Guide

This directory contains test scripts for the Home Assistant Realtime Voice Gateway.

## 📁 Test Scripts

### 1. `audio_bridge.py` - Full Audio Testing with Laptop

**Purpose**: Test the complete voice pipeline using your laptop's microphone and speakers.

**Use Cases**:
- Test Gemini Live backend with real audio
- Verify bidirectional audio streaming
- Test voice interactions before getting physical hardware
- Debug audio quality issues

**Requirements**:
```bash
# macOS
brew install portaudio
pip install pyaudio  # or use venv (see Step 0)

# Linux
sudo apt-get install portaudio19-dev
pip install pyaudio
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
python3 audio_bridge.py --host 192.168.1.100 --port 10200

# Enable debug logging
python3 audio_bridge.py --verbose
```

**What It Does**:
1. Connects to Wyoming gateway
2. Captures audio from your microphone (16kHz, 16-bit, mono)
3. Sends audio to gateway via Wyoming protocol
4. Gateway forwards to Gemini Live
5. Gemini processes and responds
6. Response audio comes back through gateway
7. Plays through your laptop speakers

**Try Saying**:
- "Hello, how are you?"
- "Tell me a joke"
- "What's 25 times 34?"
- "Turn on the living room lights" (if HA is configured)

### 2. `wyoming_client.py` - Protocol Testing

**Purpose**: Test Wyoming protocol connectivity without audio hardware.

**Use Cases**:
- Verify gateway is accepting connections
- Test protocol implementation
- Debug connection issues
- CI/CD testing

**Requirements**:
- None (uses only standard library)

**Usage**:
```bash
# Test local gateway
python3 wyoming_client.py

# Test remote gateway
python3 wyoming_client.py --host 192.168.1.100 --port 10200
```

**What It Does**:
1. Connects to gateway
2. Sends audio-start event
3. Sends 10 simulated audio chunks (silent audio)
4. Sends audio-stop event
5. Listens for responses
6. Reports success/failure

## 🚀 Quick Start Guide

### Step 0: Set Up Python Virtual Environment (Recommended)

```bash
# Navigate to test directory and create venv
cd test/
python3 -m venv venv
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

### Step 1: Get a Gemini API Key

1. Go to https://aistudio.google.com/apikey
2. Click "Create API Key"
3. Copy the key

### Step 2: Configure Gateway

Create/update `.env` in project root:

```bash
# Gemini Configuration
BACKEND_TYPE=gemini
GEMINI_API_KEY=your_gemini_api_key_here
GEMINI_MODEL=gemini-2.0-flash-exp

# Gemini Performance (optional - sensible defaults)
GEMINI_CONNECT_TIMEOUT=30s
GEMINI_RECEIVE_TIMEOUT=60s
GEMINI_SEND_TIMEOUT=10s
GEMINI_MAX_RETRIES=3
GEMINI_RETRY_BACKOFF=1s
GEMINI_MAX_SESSIONS=5

# Wyoming Server
WYOMING_ADDR=0.0.0.0:10200

# Logging
LOG_LEVEL=debug
LOG_FORMAT=console

# System Prompt
SYSTEM_PROMPT="You are a helpful voice assistant. Be concise and friendly."

# Home Assistant (optional)
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
INFO    Using Gemini Live backend    model=gemini-2.0-flash-exp
INFO    Gemini backend initialized successfully
INFO    Pipeline initialized
INFO    Gateway ready    wyoming_addr=0.0.0.0:10200
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
```

**Speak into your microphone!** You should hear Gemini's response through your speakers.

## 🧪 Testing Scenarios

### Scenario 1: Basic Conversation Test

**Goal**: Verify Gemini responds to simple queries.

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
- Gateway logs show: "New Wyoming session connected"
- Gateway logs show: "WebSocket connection established"
- You hear Gemini's voice response

### Scenario 2: Home Assistant Control Test

**Prerequisites**:
- Home Assistant running
- `HA_URL` and `HA_TOKEN` configured in `.env`

**Say**: "Turn on the living room lights"

**Expected**:
- Gateway logs show tool execution
- Home Assistant service is called
- Lights turn on
- Gemini confirms the action

### Scenario 3: Protocol-Only Test

**Goal**: Verify gateway without audio hardware.

**Terminal 1 - Gateway:**
```bash
go run ./cmd/gateway/main.go
```

**Terminal 2 - Protocol Test:**
```bash
cd test/
source venv/bin/activate  # Optional, no dependencies needed
python3 wyoming_client.py
```

**Expected**:
```
✅ All tests passed!
• Gateway accepted Wyoming protocol connection
• Gateway processed audio-start/chunk/stop events
• Session was created and cleaned up properly
```

### Scenario 4: Long Conversation Test

**Goal**: Test extended interaction.

```bash
cd test/
source venv/bin/activate
python3 audio_bridge.py --duration 0  # Run indefinitely
```

**Try**:
- Multiple back-and-forth exchanges
- Interrupting Gemini mid-response
- Long questions
- Quick successive questions

**Monitor**:
- Gateway logs for errors
- Audio quality
- Response latency
- Memory usage

## 📊 What to Check

### Gateway Logs

**Good signs**:
```
INFO    New Wyoming session connected    session_id=...
INFO    starting Gemini session
DEBUG   WebSocket connection established
DEBUG   Audio chunk sent to Gemini
DEBUG   Received audio from Gemini
INFO    Tool call: light.turn_on         (if using HA)
```

**Warning signs**:
```
ERROR   WebSocket connection failed
ERROR   Gemini API error
WARN    Audio buffer overflow
ERROR   Failed to execute tool
```

### Audio Bridge Output

**Good signs**:
```
✅ Connected to gateway!
✅ Audio stream started - speak now!
🔊 Gateway started sending audio (Gemini is speaking!)
✅ Gateway finished sending audio
```

**Issues**:
```
❌ Connection refused  → Gateway not running
❌ Failed to open microphone  → Mic permissions or in use
❌ No response from gateway  → Check Gemini API key
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

**Problem**: Can't hear Gemini responses

**Solutions**:
1. Check volume level
2. Ensure speakers are selected as output device
3. Test speakers with: `python3 -c "import pyaudio; p = pyaudio.PyAudio(); print('Speakers:', p.get_default_output_device_info())"`
4. Check gateway logs for "Gateway started sending audio"

### Gateway Connection Refused

**Problem**: `Connection refused to localhost:10200`

**Solutions**:
1. Ensure gateway is running: `go run ./cmd/gateway/main.go`
2. Check port isn't in use: `lsof -i :10200`
3. Verify Wyoming address in `.env`: `WYOMING_ADDR=0.0.0.0:10200`
4. Check gateway logs for startup errors

### Gemini API Errors

**Problem**: `Failed to initialize Gemini backend`

**Solutions**:
1. Verify API key is correct in `.env`
2. Check API key hasn't expired
3. Ensure you have API quota remaining
4. Try creating a new API key

### High Latency

**Problem**: Slow responses (>2 seconds)

**Check**:
1. Internet connection speed
2. Gateway logs for delays
3. Audio buffer sizes in config
4. System resource usage

**Solutions**:
- Increase buffer sizes in `.env`
- Check CPU/memory usage
- Use faster internet connection
- Switch to lower-quality audio if needed

### Audio Quality Issues

**Problem**: Distorted or choppy audio

**Solutions**:
1. Check microphone input level (not too high)
2. Reduce background noise
3. Adjust buffer sizes in config
4. Check system audio settings
5. Try different audio device

## 💡 Tips & Best Practices

### For Best Results

1. **Use a good microphone**: Built-in laptop mics work, but external mics are better
2. **Quiet environment**: Reduce background noise for better recognition
3. **Clear speech**: Speak clearly and at a normal pace
4. **Check logs**: Always monitor gateway logs during testing
5. **Start simple**: Test basic queries before complex HA commands


### Performance Optimization

- **Debug logging**: Use `LOG_LEVEL=info` in production, `debug` only for testing
- **Buffer sizes**: Adjust `AUDIO_BUFFER_SIZE` if experiencing glitches
- **System resources**: Close unnecessary applications during testing

### Security Considerations

- **Never commit** `.env` files with real API keys
- **Use allow-lists** for HA service calls in production
- **Test in isolated environment** before production deployment
- **Monitor API usage** to avoid unexpected charges

## 📈 Test Metrics

Track these metrics during testing:

- **Latency**: Time from speaking to hearing response (<500ms is good, <2s is acceptable)
- **Audio Quality**: Clear, understandable speech
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

2. **Configure Home Assistant**:
   - Add Wyoming integration
   - Point to gateway:10200
   - Create assist pipeline

3. **Connect physical device**:
   - Configure HA Voice Preview device
   - Point to gateway IP:10200
   - Test with "OK Nabu" wake word

4. **Monitor in production**:
   - Enable metrics
   - Set up logging
   - Configure alerts

## 📚 Additional Resources

- [Wyoming Protocol Spec](https://github.com/rhasspy/wyoming)
- [Gemini Live API Docs](https://ai.google.dev/gemini-api/docs/live)
- [Home Assistant Voice](https://www.home-assistant.io/voice_control/)
- [Project README](../Readme.md)
- [Docker Deployment Guide](../docs/DOCKER.md)
- [Autodiscovery Guide](../docs/AUTODISCOVERY.md)
- [VAD Tuning Guide](../docs/VAD_TUNING_GUIDE.md)

## 🤝 Contributing

Found an issue or have an improvement? Please open an issue or PR on GitHub!

---

**Happy Testing!** 🎉

