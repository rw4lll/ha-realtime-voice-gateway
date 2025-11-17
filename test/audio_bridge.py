#!/usr/bin/env python3
"""
WebSocket Audio Bridge - Test gateway with laptop microphone and speakers.

This script bridges your laptop's audio to the WebSocket gateway, allowing you
to test the complete voice pipeline (including LLM backends) without physical hardware.

Requirements:
    pip install pyaudio websocket-client

Usage:
    # Basic test (30 seconds)
    python3 audio_bridge.py
    
    # Custom duration
    python3 audio_bridge.py --duration 60
    
    # Custom gateway address
    python3 audio_bridge.py --host 192.168.1.100 --port 8080
"""

import json
import threading
import queue
import time
import sys
import argparse

try:
    import pyaudio
except ImportError:
    print("❌ PyAudio not installed!")
    print("\nInstall with:")
    print("  pip install pyaudio")
    print("\nOn macOS:")
    print("  brew install portaudio")
    print("  pip install pyaudio")
    print("\nOn Linux:")
    print("  sudo apt-get install portaudio19-dev python3-pyaudio")
    print("  pip install pyaudio")
    sys.exit(1)

try:
    import websocket
except ImportError:
    print("❌ websocket-client not installed!")
    print("\nInstall with:")
    print("  pip install websocket-client")
    sys.exit(1)

# Audio configuration (must match gateway/LLM format)
# Note: Microphone input is 16kHz, but Gemini outputs 24kHz
MIC_SAMPLE_RATE = 16000  # 16kHz for microphone input
SPEAKER_SAMPLE_RATE = 24000  # 24kHz for Gemini audio output
CHANNELS = 1         # Mono
SAMPLE_WIDTH = 2     # 16-bit (2 bytes)
MIC_CHUNK_SIZE = 160     # 10ms at 16kHz (160 samples = 320 bytes)
SPEAKER_CHUNK_SIZE = 480  # 20ms at 24kHz (480 samples = 960 bytes)


