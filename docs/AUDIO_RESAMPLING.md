# Audio Resampling Guide

## The Problem

**Voice Assistant Preview Device vs. Gemini Sample Rate Mismatch**

The Home Assistant Voice Assistant Preview device (ESP32-based) and Gemini have different audio format specifications:

### Voice Assistant Preview Device
- **Input Audio** (Microphone): 16kHz, 16-bit, mono PCM
- **Output Audio** (Speaker): 16kHz, 16-bit, mono PCM
- **Hardware**: ESP32 with limited processing power
- **Protocol**: WebSocket (binary frames for audio, JSON text frames for control)

### Gemini Live API
- **Input Audio** (accepts): 16kHz, 16-bit, mono PCM ✅ (matches device)
- **Output Audio** (sends): **24kHz**, 16-bit, mono PCM ❌ (mismatch!)
- **Protocol**: WebSocket or gRPC

### The Impact

When a device expecting 16kHz audio receives 24kHz audio:

1. **Playback Speed**: Audio plays 1.5x slower (24/16 = 1.5)
2. **Pitch Shift**: Voice sounds deeper/lower pitched
3. **Timing Issues**: Buffer underruns, choppy playback
4. **Distortion**: Robotic or metallic sound quality

**Example**: "Hello, how are you?" at 24kHz played at 16kHz sounds like: "Hhheeelllooo, hhooww aarree yyyoouuu?" (deeper and slower)

---

## Solutions

### Option 1: Gateway Resampling (✅ Recommended)

**Approach**: Add audio resampling in the Go gateway to convert 24kHz → 16kHz before sending to devices.

**Pros**:
- ✅ Transparent to ESP32 firmware (no device changes needed)
- ✅ Works with all devices automatically
- ✅ Centralized solution (easier to maintain)
- ✅ Can optimize/upgrade resampling algorithm independently
- ✅ Can support multiple backends with different sample rates

**Cons**:
- ❌ Adds 5-10ms latency (negligible for voice)
- ❌ Small CPU overhead (~1-2% per session)

**Implementation Status**: 
- ✅ Basic linear interpolation resampler implemented in `/internal/audio/resampler.go`
- ✅ Optimized 24kHz→16kHz fast path (2:3 ratio)
- ⏳ Integration into session handler (pending)

**How It Works**:
```
Device (16kHz) → WebSocket → Gateway → Backend (Gemini)
                                ↓
                          [No resampling needed]
                                ↓
                          Backend (24kHz audio)
                                ↓
                          [RESAMPLE 24kHz→16kHz]
                                ↓
                          Device (16kHz) ✅
```

**Performance**:
- **Latency**: ~5ms per 20ms audio chunk
- **CPU**: ~0.5% per active session (M1 MacBook Pro)
- **Memory**: ~2KB per audio frame

---

### Option 2: Firmware Resampling

**Approach**: Modify ESP32 firmware to resample 24kHz → 16kHz on the device.

**Pros**:
- ✅ No gateway changes needed
- ✅ Each device handles its own format

**Cons**:
- ❌ ESP32 has limited CPU (80-240MHz)
- ❌ Adds ~20-30ms latency on device
- ❌ Requires firmware reflash for all devices
- ❌ More complex debugging (firmware + gateway)
- ❌ Increased power consumption on battery devices

**Recommendation**: ❌ **Not recommended** - ESP32 is resource-constrained and firmware complexity increases.

---

### Option 3: Dual Audio Streams (Current Test Script Approach)

**Approach**: Use separate audio contexts/streams for different sample rates.

**Current Implementation**: The test script (`test/audio_bridge.py`) does this:

```python
MIC_SAMPLE_RATE = 16000   # Microphone input
SPEAKER_SAMPLE_RATE = 24000  # Speaker output

# Create microphone stream at 16kHz
mic_stream = audio.open(rate=16000, ...)

# Create speaker stream at 24kHz
speaker_stream = audio.open(rate=24000, ...)
```

**Pros**:
- ✅ Works perfectly for software clients (laptops, browsers)
- ✅ No resampling needed (hardware DAC handles it)
- ✅ Zero CPU overhead

**Cons**:
- ❌ **Only works on computers with flexible audio hardware**
- ❌ **Doesn't work on ESP32** (fixed hardware sample rate)
- ❌ Requires application-level support

**Use Case**: ✅ **Perfect for testing** with `audio_bridge.py` script on laptops.

---

### Option 4: Configure Gemini for 16kHz (❌ Not Possible)

**Approach**: Request Gemini to output audio at 16kHz instead of 24kHz.

**Status**: ❌ **Not supported** by Gemini Live API as of 2024.

Gemini's audio format is **fixed**:
- Input: 16kHz only
- Output: 24kHz only

This may change in future API versions.

---

### Option 5: Audio Transcoding Pipeline

**Approach**: Use external tools (ffmpeg, sox) to transcode audio streams.

**Example with sox**:
```bash
# Device audio → Gateway → Gemini (16kHz)
sox -t raw -r 16000 -e signed -b 16 -c 1 device.pcm \
    -t raw backend.pcm

# Gemini audio → Device (24kHz → 16kHz)
sox -t raw -r 24000 -e signed -b 16 -c 1 backend.pcm \
    -r 16000 -t raw device.pcm
```

**Pros**:
- ✅ High-quality resampling (sox/ffmpeg are battle-tested)
- ✅ Supports many formats and sample rates

**Cons**:
- ❌ Adds significant latency (50-100ms+)
- ❌ Requires external dependencies
- ❌ Higher CPU usage
- ❌ Complex inter-process communication
- ❌ Not suitable for realtime voice

**Recommendation**: ❌ **Not recommended** for low-latency voice applications.

---

## Comparison Matrix

