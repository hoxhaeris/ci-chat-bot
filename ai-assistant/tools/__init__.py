"""Tool registry for the cluster-bot AI assistant.

The assistant keeps only tools that must run locally for architectural reasons:
the cluster-bot internal-API tools, which call the Go bot's loopback-only API
on 127.0.0.1:8082 (only this sidecar can reach it). Everything else —
knowledge, troubleshooting, CI/log investigation, release lookups, and code
browsing — is delegated to ship-help-bot's cluster_bot persona via the
`researcher` tool (see tools/ship_help_research.py, appended in agent.py).

All tools are plain Python functions with docstrings — ADK wraps them
automatically as FunctionTool instances.
"""

from .cluster_bot_api import (
    check_command,
    check_mce_command,
    check_rosa_command,
    list_supported_options,
    lookup_capacity_status,
    lookup_hypershift_versions,
    lookup_mce_versions,
    lookup_quota_status,
    lookup_rosa_versions,
    validate_job,
    validate_workflow,
)


def get_all_tools() -> list:
    """Return the local tool functions for ADK agent registration.

    Only the cluster-bot internal-API tools live here. Research and
    troubleshooting are delegated to the ship-help-bot persona via
    `researcher`, which agent.py appends to this list.
    """
    return [
        # Cluster bot API tools (call the Go bot's internal API via localhost)
        validate_job,
        check_command,
        list_supported_options,
        validate_workflow,
        lookup_mce_versions,
        check_mce_command,
        lookup_rosa_versions,
        check_rosa_command,
        lookup_hypershift_versions,
        lookup_quota_status,
        lookup_capacity_status,
    ]
