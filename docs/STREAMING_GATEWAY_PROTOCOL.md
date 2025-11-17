# Streaming Gateway Protocol Specification

## Overview

This document describes the WebSocket protocol between the ESP32 voice device running the **Home Assistant Voice PE** firmware with streaming gateway mode enabled and a streaming gateway server. The gateway has full control over conversation flow, VAD (Voice Activity Detection), and session management.

**Firmware Version:** Home Assistant Voice PE with `streaming_voice_gateway` component  
**Protocol Version:** 1.0  
**Last Updated:** November 2025

## Quick Start

### Device Configuration (Home Assistant)

After flashing the firmware with streaming gateway support:

1. **Configure Gateway URL:**
   - In Home Assistant, go to your voice device
   - Set **"Streaming Gateway URL"** to your gateway server
   - Example: `ws://192.168.1.100:8080/voice-stream`

2. **Enable Streaming Mode:**
   - Toggle **"Streaming Mode"** switch to ON
   - Or use 4-click button on device (high beep = streaming mode)

3. **Start Conversation:**
   - Say wake word or single-click button
   - Device connects to gateway and streams audio
   - Gateway controls conversation flow via JSON messages

### Activation Methods

**Method 1: Wake Word**
- Say configured wake word (default: "Hey Jarvis")
- Device automatically connects to gateway
- Starts streaming audio

**Method 2: Button Press**
- Single-click center button
- Device connects and waits for your voice
- Single-click again to stop

**Method 3: Home Assistant**
- Call `streaming_voice_gateway.start` action
- Programmatically trigger conversations

## Connection Flow

```
┌─────────┐                    ┌─────────┐                  ┌────────────┐
│ Device  │                    │ Gateway │                  │ AI Provider│
└────┬────┘                    └────┬────┘                  └─────┬──────┘
     │                              │                              │
     │ 1. Wake Word Detected        │                              │
     │──────────────────────────────>│                              │
     │                              │ 2. Establish AI Connection   │
     │                              │─────────────────────────────>│
     │                              │                              │
     │ 3. Stream Audio (continuous) │                              │
     │─────────────────────────────>│ 4. Stream to AI              │
     │                              │─────────────────────────────>│
     │                              │                              │
     │ {"state": "listening"}       │                              │
     │<─────────────────────────────│                              │
     │                              │                              │
     │                              │ 5. AI Detects End of Speech  │
     │                              │     (VAD on Gateway/AI)      │
     │                              │                              │
     │ {"state": "thinking"}        │ 6. AI Processing             │
     │<─────────────────────────────│<─────────────────────────────│
     │                              │                              │
     │                              │ 7. AI Streams Response       │
     │ {"state": "speaking"}        │<─────────────────────────────│
     │<─────────────────────────────│                              │
     │                              │                              │
     │ Binary Audio Frames          │                              │
     │<─────────────────────────────│                              │
     │                              │                              │
     │ (Device still streams audio  │                              │
     │  for potential interruption) │                              │
     │─────────────────────────────>│                              │
     │                              │                              │
     │ {"state": "done"}            │ 8. Conversation Complete     │
     │<─────────────────────────────│     (VAD timeout, goodbye)   │
     │                              │                              │
     │ 9. Close Connection          │                              │
     │──────────────────────────────>│                              │
```

## WebSocket Connection

**URL Format:** `ws://gateway-host:port/voice-stream`

**Connection:** Initiated by device after wake word detection

**Protocol:** Binary WebSocket with mixed message types

## Message Types

### 1. Device → Gateway (Audio Stream)

**Type:** Binary WebSocket frames

**Format:** Raw PCM audio
- Sample Rate: 16 kHz
- Bit Depth: 16-bit signed integer
- Channels: Mono (1 channel)
- Encoding: Little-endian PCM
- Frame Size: Variable (typically 1024 bytes)

**Behavior:**
- Continuous streaming from wake word until gateway sends `{"state": "done"}` or `{"command": "stop"}`
- Device does **not** perform VAD - streams everything to gateway
- Only stops on gateway command or safety timeout (5 minutes max)

