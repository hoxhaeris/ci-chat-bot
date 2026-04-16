"""Workspace command execution with environment allowlisting.

Adapted from ship-help-bot's workspace tools for ci-chat-bot.
Runs shell commands in workspace directories with a sanitized environment.
"""

import logging
import os
import subprocess
import threading
from typing import Any

from .workspace_sessions import validate_and_touch

logger = logging.getLogger(__name__)

# Limits concurrent ws_exec calls to prevent OOM from overlapping
# memory-intensive operations (e.g. git clones).
_exec_semaphore = threading.Semaphore(1)

# Only these environment variables are passed to subprocess.
# Narrowed from ship-help-bot: no GitLab vars (not needed for ci-chat-bot).
ENV_ALLOWLIST = {
    "GH_TOKEN",
    "GITHUB_TOKEN",
    "HOME",
    "PATH",
    "LANG",
    "LC_ALL",
    "LC_CTYPE",
    "TERM",
    "USER",
    "LOGNAME",
    "SHELL",
    "TMPDIR",
    "TMP",
    "GIT_AUTHOR_NAME",
    "GIT_AUTHOR_EMAIL",
    "GIT_COMMITTER_NAME",
    "GIT_COMMITTER_EMAIL",
    "HTTP_PROXY",
    "HTTPS_PROXY",
    "http_proxy",
    "https_proxy",
    "NO_PROXY",
    "no_proxy",
    "REQUESTS_CA_BUNDLE",
}


def _build_safe_env() -> dict[str, str]:
    """Build a subprocess environment with only allowlisted variables."""
    env = {k: v for k, v in os.environ.items() if k in ENV_ALLOWLIST}
    # Mirror GH_TOKEN <-> GITHUB_TOKEN so both `gh` CLI and git credential
    # helpers work.
    if "GITHUB_TOKEN" in env and "GH_TOKEN" not in env:
        env["GH_TOKEN"] = env["GITHUB_TOKEN"]
    gh_token = env.get("GH_TOKEN", "")
    if gh_token:
        env["GH_TOKEN"] = gh_token
        env["GITHUB_TOKEN"] = gh_token
    return env


def ws_exec(
    workspace_path: str, command: str, timeout: int = 120
) -> dict[str, Any]:
    """Execute a shell command in a workspace directory.

    Runs the command via bash in the workspace with a sanitized environment
    (only safe variables like PATH, HOME, GH_TOKEN are inherited). Only one
    ws_exec runs at a time to prevent OOM from concurrent git clones.

    Use this for git commands, grep, find, and other read-only shell
    operations. For file reading, prefer ws_read_file (has path traversal
    protection).

    Args:
        workspace_path: The workspace directory path (from workspace_new
            or clone_openshift_release).
        command: Shell command to execute (run via bash -c).
        timeout: Maximum execution time in seconds (default 120).

    Returns:
        dict with 'stdout', 'stderr', and 'returncode'.
        On error, returns stderr with the error message and returncode -1.
    """
    error = validate_and_touch(workspace_path)
    if error:
        return {"stdout": "", "stderr": error, "returncode": -1}

    safe_env = _build_safe_env()

    with _exec_semaphore:
        try:
            result = subprocess.run(
                ["bash", "-c", command],
                cwd=workspace_path,
                env=safe_env,
                capture_output=True,
                text=True,
                timeout=timeout,
            )
            return {
                "stdout": result.stdout[-10_000:] if len(result.stdout) > 10_000 else result.stdout,
                "stderr": result.stderr[-5_000:] if len(result.stderr) > 5_000 else result.stderr,
                "returncode": result.returncode,
            }
        except subprocess.TimeoutExpired:
            return {
                "stdout": "",
                "stderr": f"Command timed out after {timeout} seconds",
                "returncode": -1,
            }
        except Exception as e:
            logger.error(f"ws_exec error in {workspace_path}: {e}")
            return {"stdout": "", "stderr": str(e), "returncode": -1}
