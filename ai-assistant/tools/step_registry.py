"""Step registry tools for searching the OpenShift CI step registry.

Queries the OpenShift CI step registry at steps.ci.openshift.org to search
and inspect CI workflow steps, chains, and workflows.
"""

import logging
import re
import time
import urllib.request
from typing import Any

logger = logging.getLogger(__name__)

REGISTRY_URL = "https://steps.ci.openshift.org"

# Simple in-memory cache for the registry index (main page is ~3.8 MB,
# contains 1300+ workflows, 5800+ chains, 11000+ steps).
_INDEX_CACHE: dict | None = None
_INDEX_CACHE_TIME: float = 0
_INDEX_CACHE_TTL: float = 600  # 10 minutes


def _parse_index(html: str) -> dict:
    """Parse the main page HTML into a dict of {category: [name, ...]}."""
    return {
        "workflows": re.findall(r'<a href="/workflow/([^"]+)"', html),
        "chains": re.findall(r'<a href="/chain/([^"]+)"', html),
        "steps": re.findall(r'<a href="/reference/([^"]+)"', html),
    }


def _get_index() -> dict:
    """Fetch (or return cached) registry index."""
    global _INDEX_CACHE, _INDEX_CACHE_TIME

    now = time.monotonic()
    if _INDEX_CACHE is not None and (now - _INDEX_CACHE_TIME) < _INDEX_CACHE_TTL:
        return _INDEX_CACHE

    req = urllib.request.Request(
        REGISTRY_URL,
        headers={"User-Agent": "cluster-bot-ai/1.0"},
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        html = resp.read().decode("utf-8", errors="replace")

    _INDEX_CACHE = _parse_index(html)
    _INDEX_CACHE_TIME = now
    return _INDEX_CACHE


def _parse_detail_page(html: str) -> dict:
    """Extract structured data from a workflow/chain/reference detail page."""
    info: dict = {}

    doc_match = re.search(
        r'<p id="documentation">(.*?)</p>', html, re.DOTALL
    )
    if doc_match:
        info["documentation"] = doc_match.group(1).strip()

    for phase in ("pre", "test", "post"):
        header = re.search(
            rf'<h3 id="{phase}"[^>]*>.*?</h3>(.*?)(?=<h3|<h2|$)',
            html,
            re.DOTALL,
        )
        if header:
            refs = re.findall(
                r'<a href="/(?:reference|chain)/([^"]+)"',
                header.group(1),
            )
            seen = set()
            unique = []
            for r in refs:
                if r not in seen:
                    seen.add(r)
                    unique.append(r)
            if unique:
                info.setdefault("phases", {})[phase] = unique

    chain_refs = re.findall(
        r'<a href="/(?:reference|chain)/([^"]+)"', html
    )
    if chain_refs and "phases" not in info:
        seen = set()
        unique = []
        for r in chain_refs:
            if r not in seen:
                seen.add(r)
                unique.append(r)
        info["chain_steps"] = unique

    envs = re.findall(
        r'<td[^>]*style="font-family:monospace"[^>]*>(\w+)</td>',
        html,
    )
    if envs:
        info["environment_variables"] = envs

    return info


def search_steps(query: str) -> dict[str, Any]:
    """Search the OpenShift CI step registry for steps, chains, or workflows.

    Returns matching names grouped by category (workflow, chain, step).
    Use this to find workflows for a specific purpose, e.g. 'e2e gcp'
    or 'upgrade azure'.

    Args:
        query: Search query (partial name match), e.g. 'e2e-gcp', 'ipi-install'.

    Returns:
        dict with matches grouped by category.
    """
    try:
        index = _get_index()
    except Exception as e:
        return {"error": f"Error fetching step registry: {e}"}

    query_lower = query.lower()
    matches: dict[str, list[str]] = {}

    for category in ("workflows", "chains", "steps"):
        found = [
            name for name in index.get(category, [])
            if query_lower in name.lower()
        ]
        if found:
            matches[category] = sorted(found)[:20]
            if len(found) > 20:
                matches[f"{category}_total"] = len(found)

    if not matches:
        return {"matches": {}, "message": f"No entries found matching '{query}'."}

    return {"query": query, "matches": matches}


def get_step_details(name: str) -> dict[str, Any]:
    """Get detailed information about a specific CI step, chain, or workflow.

    Returns documentation, step phases, environment variables, and
    dependencies for the named registry entry.

    Args:
        name: Full step/chain/workflow name, e.g. 'openshift-e2e-gcp',
              'ipi-install-install', 'ipi-gcp-pre'.

    Returns:
        dict with documentation, phases, environment variables.
    """
    try:
        index = _get_index()
    except Exception as e:
        return {"error": f"Error fetching step registry: {e}"}

    category_map = {
        "workflows": "workflow",
        "chains": "chain",
        "steps": "reference",
    }
    found_category = None
    for cat_plural, cat_path in category_map.items():
        if name in index.get(cat_plural, []):
            found_category = (cat_plural, cat_path)
            break

    if not found_category:
        return {
            "error": (
                f"No step, chain, or workflow found with name '{name}'. "
                "Use search_steps to find the correct name."
            )
        }

    cat_plural, cat_path = found_category

    try:
        req = urllib.request.Request(
            f"{REGISTRY_URL}/{cat_path}/{name}",
            headers={"User-Agent": "cluster-bot-ai/1.0"},
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            html = resp.read().decode("utf-8", errors="replace")
    except Exception as e:
        return {"error": f"Error fetching details for '{name}': {e}"}

    info = _parse_detail_page(html)
    if not info:
        return {"name": name, "category": cat_plural[:-1], "message": "Could not parse details."}

    result: dict[str, Any] = {
        "name": name,
        "category": cat_plural[:-1],
    }
    result.update(info)
    return result


def list_workflows(prefix: str = "") -> dict[str, Any]:
    """List available CI workflows from the step registry.

    Workflows are complete test pipelines that combine multiple steps
    and chains. Use this to discover what workflows exist.

    Args:
        prefix: Optional name prefix to filter, e.g. 'openshift-e2e'.
                Leave empty to list all workflows.

    Returns:
        dict with list of workflow names.
    """
    try:
        index = _get_index()
    except Exception as e:
        return {"error": f"Error fetching step registry: {e}"}

    workflows = index.get("workflows", [])

    if prefix:
        prefix_lower = prefix.lower()
        filtered = [w for w in workflows if prefix_lower in w.lower()]
    else:
        filtered = workflows

    if not filtered:
        msg = "No workflows found"
        if prefix:
            msg += f" matching '{prefix}'"
        return {"workflows": [], "message": msg}

    return {
        "total": len(filtered),
        "workflows": sorted(set(filtered))[:50],
        "truncated": len(filtered) > 50,
    }
