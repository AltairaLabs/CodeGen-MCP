#!/usr/bin/env python3
"""
End-to-End MCP Test Script (Docker)

Tests complete workflow with Docker Compose:
1. Start coordinator + worker via Docker
2. Connect to coordinator via docker exec
3. Execute full test workflow
"""

import asyncio
import subprocess
import sys
import time
from pathlib import Path

try:
    from mcp import ClientSession, StdioServerParameters
    from mcp.client.stdio import stdio_client
except ImportError:
    print("❌ Error: MCP SDK not installed")
    print("Run: make setup-e2e-tests")
    sys.exit(1)


def run_cmd(cmd, check=True):
    """Run a shell command and return the result"""
    result = subprocess.run(
        cmd, shell=True, capture_output=True, text=True
    )
    if check and result.returncode != 0:
        print(f"❌ Command failed: {cmd}")
        print(f"   Error: {result.stderr}")
        sys.exit(1)
    return result


async def run_e2e_test():
    """Run the complete end-to-end test workflow"""
    
    print("=" * 70)
    print("🚀 CodeGen-MCP End-to-End Test (Docker)")
    print("=" * 70)
    
    # Phase 0: Start Docker services
    print("\n🐳 Phase 0: Starting Docker Compose services...")
    run_cmd("docker compose up -d --wait", check=False)
    
    print("   Waiting for services to be ready...")
    time.sleep(5)
    
    # Check coordinator health
    result = run_cmd("docker compose ps coordinator", check=False)
    if "healthy" not in result.stdout:
        print("   ❌ Coordinator not healthy")
        run_cmd("docker compose logs coordinator")
        run_cmd("docker compose down")
        sys.exit(1)
    print("   ✅ Coordinator healthy")
    
    # Check worker registration
    result = run_cmd(
        "docker compose logs coordinator | grep 'Worker registered'",
        check=False
    )
    if result.returncode == 0:
        print("   ✅ Worker registered")
    else:
        print("   ⚠️  Worker not yet registered, waiting...")
        time.sleep(3)
    
    # Phase 1: Connect via docker exec
    print("\n🔌 Phase 1: Connecting to MCP server via Docker...")
    
    # Use docker exec to connect to the running coordinator
    server_params = StdioServerParameters(
        command="docker",
        args=["exec", "-i", "codegen-coordinator", "/app/coordinator"],
        env=None
    )
    
    try:
        async with stdio_client(server_params) as (read, write):
            async with ClientSession(read, write) as session:
                await session.initialize()
                print("   ✅ Connected successfully")
                
                # Phase 2: List tools
                print("\n📋 Phase 2: Listing available tools...")
                tools_result = await session.list_tools()
                tool_names = [t.name for t in tools_result.tools]
                print(f"   Available tools: {', '.join(tool_names)}")
                
                # Verify our new tools are available
                required_tools = [
                    "echo", "fs.read", "fs.write",
                    "fs.list", "run.python", "pkg.install"
                ]
                for tool in required_tools:
                    if tool in tool_names:
                        print(f"   ✅ {tool}")
                    else:
                        print(f"   ❌ {tool} - MISSING!")
                        sys.exit(1)
                
                # Phase 3: Write hello world
                print("\n✍️  Phase 3: Writing hello.py...")
                hello_code = """print('Hello from CodeGen-MCP!')
print(f'2 + 2 = {2 + 2}')
print('Python is working!')
"""
                write_result = await session.call_tool(
                    "fs.write",
                    arguments={
                        "path": "hello.py",
                        "contents": hello_code
                    }
                )
                write_msg = (
                    write_result.content[0].text
                    if write_result.content else 'success'
                )
                print(f"   ✅ File written: {write_msg}")
                
                # Phase 4: Execute Python
                print("\n🐍 Phase 4: Executing hello.py...")
                run_result = await session.call_tool(
                    "run.python",
                    arguments={
                        "file": "hello.py"
                    }
                )
                output = (
                    run_result.content[0].text
                    if run_result.content else ""
                )
                print("   Execution output:")
                for line in output.split('\n'):
                    if line.strip():
                        print(f"      {line}")
                
                # Verify output contains expected text
                if ("Hello from CodeGen-MCP!" in output and
                        "2 + 2 = 4" in output):
                    print("   ✅ Python execution successful")
                else:
                    print("   ❌ Output doesn't match expected")
                    print(f"   Actual output: {output}")
                    sys.exit(1)
                
                # Phase 5: Read file back
                print("\n📖 Phase 5: Reading hello.py back...")
                read_result = await session.call_tool(
                    "fs.read",
                    arguments={
                        "path": "hello.py"
                    }
                )
                file_contents = (
                    read_result.content[0].text
                    if read_result.content else ""
                )
                if "Hello from CodeGen-MCP!" in file_contents:
                    print("   ✅ File read successfully")
                    print(f"   File size: {len(file_contents)} bytes")
                else:
                    print("   ❌ File contents don't match")
                    sys.exit(1)
                
                # Phase 6: List directory
                print("\n📂 Phase 6: Listing workspace directory...")
                list_result = await session.call_tool(
                    "fs.list",
                    arguments={
                        "path": ""
                    }
                )
                dir_listing = (
                    list_result.content[0].text
                    if list_result.content else ""
                )
                if "hello.py" in dir_listing:
                    print("   ✅ Directory listing successful")
                    files = dir_listing.strip().split('\n')[:5]
                    print(f"   First few files: {', '.join(files)}")
                else:
                    print("   ❌ hello.py not found in listing")
                    sys.exit(1)
                
                # Phase 7: Install package
                print("\n📦 Phase 7: Installing requests package...")
                print("   (This may take a few seconds...)")
                install_result = await session.call_tool(
                    "pkg.install",
                    arguments={
                        "packages": "requests"
                    }
                )
                install_output = (
                    install_result.content[0].text
                    if install_result.content else ""
                )
                if ("Successfully installed" in install_output or
                        "Requirement already satisfied" in install_output):
                    print("   ✅ Package installed successfully")
                else:
                    print(f"   ⚠️  Output: {install_output[:100]}...")
                
                # Phase 8: Use installed package
                print("\n🌐 Phase 8: Testing installed package...")
                test_code = """
import requests
print(f"requests version: {requests.__version__}")
print("✅ Package import successful!")
print(f"has get: {hasattr(requests, 'get')}")
"""
                test_result = await session.call_tool(
                    "run.python",
                    arguments={
                        "code": test_code
                    }
                )
                test_output = (
                    test_result.content[0].text
                    if test_result.content else ""
                )
                print("   Test output:")
                for line in test_output.split('\n'):
                    if line.strip():
                        print(f"      {line}")
                
                if ("Package import successful!" in test_output and
                        "requests version:" in test_output):
                    print("   ✅ Installed package works correctly")
                else:
                    print("   ❌ Package import failed")
                    sys.exit(1)
                
                # Phase 9: Test echo (sanity check)
                print("\n🔊 Phase 9: Testing echo tool...")
                echo_result = await session.call_tool(
                    "echo",
                    arguments={
                        "message": "End-to-end test complete!"
                    }
                )
                echo_output = (
                    echo_result.content[0].text
                    if echo_result.content else ""
                )
                if "End-to-end test complete!" in echo_output:
                    print(f"   ✅ Echo: {echo_output.strip()}")
                else:
                    print("   ❌ Echo failed")
                    sys.exit(1)
                
                print("\n" + "=" * 70)
                print("🎉 All Tests Passed! E2E Test Complete!")
                print("=" * 70)
                print("\n✅ Summary:")
                print("   • Docker Compose services started")
                print("   • MCP connection established via docker exec")
                print("   • All 6 tools registered and working")
                print("   • File write/read operations successful")
                print("   • Python code execution working")
                print("   • Package installation functional")
                print("   • Installed packages usable")
                print("   • Directory listing operational")
                
    except Exception as e:
        print(f"\n❌ Test failed with error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
    finally:
        # Cleanup
        print("\n🧹 Cleaning up...")
        run_cmd("docker compose down", check=False)
        print("   ✅ Docker services stopped")


if __name__ == "__main__":
    try:
        asyncio.run(run_e2e_test())
    except KeyboardInterrupt:
        print("\n\n⚠️  Test interrupted by user")
        run_cmd("docker compose down", check=False)
        sys.exit(1)