**Example (pseudocode):**
```cpp
// Continuous loop while connected
while (ws_connected) {
  int16_t audio_chunk[512]; // 512 samples = 1024 bytes
  read_microphone(audio_chunk);
  ws.send_binary(audio_chunk, 1024);
}
```

### 2. Gateway → Device (Control Messages)

**Type:** Text WebSocket frames (JSON)

**Format:** JSON objects with control commands

#### State Updates

Tell device what phase the conversation is in (for LED control, logging):

```json
{"state": "listening"}
```
- Gateway is actively listening to user speech
- Microphone continues streaming
- Device can show "listening" LED pattern

```json
{"state": "thinking"}
```
- Gateway/AI is processing user input
- Microphone continues streaming (for interruption)
- Device can show "thinking" LED pattern

```json
{"state": "speaking"}
```
- Gateway/AI is delivering audio response
- Microphone continues streaming (for interruption)
- Device can show "speaking" LED pattern

```json
{"state": "done"}
```
- **Conversation complete** - device should close connection
- Sent when:
  - VAD detects silence after AI response (timeout)
  - User says goodbye/stop/exit
  - Gateway-specific logic determines conversation is complete
- Device stops microphone and transitions to IDLE

#### Stop Command

Force immediate stop (emergency/error cases):

```json
{"command": "stop"}
```
- Device immediately stops streaming and closes connection
- Use for gateway shutdown, rate limiting, or critical errors

#### Error Messages

Notify device of gateway errors:

```json
{"error": "AI provider unreachable"}
```
- Device logs error, triggers error callback
- Device transitions to ERROR state and cleans up

### 3. Gateway → Device (Audio Response)

**Type:** Binary WebSocket frames

**Format:** Raw PCM audio (same format as device sends)
- Sample Rate: 16 kHz
- Bit Depth: 16-bit signed integer
- Channels: Mono
- Encoding: Little-endian PCM

**Behavior:**
- Gateway streams AI-generated speech
- Can arrive **during any state** (true full-duplex)
- Device buffers and plays through speaker
- Multiple binary frames can be sent continuously

**Example Flow:**
```
1. Gateway: {"state": "speaking"}
2. Gateway: [binary audio frame 1]
3. Gateway: [binary audio frame 2]
4. Gateway: [binary audio frame 3]
   ...
N. Gateway: {"state": "done"}
```

## Full-Duplex Behavior

**Key Concept:** Microphone streams continuously; gateway controls everything.

```
Time →
│
├─ Wake Word Detected
│  └─ Device: Connect WebSocket + Start Mic
│
├─ Device: Stream audio ──────────────────────────────────────────>
│          (continuous until "done")
│
├─ Gateway: {"state": "listening"}
│
├─ Gateway: (VAD detects end of speech internally)
│
├─ Gateway: {"state": "thinking"}
│
├─ Device: Still streaming audio ────────────────────────────────>
│          (in case user wants to interrupt)
│
├─ Gateway: {"state": "speaking"}
├─ Gateway: [Audio] [Audio] [Audio] ... (AI response)
│
├─ Device: Still streaming audio ────────────────────────────────>
│          (user can interrupt AI mid-response)
│
├─ Gateway: (VAD detects silence after response - 2-3 seconds)
│
├─ Gateway: {"state": "done"}
│
└─ Device: Stop microphone, close connection
```

## Safety Mechanisms

### Device-Side Safety Timeouts

**Purpose:** Prevent stuck connections (not for normal flow control)

1. **Conversation Timeout: 5 minutes**
   - Maximum duration of any single conversation
   - Triggers ERROR state if exceeded
   - `ESP_LOGW("Safety timeout - conversation too long")`

2. **Gateway Response Timeout: 60 seconds**
   - Maximum time without any message from gateway
   - Triggers ERROR if no JSON/binary data received
   - `ESP_LOGE("Gateway not responding")`

**Note:** These are **safety nets only** - normal conversations should end via gateway's `{"state": "done"}` command.

## Gateway Implementation Requirements

### Required Features

1. **WebSocket Server**
   - Accept connections from devices
   - Handle binary audio frames (16kHz, 16-bit, mono PCM)
   - Send JSON control messages and binary audio responses

2. **Voice Activity Detection (VAD)**
   - Detect when user starts speaking
   - Detect when user stops speaking
   - Detect silence after AI response (typically 2-3 seconds)

