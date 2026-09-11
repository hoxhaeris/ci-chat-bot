"""Unit tests for the ship-help-bot MCP research client.

Hermetic: the MCP client transport is faked, so these run with no network,
no GCP, and no ship-help-bot — safe for CI. (pytest ``asyncio_mode = auto``
runs the ``async def`` tests directly.)
"""

from __future__ import annotations

import contextlib

import tools.ship_help_research as shr


class _Block:
    def __init__(self, text: str):
        self.type = "text"
        self.text = text


class _Result:
    def __init__(self, *, text: str = "", is_error: bool = False):
        self.content = [_Block(text)] if text else []
        self.isError = is_error
        self.structuredContent = None
        self.meta = None


def _install_fakes(monkeypatch, *, result=None, call_exc=None, connect_exc=None):
    """Patch the mcp client transport used inside researcher()."""

    @contextlib.asynccontextmanager
    async def fake_streamable(url, headers=None, **kwargs):
        assert headers and headers.get("Authorization", "").startswith("Bearer ")
        if connect_exc is not None:
            raise connect_exc
        yield ("read", "write", None)

    class FakeSession:
        def __init__(self, read, write, *args, **kwargs):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *exc):
            return False

        async def initialize(self):
            return None

        async def call_tool(self, name, arguments=None, **kwargs):
            assert name == "ask_persona"
            assert (arguments or {}).get("question")
            if call_exc is not None:
                raise call_exc
            return result

    monkeypatch.setattr("mcp.client.streamable_http.streamablehttp_client", fake_streamable)
    monkeypatch.setattr("mcp.ClientSession", FakeSession)


def _configure(monkeypatch):
    monkeypatch.setattr(shr, "_MCP_URL", "http://localhost:8091/mcp")
    monkeypatch.setattr(shr, "_MCP_TOKEN", "test-token")


async def test_returns_unavailable_when_not_configured(monkeypatch):
    monkeypatch.setattr(shr, "_MCP_URL", "")
    monkeypatch.setattr(shr, "_MCP_TOKEN", "")
    out = await shr.researcher("how do I launch gcp?")
    assert out["error"] == "research_unavailable"


async def test_success_returns_findings(monkeypatch):
    _configure(monkeypatch)
    _install_fakes(monkeypatch, result=_Result(text="Here is the answer."))
    out = await shr.researcher("why did my job fail?")
    assert out == {"findings": "Here is the answer."}


async def test_empty_content_returns_no_results(monkeypatch):
    _configure(monkeypatch)
    _install_fakes(monkeypatch, result=_Result(text=""))
    out = await shr.researcher("obscure question")
    assert "findings" in out
    assert "No relevant results" in out["findings"]


async def test_tool_error_passthrough(monkeypatch):
    _configure(monkeypatch)
    _install_fakes(monkeypatch, result=_Result(text="Authentication failed", is_error=True))
    out = await shr.researcher("q")
    assert out["error"] == "research_error"
    assert "Authentication failed" in out["message"]


async def test_call_exception_degrades(monkeypatch):
    _configure(monkeypatch)
    _install_fakes(monkeypatch, call_exc=RuntimeError("boom"))
    out = await shr.researcher("q")
    assert out["error"] == "research_unavailable"


async def test_connect_exception_degrades(monkeypatch):
    _configure(monkeypatch)
    _install_fakes(monkeypatch, connect_exc=ConnectionError("refused"))
    out = await shr.researcher("q")
    assert out["error"] == "research_unavailable"


async def test_timeout_degrades(monkeypatch):
    _configure(monkeypatch)
    _install_fakes(monkeypatch, call_exc=TimeoutError())
    out = await shr.researcher("q")
    assert out["error"] == "research_unavailable"
