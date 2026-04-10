"""Release controller tools for querying OpenShift release payload status.

Provides functions that query the release controller APIs to report on
release streams, tag status, verification job results, and rejection reasons
across five architectures (amd64, arm64, s390x, ppc64le, multi).

Self-contained, stdlib-only.
"""

import gzip
import json
import logging
import re
import urllib.request
from typing import Any
from urllib.parse import quote, urlparse

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# API client
# ---------------------------------------------------------------------------

ARCHITECTURE_HOSTS: dict[str, str] = {
    "amd64": "amd64.ocp.releases.ci.openshift.org",
    "arm64": "arm64.ocp.releases.ci.openshift.org",
    "s390x": "s390x.ocp.releases.ci.openshift.org",
    "ppc64le": "ppc64le.ocp.releases.ci.openshift.org",
    "multi": "multi.ocp.releases.ci.openshift.org",
}

_ALLOWED_HOSTS = frozenset(ARCHITECTURE_HOSTS.values())
_SAFE_NAME_PATTERN = re.compile(r"^[a-zA-Z0-9._-]+$")

MAX_RESPONSE_BYTES = 5 * 1024 * 1024  # 5 MB
REQUEST_TIMEOUT = 30

_PAYLOAD_ANALYSIS_JOB_MARKER = "claude-payload-agent"
_PROW_GCS_VIEW_PREFIX = "https://prow.ci.openshift.org/view/gs/"
_GCSWEB_BASE = "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs"
_PAYLOAD_ANALYSIS_ARTIFACT_SUBPATH = (
    "artifacts/claude-payload-agent/openshift-claude-payload-agent/artifacts"
)


def validate_architecture(architecture: str) -> str:
    """Validate and normalize an architecture name. Returns the host."""
    arch = architecture.strip().lower()
    if arch not in ARCHITECTURE_HOSTS:
        raise ValueError(
            f"Unknown architecture '{architecture}'. "
            f"Valid values: {', '.join(sorted(ARCHITECTURE_HOSTS))}"
        )
    return ARCHITECTURE_HOSTS[arch]


def validate_name(value: str, label: str) -> str:
    """Validate a stream or tag name against a safe pattern."""
    value = value.strip()
    if not value:
        raise ValueError(f"{label} must not be empty")
    if not _SAFE_NAME_PATTERN.match(value):
        raise ValueError(
            f"{label} '{value}' contains invalid characters. "
            "Only alphanumeric, dots, dashes, and underscores are allowed."
        )
    return value


def _fetch_api(url: str) -> dict | list:
    """Fetch a JSON API response with safety checks."""
    parsed = urlparse(url)
    if parsed.hostname not in _ALLOWED_HOSTS:
        raise ValueError(f"Refusing to fetch from disallowed host: {parsed.hostname}")

    headers = {
        "User-Agent": "cluster-bot-ai/1.0",
        "Accept": "application/json",
    }
    req = urllib.request.Request(url, headers=headers)

    try:
        with urllib.request.urlopen(req, timeout=REQUEST_TIMEOUT) as resp:
            content_length = resp.headers.get("Content-Length")
            if content_length and int(content_length) > MAX_RESPONSE_BYTES:
                raise ValueError(
                    f"Response too large ({int(content_length)} bytes, limit {MAX_RESPONSE_BYTES})"
                )

            chunks = []
            total = 0
            while True:
                chunk = resp.read(64 * 1024)
                if not chunk:
                    break
                total += len(chunk)
                if total > MAX_RESPONSE_BYTES:
                    raise ValueError(
                        f"Response exceeded {MAX_RESPONSE_BYTES} byte limit during download"
                    )
                chunks.append(chunk)

            raw = b"".join(chunks)
            if raw[:2] == b"\x1f\x8b":
                raw = gzip.decompress(raw)

            return json.loads(raw.decode("utf-8", errors="replace"))
    except urllib.error.HTTPError as exc:
        raise ValueError(f"HTTP {exc.code} fetching {url}: {exc.reason}") from exc
    except urllib.error.URLError as exc:
        raise ValueError(f"Network error fetching {url}: {exc.reason}") from exc


def fetch_release_api(architecture: str, path: str) -> dict | list:
    """Fetch from a release controller API endpoint."""
    host = validate_architecture(architecture)
    url = f"https://{host}{path}"
    return _fetch_api(url)


def build_release_stream_url(architecture: str, stream: str) -> str:
    """Build the human-readable URL for a release stream page."""
    host = validate_architecture(architecture)
    stream = validate_name(stream, "stream")
    return f"https://{host}/releasestream/{quote(stream, safe='')}"


def build_release_tag_url(architecture: str, stream: str, tag: str) -> str:
    """Build the human-readable URL for a release tag detail page."""
    host = validate_architecture(architecture)
    stream = validate_name(stream, "stream")
    tag = validate_name(tag, "tag")
    return f"https://{host}/releasestream/{quote(stream, safe='')}/release/{quote(tag, safe='')}"