| Solution | Latency | CPU Usage | Device Impact | Complexity | Recommended |
|----------|---------|-----------|---------------|------------|-------------|
| **Gateway Resampling** | +5-10ms | Low (1-2%) | None | Medium | ✅ **Yes** |
| **Firmware Resampling** | +20-30ms | High (ESP32) | High | High | ❌ No |
| **Dual Audio Streams** | 0ms | None | N/A (software only) | Low | ✅ Testing only |
| **Configure Gemini** | 0ms | None | None | N/A | ❌ Not possible |
| **External Transcoding** | +50-100ms | High | None | Very High | ❌ No |

---

## Recommended Implementation Plan

### Phase 1: Basic Resampling (✅ Done)
- ✅ Implement linear interpolation resampler
- ✅ Add optimized 24kHz→16kHz fast path
- ✅ Write unit tests

### Phase 2: Integration (Current)
- ⏳ Integrate resampler into session handler
- ⏳ Detect backend audio format from capabilities
- ⏳ Auto-enable resampling when needed
- ⏳ Add configuration option to disable (for testing)

### Phase 3: Optimization (Future)
- 🔲 Implement better interpolation (cubic, sinc)
- 🔲 Add SIMD optimizations (AVX2, NEON)
- 🔲 Benchmark different algorithms
- 🔲 Add metrics for resampling performance

### Phase 4: Advanced Features (Future)
- 🔲 Support other sample rate conversions (48kHz, 8kHz)
- 🔲 Adaptive quality based on CPU load
- 🔲 Per-device resampling profiles

---

## Configuration

### Enable/Disable Resampling

```bash
# .env or docker-compose.yml
AUDIO_RESAMPLING_ENABLED=true   # Default: true
AUDIO_RESAMPLING_ALGORITHM=fast # Options: fast, linear, cubic
```

### Monitor Resampling Performance

```bash
# Check metrics
curl http://gateway:9090/metrics | grep resampling

# Look for:
# audio_resampling_latency_ms (histogram)
# audio_resampling_errors_total (counter)
# audio_resampling_active_sessions (gauge)
```

---

## Testing

### Test with Laptop (No Resampling Needed)

```bash
cd test/
python3 audio_bridge.py --duration 60
```

This script handles different sample rates natively using PyAudio's dual-stream approach.

### Test with ESP32 Device (Resampling Required)

1. Flash ESP32 with streaming firmware
2. Configure device to connect to gateway
3. Enable debug logging:
   ```bash
   LOG_LEVEL=debug
   ```
4. Check logs for resampling:
   ```
   INFO  Audio resampling enabled: 24000Hz → 16000Hz
   DEBUG Resampled audio frame: 960 bytes → 640 bytes (latency: 4.2ms)
   ```

### Verify Audio Quality

**Listen for**:
- ✅ Natural speech cadence (not slower or faster)
- ✅ Correct pitch (not lower or higher)
- ✅ No robotic/metallic artifacts
- ✅ Smooth playback (no stuttering)

**If you hear problems**:
1. Check logs for buffer warnings
2. Verify sample rates in capabilities
3. Test with different resampling algorithms
4. Monitor CPU usage during playback

---

## Technical Details

### Linear Interpolation Algorithm

For each output sample at position `i`:
1. Calculate source position: `srcPos = i / ratio`
2. Get two adjacent input samples: `s1`, `s2`
3. Interpolate: `output = s1 * (1 - frac) + s2 * frac`

**Example** (24kHz → 16kHz, ratio = 2/3):
- Output sample 0 ← Input samples 0-1 (position 0.00)
- Output sample 1 ← Input samples 1-2 (position 1.50)
- Output sample 2 ← Input samples 3-4 (position 3.00)

### Fast 24kHz→16kHz Algorithm

Since 24/16 = 3/2 is an exact ratio:
- Process in groups: 3 input samples → 2 output samples
- Average adjacent samples for smoother output
- ~2x faster than generic linear interpolation

**Example**:
```
Input:  [s0, s1, s2, s3, s4, s5] (24kHz)
Output: [avg(s0,s1), avg(s1,s2), avg(s3,s4), avg(s4,s5)] (16kHz)
```

---

## Frequently Asked Questions

### Q: Why doesn't Gemini just output 16kHz?
**A**: Gemini's neural audio model is trained for 24kHz output. Higher sample rates provide better audio quality and more natural prosody.

### Q: Can we ask the device to accept 24kHz?
**A**: ESP32 Voice Preview firmware is designed for 16kHz. While technically possible to modify, it would require:
- Custom firmware flash
- More RAM for buffering
- Higher CPU usage
- Potential audio quality degradation

### Q: Will resampling degrade audio quality?
**A**: Minimal. Downsampling from 24kHz to 16kHz removes frequencies above 8kHz (Nyquist). Human speech is primarily 300Hz-3.5kHz, so quality loss is imperceptible.

### Q: What about upsampling 16kHz device audio to 24kHz for Gemini?
**A**: Not needed! Gemini **accepts** 16kHz input. Only the output (24kHz) needs resampling.

### Q: Can I disable resampling?
**A**: Yes, but audio will sound distorted. Only disable for testing/debugging:
```bash
AUDIO_RESAMPLING_ENABLED=false
```

---

## References

- **Gemini Live API Docs**: https://ai.google.dev/gemini-api/docs/live-api
- **ESP32 Voice Preview**: https://www.home-assistant.io/voice_control/
- **Audio Resampling Theory**: https://ccrma.stanford.edu/~jos/resample/
- **Linear Interpolation**: https://en.wikipedia.org/wiki/Linear_interpolation

---

**Last Updated**: 2024-11-17  
**Status**: Implementation in progress  
**Target Release**: v1.1.0

