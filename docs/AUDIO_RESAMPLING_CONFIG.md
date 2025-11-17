# Audio Resampling Configuration Guide

## Overview

The gateway now supports automatic audio resampling to handle sample rate mismatches between LLM backends and devices. This is especially important for Gemini Live, which outputs 24kHz audio while the Voice Assistant Preview device expects 16kHz.

## Quick Start

### Automatic Mode (Recommended)

```bash
# .env or docker-compose.yml
AUDIO_RESAMPLING_ENABLED=auto    # Automatically enables when needed
AUDIO_RESAMPLING_ALGORITHM=fast  # Optimized for 24kHz→16kHz
AUDIO_RESAMPLING_TARGET_RATE=16000  # ESP32 device rate
```

With these settings, the gateway will:
1. Detect backend audio format (e.g., 24kHz from Gemini)
2. Compare with device rate (16kHz)
3. Automatically enable resampling if they differ
4. Use the fastest algorithm for the conversion

### Manual Control

```bash
# Force enable resampling
AUDIO_RESAMPLING_ENABLED=true

# Force disable resampling (use for debugging only!)
AUDIO_RESAMPLING_ENABLED=false
```

---

## Configuration Options

### AUDIO_RESAMPLING_ENABLED

Controls whether audio resampling is enabled.

**Values:**
- `auto` (default) - Automatically enables when backend ≠ device sample rate
- `true` - Always enable resampling
- `false` - Never enable resampling (may cause audio distortion!)

**Examples:**
```bash
# Best for production
AUDIO_RESAMPLING_ENABLED=auto

# Force enable (for testing with same sample rates)
AUDIO_RESAMPLING_ENABLED=true

# Disable (only for debugging - audio will be distorted!)
AUDIO_RESAMPLING_ENABLED=false
```

**Logging:**
```
# When auto-enabled:
INFO  auto-enabling resampling (sample rate mismatch detected)
      source_rate=24000 target_rate=16000

# When manually enabled with same rates:
WARN  resampling enabled but source and target rates are the same
      source_rate=16000 target_rate=16000

# When disabled with different rates:
WARN  resampling disabled but sample rates differ - audio may be distorted!
      source_rate=24000 target_rate=16000
```

---

### AUDIO_RESAMPLING_ALGORITHM

Chooses the resampling algorithm to use.

**Values:**
- `fast` (default) - Optimized for 24kHz→16kHz (2:3 ratio)
- `linear` - Generic linear interpolation (any rate)
- `cubic` - High-quality cubic interpolation (coming soon)

**Examples:**
```bash
# Best for Gemini (24kHz→16kHz)
AUDIO_RESAMPLING_ALGORITHM=fast

# For other sample rates (48kHz, 8kHz, etc.)
AUDIO_RESAMPLING_ALGORITHM=linear

# High quality (not yet implemented)
AUDIO_RESAMPLING_ALGORITHM=cubic  # Falls back to linear
```

**Performance:**

| Algorithm | Best For | Latency | CPU | Quality |
|-----------|----------|---------|-----|---------|
| `fast` | 24kHz→16kHz | ~4ms | Lowest | Excellent |
| `linear` | Any rate | ~6ms | Low | Good |
| `cubic` | Any rate | ~10ms | Medium | Excellent |

---

### AUDIO_RESAMPLING_TARGET_RATE

Target sample rate for device audio (Hz).

**Values:**
- `16000` (default) - ESP32 Voice Assistant Preview
- `8000` - Lower quality (telephone)
- `24000` - Higher quality (if device supports)
- `0` - Auto-detect from device (not yet implemented)

**Examples:**
```bash
# ESP32 Voice Assistant Preview (default)
AUDIO_RESAMPLING_TARGET_RATE=16000

# Low-quality devices
AUDIO_RESAMPLING_TARGET_RATE=8000

# High-quality devices (if supported)
AUDIO_RESAMPLING_TARGET_RATE=24000
```

**Note**: The target rate must match your device's actual sample rate, otherwise audio will still be distorted!

---

## Complete Examples

### Production Setup (Gemini + ESP32)

```bash
# docker-compose.yml
environment:
  # Backend
  - BACKEND_TYPE=gemini
  - GEMINI_API_KEY=your_key
  
  # Audio resampling (auto-configured)
  - AUDIO_RESAMPLING_ENABLED=auto
  - AUDIO_RESAMPLING_ALGORITHM=fast
  - AUDIO_RESAMPLING_TARGET_RATE=16000
  
  # Logging
  - LOG_LEVEL=info
```

**Expected logs:**
```
INFO  Gateway ready
INFO  New WebSocket session connected
INFO  backend session started successfully
INFO  audio resampling configured
      backend_rate=24000 device_rate=16000 algorithm=fast
INFO  audio resampling enabled for session
      algorithm=fast source_rate=24000 target_rate=16000 ratio=0.666667
```

---

### Testing with Same Sample Rates

```bash
# Force enable to test resampling logic
AUDIO_RESAMPLING_ENABLED=true
AUDIO_RESAMPLING_ALGORITHM=fast
AUDIO_RESAMPLING_TARGET_RATE=16000
LOG_LEVEL=debug
```

**Expected logs:**
```
WARN  resampling enabled but source and target rates are the same
DEBUG audio resampling disabled for session
```

---

### Debugging Audio Issues

