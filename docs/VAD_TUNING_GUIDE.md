# Gemini VAD Tuning Guide

This guide helps you optimize voice activity detection (VAD) and interruption handling for your environment and conversation style.

## 🎯 Quick Start

The default settings in `ha-config.yaml` work well for most quiet home environments:

```yaml
vad_start_sensitivity: "high"          # Fast detection
vad_end_sensitivity: "low"             # Patient listening
vad_silence_duration_ms: 500           # Balanced timing
activity_handling: "start_interrupts"  # Natural interruptions
```

**Works well for:** Quiet rooms, clear speech, single speaker

## 🔧 Configuration Parameters

### 1. `vad_start_sensitivity` (Start-of-Speech Detection)

Controls how quickly Gemini detects when you START speaking.

| Value | Behavior | Best For | Trade-off |
|-------|----------|----------|-----------|
| `high` | Detects speech very quickly | Quiet environments, fast responses | May trigger on background noise |
| `low` | Waits longer to confirm speech | Noisy environments, TV/music playing | Slightly slower to respond |

**Example:**
```yaml
# Quiet home office
vad_start_sensitivity: "high"

# Kitchen with appliances running
vad_start_sensitivity: "low"
```

### 2. `vad_end_sensitivity` (End-of-Speech Detection)

Controls how quickly Gemini decides you've FINISHED speaking.

| Value | Behavior | Best For | Trade-off |
|-------|----------|----------|-----------|
| `high` | Ends detection quickly | Fast-paced conversation, quick responses | May cut you off mid-thought |
| `low` | Waits longer before ending | Natural pauses, complete thoughts | Slightly slower responses |

**Example:**
```yaml
# Fast Q&A style
vad_end_sensitivity: "high"

# Thoughtful conversation
vad_end_sensitivity: "low"
```

### 3. `vad_silence_duration_ms` (Silence Threshold)

How many milliseconds of silence before Gemini considers you done speaking.

| Value | Behavior | Best For | Trade-off |
|-------|----------|----------|-----------|
| 300ms | Very snappy | Quick commands, fast interaction | May cut off during natural pauses |
| 500ms | Balanced | General conversation | Good middle ground |
| 700ms | Patient | Long sentences, complex requests | Slower to respond |

**Example:**
```yaml
# Quick commands: "Turn on the lights"
vad_silence_duration_ms: 300

# Complex requests: "Turn on the lights... umm... in the living room"
vad_silence_duration_ms: 700
```

### 4. `vad_prefix_padding_ms` (Pre-Speech Audio Capture)

How much audio BEFORE detected speech to include (prevents cutting off first words).

| Value | Behavior | Recommendation |
|-------|----------|----------------|
| 200ms | Minimal padding | Only if experiencing issues |
| 300ms | Standard padding | **Recommended default** |
| 400ms | Extra safety | If first words are often cut off |

**Example:**
```yaml
# Standard (recommended)
vad_prefix_padding_ms: 300
```

### 5. `activity_handling` (Interruption Behavior)

How Gemini responds when you start speaking while it's talking.

| Value | Behavior | Best For |
|-------|----------|----------|
| `start_interrupts` | User can interrupt Gemini | Natural conversation, quick corrections |
| `no_interruption` | Gemini finishes before listening | Formal announcements, long responses |

**Example:**
```yaml
# Natural conversation (recommended)
activity_handling: "start_interrupts"

# Let assistant finish
activity_handling: "no_interruption"
```

### 6. `turn_coverage` (Audio Inclusion)

What audio to include in conversation turns.

| Value | Behavior | Best For |
|-------|----------|----------|
| `only_activity` | Only detected speech | Faster, cleaner | **Recommended** |
| `all_input` | All audio input | Maximum context |

**Example:**
```yaml
# Standard (recommended)
turn_coverage: "only_activity"
```

## 📋 Pre-configured Profiles

### Profile 1: Default Home Use
**Best for:** Quiet rooms, clear speech, minimal background noise

```yaml
vad_start_sensitivity: "high"
vad_end_sensitivity: "low"
vad_prefix_padding_ms: 300
vad_silence_duration_ms: 500
activity_handling: "start_interrupts"
turn_coverage: "only_activity"
```

### Profile 2: Noisy Environment
**Best for:** Kitchen, TV playing, kids in background

```yaml
vad_start_sensitivity: "low"       # Reduce false positives
vad_end_sensitivity: "low"         # Be patient
vad_prefix_padding_ms: 400         # Capture more context
vad_silence_duration_ms: 700       # Wait longer
activity_handling: "start_interrupts"
turn_coverage: "only_activity"
```

### Profile 3: Fast Commands
**Best for:** Quick home automation commands, brief interactions

```yaml
vad_start_sensitivity: "high"      # Fast detection
vad_end_sensitivity: "high"        # Quick cutoff
vad_prefix_padding_ms: 200         # Minimal padding
vad_silence_duration_ms: 300       # Snappy responses
activity_handling: "start_interrupts"
turn_coverage: "only_activity"
```

### Profile 4: Thoughtful Conversation
**Best for:** Complex questions, detailed discussions

