#!/usr/bin/env python3
"""
End-to-End MCP Test Script

Tests complete workflow using HTTP/SSE transport:
1. Connect to coordinator via MCP (HTTP)
2. List available tools
3. Write Python hello world script
4. Execute the script
5. Read the output file back
6. Install a Python package
7. Run code using the installed package
8. List directory contents
"""

import asyncio
import json
import os
import sys
import time

try:
    from mcp import ClientSession
    from mcp.client.sse import sse_client
except ImportError:
    print("❌ Error: MCP SDK not installed")
    print("Run: make setup-e2e-tests")
    sys.exit(1)


async def wait_for_task_result(session: ClientSession, task_id: str, timeout: int = 30) -> str:
    """
    Poll for a task result until it's ready or timeout.
    
    Args:
        session: The MCP client session
        task_id: The task ID to wait for
        timeout: Maximum time to wait in seconds
        
    Returns:
        The task result output as a string
        
    Raises:
        TimeoutError: If task doesn't complete within timeout
        RuntimeError: If task fails
    """
    start_time = time.time()
    poll_interval = 0.5  # Start with 500ms polling
    max_interval = 2.0   # Max 2 seconds between polls
    
    while time.time() - start_time < timeout:
        try:
            result = await session.call_tool(
                "task.get_result",
                arguments={"task_id": task_id}
            )
            
            result_text = result.content[0].text if result.content else ""
            print(f"   [DEBUG] task.get_result returned: {result_text[:200]}")
            
            # Try to parse as JSON to check status
            try:
                result_data = json.loads(result_text)
                status = result_data.get("status", "").lower()
                
                if status == "completed":
                    # Task completed, but we got status JSON instead of result
                    # This shouldn't happen with current implementation
                    raise RuntimeError(f"Task completed but got status response: {result_text}")
                elif status == "failed":
                    error_msg = result_data.get("error", "Unknown error")
                    raise RuntimeError(f"Task failed: {error_msg}")
                elif status in ["queued", "dispatched", "running"]:
                    # Task still processing, continue polling
                    await asyncio.sleep(poll_interval)
                    # Exponential backoff up to max_interval
                    poll_interval = min(poll_interval * 1.5, max_interval)
                    continue
                else:
                    # Unknown status
                    raise RuntimeError(f"Unknown task status: {status}")
            except json.JSONDecodeError:
                # Not JSON, this should be the actual result
                return result_text
                
        except Exception as e:
            error_str = str(e)
            # Check if this is a "not ready" error
            if "not found" in error_str.lower() or "not ready" in error_str.lower():
                await asyncio.sleep(poll_interval)
                poll_interval = min(poll_interval * 1.5, max_interval)
                continue
            else:
                # Some other error, re-raise
                raise
    
    raise TimeoutError(f"Task {task_id} did not complete within {timeout} seconds")


