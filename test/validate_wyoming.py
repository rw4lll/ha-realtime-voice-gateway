#!/usr/bin/env python3
"""
Validate that our Wyoming protocol implementation matches the expected format.
This script connects to the gateway and checks if the info event can be properly
deserialized by the Wyoming library.
"""

import asyncio
import json
import sys
from wyoming.client import AsyncClient
from wyoming.info import Describe, Info
from wyoming.event import Event


async def test_gateway():
    """Connect to gateway and validate the info response."""
    
    print("Connecting to gateway at localhost:10200...")
    
    try:
        async with AsyncClient.from_uri("tcp://localhost:10200") as client:
            print("✓ Connected successfully")
            
            # According to Wyoming protocol, client sends describe first
            # Service Description event flow:
            # 1. → describe (required) 
            # 2. ← info (required)
            print("\nSending describe request...")
            await client.write_event(Describe().event())
            
            # Read info response
            print("Reading info response...")
            info_event = await client.read_event()
            
            if not info_event:
                print("✗ No response received")
                return False
            
            print(f"✓ Received event type: {info_event.type}")
            print(f"\nRaw event data:")
            print(json.dumps(info_event.data, indent=2))
            
            # Try to parse as Info object
            print("\nParsing as Wyoming Info object...")
            try:
                info = Info.from_event(info_event)
                print("✓ Successfully parsed Info object")
                
                # Check services
                print(f"\nASR services: {len(info.asr)}")
                for asr in info.asr:
                    print(f"  - {asr.name}")
                    print(f"    Installed: {asr.installed}")
                    print(f"    Models: {len(asr.models)}")
                    for model in asr.models:
                        print(f"      * {model.name}")
                        print(f"        Languages: {', '.join(model.languages)}")
                        print(f"        Installed: {model.installed}")
                
                print(f"\nTTS services: {len(info.tts)}")
                print(f"Wake services: {len(info.wake)}")
                print(f"Intent services: {len(info.intent)}")
                print(f"Handle services: {len(info.handle)}")
                
                # Check if any services are installed
                has_services = (
                    any(asr.installed for asr in info.asr) or
                    any(tts.installed for tts in info.tts) or
                    any(wake.installed for wake in info.wake) or
                    any(intent.installed for intent in info.intent) or
                    any(handle.installed for handle in info.handle)
                )
                
                if has_services:
                    print("\n✓ At least one service is marked as installed")
                    return True
                else:
                    print("\n✗ No services are marked as installed")
                    return False
                    
            except Exception as e:
                print(f"✗ Failed to parse Info object: {e}")
                import traceback
                traceback.print_exc()
                return False
                
    except Exception as e:
        print(f"✗ Connection failed: {e}")
        import traceback
        traceback.print_exc()
        return False


if __name__ == "__main__":
    result = asyncio.run(test_gateway())
    sys.exit(0 if result else 1)
