#!/usr/bin/env python3
"""Test echo tool"""

import asyncio
from mcp import ClientSession
from mcp.client.sse import sse_client


async def test_echo():
    try:
        async with sse_client("http://localhost:8080/mcp/sse") as (read, write):
            async with ClientSession(read, write) as session:
                await session.initialize()
                print("Connected successfully")
                
                # Try echo tool
                print("\nAttempting to call echo...")
                result = await session.call_tool(
                    "echo",
                    arguments={"message": "Hello!"}
                )
                print(f"Result: {result}")
                
    except Exception as e:
        print(f"Error: {type(e).__name__}: {e}")
        import traceback
        traceback.print_exc()


if __name__ == "__main__":
    asyncio.run(test_echo())