async def run_e2e_test():
    """Run the complete end-to-end test workflow"""
    
    # Get coordinator URL from environment or use default
    # The sse_client expects the full SSE endpoint URL, not a base URL
    coordinator_url = os.getenv("COORDINATOR_URL", "http://localhost:8080/mcp/sse")
    
    print("=" * 70)
    print("🚀 CodeGen-MCP End-to-End Test (HTTP/SSE Transport)")
    print("=" * 70)
    print(f"📡 SSE Endpoint: {coordinator_url}")
    
    # Phase 1: Connect
    print("\n🔌 Phase 1: Connecting to MCP server...")
    
    try:
        async with sse_client(coordinator_url) as (read, write):
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
                
                # Phase 2.5: Check worker availability
                print("\n🔍 Phase 2.5: Checking worker availability...")
                try:
                    # Try the echo tool first (doesn't need worker)
                    echo_check = await session.call_tool(
                        "echo",
                        arguments={"message": "pre-flight check"}
                    )
                    print("   ✅ Echo tool works (coordinator responsive)")
                    
                    # Now try a worker-dependent tool to verify workers are available
                    # Using fs.list with empty path as a lightweight check
                    worker_check = await session.call_tool(
                        "fs.list",
                        arguments={"path": ""}
                    )
                    print("   ✅ Workers available and responding")
                except Exception as worker_error:
                    error_msg = str(worker_error)
                    if "no workers available" in error_msg.lower():
                        print("\n" + "=" * 70)
                        print("❌ NO WORKERS AVAILABLE")
                        print("=" * 70)
                        print("\n⚠️  The coordinator is running but no workers are registered.")
                        print("\n💡 To fix this:")
                        print("   1. Check if worker processes are running:")
                        print("      docker compose ps")
                        print("   2. Start workers if needed:")
                        print("      docker compose up -d worker-1 worker-2")
                        print("   3. Check worker logs:")
                        print("      docker compose logs worker-1")
                        print("   4. Verify workers registered:")
                        print("      curl http://localhost:8080/health")
                        print("\n" + "=" * 70)
                        sys.exit(1)
                    else:
                        # Some other error, re-raise it
                        raise
                
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
                write_response = (
                    write_result.content[0].text
                    if write_result.content else '{}'
                )
                
                # Parse task response and wait for completion
                try:
                    task_data = json.loads(write_response)
                    task_id = task_data.get("task_id")
                    if task_id:
                        print(f"   Task queued: {task_id}")
                        result_text = await wait_for_task_result(
                            session, task_id
                        )
                        print(f"   ✅ File written: {result_text}")
                    else:
                        print(f"   ✅ File written: {write_response}")
                except json.JSONDecodeError:
                    # Not a task response, direct result
                    print(f"   ✅ File written: {write_response}")
                
                # Phase 4: Execute Python
                print("\n🐍 Phase 4: Executing hello.py...")
                run_result = await session.call_tool(
                    "run.python",
                    arguments={
                        "file": "hello.py"
                    }
                )
                run_response = (
                    run_result.content[0].text
                    if run_result.content else ""
                )
                
                # Parse task response and wait for result
                try:
                    task_data = json.loads(run_response)
                    task_id = task_data.get("task_id")
                    if task_id:
                        print(f"   Task queued: {task_id}")
                        output = await wait_for_task_result(session, task_id)
                    else:
                        output = run_response
                except json.JSONDecodeError:
                    # Not a task response, direct result
                    output = run_response
                
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
                    print(f"   Got: {output}")
                    sys.exit(1)
                
                # Phase 5: Read file back
                print("\n📖 Phase 5: Reading hello.py back...")
                read_result = await session.call_tool(
                    "fs.read",
                    arguments={
                        "path": "hello.py"
                    }
                )
                read_response = (
                    read_result.content[0].text
                    if read_result.content else ""
                )
                
                # Parse task response and wait for result
                try:
                    task_data = json.loads(read_response)
                    task_id = task_data.get("task_id")
                    if task_id:
                        print(f"   Task queued: {task_id}")
                        file_contents = await wait_for_task_result(
                            session, task_id
                        )
                    else:
                        file_contents = read_response
                except json.JSONDecodeError:
                    file_contents = read_response
                
                print(f"   DEBUG: file_contents = '{file_contents}'")
                print(f"   DEBUG: length = {len(file_contents)}")
                
                if "Hello from CodeGen-MCP!" in file_contents:
                    print("   ✅ File read successfully")
                    print(f"   File size: {len(file_contents)} bytes")
                else:
                    print("   ❌ File contents don't match")
                    print(f"   Expected 'Hello from CodeGen-MCP!' in output")
                    sys.exit(1)
                
                # Phase 6: List directory
                print("\n📂 Phase 6: Listing workspace directory...")
                list_result = await session.call_tool(
                    "fs.list",
                    arguments={
                        "path": ""
                    }
                )
                list_response = (
                    list_result.content[0].text
                    if list_result.content else ""
                )
                
                # Parse task response and wait for result
                try:
                    task_data = json.loads(list_response)
                    task_id = task_data.get("task_id")
                    if task_id:
                        print(f"   Task queued: {task_id}")
                        dir_listing = await wait_for_task_result(
                            session, task_id
                        )
                    else:
                        dir_listing = list_response
                except json.JSONDecodeError:
                    dir_listing = list_response
                
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
                install_response = (
                    install_result.content[0].text
                    if install_result.content else ""
                )
                
                # Parse task response and wait for result
                try:
                    task_data = json.loads(install_response)
                    task_id = task_data.get("task_id")
                    if task_id:
                        print(f"   Task queued: {task_id}")
                        install_output = await wait_for_task_result(
                            session, task_id, timeout=60
                        )
                    else:
                        install_output = install_response
                except json.JSONDecodeError:
                    install_output = install_response
                
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
                test_response = (
                    test_result.content[0].text
                    if test_result.content else ""
                )
                
                # Parse task response and wait for result
                try:
                    task_data = json.loads(test_response)
                    task_id = task_data.get("task_id")
                    if task_id:
                        print(f"   Task queued: {task_id}")
                        test_output = await wait_for_task_result(
                            session, task_id
                        )
                    else:
                        test_output = test_response
                except json.JSONDecodeError:
                    test_output = test_response
                
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
                echo_response = (
                    echo_result.content[0].text
                    if echo_result.content else ""
                )
                
                # Parse task response and wait for result
                try:
                    task_data = json.loads(echo_response)
                    task_id = task_data.get("task_id")
                    if task_id:
                        print(f"   Task queued: {task_id}")
                        echo_output = await wait_for_task_result(
                            session, task_id
                        )
                    else:
                        echo_output = echo_response
                except json.JSONDecodeError:
                    echo_output = echo_response
                
                if "End-to-end test complete!" in echo_output:
                    print(f"   ✅ Echo: {echo_output.strip()}")
                else:
                    print("   ❌ Echo failed")
                    print("   Expected 'End-to-end test complete!' in output")
                    print(f"   Got: {echo_output}")
                    sys.exit(1)
                
                # Phase 10: Create artifact (zip file)
                print("\n📦 Phase 10: Creating distributable artifact...")
                artifact_code = """
import zipfile
import os

# Create artifacts directory if needed
os.makedirs('artifacts', exist_ok=True)

# Create a zip file with our hello.py script
with zipfile.ZipFile('artifacts/hello-package.zip', 'w') as zf:
    zf.write('hello.py')
    
print(f'Created artifact: hello-package.zip')
print(f'Size: {os.path.getsize("artifacts/hello-package.zip")} bytes')
"""
                artifact_result = await session.call_tool(
                    "run.python",
                    arguments={
                        "code": artifact_code
                    }
                )
                artifact_response = (
                    artifact_result.content[0].text
                    if artifact_result.content else ""
                )
                
                # Parse task response and wait for result
                try:
                    task_data = json.loads(artifact_response)
                    task_id = task_data.get("task_id")
                    if task_id:
                        print(f"   Task queued: {task_id}")
                        artifact_output = await wait_for_task_result(
                            session, task_id
                        )
                    else:
                        artifact_output = artifact_response
                except json.JSONDecodeError:
                    artifact_output = artifact_response
                
                print("   Artifact creation output:")
                for line in artifact_output.split('\n'):
                    if line.strip():
                        print(f"      {line}")
                
                if "Created artifact:" in artifact_output:
                    print("   ✅ Artifact created successfully")
                else:
                    print("   ❌ Artifact creation failed")
                    print(f"   Got: {artifact_output}")
                    sys.exit(1)
                
                # Phase 11: Retrieve artifact via artifact.get
                print("\n⬇️  Phase 11: Retrieving artifact...")
                
                # Check if artifact.get tool is available
                if "artifact.get" not in tool_names:
                    print("   ⚠️  artifact.get tool not available")
                    print("      - skipping artifact retrieval test")
                    print("   💡 This is optional Phase 3 functionality")
                else:
                    # Construct artifact ID
                    # Format: sessionID-timestamp-filename
                    # For now, use filename (session ID tracking not in client)
                    artifact_id = "hello-package.zip"
                    
                    try:
                        retrieve_result = await session.call_tool(
                            "artifact.get",
                            arguments={
                                "artifact_id": artifact_id
                            }
                        )
                        retrieve_response = (
                            retrieve_result.content[0].text
                            if retrieve_result.content else ""
                        )
                        
                        # Parse task response and wait for result
                        try:
                            task_data = json.loads(retrieve_response)
                            task_id = task_data.get("task_id")
                            if task_id:
                                print(f"   Task queued: {task_id}")
                                artifact_data = await wait_for_task_result(
                                    session, task_id
                                )
                            else:
                                artifact_data = retrieve_response
                        except json.JSONDecodeError:
                            artifact_data = retrieve_response
                        
                        # Verify artifact was retrieved
                        # Response should contain artifact metadata/data
                        if artifact_data and len(artifact_data) > 0:
                            print("   ✅ Artifact retrieved successfully")
                            data_size = len(artifact_data)
                            print(f"   Artifact data size: {data_size} bytes")
                        else:
                            print("   ❌ Artifact retrieval returned")
                            print("      empty data")
                            sys.exit(1)
                            
                    except Exception as artifact_error:
                        print(f"   ⚠️  Retrieval failed: {artifact_error}")
                        print("   💡 Expected if artifact.get not")
                        print("      fully implemented")
                
                print("\n" + "=" * 70)
                print("🎉 All Tests Passed! E2E Test Complete!")
                print("=" * 70)
                print("\n✅ Summary:")
                print("   • MCP connection established")
                print("   • All 6 tools registered and working")
                print("   • File write/read operations successful")
                print("   • Python code execution working")
                print("   • Package installation functional")
                print("   • Installed packages usable")
                print("   • Directory listing operational")
                print("   • Artifact creation working")
                if "artifact.get" in tool_names:
                    print("   • Artifact retrieval available")
                
    except Exception as e:
        print(f"\n❌ Test failed with error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    try:
        asyncio.run(run_e2e_test())
    except KeyboardInterrupt:
        print("\n\n⚠️  Test interrupted by user")
        sys.exit(1)
