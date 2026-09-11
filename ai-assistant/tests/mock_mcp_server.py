"""Local mock ship-help-bot MCP server for manual end-to-end testing.

Serves an ``ask_persona`` tool (canned/echo response) over streamable-HTTP so
the cluster-bot assistant's ``researcher()`` client can be exercised with zero
external dependencies — no GCP, no ship-help-bot, no production.

Usage:
    uv run python tests/mock_mcp_server.py       # serves at http://127.0.0.1:8091/mcp

Then, in another shell:
    export SHIP_HELP_MCP_URL=http://127.0.0.1:8091/mcp
    export SHIP_HELP_MCP_TOKEN=any-nonempty-value   # the mock does not verify it
    make run
    curl -X POST localhost:3000/ask -H 'Content-Type: application/json' \
      -d '{"question":"why did my launch job fail?","user_id":"u","thread_id":"t"}'
"""

from __future__ import annotations

import os

from mcp.server.fastmcp import FastMCP


def build_server() -> FastMCP:
    host = os.environ.get("MOCK_MCP_HOST", "127.0.0.1")
    port = int(os.environ.get("MOCK_MCP_PORT", "8091"))
    server = FastMCP("mock-ship-help", host=host, port=port)

    @server.tool()
    async def ask_persona(question: str) -> str:
        """Mock of ship-help-bot's ask_persona — returns a canned answer."""
        return (
            "[mock ship-help-bot] Canned research answer for local testing.\n\n"
            f"Question received: {question}\n\n"
            "A real run would return a synthesized answer grounded in Slack "
            "history, Jira, GitHub, curated docs, and verified knowledge."
        )

    return server


if __name__ == "__main__":
    build_server().run(transport="streamable-http")
