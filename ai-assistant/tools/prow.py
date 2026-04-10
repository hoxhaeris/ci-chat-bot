"""Prow job analysis tools.

Provides functions that accept Prow job URLs (from prow.ci.openshift.org
or gcsweb-ci) and return structured data about the job's execution,
pod lifecycle, and build output.

Self-contained, stdlib-only.
"""

import gzip
import json
import logging
import re
import urllib.request
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import Any
from urllib.parse import urlparse

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

PROW_HOST = "prow.ci.openshift.org"
GCSWEB_HOST = "gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com"
_ALLOWED_HOSTS = frozenset({PROW_HOST, GCSWEB_HOST})

MAX_BUILD_LOG_BYTES = 10 * 1024 * 1024  # 10 MB
MAX_JSON_ARTIFACT_BYTES = 5 * 1024 * 1024  # 5 MB
REQUEST_TIMEOUT = 30
MAX_MATCHES_CAP = 50
MAX_LINES_CAP = 500

_JOB_TYPE_PREFIXES = {
    "pull": "presubmit",
    "branch": "postsubmit",
    "periodic": "periodic",
}
_JOBTYPE_TO_CONFIG_SUFFIX = {
    "presubmit": "presubmits",
    "postsubmit": "postsubmits",
    "periodic": "periodics",
}

# ---------------------------------------------------------------------------
# GCS / URL parsing
# ---------------------------------------------------------------------------


@dataclass
class ProwArtifactLocation:
    """Parsed representation of a Prow job's GCS artifact location."""

    job_name: str
    build_id: str
    bucket: str
    gcs_path: str
    job_type: str = ""
    org: str = ""
    repo: str = ""
    pr_number: str = ""
    extra: dict = field(default_factory=dict)

    @property
    def artifacts_base_url(self) -> str:
        return f"https://{GCSWEB_HOST}/gcs/{self.bucket}/{self.gcs_path}"

    @property
    def prow_ui_url(self) -> str:
        return f"https://{PROW_HOST}/view/gs/{self.bucket}/{self.gcs_path}"

    def artifact_url(self, filename: str) -> str:
        return f"{self.artifacts_base_url}/{filename}"


def _validate_path(path: str) -> None:
    if ".." in path.split("/"):
        raise ValueError("Path traversal detected in URL")


def parse_prow_url(url_str: str) -> ProwArtifactLocation:
    """Parse a Prow view URL or GCSWeb URL into structured components."""
    url_str = url_str.strip().rstrip("/")
    parsed = urlparse(url_str)

    if parsed.hostname not in _ALLOWED_HOSTS:
        raise ValueError(
            f"Unsupported host '{parsed.hostname}'. "
            f"Only {PROW_HOST} and {GCSWEB_HOST} URLs are accepted."
        )

    if parsed.hostname == PROW_HOST:
        segments = [s for s in parsed.path.split("/") if s]
        if len(segments) < 4 or segments[0] != "view" or segments[1] != "gs":
            raise ValueError(
                "Prow URL must match https://prow.ci.openshift.org/view/gs/{bucket}/{path}"
            )
        bucket = segments[2]
        path_segments = segments[3:]
    elif parsed.hostname == GCSWEB_HOST:
        segments = [s for s in parsed.path.split("/") if s]
        if len(segments) < 3 or segments[0] != "gcs":
            raise ValueError(
                "GCSWeb URL must match https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com"
                "/gcs/{bucket}/{path}"
            )
        bucket = segments[1]
        path_segments = segments[2:]
        if path_segments and "." in path_segments[-1]:
            path_segments = path_segments[:-1]
    else:
        raise ValueError(f"Unsupported host: {parsed.hostname}")

    if len(path_segments) < 2:
        raise ValueError("URL path too short to contain a job name and build ID")

    _validate_path("/".join(path_segments))

    build_id = path_segments[-1]
    job_name = path_segments[-2]

    if not re.fullmatch(r"\d+", build_id):
        raise ValueError(f"Build ID '{build_id}' is not numeric")

    gcs_path = "/".join(path_segments)

    job_type = ""
    org = ""
    repo = ""
    pr_number = ""

    if path_segments[0] == "pr-logs" and len(path_segments) >= 2 and path_segments[1] == "pull":
        if len(path_segments) >= 3 and path_segments[2] == "batch":
            job_type = "batch"
        elif len(path_segments) >= 5:
            job_type = "presubmit"
            org_repo = path_segments[2]
            if "_" in org_repo:
                org, repo = org_repo.split("_", 1)
            else:
                org = "openshift"
                repo = org_repo
            pr_number = path_segments[3]
    elif path_segments[0] == "logs":
        for prefix, jtype in _JOB_TYPE_PREFIXES.items():
            if job_name.startswith(f"{prefix}-ci-"):
                job_type = jtype
                break
        if not job_type:
            job_type = "periodic_or_postsubmit"

    return ProwArtifactLocation(
        job_name=job_name,
        build_id=build_id,
        bucket=bucket,
        gcs_path=gcs_path,
        job_type=job_type,
        org=org,
        repo=repo,
        pr_number=pr_number,
    )


