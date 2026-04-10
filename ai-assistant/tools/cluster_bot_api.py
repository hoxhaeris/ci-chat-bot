"""Cluster bot API tools.

Calls the Go ci-chat-bot HTTP API to validate job configurations,
check MCE/ROSA/Hypershift versions, and query infrastructure status.
All tools communicate with the Go bot via localhost.
"""

import json
import logging
import os
import re
import urllib.request
from typing import Any

logger = logging.getLogger(__name__)

_CI_CHAT_BOT_URL = os.environ.get("CI_CHAT_BOT_URL", "http://localhost:8080")

# Known options used by check_command to categorise comma-separated tokens.
KNOWN_PLATFORMS = {
    "aws", "gcp", "azure", "vsphere", "metal", "ovirt", "openstack",
    "hypershift-hosted", "nutanix", "alibaba", "hypershift-hosted-powervs",
    "azure-stackhub",
}
KNOWN_ARCHITECTURES = {"amd64", "arm64", "multi"}


def _api_get(path: str, params: dict | None = None) -> dict | list:
    """Make a GET request to the Go bot API and return parsed JSON."""
    url = f"{_CI_CHAT_BOT_URL}{path}"
    if params:
        query = "&".join(f"{k}={urllib.parse.quote(str(v))}" for k, v in params.items() if v)
        url = f"{url}?{query}"

    req = urllib.request.Request(url, headers={"User-Agent": "cluster-bot-ai/1.0"})
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        raise ValueError(f"HTTP {exc.code} from {path}: {exc.reason}") from exc
    except urllib.error.URLError as exc:
        raise ValueError(f"Network error calling {path}: {exc.reason}") from exc


def _parse_go_duration(s: str) -> float | None:
    """Parse a Go-style duration string (e.g. '4h', '2h30m') into hours."""
    pattern = re.compile(r'^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$')
    m = pattern.match(s.strip())
    if not m:
        return None
    hours = int(m.group(1) or 0)
    minutes = int(m.group(2) or 0)
    seconds = int(m.group(3) or 0)
    if hours == 0 and minutes == 0 and seconds == 0:
        return None
    return hours + minutes / 60 + seconds / 3600


# Need urllib.parse for query string building
import urllib.parse


# ---------------------------------------------------------------------------
# Job validation tools
# ---------------------------------------------------------------------------


def validate_job(
    platform: str,
    version: str,
    params: str = "",
    architecture: str = "amd64",
    job_type: str = "launch",
) -> dict[str, Any]:
    """Validate whether a platform/version/params combination has a backing prow job.

    Call this BEFORE recommending any launch, test, or upgrade command to
    verify that the combination actually exists in the prow configuration.

    Args:
        platform: Target platform, e.g. 'aws', 'gcp', 'azure'.
        version: OpenShift version, e.g. '4.19'.
        params: Comma-separated optional parameters, e.g. 'fips,compact'.
        architecture: CPU architecture (default 'amd64'). Options: amd64, arm64, multi.
        job_type: Command type (default 'launch'). Options: launch, test, upgrade.

    Returns:
        dict with 'valid' boolean and details.
    """
    query = {
        "platform": platform,
        "version": version,
        "arch": architecture,
        "type": job_type,
    }
    if params:
        query["params"] = params

    try:
        data = _api_get("/api/v1/jobs/validate", params=query)
    except ValueError as e:
        return {"valid": False, "error": f"Error contacting validation API: {e}"}

    if data.get("valid"):
        return {
            "valid": True,
            "message": (
                f"The combination platform={platform} version={version} "
                f"params={params or '(none)'} arch={architecture} type={job_type} "
                f"has a backing prow job."
            ),
        }
    return {
        "valid": False,
        "error": data.get("error", "unknown error"),
        "message": "This combination does NOT have a backing prow job.",
    }


def list_supported_options() -> dict[str, Any]:
    """List all supported platforms, parameters, architectures, and test suites.

    Use this when you need to enumerate what options are available for
    ci-chat-bot commands. ALWAYS call this instead of guessing or listing
    options from memory.

    Returns:
        dict with platforms, parameters, architectures, tests, workflows.
    """
    try:
        data = _api_get("/api/v1/jobs/supported")
    except ValueError as e:
        return {"error": f"Error contacting supported-options API: {e}"}

    return {
        "platforms": data.get("platforms", []),
        "parameters": data.get("parameters", []),
        "architectures": data.get("architectures", []),
        "tests": data.get("tests", []),
        "upgrade_tests": data.get("upgrade_tests", []),
        "workflows": data.get("workflows", []),
    }


