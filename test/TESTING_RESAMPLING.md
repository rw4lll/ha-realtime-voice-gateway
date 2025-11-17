# Testing Audio Resampling

## Overview

The `audio_bridge.py` test script has been updated to **simulate ESP32 Voice Assistant Preview device limitations** (16kHz audio only). This allows you to test the gateway's audio resampling feature without physical hardware.

## What Changed

### Before (Old Behavior)
```python
MIC_SAMPLE_RATE = 16000   # 16kHz microphone
SPEAKER_SAMPLE_RATE = 24000  # 24kHz speakers (laptop hardware flexibility)
```

**Result**: Audio played at native Gemini rate (24kHz). Resampling was NOT tested because laptop hardware could handle different rates natively.

### After (New Behavior - ESP32 Simulation)
```python
MIC_SAMPLE_RATE = 16000   # 16kHz microphone (ESP32 hardware)
SPEAKER_SAMPLE_RATE = 16000  # 16kHz speakers (ESP32 hardware)
```

**Result**: Audio must be resampled by gateway (24kHz → 16kHz) or it will sound slow/deep. This tests the real device scenario!

---

## How to Test

### 1. Start Gateway
```bash
cd /Users/rw4lll/Projects/ha-realtime-voice-gateway
go run ./cmd/gateway/main.go
```

**Look for these logs:**
```
INFO  Gateway ready
INFO  Pipeline initialized
```

### 2. Run Test Script
```bash
cd test/
source venv/bin/activate  # If using venv
python3 audio_bridge.py --duration 60
```

**You'll see:**
```
============================================================
   WebSocket Audio Bridge - Gateway Test
   (Simulates ESP32 Voice Preview - 16kHz)
============================================================

🔊 Audio: 16kHz microphone + 16kHz speakers
🎯 Tests: Gateway resampling (24kHz → 16kHz)

============================================================
🎙️  READY TO TEST!
============================================================
This script simulates ESP32 Voice Preview (16kHz audio only)
Gateway should resample Gemini's 24kHz → 16kHz automatically

✅ If speech sounds NATURAL: Resampling is working!
❌ If speech sounds SLOW/DEEP: Resampling may be disabled
```

### 3. Speak and Listen

**Say**: "Hello, how are you?"

**Expected Gateway Logs:**
```
INFO  New WebSocket session connected session_id=...
INFO  audio resampling configured
      backend_rate=24000 device_rate=16000 algorithm=fast
INFO  audio resampling enabled for session
      algorithm=fast source_rate=24000 target_rate=16000 ratio=0.666667
INFO  starting session handler
DEBUG audio resampling applied
      input_bytes=960 output_bytes=640 frame_count=1
```

**Listen to the response**:
- ✅ **Natural speech** (correct pitch and speed) = Resampling works! 🎉
- ❌ **Slow/deep speech** (like slow-motion) = Resampling not working ⚠️

---

## Audio Flow

```
┌──────────────────────────────────────────────────────────┐
│                   Test Script (Laptop)                   │
│              Simulates ESP32 Device (16kHz)              │
└───────────────────────┬──────────────────────────────────┘
                        │
                        │ 16kHz audio (microphone)
                        ↓
┌──────────────────────────────────────────────────────────┐
│                        Gateway                           │
│                                                          │
│  1. Receives: 16kHz from device                         │
│  2. Sends to: Gemini (accepts 16kHz)                    │
│  3. Receives: 24kHz from Gemini                         │
│  4. RESAMPLES: 24kHz → 16kHz                            │
│  5. Sends: 16kHz to device                              │
│                                                          │
│  Backend Capabilities:                                   │
│    - Input: 16kHz ✅                                     │
│    - Output: 24kHz ⚠️ (needs resampling)                │
│                                                          │
│  Audio Processor:                                        │
│    - Enabled: auto (detects 24≠16)                      │
│    - Algorithm: fast (2:3 ratio)                        │
│    - Input: 960 bytes (24kHz, 20ms)                     │
│    - Output: 640 bytes (16kHz, 20ms)                    │
└───────────────────────┬──────────────────────────────────┘
                        │
                        │ 16kHz audio (resampled)
                        ↓
┌──────────────────────────────────────────────────────────┐
│                   Test Script (Laptop)                   │
│              Plays at 16kHz (ESP32 simulation)           │
│                                                          │
│  ✅ Resampling Working: Natural speech                   │
│  ❌ No Resampling: Slow/deep speech                      │
└──────────────────────────────────────────────────────────┘
```