# ---------------------------------------------------------------------------
# Tool functions
# ---------------------------------------------------------------------------


def get_release_streams(architecture: str = "amd64") -> dict[str, Any]:
    """List all OpenShift release streams from the release controller.

    The release controller tracks release payloads in "streams" — each
    stream represents a channel of builds for a specific OCP version.
    Common stream names: 4.17.0-0.nightly, 4-stable, 4-dev-preview.

    There are five release controllers, one per architecture:
    amd64, arm64, s390x, ppc64le, and multi.

    Args:
        architecture: The CPU architecture to query. One of: amd64
                      (default), arm64, s390x, ppc64le, multi.

    Returns:
        dict with 'streams' (list of stream summaries) and 'architecture'.
        On error, returns a dict with an 'error' key.
    """
    try:
        data = fetch_release_api(architecture, "/api/v1/releasestreams/all")
    except ValueError as exc:
        return {"error": str(exc)}

    if not isinstance(data, dict):
        return {"error": "Unexpected response format from release controller"}

    streams = []
    for name, tags in sorted(data.items()):
        tag_list = tags if isinstance(tags, list) else []
        latest_tag = tag_list[0] if tag_list else None
        streams.append(
            {
                "name": name,
                "tag_count": len(tag_list),
                "latest_tag": latest_tag if isinstance(latest_tag, str) else None,
                "url": build_release_stream_url(architecture, name) if name else None,
            }
        )

    return {
        "architecture": architecture,
        "stream_count": len(streams),
        "streams": streams,
    }


def get_release_stream_tags(
    stream: str,
    architecture: str = "amd64",
    phase: str = "",
) -> dict[str, Any]:
    """List release tags in a stream, optionally filtered by phase.

    Tags have a phase indicating their verification status:
      - Pending, Ready, Accepted, Rejected, Failed

    Args:
        stream: The release stream name (e.g. "4.17.0-0.nightly", "4-stable").
        architecture: CPU architecture (default: amd64).
        phase: Optional phase filter (Accepted, Rejected, Ready, Pending, Failed).

    Returns:
        dict with 'tags' (list of tag info) and metadata.
        On error, returns a dict with an 'error' key.
    """
    try:
        stream = validate_name(stream, "stream")
    except ValueError as exc:
        return {"error": str(exc)}

    path = f"/api/v1/releasestream/{quote(stream, safe='')}/tags"
    if phase:
        valid_phases = {"Accepted", "Rejected", "Ready", "Pending", "Failed"}
        if phase not in valid_phases:
            return {
                "error": f"Invalid phase '{phase}'. Valid values: {', '.join(sorted(valid_phases))}"
            }
        path += f"?phase={quote(phase, safe='')}"

    try:
        data = fetch_release_api(architecture, path)
    except ValueError as exc:
        return {"error": str(exc)}

    if not isinstance(data, dict):
        return {"error": "Unexpected response format from release controller"}

    raw_tags = data.get("tags") or []
    tags = []
    for t in raw_tags:
        if not isinstance(t, dict):
            continue
        tag_name = t.get("name", "")
        tags.append(
            {
                "name": tag_name,
                "phase": t.get("phase", ""),
                "pull_spec": t.get("pullSpec", ""),
                "download_url": t.get("downloadURL", ""),
                "url": build_release_tag_url(architecture, stream, tag_name) if tag_name else None,
            }
        )

    return {
        "architecture": architecture,
        "stream": stream,
        "phase_filter": phase or "(all)",
        "tag_count": len(tags),
        "tags": tags,
    }


def get_release_info(
    stream: str,
    tag: str,
    architecture: str = "amd64",
) -> dict[str, Any]:
    """Get detailed information about a specific release tag.

    Downloads the full release info including verification job results
    and upgrade test history. This is the primary tool for understanding
    why a release was accepted or rejected.

    The response includes verification jobs (blocking, informing, async,
    pending), upgrade test history, and AI payload analysis links when
    available.

    Args:
        stream: The release stream name (e.g. "4.17.0-0.nightly").
        tag: The release tag name (e.g. "4.17.0-0.nightly-2026-03-17-052833").
        architecture: CPU architecture (default: amd64).

    Returns:
        dict with release metadata, verification results, upgrade history.
        On error, returns a dict with an 'error' key.
    """
    try:
        stream = validate_name(stream, "stream")
        tag = validate_name(tag, "tag")
    except ValueError as exc:
        return {"error": str(exc)}

    path = f"/api/v1/releasestream/{quote(stream, safe='')}/release/{quote(tag, safe='')}"

    try:
        data = fetch_release_api(architecture, path)
    except ValueError as exc:
        return {"error": str(exc)}

    if not isinstance(data, dict):
        return {"error": "Unexpected response format from release controller"}

    results = data.get("results")
    verification = None
    if isinstance(results, dict):
        verification = {
            "blocking_jobs": _format_verification_map(results.get("blockingJobs")),
            "informing_jobs": _format_verification_map(results.get("informingJobs")),
            "async_jobs": _format_verification_map(results.get("asyncJobs")),
            "pending_jobs": _format_verification_map(results.get("pendingJobs")),
        }

    upgrades_to = _format_upgrade_history(data.get("upgradesTo"))
    upgrades_from = _format_upgrade_history(data.get("upgradesFrom"))

    result: dict[str, Any] = {
        "architecture": architecture,
        "stream": stream,
        "name": data.get("name", tag),
        "phase": data.get("phase", "unknown"),
        "verification": verification,
        "upgrades_to": upgrades_to,
        "upgrades_from": upgrades_from,
        "release_page_url": build_release_tag_url(architecture, stream, tag),
    }

    ai_analysis = _find_ai_payload_analysis(verification, tag)
    if ai_analysis:
        result["ai_payload_analysis"] = ai_analysis

    return result