class WebSocketAudioBridge:
    """Bridges laptop audio to/from WebSocket gateway."""
    
    def __init__(self, host="localhost", port=8080, path="/voice-stream", verbose=False):
        self.host = host
        self.port = port
        self.path = path
        self.verbose = verbose
        self.ws = None
        self.audio = pyaudio.PyAudio()
        self.running = False
        
        # Queues for audio data
        self.recv_audio_queue = queue.Queue()
        
        # Output audio stream
        self.speaker_stream = None
        self.speaker_lock = threading.Lock()
        
        # Stats
        self.sent_chunks = 0
        self.received_chunks = 0
        self.current_state = "connecting"
        
    def log(self, message):
        """Print log message if verbose."""
        if self.verbose:
            print(f"[DEBUG] {message}")
    
    def on_message(self, ws, message):
        """Handle incoming WebSocket messages."""
        # Check if it's binary (audio) or text (JSON control)
        if isinstance(message, bytes):
            # Raw PCM audio from gateway
            self.recv_audio_queue.put(message)
            self.received_chunks += 1
            self.log(f"Received audio chunk: {len(message)} bytes")
        else:
            # JSON control message
            try:
                data = json.loads(message)
                if "state" in data:
                    state = data["state"]
                    self.log(f"State change: {self.current_state} -> {state}")
                    self.current_state = state
                    
                    if state == "listening":
                        print("🎤 Gateway is listening...")
                    elif state == "thinking":
                        print("🤔 Gateway is thinking...")
                    elif state == "speaking":
                        print("🔊 Gateway is speaking...")
                        # Initialize speaker stream if not already done
                        with self.speaker_lock:
                            if self.speaker_stream is None:
                                try:
                                    self.speaker_stream = self.audio.open(
                                        format=pyaudio.paInt16,
                                        channels=CHANNELS,
                                        rate=SPEAKER_SAMPLE_RATE,
                                        output=True,
                                        frames_per_buffer=SPEAKER_CHUNK_SIZE * 2
                                    )
                                    self.log("Speaker stream initialized")
                                except Exception as e:
                                    print(f"❌ Failed to open speaker: {e}")
                    elif state == "done":
                        print("✅ Session complete")
                        self.running = False
                elif "error" in data:
                    print(f"❌ Error from gateway: {data['error']}")
                    self.running = False
                elif "command" in data:
                    self.log(f"Command received: {data['command']}")
            except json.JSONDecodeError:
                self.log(f"Received non-JSON text: {message}")
    
    def on_error(self, ws, error):
        """Handle WebSocket errors."""
        if self.running:
            print(f"❌ WebSocket error: {error}")
    
    def on_close(self, ws, close_status_code, close_msg):
        """Handle WebSocket connection close."""
        self.log(f"WebSocket closed: {close_status_code} - {close_msg}")
        self.running = False
    
    def on_open(self, ws):
        """Handle WebSocket connection open."""
        print("✅ Connected to gateway!")
        self.running = True
        
        # Start audio capture thread
        threading.Thread(target=self.mic_thread, daemon=True).start()
        
        # Start speaker playback thread
        threading.Thread(target=self.speaker_thread, daemon=True).start()
    
    def mic_thread(self):
        """Capture audio from microphone and send to gateway."""
        print("🎤 Starting microphone capture...")
        
        try:
            stream = self.audio.open(
                format=pyaudio.paInt16,
                channels=CHANNELS,
                rate=MIC_SAMPLE_RATE,
                input=True,
                frames_per_buffer=MIC_CHUNK_SIZE
            )
        except Exception as e:
            print(f"❌ Failed to open microphone: {e}")
            print("Make sure your microphone is connected and not in use.")
            self.running = False
            return
        
        print("✅ Microphone ready - speak now!")
        
        try:
            while self.running:
                # Read audio chunk from microphone
                audio_data = stream.read(MIC_CHUNK_SIZE, exception_on_overflow=False)
                
                # Send raw PCM audio to gateway (binary WebSocket message)
                if self.ws and self.ws.sock and self.ws.sock.connected:
                    self.ws.send(audio_data, opcode=websocket.ABNF.OPCODE_BINARY)
                    self.sent_chunks += 1
                else:
                    break
                    
        except Exception as e:
            if self.running:
                print(f"❌ Microphone error: {e}")
        finally:
            try:
                stream.stop_stream()
                stream.close()
            except Exception as e:
                self.log(f"Microphone cleanup error (can be ignored): {e}")
            print(f"🎤 Microphone stopped (sent {self.sent_chunks} chunks)")
    
    def speaker_thread(self):
        """Receive audio from gateway and play through speakers."""
        self.log("Speaker thread ready...")
        
        try:
            while self.running:
                # Wait for speaker stream to be initialized by state transition
                with self.speaker_lock:
                    stream = self.speaker_stream
                
                if stream is None:
                    time.sleep(0.1)
                    continue
                
                try:
                    # Get audio from queue with timeout
                    audio_data = self.recv_audio_queue.get(timeout=0.1)
                    stream.write(audio_data)
                except queue.Empty:
                    continue
                except Exception as e:
                    if self.running:
                        self.log(f"Speaker playback error: {e}")
        except Exception as e:
            if self.running:
                print(f"❌ Speaker error: {e}")
        finally:
            with self.speaker_lock:
                if self.speaker_stream:
                    try:
                        self.speaker_stream.stop_stream()
                        self.speaker_stream.close()
                        self.speaker_stream = None
                    except Exception as e:
                        self.log(f"Speaker cleanup error (can be ignored): {e}")
            print(f"🔊 Speaker stopped (played {self.received_chunks} chunks)")
    
    def connect(self):
        """Connect to WebSocket gateway."""
        ws_url = f"ws://{self.host}:{self.port}{self.path}"
        print(f"Connecting to gateway at {ws_url}...")
        
        # Create WebSocket connection
        self.ws = websocket.WebSocketApp(
            ws_url,
            on_open=self.on_open,
            on_message=self.on_message,
            on_error=self.on_error,
            on_close=self.on_close
        )
    
    def run(self, duration=30):
        """Run the audio bridge for specified duration."""
        
        print(f"\n{'='*60}")
        print("🎙️  READY TO TEST!")
        print(f"{'='*60}")
        print("Speak into your microphone and listen for responses...")
        print()
        print("Try saying:")
        print("  • 'Hello, how are you?'")
        print("  • 'Tell me a joke'")
        print("  • 'What's 25 times 34?'")
        print()
        if duration > 0:
            print(f"Recording for {duration} seconds...")
            print("Press Ctrl+C to stop early")
        else:
            print("Press Ctrl+C to stop")
        print(f"{'='*60}\n")
        
        # Run WebSocket in separate thread
        ws_thread = threading.Thread(target=self.ws.run_forever, daemon=True)
        ws_thread.start()
        
        # Wait for connection
        time.sleep(1)
        
        # Run for specified duration or until interrupted
        try:
            start_time = time.time()
            if duration > 0:
                while self.running and (time.time() - start_time) < duration:
                    time.sleep(0.1)
            else:
                # Run indefinitely
                while self.running:
                    time.sleep(0.1)
        except KeyboardInterrupt:
            print("\n⚠️  Interrupted by user")
        
        # Cleanup
        print("\nShutting down...")
        self.running = False
        
        if self.ws:
            self.ws.close()
        
        # Wait for threads to finish and clean up their streams
        ws_thread.join(timeout=2)
        time.sleep(0.5)  # Give threads time to clean up streams
        
        # Now terminate PyAudio after streams are closed
        try:
            self.audio.terminate()
        except Exception as e:
            self.log(f"PyAudio termination error (can be ignored): {e}")
        
        print(f"\n{'='*60}")
        print("📊 Session Statistics:")
        print(f"  Sent to gateway: {self.sent_chunks} audio chunks")
        print(f"  Received from gateway: {self.received_chunks} audio chunks")
        print(f"  Final state: {self.current_state}")
        print(f"{'='*60}")
        print("✅ Session ended")