def validate_workflow(workflow_name: str) -> dict[str, Any]:
    """Validate whether a workflow name exists in the workflows configuration.

    Call this BEFORE recommending any workflow-launch, workflow-test, or
    workflow-upgrade command to verify the workflow name is valid.

    Args:
        workflow_name: The workflow name, e.g. 'openshift-e2e-gcp'.

    Returns:
        dict with 'valid' boolean, platform/architecture if valid,
        or available_workflows if invalid.
    """
    try:
        data = _api_get("/api/v1/workflows/validate", params={"name": workflow_name})
    except ValueError as e:
        return {"valid": False, "error": f"Error contacting workflow validation API: {e}"}

    if data.get("valid"):
        return {
            "valid": True,
            "workflow": workflow_name,
            "platform": data.get("platform", "unknown"),
            "architecture": data.get("architecture", "unknown"),
        }

    return {
        "valid": False,
        "error": data.get("error", "unknown error"),
        "available_workflows": data.get("available_workflows", []),
    }


def check_command(command: str) -> dict[str, Any]:
    """Validate a full ci-chat-bot command string.

    Parses a command like 'launch 4.19 aws,fips' into its components and
    validates the combination against the prow configuration. Also handles
    workflow commands: workflow-launch, workflow-test, workflow-upgrade.

    Args:
        command: A ci-chat-bot command string, e.g. 'launch 4.19 aws,fips',
                 'test e2e 4.18 gcp,compact', or
                 'workflow-launch openshift-e2e-gcp 4.19'.

    Returns:
        dict with parsed command info and validation result.
    """
    parts = command.strip().split()
    if not parts:
        return {"error": "empty command string"}

    cmd = parts[0].lower()

    if cmd in ("workflow-launch", "workflow-test", "workflow-upgrade"):
        remaining = parts[1:]
        if not remaining:
            return {"error": f"{cmd} requires a workflow name as the second token"}
        workflow_name = remaining[0]
        validation = validate_workflow(workflow_name)
        return {
            "command_type": cmd,
            "workflow": workflow_name,
            "args": remaining[1:],
            "validation": validation,
        }

    if cmd not in ("launch", "test", "upgrade"):
        return {
            "error": (
                f"Unrecognized command '{cmd}'. "
                "Expected one of: launch, test, upgrade, "
                "workflow-launch, workflow-test, workflow-upgrade"
            )
        }

    job_type = cmd
    remaining = parts[1:]
    test_suite = ""
    if job_type == "test" and remaining:
        test_suite = remaining[0]
        remaining = remaining[1:]

    version = ""
    options_tokens = []
    for token in remaining:
        if not version and token and (
            token[0].isdigit() or token in ("nightly", "ci", "prerelease")
        ):
            version = token
        else:
            options_tokens.append(token)

    if not version:
        return {"error": "could not identify a version in the command"}

    all_options: list[str] = []
    for token in options_tokens:
        all_options.extend(opt.strip() for opt in token.split(",") if opt.strip())

    platform = ""
    architecture = "amd64"
    params_list: list[str] = []

    for opt in all_options:
        if opt in KNOWN_PLATFORMS:
            if platform:
                return {"error": f"multiple platforms specified ('{platform}' and '{opt}')"}
            platform = opt
        elif opt in KNOWN_ARCHITECTURES:
            architecture = opt
        else:
            params_list.append(opt)

    if not platform:
        platform = "aws"

    params_str = ",".join(params_list)

    validation = validate_job(
        platform=platform,
        version=version,
        params=params_str,
        architecture=architecture,
        job_type=job_type,
    )

    result = {
        "command_type": job_type,
        "version": version,
        "platform": platform,
        "architecture": architecture,
        "params": params_str or None,
        "validation": validation,
    }
    if test_suite:
        result["test_suite"] = test_suite

    return result


# ---------------------------------------------------------------------------
# MCE tools
# ---------------------------------------------------------------------------


