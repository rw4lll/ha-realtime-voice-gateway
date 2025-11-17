# Audio Formats Quick Reference

## Device & Backend Compatibility

### Voice Assistant Preview Device (ESP32)
```
Input (Mic):    16kHz, 16-bit, mono PCM
Output (Speaker): 16kHz, 16-bit, mono PCM
Protocol:       WebSocket
Encoding:       Little-endian PCM (pcm_s16le)
```

### Gemini Live API
```
Input:          16kHz, 16-bit, mono PCM ✅
Output:         24kHz, 16-bit, mono PCM ⚠️ (requires resampling)
Protocol:       WebSocket/gRPC
Encoding:       Little-endian PCM (pcm_s16le)
```

### OpenAI Realtime API (Coming Soon)
```
Input:          24kHz, 16-bit, mono PCM
Output:         24kHz, 16-bit, mono PCM
Protocol:       WebSocket
Encoding:       Little-endian PCM (pcm_s16le)
```

---

## Gateway Audio Pipeline

```
┌─────────────────────────────────────────────────────────┐
│                   Voice Assistant Device                │
│              (16kHz PCM, WebSocket)                     │
└────────────────────┬────────────────────────────────────┘
                     │ Binary WebSocket frames
                     │ (16kHz audio)
                     ↓
┌─────────────────────────────────────────────────────────┐
│                      Gateway                            │
│  ┌──────────────────────────────────────────────────┐  │
│  │  Session Handler                                 │  │
│  │  • Audio buffering                              │  │
│  │  • State management                             │  │
│  │  • Tool execution                               │  │
│  └──────────────────┬───────────────────────────────┘  │
│                     │                                    │
│                     │ 16kHz (no conversion needed)      │
│                     ↓                                    │
│  ┌──────────────────────────────────────────────────┐  │
│  │  Backend Connector (Gemini/OpenAI)              │  │
│  └──────────────────┬───────────────────────────────┘  │
└────────────────────┼────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│                  LLM Backend (Gemini)                   │
│                                                         │
│  Input:  16kHz ✅                                       │
│  Output: 24kHz ⚠️                                        │
└────────────────────┬────────────────────────────────────┘
                     │
                     │ 24kHz audio response
                     ↓
┌─────────────────────────────────────────────────────────┐
│                      Gateway                            │
│  ┌──────────────────────────────────────────────────┐  │
│  │  Audio Resampler                                │  │
│  │  • Detects format mismatch (24kHz ≠ 16kHz)     │  │
│  │  • Resamples 24kHz → 16kHz                      │  │
│  │  • Linear interpolation (~5ms latency)         │  │
│  └──────────────────┬───────────────────────────────┘  │
│                     │                                    │
│                     │ 16kHz (resampled)                 │
│                     ↓                                    │
│  ┌──────────────────────────────────────────────────┐  │
│  │  Session Handler                                 │  │
│  │  • Buffers audio                                │  │
│  │  • Handles interruptions                        │  │
│  └──────────────────┬───────────────────────────────┘  │
└────────────────────┼────────────────────────────────────┘
                     │
                     │ Binary WebSocket frames
                     │ (16kHz audio)
                     ↓
┌─────────────────────────────────────────────────────────┐
│                   Voice Assistant Device                │
│              Plays at 16kHz ✅                          │
└─────────────────────────────────────────────────────────┘
```

---

## Sample Rate Conversion Chart

| Source | Target | Ratio | Algorithm | Latency | Quality |
|--------|--------|-------|-----------|---------|---------|
| 24kHz  | 16kHz  | 2:3   | Fast (averaging) | ~4ms | Excellent |
| 16kHz  | 24kHz  | 3:2   | Linear interp | ~6ms | Good |
| 48kHz  | 16kHz  | 1:3   | Linear interp | ~5ms | Excellent |
| 8kHz   | 16kHz  | 2:1   | Linear interp | ~3ms | Good |

---

## Frequency Response

### 16kHz PCM
- **Nyquist Frequency**: 8kHz
- **Human Speech**: 300Hz - 3.5kHz (fully preserved)
- **Usable Range**: 0Hz - 7kHz
- **Telephone Quality**: Equivalent

### 24kHz PCM
- **Nyquist Frequency**: 12kHz
- **Human Speech**: 300Hz - 3.5kHz (fully preserved)
- **Usable Range**: 0Hz - 11kHz
- **Near-CD Quality**: Better than 16kHz, not quite CD (44.1kHz)

### Impact of 24kHz → 16kHz Downsampling
- ✅ **Speech intelligibility**: Unaffected (speech is 300Hz-3.5kHz)
- ✅ **Voice tone**: Fully preserved
- ⚠️ **High frequencies**: Frequencies above 8kHz removed (anti-aliasing)
- ⚠️ **Sibilants**: Slight softening of "s", "sh" sounds (minor)
- ✅ **Overall quality**: Imperceptible for voice applications

---

## Audio Frame Sizes

### Device → Gateway (16kHz)
```
10ms frame  = 160 samples  = 320 bytes
20ms frame  = 320 samples  = 640 bytes (typical)
30ms frame  = 480 samples  = 960 bytes
```

### Gemini → Gateway (24kHz)
```
10ms frame  = 240 samples  = 480 bytes
20ms frame  = 480 samples  = 960 bytes (typical)
30ms frame  = 720 samples  = 1440 bytes
```

### After Resampling (24kHz → 16kHz)
```
480 bytes (24kHz, 10ms) → 320 bytes (16kHz, 10ms)
960 bytes (24kHz, 20ms) → 640 bytes (16kHz, 20ms)
1440 bytes (24kHz, 30ms) → 960 bytes (16kHz, 30ms)

Reduction: 33.3% (2/3 of original size)
```

---

## Buffer Calculations

### Gateway Audio Buffer (Default: 100ms)

**At 16kHz:**
- Buffer size: 100ms × 16000 samples/s = 1600 samples = 3200 bytes
- Frame count: 100ms / 20ms = 5 frames

**At 24kHz (before resampling):**
- Buffer size: 100ms × 24000 samples/s = 2400 samples = 4800 bytes
- Frame count: 100ms / 20ms = 5 frames

---

## Testing Audio Formats

### Test with Software Client (Laptop)
```bash
cd test/
python3 audio_bridge.py

# Script automatically handles:
# - 16kHz microphone input
# - 24kHz speaker output
# (Works because laptop audio hardware is flexible)
```

### Test with ESP32 Device
```bash
# Gateway automatically resamples
# Check logs:
LOG_LEVEL=debug docker-compose logs gateway | grep -i resample

# Expected output:
# INFO  Resampling enabled: 24000Hz → 16000Hz
# DEBUG Resampled frame: 960→640 bytes, 4.2ms
```

### Verify Audio Quality
```bash
# Listen for:
# ✅ Natural speech speed
# ✅ Correct pitch
# ✅ No robotic artifacts
# ✅ Smooth playback

# If issues:
# - Check buffer warnings: grep "buffer" logs
# - Monitor CPU: docker stats gateway
# - Verify formats: grep "audio_format" logs
```

---

## See Also

- **[AUDIO_RESAMPLING.md](AUDIO_RESAMPLING.md)** - Technical deep dive and solutions comparison
- **[AUDIO_RESAMPLING_CONFIG.md](AUDIO_RESAMPLING_CONFIG.md)** - Configuration guide and troubleshooting
- **[CONFIGURATION.md](CONFIGURATION.md)** - All configuration options
- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - General debugging guide
- **[../test/TESTING_RESAMPLING.md](../test/TESTING_RESAMPLING.md)** - How to test resampling