```yaml
vad_start_sensitivity: "high"      # Normal detection
vad_end_sensitivity: "low"         # Patient listening
vad_prefix_padding_ms: 300         # Standard padding
vad_silence_duration_ms: 600       # Allow pauses
activity_handling: "start_interrupts"
turn_coverage: "only_activity"
```

## 🧪 Testing Your Configuration

### Step 1: Start with Defaults
Begin with the default profile and test basic conversation.

### Step 2: Identify Issues

| Symptom | Likely Cause | Solution |
|---------|--------------|----------|
| Gemini interrupts me too early | End sensitivity too high | Lower `vad_end_sensitivity` or increase `vad_silence_duration_ms` |
| Gemini is slow to respond | Start sensitivity too low | Increase `vad_start_sensitivity` |
| False triggers (responds to TV/noise) | Start sensitivity too high | Lower `vad_start_sensitivity` |
| First words are cut off | Insufficient padding | Increase `vad_prefix_padding_ms` |
| Can't interrupt Gemini | Wrong activity handling | Use `activity_handling: "start_interrupts"` |

### Step 3: Tune Incrementally
Make one change at a time and test. Most issues can be resolved with just 1-2 parameter adjustments.

### Step 4: Test Scenarios

**Basic Commands:**
```
"Turn on the kitchen lights"
"What's the weather?"
"Set a timer for 5 minutes"
```

**Complex Requests:**
```
"Turn on the lights in the living room... umm... and also the bedroom"
"What's the temperature... no wait... what's the humidity in the bedroom?"
```

**Interruption Test:**
```
1. Ask: "Tell me about the weather forecast for the next week"
2. Interrupt mid-response: "Stop, just tell me today's weather"
3. Check: Does Gemini stop talking immediately?
```

## 🔍 Troubleshooting

### Problem: Too Many False Positives
**Symptoms:** Gemini responds when no one is speaking, triggers on TV/music

**Solutions:**
1. Lower start sensitivity: `vad_start_sensitivity: "low"`
2. Increase silence threshold: `vad_silence_duration_ms: 600`
3. Check microphone placement (too close to TV/speakers)

### Problem: Cuts Me Off
**Symptoms:** Gemini starts responding before I finish my sentence

**Solutions:**
1. Lower end sensitivity: `vad_end_sensitivity: "low"`
2. Increase silence threshold: `vad_silence_duration_ms: 600`
3. Speak more continuously (fewer pauses)

### Problem: Slow to Respond
**Symptoms:** Long pause after I finish speaking

**Solutions:**
1. Increase start sensitivity: `vad_start_sensitivity: "high"`
2. Decrease silence threshold: `vad_silence_duration_ms: 400`
3. Check network latency (ping Gemini API)

### Problem: First Words Cut Off
**Symptoms:** "...urn on the lights" instead of "Turn on the lights"

**Solutions:**
1. Increase prefix padding: `vad_prefix_padding_ms: 400`
2. Speak slightly slower at the beginning
3. Check microphone quality/placement

### Problem: Can't Interrupt
**Symptoms:** Gemini keeps talking even when I start speaking

**Solutions:**
1. Check activity handling: `activity_handling: "start_interrupts"`
2. Increase start sensitivity: `vad_start_sensitivity: "high"`
3. Speak louder/clearer when interrupting

## 📊 Advanced: Monitoring & Metrics

Enable debug logging to see VAD events:

```yaml
logging:
  level: "debug"
```

Look for these log messages:
- `"received interruption signal from Gemini"` - User interrupted
- `"audio start event from backend"` - Gemini started speaking
- `"audio end event from backend"` - Gemini finished speaking
- `"audio suppression cleared"` - Ready for new audio

## 🎛️ Environment Variables for Quick Testing

You can override YAML settings via `.env` for quick testing:

```bash
# Test noisy environment profile
GEMINI_VAD_START_SENSITIVITY=low
GEMINI_VAD_SILENCE_DURATION_MS=700

# Test fast command profile
GEMINI_VAD_END_SENSITIVITY=high
GEMINI_VAD_SILENCE_DURATION_MS=300
```

Changes take effect on next restart.

## 📚 Best Practices

1. **Start with defaults** - They work well for most use cases
2. **Change one parameter at a time** - Makes it easier to identify what helps
3. **Test in your actual environment** - Background noise patterns vary
4. **Document what works** - Note your final settings for reference
5. **Consider time of day** - Noise levels change (quiet night vs busy morning)

## 🚀 Quick Reference

| Goal | Key Settings |
|------|--------------|
| Fastest response | `vad_start_sensitivity: high`, `vad_silence_duration_ms: 300` |
| Fewest interruptions | `vad_end_sensitivity: low`, `vad_silence_duration_ms: 700` |
| Noise immunity | `vad_start_sensitivity: low`, `vad_silence_duration_ms: 600` |
| Natural conversation | Use defaults |

## 🤝 Need Help?

If you're still having issues:
1. Enable debug logging and check logs
2. Share your environment details (room size, noise sources, microphone type)
3. Describe the specific symptoms you're experiencing
4. Try the pre-configured profiles first

---

**Remember:** There's no "perfect" configuration - it depends on your voice, environment, and conversation style. Start with defaults and tune based on your experience! 🎤