def lookup_mce_versions() -> dict[str, Any]:
    """List available OpenShift versions for MCE (Multi-Cluster Engine) clusters.

    MCE is a private, access-controlled feature that creates clusters via
    Hive/OCM. Only users with explicit configuration can use MCE.

    Returns:
        dict with available MCE versions and cluster limits.
    """
    try:
        data = _api_get("/api/v1/mce/versions")
    except ValueError as e:
        return {"error": f"Error contacting MCE versions API: {e}"}

    return {
        "versions": data.get("versions", []),
        "count": data.get("count", 0),
    }


def check_mce_command(command: str) -> dict[str, Any]:
    """Validate an MCE create command.

    Parses a command like 'mce create 4.19 6h aws' and validates:
    - Platform is supported (AWS or GCP only)
    - Duration format is valid and within the maximum
    - Version exists in available MCE imagesets

    Args:
        command: An MCE command string, e.g. 'mce create 4.19.0 6h aws'.

    Returns:
        dict with parsed command and validation issues (if any).
    """
    parts = command.strip().split()
    if not parts:
        return {"error": "empty command string"}

    if parts[0].lower() == "mce":
        parts = parts[1:]

    if not parts or parts[0].lower() != "create":
        return {"error": "expected 'mce create <version> <duration> <platform>'"}

    args = parts[1:]
    if len(args) < 3:
        return {"error": f"'mce create' requires 3 arguments: <version> <duration> <platform>. Got {len(args)}"}

    version, duration_str, platform = args[0], args[1], args[2].lower()
    issues = []

    try:
        info = _api_get("/api/v1/mce/info")
    except ValueError as e:
        return {"error": f"Error contacting MCE info API: {e}"}

    supported_platforms = info.get("platforms", [])
    if platform not in supported_platforms:
        issues.append(f"Platform '{platform}' not supported. MCE supports: {', '.join(supported_platforms)}")

    max_hours = info.get("max_duration_hours", 8)
    parsed_hours = _parse_go_duration(duration_str)
    if parsed_hours is None:
        issues.append(f"Duration '{duration_str}' is not valid. Use '4h', '6h', '2h30m'. Max: {max_hours}h.")
    elif parsed_hours > max_hours:
        issues.append(f"Duration '{duration_str}' ({parsed_hours:.1f}h) exceeds max {max_hours}h.")
    elif parsed_hours <= 0:
        issues.append("Duration must be greater than 0.")

    try:
        ver_data = _api_get("/api/v1/mce/versions")
        available_versions = ver_data.get("versions", [])
    except ValueError:
        available_versions = []

    if available_versions:
        exact_match = version in available_versions
        prefix_matches = [v for v in available_versions if v.startswith(version)]
        if not exact_match and not prefix_matches:
            issues.append(f"Version '{version}' not available. Available: {', '.join(available_versions)}")
        elif not exact_match and prefix_matches:
            issues.append(f"Version '{version}' not exact match. Did you mean: {', '.join(prefix_matches)}?")

    return {
        "version": version,
        "duration": duration_str,
        "platform": platform,
        "valid": len(issues) == 0,
        "issues": issues,
        "max_duration_hours": max_hours,
        "max_total_clusters": info.get("max_total_clusters"),
        "default_max_per_user": info.get("default_max_clusters_per_user"),
    }


# ---------------------------------------------------------------------------
# ROSA tools
# ---------------------------------------------------------------------------


def lookup_rosa_versions() -> dict[str, Any]:
    """Check which OpenShift versions are available for ROSA clusters.

    ROSA (Red Hat OpenShift on AWS) clusters are HCP-based (Hosted Control
    Plane) and are created via the OCM API. Max duration 8h (default 6h).

    Returns:
        dict with available versions, duration limits, and commands.
    """
    try:
        data = _api_get("/api/v1/rosa/info")
    except ValueError as e:
        return {"error": f"Error contacting ROSA info API: {e}"}

    return {
        "supported_versions": data.get("supported_versions", []),
        "max_duration_hours": data.get("max_duration_hours", 8),
        "default_duration_hours": data.get("default_duration_hours", 6),
        "commands": data.get("commands", []),
    }