3. **AI Provider Integration**
   - Connect to chosen AI service (Gemini, OpenAI, custom)
   - Stream audio to AI
   - Stream AI responses back to device

4. **Session Management**
   - Track conversation state per device
   - Send appropriate state updates
   - Clean termination via `{"state": "done"}`

### Example Go Implementation (Pseudocode)

```go
package main

import (
    "github.com/gorilla/websocket"
    "your-ai-provider-sdk"
)

type VoiceSession struct {
    conn        *websocket.Conn
    aiStream    *ai.StreamingSession
    vadDetector *VAD
}

func handleVoiceConnection(w http.ResponseWriter, r *http.Request) {
    // Upgrade to WebSocket
    conn, _ := upgrader.Upgrade(w, r, nil)
    defer conn.Close()

    // Initialize AI connection
    aiStream := ai.NewStreamingSession(apiKey)
    defer aiStream.Close()

    // Start listening state
    conn.WriteJSON(map[string]string{"state": "listening"})

    // Audio streaming goroutine
    go func() {
        for {
            msgType, audioData, err := conn.ReadMessage()
            if err != nil {
                break
            }
            if msgType == websocket.BinaryMessage {
                // Forward to AI provider
                aiStream.SendAudio(audioData)
                
                // VAD: Detect speech end
                if vad.DetectSpeechEnd(audioData) {
                    conn.WriteJSON(map[string]string{"state": "thinking"})
                    aiStream.FinalizeInput()
                }
            }
        }
    }()

    // AI response handler
    for audioChunk := range aiStream.AudioResponse() {
        // Update state when AI starts speaking
        if firstChunk {
            conn.WriteJSON(map[string]string{"state": "speaking"})
        }
        
        // Stream audio to device
        conn.WriteMessage(websocket.BinaryMessage, audioChunk)
    }

    // VAD: Detect silence after response (2-3 seconds)
    time.Sleep(2 * time.Second)
    if vad.IsSilent() {
        conn.WriteJSON(map[string]string{"state": "done"})
    }
}
```

## Home Assistant Integration

The device can still be managed via Home Assistant:

1. **Mode Toggle:** Switch between HA pipeline and streaming gateway
   - Entity: `switch.voice_kit_use_streaming_gateway`
   - Persisted across reboots

2. **Settings:** Configure via HA
   - Gateway URL
   - Audio quality settings (if needed)
   - Timeout values

3. **Status:** Monitor via HA sensors
   - Current conversation state
   - Error messages
   - Connection status

## Error Handling

### Gateway Errors

**Connection Failed:**
```cpp
// Device logs: "Failed to connect to gateway"
// Triggers: error_callback_("WebSocket connection failed")
// State: ERROR → IDLE
```

**Mid-Conversation Disconnect:**
```cpp
// Device logs: "WebSocket disconnected unexpectedly"
// Triggers: error_callback_("WebSocket disconnected")
// State: ERROR → IDLE (cleanup)
```

**Gateway Error Message:**
```json
{"error": "AI provider rate limit exceeded"}
```
```cpp
// Device logs: "Gateway error: AI provider rate limit exceeded"
// Triggers: error_callback_(error_message)
// State: ERROR → IDLE
```

### Device Errors

**Microphone Failure:**
```cpp
// Device logs: "Microphone not configured"
// Triggers: error_callback_("Microphone not configured")
// State: ERROR → IDLE
```

**Safety Timeout:**
```cpp
// Device logs: "Safety timeout reached (5 min)"
// Triggers: error_callback_("Safety timeout - conversation too long")
// State: ERROR → IDLE (forced cleanup)
```

## Testing Checklist

- [ ] Wake word triggers connection
- [ ] Audio streams continuously after connection
- [ ] Gateway state changes reflect in device LEDs
- [ ] AI audio plays through speaker
- [ ] `{"state": "done"}` properly terminates session
- [ ] Mid-conversation interruption works
- [ ] Gateway disconnect handled gracefully
- [ ] 5-minute safety timeout works
- [ ] Manual stop button works
- [ ] HA mode toggle works
- [ ] Multiple consecutive conversations work