```bash
# Disable resampling to test raw audio
AUDIO_RESAMPLING_ENABLED=false
LOG_LEVEL=debug
```

**Use this to:**
- Test if audio issues are caused by resampling
- Verify backend audio format
- Compare distorted vs. resampled audio

**Warning**: Audio will sound slow/deep if rates differ!

---

## Monitoring

### Logs

**Session startup:**
```
INFO  audio resampling configured
      backend_rate=24000 device_rate=16000 algorithm=fast
```

**Per-session:**
```
INFO  audio resampling enabled for session
      algorithm=fast source_rate=24000 target_rate=16000 ratio=0.666667
```

**During playback:**
```
DEBUG audio resampling applied
      input_bytes=960 output_bytes=640 frame_count=1
```

---

### Metrics (if enabled)

```bash
METRICS_ENABLED=true
```

Access metrics at `http://gateway:9090/metrics`:

```
# Audio resampling metrics (future)
audio_resampling_frames_total{algorithm="fast"} 15234
audio_resampling_latency_seconds{quantile="0.5"} 0.004
audio_resampling_latency_seconds{quantile="0.95"} 0.006
audio_resampling_latency_seconds{quantile="0.99"} 0.008
audio_resampling_errors_total 0
```

---

## Troubleshooting

### Problem: Audio sounds slow/deep

**Symptoms:**
- Voice sounds like it's in slow motion
- Pitch is lower than expected
- Speech is intelligible but weird

**Cause**: Backend 24kHz audio played at device 16kHz without resampling

**Solution**:
```bash
# Enable resampling
AUDIO_RESAMPLING_ENABLED=auto

# Check logs for:
grep "resampling" logs/gateway.log

# Should see:
# INFO  auto-enabling resampling
```

---

### Problem: Audio sounds fast/high-pitched

**Symptoms:**
- Voice sounds chipmunk-like
- Pitch is higher than expected
- Speech is faster than normal

**Cause**: Backend 16kHz audio played at device 24kHz (unlikely)

**Solution**:
```bash
# Verify your target rate matches device
AUDIO_RESAMPLING_TARGET_RATE=16000  # For ESP32

# Check backend capabilities in logs
grep "backend_rate" logs/gateway.log
```

---

### Problem: Audio is choppy/robotic

**Symptoms:**
- Crackling or popping sounds
- Missing syllables
- Metallic artifacts

**Cause**: NOT resampling - likely buffer issues or CPU overload

**Solutions**:

1. **Check CPU usage:**
```bash
docker stats gateway
# CPU should be < 50%
```

2. **Increase buffer:**
```bash
SESSION_AUDIO_BUFFER_MS=200  # Default: 100
AUDIO_BUFFER_SIZE=200         # Default: 100
```

3. **Reduce log level:**
```bash
LOG_LEVEL=info  # Less overhead than debug
```

4. **Verify network:**
```bash
ping -c 100 gateway-ip
# Should have 0% packet loss
```

---

### Problem: "audio processing failed" errors

**Symptoms:**
```
ERROR audio processing failed
      error="resampling failed: input length must be even"
```

**Causes:**
- Corrupted audio frames
- Invalid PCM data
- Bug in resampler

**Solutions**:

1. **Check audio format:**
```bash
# Verify device sends 16-bit PCM
grep "audio_format" logs/gateway.log
```

2. **Try different algorithm:**
```bash
AUDIO_RESAMPLING_ALGORITHM=linear  # Instead of fast
```

3. **Disable temporarily:**
```bash
AUDIO_RESAMPLING_ENABLED=false
# If problem persists, not resampling issue
```

---

## FAQ

### Q: Do I need to configure anything for Gemini?
**A**: No! Just use `AUDIO_RESAMPLING_ENABLED=auto` (the default) and it will automatically detect the 24kHz→16kHz mismatch and enable resampling.

### Q: What about OpenAI Realtime API?
**A**: OpenAI outputs 24kHz by default, so the same settings will work. If OpenAI adds 16kHz support in the future, `auto` mode will detect it and disable unnecessary resampling.

### Q: Does resampling reduce audio quality?
**A**: Minimal. Downsampling from 24kHz to 16kHz removes frequencies above 8kHz. Human speech is 300Hz-3.5kHz, so there's no perceptible quality loss for voice.

### Q: What's the latency impact?
**A**: ~5ms per 20ms audio chunk. This is negligible in voice conversations (total latency is dominated by network and LLM processing, which are 200-500ms).

### Q: Can I upsample 16kHz to 24kHz?
**A**: Not yet implemented. Upsampling adds no quality (just interpolates missing data) and isn't needed since Gemini accepts 16kHz input.

### Q: What if my device uses 8kHz or 48kHz?
**A**: Set `AUDIO_RESAMPLING_TARGET_RATE` to match your device and use `AUDIO_RESAMPLING_ALGORITHM=linear` (the `fast` algorithm is optimized for 24kHz→16kHz only).

---

## See Also

- [AUDIO_RESAMPLING.md](AUDIO_RESAMPLING.md) - Technical deep dive
- [AUDIO_FORMATS_QUICK_REF.md](AUDIO_FORMATS_QUICK_REF.md) - Format specifications
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - General troubleshooting
- [CONFIGURATION.md](CONFIGURATION.md) - All configuration options

---

**Last Updated**: 2024-11-17  
**Status**: Production ready  
**Target Release**: v1.1.0