def _fetch_url(url: str, max_bytes: int, *, accept_encoding: str = "") -> bytes:
    """Fetch a URL with size and host safety checks."""
    parsed = urlparse(url)
    if parsed.hostname not in _ALLOWED_HOSTS:
        raise ValueError(f"Refusing to fetch from disallowed host: {parsed.hostname}")
    _validate_path(parsed.path)

    headers = {"User-Agent": "cluster-bot-ai/1.0"}
    if accept_encoding:
        headers["Accept-Encoding"] = accept_encoding
    req = urllib.request.Request(url, headers=headers)

    try:
        with urllib.request.urlopen(req, timeout=REQUEST_TIMEOUT) as resp:
            content_length = resp.headers.get("Content-Length")
            if content_length and int(content_length) > max_bytes:
                raise ValueError(
                    f"Artifact too large ({int(content_length)} bytes, limit {max_bytes})"
                )

            chunks = []
            total = 0
            while True:
                chunk = resp.read(64 * 1024)
                if not chunk:
                    break
                total += len(chunk)
                if total > max_bytes:
                    raise ValueError(f"Artifact exceeded {max_bytes} byte limit during download")
                chunks.append(chunk)
            return b"".join(chunks)
    except urllib.error.HTTPError as exc:
        raise ValueError(f"HTTP {exc.code} fetching {url}: {exc.reason}") from exc
    except urllib.error.URLError as exc:
        raise ValueError(f"Network error fetching {url}: {exc.reason}") from exc


def fetch_artifact_json(location: ProwArtifactLocation, filename: str) -> dict:
    """Download a JSON artifact and return parsed dict."""
    allowed_json = {"prowjob.json", "started.json", "finished.json", "podinfo.json"}
    if filename not in allowed_json:
        raise ValueError(f"Artifact '{filename}' is not in the allowlist: {allowed_json}")

    url = location.artifact_url(filename)
    raw = _fetch_url(url, MAX_JSON_ARTIFACT_BYTES)

    if raw[:2] == b"\x1f\x8b":
        raw = gzip.decompress(raw)

    return json.loads(raw.decode("utf-8", errors="replace"))


def fetch_build_log(location: ProwArtifactLocation) -> list[str]:
    """Download build-log.txt and return as a list of lines."""
    url = location.artifact_url("build-log.txt")
    raw = _fetch_url(url, MAX_BUILD_LOG_BYTES, accept_encoding="gzip")

    if raw[:2] == b"\x1f\x8b":
        raw = gzip.decompress(raw)

    text = raw.decode("utf-8", errors="replace")
    return text.splitlines()


def build_config_links(prowjob: dict) -> dict:
    """Construct links to the job's configuration in openshift/release."""
    spec = prowjob.get("spec", {})
    refs = spec.get("refs") or {}
    org = refs.get("org", "")
    repo = refs.get("repo", "")
    base_ref = refs.get("base_ref", "")
    job_type = spec.get("type", "")

    result: dict[str, str | None] = {
        "ci_operator_config_url": None,
        "prow_job_config_url": None,
    }

    if not (org and repo and base_ref):
        return result

    labels = prowjob.get("metadata", {}).get("labels", {})
    variant = labels.get("ci-operator.openshift.io/variant", "")

    basename = f"{org}-{repo}-{base_ref}"
    if variant:
        basename = f"{basename}__{variant}"

    base_url = "https://github.com/openshift/release/tree/main"

    result["ci_operator_config_url"] = f"{base_url}/ci-operator/config/{org}/{repo}/{basename}.yaml"

    config_suffix = _JOBTYPE_TO_CONFIG_SUFFIX.get(job_type)
    if config_suffix:
        job_basename = f"{org}-{repo}-{base_ref}-{config_suffix}"
        result["prow_job_config_url"] = (
            f"{base_url}/ci-operator/jobs/{org}/{repo}/{job_basename}.yaml"
        )

    return result