---

## Implementation Guide

### Device Firmware Implementation

The Home Assistant Voice PE device firmware with streaming gateway support is built on ESPHome with a custom C++ component.

#### Device States

```cpp
enum StreamingState {
  STREAMING_STATE_IDLE = 0,        // Not connected
  STREAMING_STATE_START = 1,       // Connection initiated
  STREAMING_STATE_LISTENING = 2,   // Gateway listening for speech
  STREAMING_STATE_THINKING = 3,    // AI processing
  STREAMING_STATE_REPLYING = 4,    // AI responding
  STREAMING_STATE_ERROR = 5,       // Error occurred
};
```

#### Audio Conversion

The device microphone produces **32-bit PCM samples** which are converted to **16-bit PCM** before transmission:

```cpp
// Input: 32-bit samples from I2S microphone
// Output: 16-bit PCM for gateway
for (size_t i = 0; i + 3 < data.size(); i += 4) {
  int32_t sample = *(int32_t*)(data.data() + i);
  int16_t sample_16bit = (int16_t)(sample >> 16);  // Take upper 16 bits
  pcm16_buffer.push_back(sample_16bit);
}
```

#### WebSocket Message Handling

**Receiving Gateway Messages:**

```cpp
void handle_websocket_event(esp_websocket_event_id_t event_id, 
                            const uint8_t *data, size_t len) {
  switch (event_id) {
    case WEBSOCKET_EVENT_DATA:
      if (data[0] == '{') {
        // JSON control message
        parse_json_command(data, len);
      } else {
        // Binary audio from gateway
        speaker_->play(data, len);
      }
      break;
      
    case WEBSOCKET_EVENT_DISCONNECTED:
      // Handle disconnect
      cleanup_and_return_to_idle();
      break;
  }
}
```

**Parsing JSON Commands:**

```cpp
void parse_json_command(const uint8_t *data, size_t len) {
  cJSON *root = cJSON_Parse((const char*)data);
  
  // State transitions
  cJSON *state = cJSON_GetObjectItem(root, "state");
  if (state && cJSON_IsString(state)) {
    if (strcmp(state->valuestring, "listening") == 0) {
      set_state(STREAMING_STATE_LISTENING);
    } else if (strcmp(state->valuestring, "thinking") == 0) {
      set_state(STREAMING_STATE_THINKING);
    } else if (strcmp(state->valuestring, "speaking") == 0) {
      set_state(STREAMING_STATE_REPLYING);
    } else if (strcmp(state->valuestring, "done") == 0) {
      stop_and_cleanup();
    }
  }
  
  // Stop command
  cJSON *command = cJSON_GetObjectItem(root, "command");
  if (command && strcmp(command->valuestring, "stop") == 0) {
    stop_and_cleanup();
  }
  
  // Error handling
  cJSON *error = cJSON_GetObjectItem(root, "error");
  if (error && cJSON_IsString(error)) {
    ESP_LOGE(TAG, "Gateway error: %s", error->valuestring);
    trigger_error_callback(error->valuestring);
  }
  
  cJSON_Delete(root);
}
```

#### LED Control Integration

Device LEDs reflect the streaming gateway state:

```cpp
// LED patterns mapped to streaming states
switch (streaming_state) {
  case STREAMING_STATE_LISTENING:
    voice_assistant_phase = VOICE_ASSIST_LISTENING_FOR_COMMAND_PHASE;
    break;
  case STREAMING_STATE_THINKING:
    voice_assistant_phase = VOICE_ASSIST_THINKING_PHASE;
    break;
  case STREAMING_STATE_REPLYING:
    voice_assistant_phase = VOICE_ASSIST_REPLYING_PHASE;
    break;
}
control_leds();  // Update LED ring
```

#### Audio Ducking

When streaming gateway speaks, other audio (media player) is automatically ducked:

```cpp
on_speaking:
  - mixer_speaker.apply_ducking:
      id: media_mixing_input
      decibel_reduction: -30  # Duck media by 30dB
      duration: 0.5s

on_end:
  - mixer_speaker.apply_ducking:
      id: media_mixing_input
      decibel_reduction: 0    # Restore media volume
      duration: 1.0s
```

---

