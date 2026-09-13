#!/usr/bin/env bash
#
# run-with-research.sh — start the ci-chat-bot ai-assistant with the
# ship-help-bot MCP research backend wired in.
#
# Prereq: start the MCP server first (it writes the env file this sources):
#     ~/source/ci-chat-bot/ai-assistant/hack/run-ship-help-mcp.sh
#
# Without that env file the ai-assistant still runs, but the `researcher` tool
# is disabled (it degrades to "research_unavailable").
#
set -euo pipefail

SHIP_HELP_MCP_ENV_FILE="${SHIP_HELP_MCP_ENV_FILE:-/tmp/ship-help-mcp.env}"

# ai-assistant repo root (parent of this hack/ dir)
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -f "$SHIP_HELP_MCP_ENV_FILE" ]]; then
  # A stale directly-set token would shadow the file-based one; drop it so the
  # researcher always reads the current token from SHIP_HELP_MCP_TOKEN_FILE.
  unset SHIP_HELP_MCP_TOKEN
  set -a
  # shellcheck disable=SC1090
  source "$SHIP_HELP_MCP_ENV_FILE"
  set +a
  echo "Loaded MCP env: SHIP_HELP_MCP_URL=$SHIP_HELP_MCP_URL (token file: ${SHIP_HELP_MCP_TOKEN_FILE:-<inline>})" >&2
else
  echo "warning: $SHIP_HELP_MCP_ENV_FILE not found — the researcher will be DISABLED." >&2
  echo "         Start it first: ai-assistant/hack/run-ship-help-mcp.sh" >&2
fi

exec make run