def compile_search_pattern(pattern: str) -> re.Pattern:
    """Compile a regex pattern with safety checks."""
    try:
        return re.compile(pattern, re.IGNORECASE)
    except re.error as exc:
        raise ValueError(f"Invalid regex pattern '{pattern}': {exc}") from exc


# ---------------------------------------------------------------------------
# Tool functions (ADK wraps these automatically via docstrings)
# ---------------------------------------------------------------------------


def _iso_duration(start_iso: str | None, end_iso: str | None) -> str | None:
    """Compute a human-readable duration between two ISO timestamps."""
    if not start_iso or not end_iso:
        return None
    try:
        fmt = "%Y-%m-%dT%H:%M:%SZ"
        start = datetime.strptime(start_iso, fmt).replace(tzinfo=UTC)
        end = datetime.strptime(end_iso, fmt).replace(tzinfo=UTC)
        delta = end - start
        total_seconds = int(delta.total_seconds())
        if total_seconds < 0:
            return None
        minutes, seconds = divmod(total_seconds, 60)
        hours, minutes = divmod(minutes, 60)
        if hours:
            return f"{hours}h{minutes}m{seconds}s"
        if minutes:
            return f"{minutes}m{seconds}s"
        return f"{seconds}s"
    except (ValueError, TypeError):
        return None


def get_prow_url_info(prow_url: str) -> dict[str, Any]:
    """Parse a Prow job result URL into its structured components.

    Accepts two URL formats:
      - Prow view: https://prow.ci.openshift.org/view/gs/{bucket}/{path}
      - GCSWeb:    https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/{bucket}/{path}

    This tool performs NO network requests — it only parses the URL to extract
    the job name, build ID, GCS bucket, job type, and (for presubmit jobs) the
    org, repo, and PR number.

    Args:
        prow_url: A Prow job result URL or GCSWeb artifact URL.

    Returns:
        dict with keys: job_name, build_id, bucket, gcs_path, job_type,
        org, repo, pr_number, artifacts_base_url, prow_ui_url.
        On error, returns a dict with an 'error' key.
    """
    try:
        loc = parse_prow_url(prow_url)
    except ValueError as exc:
        return {"error": str(exc)}

    return {
        "job_name": loc.job_name,
        "build_id": loc.build_id,
        "bucket": loc.bucket,
        "gcs_path": loc.gcs_path,
        "job_type": loc.job_type or "unknown",
        "org": loc.org or None,
        "repo": loc.repo or None,
        "pr_number": loc.pr_number or None,
        "artifacts_base_url": loc.artifacts_base_url,
        "prow_ui_url": loc.prow_ui_url,
    }


def get_prowjob_status(prow_url: str) -> dict[str, Any]:
    """Get the execution status, timing, and metadata for a Prow CI job.

    Downloads prowjob.json from the job's GCS artifacts. This file contains:
      - Job type (presubmit, postsubmit, periodic)
      - Execution state (success, failure, pending, aborted, error)
      - Timing (start, pending, completion, and computed duration)
      - The Git refs that triggered the job (org, repo, branch, PR info)
      - Links to the ci-operator config and Prow job config in openshift/release

    Args:
        prow_url: A Prow job result URL or GCSWeb artifact URL.

    Returns:
        dict with job metadata, status, timing, refs, and config links.
        On error, returns a dict with an 'error' key.
    """
    try:
        loc = parse_prow_url(prow_url)
    except ValueError as exc:
        return {"error": str(exc)}

    try:
        prowjob = fetch_artifact_json(loc, "prowjob.json")
    except ValueError as exc:
        return {"error": f"Failed to fetch prowjob.json: {exc}"}

    spec = prowjob.get("spec", {})
    status = prowjob.get("status", {})
    refs = spec.get("refs") or {}
    pulls = refs.get("pulls") or []

    start_time = status.get("startTime")
    pending_time = status.get("pendingTime")
    completion_time = status.get("completionTime")

    pr_info = None
    if pulls:
        pr = pulls[0]
        pr_info = {
            "number": pr.get("number"),
            "title": pr.get("title"),
            "author": pr.get("author"),
            "sha": pr.get("sha"),
            "link": pr.get("link"),
        }

    config_links = build_config_links(prowjob)

    return {
        "job_name": spec.get("job", loc.job_name),
        "job_type": spec.get("type", loc.job_type),
        "state": status.get("state", "unknown"),
        "description": status.get("description", ""),
        "cluster": spec.get("cluster", ""),
        "context": spec.get("context", ""),
        "rerun_command": spec.get("rerun_command", ""),
        "timing": {
            "start_time": start_time,
            "pending_time": pending_time,
            "completion_time": completion_time,
            "duration": _iso_duration(start_time, completion_time),
            "time_in_pending": _iso_duration(start_time, pending_time),
        },
        "refs": {
            "org": refs.get("org", ""),
            "repo": refs.get("repo", ""),
            "base_ref": refs.get("base_ref", ""),
            "base_sha": refs.get("base_sha", ""),
        },
        "pull_request": pr_info,
        "config_links": config_links,
        "prow_ui_url": loc.prow_ui_url,
    }