### Gateway Server Implementation

#### Minimal Server Structure (Go Example)

```go
type VoiceSession struct {
    conn          *websocket.Conn
    aiClient      *AIProvider
    vadEngine     *VAD
    lastAudioTime time.Time
    ctx           context.Context
    cancel        context.CancelFunc
}

func (s *VoiceSession) Start() {
    // Send initial state
    s.SendState("listening")
    
    // Handle incoming audio
    go s.handleDeviceAudio()
    
    // Handle AI responses
    go s.handleAIResponses()
    
    // Monitor VAD timeouts
    go s.monitorVAD()
}

func (s *VoiceSession) handleDeviceAudio() {
    for {
        msgType, data, err := s.conn.ReadMessage()
        if err != nil {
            return
        }
        
        if msgType == websocket.BinaryMessage {
            s.lastAudioTime = time.Now()
            
            // Forward to AI
            s.aiClient.SendAudio(data)
            
            // Run VAD
            if s.vadEngine.DetectEndOfSpeech(data) {
                s.SendState("thinking")
                s.aiClient.FinalizeInput()
            }
        }
    }
}

func (s *VoiceSession) handleAIResponses() {
    for audioChunk := range s.aiClient.AudioOutput() {
        if firstChunk {
            s.SendState("speaking")
        }
        s.conn.WriteMessage(websocket.BinaryMessage, audioChunk)
    }
}

func (s *VoiceSession) monitorVAD() {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            silenceDuration := time.Since(s.lastAudioTime)
            
            // Post-response timeout (conversation done)
            if s.aiFinishedSpeaking && silenceDuration > 2*time.Second {
                s.SendState("done")
                return
            }
        case <-s.ctx.Done():
            return
        }
    }
}

func (s *VoiceSession) SendState(state string) error {
    msg := map[string]string{"state": state}
    return s.conn.WriteJSON(msg)
}
```

#### Python WebSocket Server Example

```python
import asyncio
import websockets
import json

async def handle_voice_session(websocket, path):
    """Handle a voice streaming session"""
    
    # Send initial listening state
    await websocket.send(json.dumps({"state": "listening"}))
    
    audio_buffer = bytearray()
    ai_client = AIProvider()
    vad = VoiceActivityDetector()
    
    try:
        async for message in websocket:
            if isinstance(message, bytes):
                # Binary audio from device
                audio_buffer.extend(message)
                
                # Forward to AI
                await ai_client.send_audio(message)
                
                # Check VAD
                if vad.is_speech_ended(audio_buffer):
                    await websocket.send(json.dumps({"state": "thinking"}))
                    
                    # Process with AI
                    response = await ai_client.finalize_and_get_response()
                    
                    # Send AI response
                    await websocket.send(json.dumps({"state": "speaking"}))
                    async for audio_chunk in response:
                        await websocket.send(audio_chunk)
                    
                    # Wait for silence, then end
                    await asyncio.sleep(2)
                    await websocket.send(json.dumps({"state": "done"}))
                    break
                    
    except websockets.exceptions.ConnectionClosed:
        print("Device disconnected")

# Start server
start_server = websockets.serve(handle_voice_session, "0.0.0.0", 8080)
asyncio.get_event_loop().run_until_complete(start_server)
asyncio.get_event_loop().run_forever()
```

---

### AI Provider Integration Examples

#### Google Gemini Live API

```python
import google.generativeai as genai

class GeminiProvider:
    def __init__(self, api_key):
        genai.configure(api_key=api_key)
        self.client = genai.GenerativeModel('gemini-2.0-flash-exp')
        
    async def process_audio_stream(self, audio_iterator):
        """Stream audio to Gemini and get response"""
        
        # Start live session
        session = self.client.start_live_session()
        
        # Send audio chunks
        for chunk in audio_iterator:
            session.send_audio(chunk)
        
        # Finalize input
        session.finalize_input()
        
        # Stream response
        async for response in session.stream_response():
            if response.audio:
                yield response.audio
```

#### OpenAI Realtime API