---

## What You're Testing

### ✅ Resampling Works Correctly
**Symptoms:**
- Speech sounds natural
- Pitch is correct
- Speed is normal
- No distortion

**Gateway Logs:**
```
INFO  audio resampling configured backend_rate=24000 device_rate=16000
INFO  audio resampling enabled for session
DEBUG audio resampling applied input_bytes=960 output_bytes=640
```

**Means:**
- Gateway detected sample rate mismatch (24kHz ≠ 16kHz)
- Auto-enabled resampling
- Successfully converting 24kHz → 16kHz
- Audio quality preserved

### ❌ Resampling Not Working
**Symptoms:**
- Speech sounds slow (like slow-motion)
- Pitch is too low (deeper voice)
- Still intelligible but weird
- No clicking/popping (just slow)

**Gateway Logs:**
```
WARN  resampling disabled but sample rates differ - audio may be distorted!
      source_rate=24000 target_rate=16000
```

**Causes:**
1. Resampling manually disabled: `AUDIO_RESAMPLING_ENABLED=false`
2. Auto-detection failed (bug)
3. Wrong target rate: `AUDIO_RESAMPLING_TARGET_RATE=24000`

**Fix:**
```bash
# Enable resampling
AUDIO_RESAMPLING_ENABLED=auto  # Or "true"

# Verify target rate
AUDIO_RESAMPLING_TARGET_RATE=16000

# Restart gateway
docker-compose restart gateway
```

---

## Troubleshooting

### Audio Sounds Slow/Deep

**Step 1: Check configuration**
```bash
# In .env or docker-compose.yml
AUDIO_RESAMPLING_ENABLED=auto     # Should be "auto" or "true"
AUDIO_RESAMPLING_ALGORITHM=fast   # Use "fast" for 24→16kHz
AUDIO_RESAMPLING_TARGET_RATE=16000  # Must be 16000
```

**Step 2: Check gateway logs**
```bash
docker-compose logs gateway | grep -i resampling

# Expected:
# INFO  audio resampling configured backend_rate=24000 device_rate=16000
# INFO  audio resampling enabled for session

# Problem:
# WARN  resampling disabled but sample rates differ
# DEBUG audio resampling disabled for session
```

**Step 3: Enable debug logging**
```bash
LOG_LEVEL=debug
docker-compose restart gateway
```

**Step 4: Force enable resampling**
```bash
AUDIO_RESAMPLING_ENABLED=true  # Override auto-detection
```

### Audio Still Sounds Wrong

**Check backend capabilities:**
```bash
docker-compose logs gateway | grep "Capabilities\|SupportedAudioFormats"
```

**Verify frame sizes:**
```bash
# In debug mode, should see:
# DEBUG audio resampling applied input_bytes=960 output_bytes=640
#
# 960 bytes = 480 samples × 2 bytes = 20ms at 24kHz (correct)
# 640 bytes = 320 samples × 2 bytes = 20ms at 16kHz (correct)
```

---

## Comparison: Old vs New Test Behavior

| Aspect | Old Behavior | New Behavior (ESP32 Simulation) |
|--------|--------------|--------------------------------|
| **Mic Rate** | 16kHz | 16kHz |
| **Speaker Rate** | 24kHz | **16kHz** |
| **Backend Output** | 24kHz | 24kHz |
| **Resampling Needed?** | No (laptop handles natively) | **Yes** (gateway must resample) |
| **Tests Resampling?** | ❌ No | ✅ **Yes** |
| **Real Device Behavior?** | ❌ No | ✅ **Yes** |
| **Audio Without Resampling** | Natural (laptop resamples) | Slow/deep (no hardware resampling) |

