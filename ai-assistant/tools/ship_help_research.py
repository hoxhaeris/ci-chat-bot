"""Research tool backed by the ship-help-bot ("Chai Bot") MCP server.

This replaces the in-process Vertex AI Search research pipeline. Instead of
querying datastores directly, ``researcher()`` delegates to ship-help-bot's
``ask_persona`` MCP tool, which runs a fully-instructed OpenShift engineering
persona (researcher + tools + curated knowledge) and returns a synthesized,
grounded answer.

Configuration (all via environment):
- ``SHIP_HELP_MCP_URL``   Full MCP endpoint. Examples:
      local (standalone):  http://localhost:8091/mcp
      deployed (embedded): https://<host>/personas/cluster_bot/mcp
- ``SHIP_HELP_MCP_TOKEN`` Bearer JWT minted for the cluster-bot service identity.
- ``SHIP_HELP_MCP_TOKEN_FILE`` Path to a file holding the Bearer JWT, read fresh
      on each call (used when SHIP_HELP_MCP_TOKEN is unset). Lets a re-minted
      token — minting a new one revokes the previous — be picked up without
      restarting this service.
- ``SHIP_HELP_MCP_TIMEOUT``          Overall per-call ceiling, seconds (default 110,
      kept under the Go AIClient's 120s request timeout).
- ``SHIP_HELP_MCP_CONNECT_TIMEOUT``  Connect/handshake timeout, seconds (default 30).

If the endpoint or token is unset, or the call fails/times out, the tool
returns a soft-error dict so the agent degrades gracefully to its local tools
and prompt knowledge (mirrors the old _SafeAgentTool behavior).
"""

from __future__ import annotations

import logging
import os
from datetime import timedelta

logger = logging.getLogger(__name__)


def _read_float_env(name: str, default: float) -> float:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    try:
        value = float(raw)
    except ValueError:
        logger.warning("Invalid %s=%r (must be a number); using default=%s", name, raw, default)
        return default
    return value if value > 0 else default


_TOTAL_TIMEOUT = _read_float_env("SHIP_HELP_MCP_TIMEOUT", 110.0)
_CONNECT_TIMEOUT = _read_float_env("SHIP_HELP_MCP_CONNECT_TIMEOUT", 30.0)

# Soft-failure payload: keeps the agent working (it falls back to local tools
# and prompt knowledge) instead of surfacing a hard error to the user.
_UNAVAILABLE = {
    "error": "research_unavailable",
    "message": (
        "The knowledge research backend could not be reached. Answer using your "
        "existing tools and prompt knowledge, and say so explicitly if unsure."
    ),
}


def _resolve_url() -> str:
    return os.environ.get("SHIP_HELP_MCP_URL", "").strip()


def _resolve_token() -> str:
    """Return the MCP bearer token, read fresh on each call.

    Prefers SHIP_HELP_MCP_TOKEN (e.g. a secret mounted in prod); otherwise
    reads SHIP_HELP_MCP_TOKEN_FILE. Reading at call time means a re-minted
    token (minting a new one revokes the previous) is picked up without
    restarting this service.
    """
    token = os.environ.get("SHIP_HELP_MCP_TOKEN", "").strip()
    if token:
        return token
    path = os.environ.get("SHIP_HELP_MCP_TOKEN_FILE", "").strip()
    if path:
        try:
            with open(path) as fh:
                return fh.read().strip()
        except OSError as exc:
            logger.warning(
                "researcher: could not read SHIP_HELP_MCP_TOKEN_FILE=%s: %s", path, exc
            )
    return ""


def _looks_like_auth_failure(text: str) -> bool:
    """ship-help-bot returns some auth failures as plain text (isError unset)."""
    low = text.lower()
    return (
        low.startswith("authentication failed")
        or "token has been revoked" in low
        or "token is invalid" in low
        or "token has expired" in low
    )


def _extract_text(result) -> str:
    """Concatenate text blocks from an MCP CallToolResult."""
    parts: list[str] = []
    for block in getattr(result, "content", None) or []:
        if getattr(block, "type", None) == "text":
            text = getattr(block, "text", "") or ""
            if text:
                parts.append(text)
    return "\n".join(parts).strip()


async def researcher(question: str) -> dict:
    """Search verified OpenShift/CRT knowledge and internal engineering discussions.

    Delegates the question to the OpenShift CI help persona (ship-help-bot),
    which searches Slack history, Jira, GitHub, curated docs, and verified team
    knowledge, then returns a synthesized, grounded answer. Use this when a
    question may benefit from knowledge beyond what is in your prompt — in
    particular troubleshooting, error diagnosis, and "why did X happen"
    questions about CI, releases, and workflows. Do not use it for simple
    command syntax you can already construct.

    Args:
        question: A clear, self-contained question. Include version, platform,
            workflow, job name, or error text when relevant.

    Returns:
        On success: ``{"findings": "<synthesized answer>"}``.
        On failure: ``{"error": ..., "message": ...}`` — degrade to local knowledge.
    """
    url = _resolve_url()
    token = _resolve_token()
    if not url or not token:
        logger.info("researcher: SHIP_HELP_MCP_URL/TOKEN not configured; research disabled")
        return _UNAVAILABLE

    # Imported lazily so this module (and the agent build) load even if the mcp
    # client is somehow unavailable.
    import asyncio

    from mcp import ClientSession
    from mcp.client.streamable_http import streamablehttp_client

    headers = {"Authorization": f"Bearer {token}"}
    try:
        async with asyncio.timeout(_TOTAL_TIMEOUT):
            async with streamablehttp_client(
                url,
                headers=headers,
                timeout=_CONNECT_TIMEOUT,
                sse_read_timeout=_TOTAL_TIMEOUT,
            ) as (read_stream, write_stream, _get_session_id):
                async with ClientSession(read_stream, write_stream) as session:
                    await session.initialize()
                    result = await session.call_tool(
                        "ask_persona",
                        {"question": question},
                        read_timeout_seconds=timedelta(seconds=_TOTAL_TIMEOUT),
                    )
    except TimeoutError:
        logger.warning("researcher: ask_persona timed out after %ss", _TOTAL_TIMEOUT)
        return _UNAVAILABLE
    except Exception as exc:  # degrade gracefully on any client/transport error
        logger.warning("researcher: ask_persona call failed: %s", exc)
        return _UNAVAILABLE

    text = _extract_text(result)
    if getattr(result, "isError", False) or _looks_like_auth_failure(text):
        # ask_persona reports auth/maintenance issues as text, sometimes without
        # isError set (e.g. "Authentication failed: token revoked"). Treat as a
        # soft failure and pass the reason through so the agent degrades cleanly.
        logger.warning("researcher: ask_persona returned an error: %s", text[:200])
        return {"error": "research_error", "message": text or _UNAVAILABLE["message"]}
    if not text:
        return {"findings": "No relevant results were found."}
    logger.info(
        "researcher: ask_persona returned %d chars: %s",
        len(text),
        text[:300],
    )
    return {"findings": text}