def main():
    parser = argparse.ArgumentParser(
        description="WebSocket Audio Bridge - Test gateway with laptop audio",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Test for 30 seconds (default)
  python3 audio_bridge.py
  
  # Test for 60 seconds
  python3 audio_bridge.py --duration 60
  
  # Run indefinitely (Ctrl+C to stop)
  python3 audio_bridge.py --duration 0
  
  # Connect to remote gateway
  python3 audio_bridge.py --host 192.168.1.100
  
  # Custom WebSocket path
  python3 audio_bridge.py --path /custom-path
  
  # Enable debug logging
  python3 audio_bridge.py --verbose
        """
    )
    
    parser.add_argument(
        "--host",
        default="localhost",
        help="Gateway hostname or IP (default: localhost)"
    )
    parser.add_argument(
        "--port",
        type=int,
        default=8080,
        help="Gateway WebSocket port (default: 8080)"
    )
    parser.add_argument(
        "--path",
        default="/voice-stream",
        help="WebSocket path (default: /voice-stream)"
    )
    parser.add_argument(
        "--duration",
        type=int,
        default=30,
        help="Recording duration in seconds (0 = indefinite, default: 30)"
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Enable verbose debug logging"
    )
    
    args = parser.parse_args()
    
    print("=" * 60)
    print("   WebSocket Audio Bridge - Gateway Test")
    print("=" * 60)
    print()
    
    bridge = WebSocketAudioBridge(
        host=args.host,
        port=args.port,
        path=args.path,
        verbose=args.verbose
    )
    
    try:
        bridge.connect()
        bridge.run(duration=args.duration)
    except Exception as e:
        print(f"❌ Error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()