---

## Session Statistics

At the end of each test, you'll see:

```
============================================================
📊 Session Statistics:
  Sent to gateway: 1234 audio chunks (16kHz)
  Received from gateway: 567 audio chunks (16kHz)
  Final state: done

Audio Format:
  Microphone: 16000Hz (ESP32 simulation)
  Speakers: 16000Hz (ESP32 simulation)
  Gateway resamples: 24kHz → 16kHz automatically
============================================================
✅ Session ended
```

**Interpretation:**
- **Sent chunks**: Your voice sent to gateway (16kHz)
- **Received chunks**: AI responses from gateway (16kHz, resampled from 24kHz)
- If you heard natural speech, resampling worked for all received chunks!

---

## Advanced Testing

### Test with Different Backends

**Mock Backend** (no resampling needed):
```bash
BACKEND_TYPE=mock
# Mock outputs same rate as input (16kHz)
# Resampling should be disabled
```

**Gemini Backend** (resampling needed):
```bash
BACKEND_TYPE=gemini
# Gemini outputs 24kHz
# Resampling should be auto-enabled
```

### Test Resampling Algorithms

```bash
# Fast algorithm (optimized for 24→16kHz)
AUDIO_RESAMPLING_ALGORITHM=fast

# Generic linear interpolation
AUDIO_RESAMPLING_ALGORITHM=linear

# Compare audio quality and latency
```

### Test Manual Control

```bash
# Force disable (to hear what slow audio sounds like)
AUDIO_RESAMPLING_ENABLED=false
# Audio will be slow/deep!

# Force enable (even if rates match)
AUDIO_RESAMPLING_ENABLED=true
# Tests resampling code path
```

---

## Performance Metrics

Monitor these during testing:

```bash
# Gateway CPU usage
docker stats gateway

# Resampling latency (in debug logs)
docker-compose logs gateway | grep "resampling applied"
# DEBUG audio resampling applied ... (latency ~4-6ms per frame)

# Total chunks processed
# See session statistics at end of test
```

**Expected Performance:**
- CPU overhead: ~1-2% per session
- Latency per frame: ~5ms
- Total latency impact: <10ms (negligible in conversation)
- Audio quality: Imperceptible quality loss

---

## Success Criteria

✅ **Test passes if:**
1. Audio sounds natural (correct pitch and speed)
2. Gateway logs show resampling enabled
3. Gateway logs show correct rates (24kHz → 16kHz)
4. No audio glitches or artifacts
5. Session completes without errors

❌ **Test fails if:**
1. Audio sounds slow or deep
2. Gateway logs show resampling disabled
3. Audio has glitches or distortion
4. Session errors or disconnects

---

## Real ESP32 Device Testing

After validating with test script:

1. **Flash ESP32 firmware**
2. **Configure device**: `ws://gateway-ip:8080/voice-stream`
3. **Connect and test** - audio should sound identical to test script
4. **Monitor gateway logs** - same resampling logs as test

If test script works but ESP32 doesn't:
- Verify ESP32 firmware uses 16kHz
- Check WebSocket protocol compatibility
- Ensure device sends/receives raw PCM correctly

---

## Summary

The updated `audio_bridge.py` now:
- ✅ Simulates real ESP32 device (16kHz only)
- ✅ Tests gateway resampling (24kHz → 16kHz)
- ✅ Provides clear pass/fail indicators (natural vs slow audio)
- ✅ Validates before deploying to physical hardware

**If audio sounds natural, your resampling is working and ESP32 devices will work too!** 🎉

---

**Last Updated**: 2024-11-17  
**Version**: v1.1.0  
**Status**: Ready for testing

