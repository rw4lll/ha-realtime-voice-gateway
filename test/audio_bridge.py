#!/usr/bin/env python3
"""
Wyoming Audio Bridge - Test gateway with laptop microphone and speakers.

This script bridges your laptop's audio to the Wyoming gateway, allowing you
to test the complete voice pipeline (including Gemini Live) without physical hardware.

Requirements:
    pip install pyaudio

Usage:
    # Basic test (30 seconds)
    python3 audio_bridge.py
    
    # Custom duration
    python3 audio_bridge.py --duration 60
    
    # Custom gateway address
    python3 audio_bridge.py --host 192.168.1.100 --port 10200
"""

import socket
import json
import struct
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

# Audio configuration (must match Wyoming/Gemini format)
SAMPLE_RATE = 16000  # 16kHz
CHANNELS = 1         # Mono
SAMPLE_WIDTH = 2     # 16-bit (2 bytes)
CHUNK_SIZE = 160     # 10ms at 16kHz (160 samples = 320 bytes)


class WyomingAudioBridge:
    """Bridges laptop audio to/from Wyoming gateway."""
    
    def __init__(self, host="localhost", port=10200, verbose=False):
        self.host = host
        self.port = port
        self.verbose = verbose
        self.sock = None
        self.audio = pyaudio.PyAudio()
        self.running = False
        
        # Queues for audio data
        self.recv_queue = queue.Queue()
        
        # Output audio format (from gateway's audio-start event)
        self.output_rate = None
        self.output_width = None
        self.output_channels = None
        self.speaker_stream = None
        self.speaker_lock = threading.Lock()
        
        # Stats
        self.sent_chunks = 0
        self.received_chunks = 0
        
    def log(self, message):
        """Print log message if verbose."""
        if self.verbose:
            print(f"[DEBUG] {message}")
    
    def connect(self):
        """Connect to Wyoming gateway."""
        print(f"Connecting to gateway at {self.host}:{self.port}...")
        self.sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.sock.connect((self.host, self.port))
        print("✅ Connected to gateway!")
        
    def send_event(self, event_type, data=None, payload=None):
        """Send Wyoming protocol event."""
        event = {
            "type": event_type,
            "data": data or {},
        }
        
        if payload:
            event["payload_length"] = len(payload)
        
        # Send JSONL header
        json_line = json.dumps(event) + "\n"
        self.sock.sendall(json_line.encode('utf-8'))
        
        # Send binary payload if present
        if payload:
            self.sock.sendall(payload)
        
        self.log(f"Sent: {event_type}")
    
    def receive_event(self):
        """Receive Wyoming protocol event."""
        # Read JSONL line
        line = b""
        while True:
            chunk = self.sock.recv(1)
            if not chunk:
                return None
            if chunk == b"\n":
                break
            line += chunk
        
        if not line:
            return None
            
        event = json.loads(line.decode('utf-8'))
        
        # Read binary payload if present
        if "payload_length" in event:
            payload_length = event["payload_length"]
            payload = b""
            while len(payload) < payload_length:
                remaining = payload_length - len(payload)
                chunk = self.sock.recv(min(4096, remaining))
                if not chunk:
                    break
                payload += chunk
            event["payload"] = payload
        
        self.log(f"Received: {event['type']}")
        return event
    
    def mic_thread(self):
        """Capture audio from microphone and send to gateway."""
        print("🎤 Starting microphone capture...")
        
        try:
            stream = self.audio.open(
                format=pyaudio.paInt16,
                channels=CHANNELS,
                rate=SAMPLE_RATE,
                input=True,
                frames_per_buffer=CHUNK_SIZE
            )
        except Exception as e:
            print(f"❌ Failed to open microphone: {e}")
            print("Make sure your microphone is connected and not in use.")
            self.running = False
            return
        
        # Send audio-start
        self.send_event("audio-start", {
            "rate": SAMPLE_RATE,
            "width": SAMPLE_WIDTH,
            "channels": CHANNELS,
        })
        print("✅ Audio stream started - speak now!")
        
        try:
            while self.running:
                # Read audio chunk from microphone
                audio_data = stream.read(CHUNK_SIZE, exception_on_overflow=False)
                
                # Send audio-chunk to gateway
                self.send_event("audio-chunk", {
                    "rate": SAMPLE_RATE,
                    "width": SAMPLE_WIDTH,
                    "channels": CHANNELS,
                }, audio_data)
                
                self.sent_chunks += 1
                
        except Exception as e:
            print(f"❌ Microphone error: {e}")
        finally:
            stream.stop_stream()
            stream.close()
            self.send_event("audio-stop", {})
            print(f"🎤 Microphone stopped (sent {self.sent_chunks} chunks)")
    
    def speaker_thread(self):
        """Receive audio from gateway and play through speakers."""
        print("🔊 Speaker thread ready (waiting for audio-start from gateway)...")
        
        try:
            while self.running:
                # Wait for speaker stream to be initialized by audio-start event
                with self.speaker_lock:
                    stream = self.speaker_stream
                
                if stream is None:
                    time.sleep(0.1)
                    continue
                
                try:
                    # Get audio from queue with timeout
                    audio_data = self.recv_queue.get(timeout=0.1)
                    stream.write(audio_data)
                except queue.Empty:
                    continue
        except Exception as e:
            print(f"❌ Speaker error: {e}")
        finally:
            with self.speaker_lock:
                if self.speaker_stream:
                    self.speaker_stream.stop_stream()
                    self.speaker_stream.close()
            print(f"🔊 Speaker stopped (played {self.received_chunks} chunks)")
    
    def receive_thread(self):
        """Receive events from gateway."""
        print("📡 Starting receive thread...")
        
        try:
            while self.running:
                event = self.receive_event()
                if event is None:
                    print("⚠️  Connection closed by gateway")
                    self.running = False
                    break
                
                if event['type'] == 'audio-chunk':
                    # Queue audio for playback
                    self.recv_queue.put(event['payload'])
                    self.received_chunks += 1
                elif event['type'] == 'audio-start':
                    # Parse audio format from event
                    rate = event['data'].get('rate', SAMPLE_RATE)
                    width = event['data'].get('width', SAMPLE_WIDTH)
                    channels = event['data'].get('channels', CHANNELS)
                    
                    print(f"🔊 Gateway started sending audio at {rate}Hz!")
                    print(f"   Audio format: {rate}Hz, {width*8}-bit, {channels}ch")
                    
                    # Initialize speaker stream with correct format
                    with self.speaker_lock:
                        # Close old stream if exists
                        if self.speaker_stream:
                            self.speaker_stream.stop_stream()
                            self.speaker_stream.close()
                        
                        # Open new stream with correct format
                        try:
                            self.speaker_stream = self.audio.open(
                                format=pyaudio.paInt16,
                                channels=channels,
                                rate=rate,
                                output=True,
                                frames_per_buffer=rate // 10  # 100ms buffer
                            )
                            print(f"✅ Speaker initialized at {rate}Hz")
                        except Exception as e:
                            print(f"❌ Failed to open speaker at {rate}Hz: {e}")
                            self.speaker_stream = None
                    
                elif event['type'] == 'audio-stop':
                    print("✅ Gateway finished sending audio")
                else:
                    print(f"📥 Received: {event['type']}")
        except Exception as e:
            if self.running:
                print(f"❌ Receive error: {e}")
        finally:
            print("📡 Receive thread stopped")
    
    def run(self, duration=30):
        """Run the audio bridge for specified duration."""
        self.running = True
        
        # Start threads
        mic = threading.Thread(target=self.mic_thread, daemon=True)
        speaker = threading.Thread(target=self.speaker_thread, daemon=True)
        receiver = threading.Thread(target=self.receive_thread, daemon=True)
        
        mic.start()
        speaker.start()
        receiver.start()
        
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
        
        # Run for specified duration or until interrupted
        try:
            if duration > 0:
                time.sleep(duration)
            else:
                # Run indefinitely
                while self.running:
                    time.sleep(0.1)
        except KeyboardInterrupt:
            print("\n⚠️  Interrupted by user")
        
        # Cleanup
        print("\nShutting down...")
        self.running = False
        
        # Wait for threads to finish
        mic.join(timeout=2)
        speaker.join(timeout=2)
        receiver.join(timeout=2)
        
        self.sock.close()
        self.audio.terminate()
        
        print(f"\n{'='*60}")
        print("📊 Session Statistics:")
        print(f"  Sent to gateway: {self.sent_chunks} audio chunks")
        print(f"  Received from gateway: {self.received_chunks} audio chunks")
        print(f"{'='*60}")
        print("✅ Session ended")


def main():
    parser = argparse.ArgumentParser(
        description="Wyoming Audio Bridge - Test gateway with laptop audio",
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
        default=10200,
        help="Gateway Wyoming port (default: 10200)"
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
    print("   Wyoming Audio Bridge - Gateway Test")
    print("=" * 60)
    print()
    
    bridge = WyomingAudioBridge(
        host=args.host,
        port=args.port,
        verbose=args.verbose
    )
    
    try:
        bridge.connect()
        bridge.run(duration=args.duration)
    except ConnectionRefusedError:
        print(f"❌ Connection refused to {args.host}:{args.port}")
        print("Make sure the gateway is running:")
        print("  ./gateway")
        sys.exit(1)
    except Exception as e:
        print(f"❌ Error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()