def search_build_log(
    prow_url: str,
    pattern: str,
    context_lines: int = 3,
    max_matches: int = 20,
) -> dict[str, Any]:
    """Search a Prow job's build log for lines matching a regex pattern.

    Downloads build-log.txt (up to 10 MB) from the job's GCS artifacts and
    searches for the given regex pattern. Returns matching lines with
    surrounding context, similar to grep -C. The search is case-insensitive.

    Common patterns to search for:
      - "error" or "FAIL" to find failures
      - "panic:" for Go panics
      - "level=fatal" or "level=error" for structured log errors

    Args:
        prow_url: A Prow job result URL or GCSWeb artifact URL.
        pattern: A regex pattern to search for (case-insensitive).
        context_lines: Number of lines to show before and after each match
                       (default 3, max 10).
        max_matches: Maximum number of matches to return (default 20, max 50).

    Returns:
        dict with matches, total_lines, total_matches, and whether results
        were truncated. On error, returns a dict with an 'error' key.
    """
    try:
        loc = parse_prow_url(prow_url)
    except ValueError as exc:
        return {"error": str(exc)}

    try:
        compiled = compile_search_pattern(pattern)
    except ValueError as exc:
        return {"error": str(exc)}

    context_lines = max(0, min(context_lines, 10))
    max_matches = max(1, min(max_matches, MAX_MATCHES_CAP))

    try:
        lines = fetch_build_log(loc)
    except ValueError as exc:
        return {"error": f"Failed to fetch build-log.txt: {exc}"}

    matches = []
    for i, line in enumerate(lines):
        if compiled.search(line):
            start = max(0, i - context_lines)
            end = min(len(lines), i + context_lines + 1)
            matches.append(
                {
                    "line_number": i + 1,
                    "line": line,
                    "before_context": lines[start:i],
                    "after_context": lines[i + 1 : end],
                }
            )
            if len(matches) >= max_matches:
                break

    total_matching = (
        sum(1 for line in lines if compiled.search(line)) if len(matches) < max_matches else None
    )

    return {
        "total_lines": len(lines),
        "total_matches": total_matching
        if total_matching is not None
        else f"at least {len(matches)}",
        "truncated": len(matches) >= max_matches and total_matching is None,
        "matches": matches,
    }


def get_build_log_excerpt(
    prow_url: str,
    start_line: int = -1,
    num_lines: int = 100,
) -> dict[str, Any]:
    """Get a range of lines from a Prow job's build log.

    Downloads build-log.txt (up to 10 MB) and returns the requested slice.
    Use start_line=-1 (default) to get the tail of the log — the end usually
    contains the final error or test summary.

    Args:
        prow_url: A Prow job result URL or GCSWeb artifact URL.
        start_line: 1-based line number to start from. Use -1 for tail mode.
        num_lines: Number of lines to return (default 100, max 500).

    Returns:
        dict with 'lines', 'start_line', 'end_line', 'total_lines',
        and 'truncated'. On error, returns a dict with an 'error' key.
    """
    try:
        loc = parse_prow_url(prow_url)
    except ValueError as exc:
        return {"error": str(exc)}

    num_lines = max(1, min(num_lines, MAX_LINES_CAP))

    try:
        all_lines = fetch_build_log(loc)
    except ValueError as exc:
        return {"error": f"Failed to fetch build-log.txt: {exc}"}

    total = len(all_lines)

    if start_line == -1:
        actual_start = max(0, total - num_lines)
    else:
        actual_start = max(0, start_line - 1)

    actual_end = min(total, actual_start + num_lines)
    excerpt = all_lines[actual_start:actual_end]

    return {
        "total_lines": total,
        "start_line": actual_start + 1,
        "end_line": actual_end,
        "num_lines_returned": len(excerpt),
        "truncated": (actual_end < total) if start_line != -1 else False,
        "lines": "\n".join(excerpt),
    }
