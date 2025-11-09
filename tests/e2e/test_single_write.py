#!/usr/bin/env python3
"""Simple test to reproduce fs.write panic"""

import asyncio
from mcp import ClientSession
from mcp.client.sse import sse_client


async def test_write():
    try:
        async with sse_client("http://localhost:8080/mcp/sse") as (read, write):
            async with ClientSession(read, write) as session:
                await session.initialize()
                print("Connected successfully")
                
                # Try to write a file
                print("\nAttempting to write hello.py...")
                result = await session.call_tool(
                    "fs.write",
                    arguments={
                        "path": "hello.py",
                        "contents": 'print("Hello, CodeGen MCP!")\n',
                    }
                )
                print(f"Result: {result}")
                
    except Exception as e:
        print(f"Error: {type(e).__name__}: {e}")
        import traceback
        traceback.print_exc()


if __name__ == "__main__":
    asyncio.run(test_write())
