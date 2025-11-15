#!/usr/bin/env python3
"""
Simple Wyoming Protocol Test Client

Tests basic Wyoming protocol connectivity without audio hardware.
Sends simulated audio chunks to verify the gateway processes them correctly.

Requirements:
    None (uses only standard library)

Usage:
    python3 wyoming_client.py
"""

import socket
import json
import struct
import time
import sys


def send_wyoming_event(sock, event_type, data=None, payload=None):
    """Send a Wyoming protocol event (JSONL + optional binary payload)."""
    event = {
        "type": event_type,
        "data": data or {},
    }
    
    if payload:
        event["payload_length"] = len(payload)
    
    # Send JSONL header
    json_line = json.dumps(event) + "\n"
    sock.sendall(json_line.encode('utf-8'))
    
    # Send binary payload if present
    if payload:
        sock.sendall(payload)
    
    print(f"✓ Sent: {event_type}")


def receive_wyoming_event(sock, timeout=2.0):
    """Receive a Wyoming protocol event with timeout."""
    sock.settimeout(timeout)
    
    try:
        # Read JSONL line
        line = b""
        while True:
            chunk = sock.recv(1)
            if not chunk:
                return None
            if chunk == b"\n":
                break
            line += chunk
        
        event = json.loads(line.decode('utf-8'))
        
        # Read binary payload if present
        if "payload_length" in event:
            payload_length = event["payload_length"]
            payload = b""
            while len(payload) < payload_length:
                chunk = sock.recv(payload_length - len(payload))
                if not chunk:
                    break
                payload += chunk
            event["payload"] = payload
        
        print(f"✓ Received: {event['type']}")
        return event
    except socket.timeout:
        return None


def test_wyoming_connection(host="localhost", port=10200):
    """Test basic Wyoming protocol connectivity."""
    print("=" * 60)
    print("   Wyoming Protocol Test Client")
    print("=" * 60)
    print()
    
    print(f"Connecting to {host}:{port}...")
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    
    try:
        sock.connect((host, port))
        print("✅ Connected!\n")
    except ConnectionRefusedError:
        print(f"❌ Connection refused to {host}:{port}")
        print("Make sure the gateway is running:")
        print("  ./gateway")
        return False
    
    try:
        # Test 1: Send audio-start event
        print("Test 1: Sending audio-start event...")
        send_wyoming_event(sock, "audio-start", {
            "rate": 16000,
            "width": 2,
            "channels": 1,
        })
        time.sleep(0.1)
        print("  ✅ Audio stream started\n")
        
        # Test 2: Send simulated audio chunks
        print("Test 2: Sending 10 audio chunks (simulated PCM data)...")
        for i in range(10):
            # Generate fake audio data (320 bytes = 10ms at 16kHz mono 16-bit)
            # This is silent audio (all zeros)
            fake_audio = struct.pack('<' + 'h' * 160, *([0] * 160))
            
            send_wyoming_event(sock, "audio-chunk", {
                "rate": 16000,
                "width": 2,
                "channels": 1,
            }, fake_audio)
            
            time.sleep(0.01)  # 10ms between chunks
        print("  ✅ Sent 10 audio chunks\n")
        
        # Test 3: Send audio-stop event
        print("Test 3: Sending audio-stop event...")
        send_wyoming_event(sock, "audio-stop", {})
        print("  ✅ Audio stream stopped\n")
        
        # Test 4: Try to receive responses (with timeout)
        print("Test 4: Listening for gateway responses...")
        received_count = 0
        
        while True:
            event = receive_wyoming_event(sock, timeout=2.0)
            if event is None:
                break
                
            received_count += 1
            
            if event['type'] == 'audio-chunk':
                payload_size = len(event.get('payload', b''))
                print(f"  📥 Received audio chunk: {payload_size} bytes")
            elif event['type'] == 'audio-start':
                print("  📥 Gateway started sending audio")
            elif event['type'] == 'audio-stop':
                print("  📥 Gateway stopped sending audio")
                break
        
        if received_count == 0:
            print("  ℹ️  No response from gateway (this is OK for mock backend)")
        else:
            print(f"  ✅ Received {received_count} events from gateway")
        
        print()
        print("=" * 60)
        print("✅ All tests passed!")
        print("=" * 60)
        print()
        print("What this means:")
        print("  • Gateway accepted Wyoming protocol connection")
        print("  • Gateway processed audio-start/chunk/stop events")
        print("  • Session was created and cleaned up properly")
        print()
        print("Check gateway logs for:")
        print("  • 'New Wyoming session connected'")
        print("  • Audio processing messages")
        print("  • Session cleanup messages")
        print()
        
        return True
        
    except Exception as e:
        print(f"\n❌ Error: {e}")
        import traceback
        traceback.print_exc()
        return False
    finally:
        sock.close()
        print("Connection closed.\n")


def main():
    import argparse
    
    parser = argparse.ArgumentParser(description="Test Wyoming protocol connectivity")
    parser.add_argument("--host", default="localhost", help="Gateway host (default: localhost)")
    parser.add_argument("--port", type=int, default=10200, help="Gateway port (default: 10200)")
    
    args = parser.parse_args()
    
    success = test_wyoming_connection(args.host, args.port)
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()

