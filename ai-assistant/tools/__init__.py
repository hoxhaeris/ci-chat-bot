"""Tool registry for the cluster-bot AI assistant.

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
from .prow import (
    get_build_log_excerpt,
    get_prow_url_info,
    get_prowjob_status,
    search_build_log,
)
from .release_controller import (
    get_rejected_releases,
    get_release_info,
    get_release_stream_tags,
    get_release_streams,
)
from .step_registry import (
    get_step_details,
    list_workflows,
    search_steps,
)
from .workspace_files import (
    clone_openshift_release,
    ws_grep,
    ws_list,
    ws_read_file,
    ws_tree,
)
from .workspace_exec import ws_exec
from .workspace_sessions import workspace_destroy, workspace_new


def get_all_tools() -> list:
    """Return all tool functions for ADK agent registration."""
    return [
        # Workspace tools (clone and browse repos for CI config questions)
        clone_openshift_release,
        workspace_new,
        workspace_destroy,
        ws_exec,
        ws_read_file,
        ws_grep,
        ws_list,
        ws_tree,
        # Cluster bot API tools (call Go bot via localhost)
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
        # Step registry tools (scrape steps.ci.openshift.org)
        search_steps,
        get_step_details,
        list_workflows,
        # Prow tools (fetch from GCS artifacts)
        get_prow_url_info,
        get_prowjob_status,
        search_build_log,
        get_build_log_excerpt,
        # Release controller tools (query release controller APIs)
        get_release_streams,
        get_release_stream_tags,
        get_release_info,
        get_rejected_releases,
    ]
