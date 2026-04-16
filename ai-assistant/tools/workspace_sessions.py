"""Workspace session management with TTL-based lifecycle.

Adapted from ship-help-bot's workspace tools for ci-chat-bot.
Provides ephemeral directories for cloning and browsing repositories.
"""

import logging
import os
import re
import shutil
import threading
import time
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

BASE_DIR = os.environ.get("WORKSPACE_BASE_DIR", "/tmp/ci-chat-bot-workspaces")
DEFAULT_TTL = 1800  # 30 minutes

_cleanup_thread_started = False
_lock = threading.Lock()  # Protects _sessions dict across request + cleanup threads


@dataclass
class WorkspaceSession:
    """Tracks an ephemeral workspace directory with TTL-based expiry."""

    path: str
    description: str
    created_at: float
    last_accessed: float
    ttl_seconds: int = DEFAULT_TTL


_sessions: dict[str, WorkspaceSession] = {}


def _sanitize(text: str, max_len: int = 40) -> str:
    """Sanitize a description for use in a directory name."""
    safe = re.sub(r"[^a-zA-Z0-9_-]", "-", text.strip().lower())
    safe = re.sub(r"-+", "-", safe).strip("-")
    return safe[:max_len] if safe else "workspace"


def _validate_workspace_path(path: str) -> bool:
    """Check that a path is under BASE_DIR and exists."""
    try:
        resolved = str(Path(path).resolve())
        base_resolved = str(Path(BASE_DIR).resolve())
        return resolved.startswith(base_resolved + "/") and Path(path).is_dir()
    except Exception:
        return False


def _touch_session(path: str) -> bool:
    """Extend TTL for a session. Returns False if session not found."""
    with _lock:
        session = _sessions.get(path)
        if session is None:
            return False
        session.last_accessed = time.time()
        return True


def validate_and_touch(workspace_path: str) -> str | None:
    """Validate a workspace path and extend TTL.

    Returns error message string on failure, None on success.
    Called internally by exec and file tools before every operation.
    """
    if not _validate_workspace_path(workspace_path):
        return f"Invalid or expired workspace: {workspace_path}"
    if not _touch_session(workspace_path):
        # Directory exists on disk but session was lost (e.g. TTL cleanup race).
        # Re-register so subsequent operations succeed.
        if Path(workspace_path).is_dir():
            with _lock:
                _sessions[workspace_path] = WorkspaceSession(
                    path=workspace_path,
                    description="(recovered)",
                    created_at=time.time(),
                    last_accessed=time.time(),
                )
            return None
        return f"Workspace not found: {workspace_path}"
    return None


def workspace_new(description: str) -> dict[str, Any]:
    """Create a new ephemeral workspace directory.

    Use this to create a workspace before cloning a repo or running
    commands. The workspace is automatically cleaned up after 30 minutes
    of inactivity (each tool call extends the TTL).

    For cloning openshift/release, prefer clone_openshift_release which
    creates a workspace and clones in one step.

    Args:
        description: Brief description of the workspace purpose
            (e.g. "browse must-gather-operator config").

    Returns:
        dict with 'path' (the workspace directory) and 'description'.
        On error, returns a dict with an 'error' key.
    """
    _ensure_cleanup_started()
    short_id = uuid.uuid4().hex[:8]
    dir_name = f"{short_id}-{_sanitize(description)}"
    ws_path = os.path.join(BASE_DIR, dir_name)

    try:
        os.makedirs(ws_path, exist_ok=True)
    except Exception as e:
        return {"error": f"Failed to create workspace directory: {e}"}

    with _lock:
        _sessions[ws_path] = WorkspaceSession(
            path=ws_path,
            description=description,
            created_at=time.time(),
            last_accessed=time.time(),
        )

    logger.info(f"Workspace created: {ws_path}")
    return {"path": ws_path, "description": description}


def workspace_destroy(workspace_path: str) -> dict[str, Any]:
    """Destroy a workspace directory and free its resources.

    Call this when you are done browsing a cloned repository to free
    disk space. Workspaces are also automatically cleaned up after
    30 minutes of inactivity.

    Args:
        workspace_path: The workspace directory path (from workspace_new
            or clone_openshift_release).

    Returns:
        dict with 'destroyed' boolean and 'path'.
        On error, returns a dict with an 'error' key.
    """
    if not _validate_workspace_path(workspace_path):
        return {"destroyed": False, "error": "Invalid workspace path"}

    with _lock:
        _sessions.pop(workspace_path, None)

    try:
        shutil.rmtree(workspace_path, ignore_errors=True)
        logger.info(f"Workspace destroyed: {workspace_path}")
        return {"destroyed": True, "path": workspace_path}
    except Exception as e:
        logger.warning(f"Error destroying workspace {workspace_path}: {e}")
        return {"destroyed": False, "error": str(e)}


# ---------------------------------------------------------------------------
# Background cleanup
# ---------------------------------------------------------------------------


def _cleanup_expired():
    """Remove expired workspace sessions and their directories."""
    now = time.time()
    with _lock:
        expired = [
            path for path, s in _sessions.items()
            if now - s.last_accessed > s.ttl_seconds
        ]
        for path in expired:
            _sessions.pop(path, None)

    for path in expired:
        try:
            shutil.rmtree(path, ignore_errors=True)
            logger.info(f"Expired workspace cleaned up: {path}")
        except Exception as e:
            logger.warning(f"Error cleaning up {path}: {e}")


def _cleanup_loop():
    """Background thread that periodically cleans up expired sessions."""
    while True:
        time.sleep(60)
        try:
            _cleanup_expired()
        except Exception as e:
            logger.warning(f"Workspace cleanup error: {e}")


def _ensure_cleanup_started():
    """Start the background cleanup thread if not already running."""
    global _cleanup_thread_started
    if _cleanup_thread_started:
        return
    _cleanup_thread_started = True
    os.makedirs(BASE_DIR, exist_ok=True)
    t = threading.Thread(target=_cleanup_loop, daemon=True, name="workspace-cleanup")
    t.start()
    logger.info(f"Workspace cleanup thread started (base: {BASE_DIR})")