```python
from openai import AsyncOpenAI

class OpenAIProvider:
    def __init__(self, api_key):
        self.client = AsyncOpenAI(api_key=api_key)
        
    async def process_audio_stream(self, audio_iterator):
        """Stream audio to OpenAI Realtime API"""
        
        session = await self.client.realtime.connect()
        
        # Configure session
        await session.update({
            "voice": "alloy",
            "input_audio_format": "pcm16",
            "output_audio_format": "pcm16"
        })
        
        # Stream input audio
        for chunk in audio_iterator:
            await session.send_audio(chunk)
        
        await session.commit_audio_buffer()
        
        # Receive response
        async for event in session.listen():
            if event.type == "response.audio.delta":
                yield event.delta
            elif event.type == "response.done":
                break
```

---

### Voice Activity Detection (VAD)

#### Simple Time-Based VAD

```python
class SimpleVAD:
    def __init__(self, speech_end_silence_ms=1500):
        self.speech_end_silence_ms = speech_end_silence_ms
        self.last_audio_time = None
        self.speech_detected = False
        
    def process_audio(self, audio_chunk):
        """Process audio chunk, return True if speech ended"""
        
        # Check if audio has energy (simple RMS)
        rms = self.calculate_rms(audio_chunk)
        
        if rms > self.threshold:
            self.last_audio_time = time.time()
            self.speech_detected = True
            return False
        
        if self.speech_detected:
            silence_duration = time.time() - self.last_audio_time
            if silence_duration * 1000 > self.speech_end_silence_ms:
                self.speech_detected = False
                return True  # Speech ended
        
        return False
    
    def calculate_rms(self, audio_bytes):
        """Calculate RMS of audio chunk"""
        samples = np.frombuffer(audio_bytes, dtype=np.int16)
        return np.sqrt(np.mean(samples**2))
```

#### Silero VAD (Production-Ready)

```python
import torch
import torchaudio

class SileroVAD:
    def __init__(self):
        self.model, utils = torch.hub.load(
            repo_or_dir='snakers4/silero-vad',
            model='silero_vad',
            force_reload=False
        )
        self.get_speech_timestamps = utils[0]
        
    def detect_speech_end(self, audio_chunk, sample_rate=16000):
        """Detect speech end using Silero VAD"""
        
        # Convert bytes to tensor
        audio_tensor = torch.frombuffer(audio_chunk, dtype=torch.int16)
        audio_tensor = audio_tensor.float() / 32768.0
        
        # Get speech probability
        speech_prob = self.model(audio_tensor, sample_rate).item()
        
        return speech_prob < 0.3  # Below threshold = silence
```

---

### Configuration Examples

#### Device Configuration (ESPHome YAML)

Complete configuration in `home-assistant-voice.yaml`:

```yaml
# Global variables for dynamic configuration
globals:
  - id: streaming_gateway_url
    type: std::string
    restore_value: yes
    initial_value: '"ws://192.168.1.100:8080/voice-stream"'
  
  - id: streaming_mode_enabled
    type: bool
    restore_value: yes
    initial_value: 'false'

# Text input for gateway URL (configurable from HA)
text:
  - platform: template
    name: "Streaming Gateway URL"
    id: gateway_url_input
    optimistic: true
    max_length: 200
    mode: text
    initial_value: "ws://192.168.1.100:8080/voice-stream"
    on_value:
      then:
        - lambda: |-
            id(streaming_gateway_url) = x;
            id(streaming_gateway).set_gateway_url(x);

# Switch to enable/disable streaming mode
switch:
  - platform: template
    name: "Streaming Mode"
    id: streaming_mode_switch
    icon: "mdi:microphone-message"
    optimistic: true
    restore_mode: RESTORE_DEFAULT_OFF
    lambda: return id(streaming_mode_enabled);
    turn_on_action:
      - lambda: id(streaming_mode_enabled) = true;
    turn_off_action:
      - lambda: id(streaming_mode_enabled) = false;

# Streaming voice gateway component
streaming_voice_gateway:
  id: streaming_gateway
  microphone: i2s_mics
  speaker: i2s_audio_speaker
  gateway_url: "ws://192.168.1.100:8080/voice-stream"
  
  on_start:
    - lambda: |-
        id(streaming_gateway).set_gateway_url(id(streaming_gateway_url));
        id(voice_assistant_phase) = 2;  # Waiting for command
    - script.execute: control_leds
  
  on_listening:
    - lambda: id(voice_assistant_phase) = 3;  # Listening
    - script.execute: control_leds
  
  on_thinking:
    - lambda: id(voice_assistant_phase) = 4;  # Thinking
    - script.execute: control_leds
  
  on_speaking:
    - lambda: id(voice_assistant_phase) = 5;  # Replying
    - script.execute: control_leds
  
  on_end:
    - lambda: id(voice_assistant_phase) = 1;  # Idle
    - script.execute: control_leds
  
  on_error:
    - lambda: |-
        id(voice_assistant_phase) = 11;  # Error
        ESP_LOGE("main", "Gateway error: %s", error.c_str());
    - script.execute: control_leds

# Button handler with mode detection
binary_sensor:
  - platform: gpio
    pin:
      number: GPIO0
      inverted: true
    name: "Center Button"
    on_multi_click:
      # Single click - Start/stop (mode-aware)
      - timing:
          - ON for at most 1s
          - OFF for at least 0.25s
        then:
          - if:
              condition:
                lambda: return id(streaming_mode_enabled);
              then:
                # Streaming Gateway Mode
                - if:
                    condition:
                      lambda: return id(streaming_gateway).is_running();
                    then:
                      - streaming_voice_gateway.stop:
                          id: streaming_gateway
                    else:
                      - streaming_voice_gateway.start:
                          id: streaming_gateway
              else:
                # HA Pipeline Mode
                - voice_assistant.start:
      
      # 4 clicks - Toggle mode
      - timing:
          - ON for at most 1s
          - OFF for at most 0.25s
          - ON for at most 1s
          - OFF for at most 0.25s
          - ON for at most 1s
          - OFF for at most 0.25s
          - ON for at most 1s
          - OFF for at least 0.25s
        then:
          - lambda: id(streaming_mode_enabled) = !id(streaming_mode_enabled);
```

---

### Security Considerations

#### Authentication

**API Key in URL:**
```
ws://gateway.example.com/voice-stream?api_key=your_device_key
```

**Header-Based:**
```go
func handleConnection(w http.ResponseWriter, r *http.Request) {
    apiKey := r.Header.Get("X-API-Key")
    if !validateAPIKey(apiKey) {
        w.WriteHeader(http.StatusUnauthorized)
        return
    }
    // Upgrade to WebSocket
}
```

**Device Certificate (mTLS):**
```yaml
# In ESPHome (future enhancement)
streaming_voice_gateway:
  gateway_url: "wss://secure-gateway.example.com/voice-stream"
  client_cert: !secret device_cert
  client_key: !secret device_key
```

#### Rate Limiting

```go
var rateLimiter = rate.NewLimiter(rate.Every(time.Second), 10)

func handleConnection(w http.ResponseWriter, r *http.Request) {
    if !rateLimiter.Allow() {
        sendError(conn, "Rate limit exceeded")
        return
    }
}
```

---

### Monitoring & Observability

#### Logging on Device

```cpp
ESP_LOGI(TAG, "Connecting to gateway: %s", gateway_url.c_str());
ESP_LOGD(TAG, "Audio frame sent: %d bytes", len);
ESP_LOGW(TAG, "Gateway response timeout");
ESP_LOGE(TAG, "WebSocket error: %s", error);
```

#### Prometheus Metrics (Gateway)

```go
var (
    activeConnections = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "voice_gateway_active_connections",
        Help: "Number of active voice sessions",
    })
    
    conversationDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name: "voice_gateway_conversation_duration_seconds",
        Help: "Duration of voice conversations",
    })
    
    audioFramesReceived = promauto.NewCounter(prometheus.CounterOpts{
        Name: "voice_gateway_audio_frames_received_total",
        Help: "Total audio frames received from devices",
    })
)
```

---

## Protocol Version

**Version:** 1.0  
**Date:** November 2025  
**Compatible Device Firmware:** Home Assistant Voice PE (ESP32-S3) with `streaming_voice_gateway` component  
**Gateway Requirements:** WebSocket server with binary/text frame support, VAD, AI integration

**Reference Implementation:** See `examples/go-gateway/` for complete Go server example