def check_rosa_command(command: str) -> dict[str, Any]:
    """Validate a ROSA create command.

    Parses a command like 'rosa create 4.18 6h' and validates:
    - Duration format is valid and within the maximum
    - Version exists in available ROSA versions

    ROSA is AWS-only, max 1 cluster per user.

    Args:
        command: A ROSA command string, e.g. 'rosa create 4.18.3 6h'.

    Returns:
        dict with parsed command and validation issues (if any).
    """
    parts = command.strip().split()
    if not parts:
        return {"error": "empty command string"}

    if parts[0].lower() == "rosa":
        parts = parts[1:]

    if not parts or parts[0].lower() != "create":
        return {"error": "expected 'rosa create <version> <duration>'"}

    args = parts[1:]
    if len(args) < 2:
        return {"error": f"'rosa create' requires 2 arguments: <version> <duration>. Got {len(args)}"}

    version, duration_str = args[0], args[1]
    issues = []

    try:
        info = _api_get("/api/v1/rosa/info")
    except ValueError as e:
        return {"error": f"Error contacting ROSA info API: {e}"}

    max_hours = info.get("max_duration_hours", 8)
    parsed_hours = _parse_go_duration(duration_str)
    if parsed_hours is None:
        issues.append(f"Duration '{duration_str}' is not valid. Use '4h', '6h'. Max: {max_hours}h.")
    elif parsed_hours > max_hours:
        issues.append(f"Duration '{duration_str}' ({parsed_hours:.1f}h) exceeds max {max_hours}h.")
    elif parsed_hours <= 0:
        issues.append("Duration must be greater than 0.")

    available_versions = info.get("supported_versions", [])
    if available_versions:
        exact_match = version in available_versions
        prefix_matches = [v for v in available_versions if v.startswith(version)]
        if not exact_match and not prefix_matches:
            issues.append(f"Version '{version}' not available. Available: {', '.join(available_versions)}")
        elif not exact_match and prefix_matches:
            issues.append(f"Version '{version}' not exact match. Did you mean: {', '.join(prefix_matches)}?")

    return {
        "version": version,
        "duration": duration_str,
        "valid": len(issues) == 0,
        "issues": issues,
        "max_duration_hours": max_hours,
        "default_duration_hours": info.get("default_duration_hours", 6),
    }


# ---------------------------------------------------------------------------
# Hypershift tools
# ---------------------------------------------------------------------------


def lookup_hypershift_versions() -> dict[str, Any]:
    """Check which OpenShift versions support hypershift (hypershift-hosted platform).

    Hypershift clusters require the 'multi' architecture and use the
    'hypershift-hosted' or 'hypershift-hosted-powervs' platforms.

    Returns:
        dict with supported versions, platforms, and architecture requirement.
    """
    try:
        data = _api_get("/api/v1/hypershift/info")
    except ValueError as e:
        return {"error": f"Error contacting hypershift info API: {e}"}

    return {
        "supported_versions": data.get("supported_versions", []),
        "platforms": data.get("platforms", []),
        "required_arch": data.get("required_arch", "multi"),
    }


# ---------------------------------------------------------------------------
# Infrastructure tools
# ---------------------------------------------------------------------------


def lookup_quota_status() -> dict[str, Any]:
    """Check cloud provider lease/quota availability per platform (AWS, Azure, GCP).

    Shows how many quota slots are free vs leased for each cloud platform.
    Use this to answer questions about platform availability.

    Returns:
        dict with per-platform quota status: free, leased, total.
    """
    try:
        data = _api_get("/api/v1/quota/status")
    except ValueError as e:
        return {"error": f"Error contacting quota status API: {e}"}

    if not data:
        return {"error": "No quota data available. Lease client may not be configured."}

    platforms = {}
    for platform in sorted(data.keys()):
        info = data[platform]
        free = info.get("free", 0)
        leased = info.get("leased", 0)
        platforms[platform] = {
            "free": free,
            "leased": leased,
            "total": free + leased,
        }

    return {"platforms": platforms}


def lookup_capacity_status() -> dict[str, Any]:
    """Check how many clusters are active vs their limits for Prow, ROSA, and MCE.

    Shows active cluster counts, hard limits, and remaining capacity.

    Returns:
        dict with active/limit/remaining for each cluster type and max jobs per user.
    """
    try:
        data = _api_get("/api/v1/capacity/status")
    except ValueError as e:
        return {"error": f"Error contacting capacity status API: {e}"}

    capacity = {}
    for label, key in [("prow", "prow"), ("rosa", "rosa"), ("mce", "mce")]:
        info = data.get(key, {})
        active = info.get("active", 0)
        limit = info.get("limit", 0)
        capacity[label] = {
            "active": active,
            "limit": limit,
            "remaining": limit - active,
        }

    return {
        "capacity": capacity,
        "max_jobs_per_user": data.get("max_jobs_per_user", 0),
    }
