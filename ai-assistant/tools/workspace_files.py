"""Workspace file operations and openshift/release convenience tool.

Adapted from ship-help-bot's workspace tools for ci-chat-bot.
Read-only file operations (no write/replace) plus a convenience tool
for cloning openshift/release with sparse checkout.
"""

import glob as globmod
import logging
import os
import re
import subprocess
from pathlib import Path
from typing import Any

from .workspace_exec import ws_exec
from .workspace_sessions import validate_and_touch, workspace_new

logger = logging.getLogger(__name__)


def _resolve_path(workspace_path: str, file_path: str) -> str | None:
    """Resolve a file path within a workspace, preventing traversal."""
    try:
        base = Path(workspace_path).resolve()
        target = (base / file_path).resolve()
        if not str(target).startswith(str(base)):
            return None
        return str(target)
    except Exception:
        return None


# ---------------------------------------------------------------------------
# Convenience tool: clone openshift/release
# ---------------------------------------------------------------------------


def clone_openshift_release(
    sparse_paths: str = "ci-operator/config/",
) -> dict[str, Any]:
    """Clone the openshift/release repository for browsing CI configuration.

    Creates a workspace and performs a shallow, sparse clone of
    github.com/openshift/release. By default only checks out the
    ci-operator/config/ directory, which contains the ci-operator
    configuration for every OpenShift repository.

    After cloning, use ws_list, ws_read_file, or ws_grep to browse
    the contents. Destroy the workspace with workspace_destroy when done.

    Common sparse_paths values:
    - "ci-operator/config/" (default) -- CI operator configs for all repos
    - "ci-operator/config/ ci-operator/jobs/" -- configs + generated Prow jobs
    - "ci-operator/step-registry/" -- step registry definitions
    - "core-services/ci-chat-bot/" -- ci-chat-bot's own configuration

    Args:
        sparse_paths: Space-separated directory paths to include in the
            sparse checkout. Default: "ci-operator/config/".

    Returns:
        dict with 'workspace_path', 'repo', 'sparse_paths', and 'message'.
        On error, returns a dict with an 'error' key.
    """
    ws = workspace_new("openshift-release")
    if "error" in ws:
        return ws

    ws_path = ws["path"]

    clone_cmd = (
        "git clone --depth 1 --filter=blob:none --sparse "
        "https://github.com/openshift/release.git . "
        f"&& git sparse-checkout set {sparse_paths}"
    )

    result = ws_exec(ws_path, clone_cmd, timeout=120)
    if result["returncode"] != 0:
        # Clean up the workspace on clone failure
        from .workspace_sessions import workspace_destroy
        workspace_destroy(ws_path)
        stderr = result.get("stderr", "unknown error")
        return {"error": f"Failed to clone openshift/release: {stderr}"}

    paths_list = sparse_paths.strip().split()
    return {
        "workspace_path": ws_path,
        "repo": "openshift/release",
        "sparse_paths": paths_list,
        "message": (
            "Repository cloned. Use ws_list, ws_read_file, or ws_grep "
            "to browse the contents. Call workspace_destroy when done."
        ),
    }


# ---------------------------------------------------------------------------
# File tools (read-only)
# ---------------------------------------------------------------------------


def ws_read_file(
    workspace_path: str,
    file_path: str,
    start_line: int = 0,
    end_line: int = 0,
) -> dict[str, Any]:
    """Read a file from a workspace directory.

    Use this to read ci-operator configuration files, Prow job configs,
    or any other file in a cloned repository.

    Args:
        workspace_path: The workspace directory path (from workspace_new
            or clone_openshift_release).
        file_path: Relative path to the file within the workspace.
        start_line: First line to read (1-based, 0 = from start).
        end_line: Last line to read (1-based, 0 = to end).

    Returns:
        dict with 'content' and 'path'.
        On error, returns a dict with an 'error' key.
    """
    error = validate_and_touch(workspace_path)
    if error:
        return {"error": error}

    full_path = _resolve_path(workspace_path, file_path)
    if not full_path:
        return {"error": f"Invalid path: {file_path}"}

    try:
        content = Path(full_path).read_text()
        if start_line or end_line:
            lines = content.splitlines(keepends=True)
            s = (start_line - 1) if start_line else 0
            e = end_line if end_line else len(lines)
            content = "".join(lines[s:e])
        return {"content": content, "path": file_path}
    except Exception as e:
        return {"error": str(e)}


def ws_grep(
    workspace_path: str,
    pattern: str,
    file_pattern: str = "**/*",
) -> dict[str, Any]:
    """Search for a regex pattern across workspace files.

    Use this to find specific configuration entries, test definitions,
    or references across a cloned repository.

    Args:
        workspace_path: The workspace directory path.
        pattern: Regex pattern to search for.
        file_pattern: Glob pattern to filter files (default: all files).
            Example: "**/*.yaml" to search only YAML files.

    Returns:
        dict with 'results' (newline-separated matches in
        'file:line:content' format, max 200 results).
        On error, returns a dict with an 'error' key.
    """
    error = validate_and_touch(workspace_path)
    if error:
        return {"error": error}

    matches: list[str] = []
    for filepath in globmod.glob(
        os.path.join(workspace_path, file_pattern), recursive=True
    ):
        if not os.path.isfile(filepath):
            continue
        try:
            with open(filepath) as f:
                for i, line in enumerate(f, 1):
                    if re.search(pattern, line):
                        rel = os.path.relpath(filepath, workspace_path)
                        matches.append(f"{rel}:{i}:{line.rstrip()}")
        except (UnicodeDecodeError, PermissionError):
            continue

    return {"results": "\n".join(matches[:200])}


def ws_list(
    workspace_path: str, relative_path: str = "."
) -> dict[str, Any]:
    """List directory contents in a workspace.

    Shows files and directories with a prefix indicating type
    ('d' for directory, 'f' for file).

    Args:
        workspace_path: The workspace directory path.
        relative_path: Relative path within the workspace (default: root).

    Returns:
        dict with 'listing' and 'path'.
        On error, returns a dict with an 'error' key.
    """
    error = validate_and_touch(workspace_path)
    if error:
        return {"error": error}

    full_path = _resolve_path(workspace_path, relative_path)
    if not full_path:
        return {"error": f"Invalid path: {relative_path}"}

    try:
        entries = sorted(os.listdir(full_path))
        items = []
        for e in entries:
            fp = os.path.join(full_path, e)
            prefix = "d " if os.path.isdir(fp) else "f "
            items.append(prefix + e)
        return {"listing": "\n".join(items), "path": relative_path}
    except Exception as e:
        return {"error": str(e)}


def ws_tree(workspace_path: str, depth: int = 3) -> dict[str, Any]:
    """Show directory tree of a workspace.

    Useful for understanding the structure of a cloned repository
    before diving into specific files.

    Args:
        workspace_path: The workspace directory path.
        depth: Maximum depth to display (default: 3).

    Returns:
        dict with 'tree' output.
        On error, returns a dict with an 'error' key.
    """
    error = validate_and_touch(workspace_path)
    if error:
        return {"error": error}

    try:
        result = subprocess.run(
            ["find", ".", "-maxdepth", str(depth), "-print"],
            cwd=workspace_path,
            capture_output=True,
            text=True,
            timeout=10,
        )
        return {"tree": result.stdout}
    except Exception as e:
        return {"error": str(e)}