def get_rejected_releases(architecture: str = "amd64") -> dict[str, Any]:
    """List all currently rejected releases across all streams.

    Useful for quickly finding which recent release payloads failed
    verification. Use get_release_info() to investigate specific failures.

    Args:
        architecture: CPU architecture (default: amd64).

    Returns:
        dict with 'rejected' (list of rejected tag info) and metadata.
        On error, returns a dict with an 'error' key.
    """
    try:
        data = fetch_release_api(architecture, "/api/v1/releasestreams/rejected")
    except ValueError as exc:
        return {"error": str(exc)}

    if not isinstance(data, list):
        return {"error": "Unexpected response format from release controller"}

    rejected = []
    for entry in data:
        if not isinstance(entry, dict):
            continue
        stream_name = entry.get("name", "")
        tags = entry.get("tags") or []
        for t in tags:
            if not isinstance(t, dict):
                continue
            tag_name = t.get("name", "")
            rejected.append(
                {
                    "stream": stream_name,
                    "tag": tag_name,
                    "phase": t.get("phase", "Rejected"),
                    "url": build_release_tag_url(architecture, stream_name, tag_name)
                    if stream_name and tag_name
                    else None,
                }
            )

    return {
        "architecture": architecture,
        "rejected_count": len(rejected),
        "rejected": rejected,
    }


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _find_ai_payload_analysis(
    verification: dict[str, Any] | None, tag: str
) -> dict[str, Any] | None:
    """Check verification results for an AI payload analysis job."""
    if not verification:
        return None

    for category in ("blocking_jobs", "informing_jobs", "async_jobs", "pending_jobs"):
        jobs = verification.get(category)
        if not jobs:
            continue
        for job in jobs:
            url = job.get("url", "")
            if _PAYLOAD_ANALYSIS_JOB_MARKER not in url:
                continue

            state = job.get("state", "")
            result: dict[str, Any] = {
                "prow_job_url": url,
                "state": state,
            }

            if state == "Pending":
                result["note"] = (
                    "AI payload analysis is still running. "
                    "Artifacts will be available once the job completes."
                )
                return result

            if not url.startswith(_PROW_GCS_VIEW_PREFIX):
                return result

            gcs_path = url[len(_PROW_GCS_VIEW_PREFIX) :].rstrip("/")
            artifact_base = f"{_GCSWEB_BASE}/{gcs_path}/{_PAYLOAD_ANALYSIS_ARTIFACT_SUBPATH}"

            result["summary_html_url"] = f"{artifact_base}/payload-analysis-{tag}-summary.html"
            result["results_yaml_url"] = f"{artifact_base}/payload-results-{tag}.yaml"
            result["description"] = (
                "An AI agent has already analyzed this payload's verification "
                "results. The HTML summary includes root cause analysis of each "
                "failed job, candidate PRs for revert, and comparison with the "
                "previous payload."
            )
            return result

    return None


def _format_verification_map(jobs: dict | None) -> list[dict] | None:
    """Format a VerificationStatusMap into a list of job results."""
    if not jobs or not isinstance(jobs, dict):
        return None

    result = []
    for name, status in sorted(jobs.items()):
        if not isinstance(status, dict):
            continue
        result.append(
            {
                "name": name,
                "state": status.get("state", ""),
                "url": status.get("url", ""),
                "retries": status.get("retries", 0),
                "transition_time": status.get("transitionTime"),
            }
        )
    return result


def _format_upgrade_history(upgrades: list | None) -> list[dict] | None:
    """Format upgrade history entries."""
    if not upgrades or not isinstance(upgrades, list):
        return None

    result = []
    for entry in upgrades:
        if not isinstance(entry, dict):
            continue
        result.append(
            {
                "from": entry.get("From", entry.get("from", "")),
                "to": entry.get("To", entry.get("to", "")),
                "success": entry.get("Success", entry.get("success", 0)),
                "failure": entry.get("Failure", entry.get("failure", 0)),
                "total": entry.get("Total", entry.get("total", 0)),
            }
        )
    return result
