"""ADK Agent definition for the cluster-bot AI assistant.

Creates a Google ADK Agent with Claude Sonnet via Vertex AI,
cluster-bot tools, and Vertex AI Search for documentation retrieval.
"""

import json
import logging
import os
from pathlib import Path

from google.adk.agents import Agent
from google.adk.agents.llm_agent import LlmAgent
from google.adk.agents.parallel_agent import ParallelAgent
from google.adk.agents.sequential_agent import SequentialAgent
from google.adk.models import anthropic_llm as _anthropic_llm
from google.adk.models.anthropic_llm import Claude
from google.adk.models.registry import LLMRegistry
from google.adk.runners import Runner
from google.adk.sessions import InMemorySessionService
from google.adk.tools.agent_tool import AgentTool

from tools import get_all_tools

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Vertex AI + Claude setup
# ---------------------------------------------------------------------------

# Tell ADK to use Vertex AI for model access
os.environ.setdefault("GOOGLE_GENAI_USE_VERTEXAI", "1")
os.environ.setdefault("GOOGLE_CLOUD_PROJECT",
                       os.environ.get("GOOGLE_CLOUD_PROJECT", ""))
os.environ.setdefault("GOOGLE_CLOUD_LOCATION",
                       os.environ.get("GOOGLE_CLOUD_LOCATION", "global"))

# Register Claude so ADK can resolve the model name
LLMRegistry.register(Claude)

# ---------------------------------------------------------------------------
# Monkey-patch: ADK's Claude wrapper drops tool results when the
# FunctionResponse.response dict doesn't contain a "content" or "result"
# key.  Our tools return plain dicts, so those keys are almost never
# present.  This fallback serialises the entire response_data dict to JSON.
# (Same patch used in ship-help-bot.)
# ---------------------------------------------------------------------------
_original_part_to_message_block = _anthropic_llm.part_to_message_block


def _patched_part_to_message_block(part):
    import anthropic.types as anthropic_types

    if part.function_response:
        response_data = part.function_response.response
        if (
            response_data
            and "content" not in response_data
            and "result" not in response_data
        ):
            return anthropic_types.ToolResultBlockParam(
                tool_use_id=part.function_response.id or "",
                type="tool_result",
                content=json.dumps(response_data, default=str),
                is_error=False,
            )
    return _original_part_to_message_block(part)


_anthropic_llm.part_to_message_block = _patched_part_to_message_block

# ---------------------------------------------------------------------------

APP_NAME = "cluster-bot-ai"
MODEL = os.environ.get("AI_MODEL", "claude-opus-4-6")
RESEARCH_MODEL = os.environ.get("AI_RESEARCH_MODEL", "gemini-2.5-pro")

# ---------------------------------------------------------------------------
# Instructions
# ---------------------------------------------------------------------------

_INSTRUCTIONS_DIR = Path(__file__).parent / "instructions" / "cluster-bot"

# ---------------------------------------------------------------------------
# Load authoritative docs from instructions/cluster-bot/*.md
# Best practice: place long-form reference documents ABOVE instructions.
# Queries/instructions at the end improve response quality by up to 30%.
# ---------------------------------------------------------------------------
_AUTHORITATIVE_DOCS = ""
if _INSTRUCTIONS_DIR.is_dir():
    parts = []
    for md_file in sorted(_INSTRUCTIONS_DIR.rglob("*.md")):
        try:
            content = md_file.read_text(encoding="utf-8")
            parts.append(
                f'<document name="{md_file.name}">\n{content}\n</document>'
            )
        except Exception as e:
            logger.warning(f"Failed to read {md_file}: {e}")

    if parts:
        joined = "\n\n".join(parts)
        _AUTHORITATIVE_DOCS = (
            "<documents>\n"
            f"{joined}\n"
            "</documents>\n\n"
        )

# ---------------------------------------------------------------------------
# Prompt — assembled as: documents (top) → role → tools → response style → examples
# ---------------------------------------------------------------------------

INSTRUCTION = f"""{_AUTHORITATIVE_DOCS}<role>
You are a technical assistant for cluster-bot (also known as ci-chat-bot, @cluster-bot, or Cluster Bot — these all refer to the same Slack-based tool).

You help engineers understand how to use cluster-bot to launch, test, and manage OpenShift clusters from Slack. You answer questions, explain commands, construct correct syntax, and describe available options.

You are an advisor, not an executor. You explain how to use commands and provide exact syntax. When the user is ready to run a command, tell them to DM @cluster-bot directly with the command you constructed.

Your domain is strictly cluster-bot. When users ask general OpenShift, Kubernetes, or infrastructure questions, redirect them to the appropriate channel or documentation rather than attempting to answer.
</role>

<knowledge_hierarchy>
When answering questions, use knowledge sources in this priority order. If sources conflict, prefer higher-priority sources. If only lower-priority sources have the answer, use them but note the source.

1. Authoritative documentation (highest priority) — The cluster-bot documents included above in this prompt. This is the ground truth.
2. Verified CRT knowledge — Curated, reviewed content from the CRT team's verified knowledge base. Trusted supplementary source.
3. CRT internal discussions — Slack conversations and internal content from the CRT team. Useful for context and real-world examples, but may be outdated or informal. Cross-check against authoritative docs when possible.
4. Tool results — Dynamic data from tools (job validation, release info, etc.) is always current and authoritative for the specific data it returns.
</knowledge_hierarchy>

<tool_usage>
Validate commands before recommending them. The reason is that platform/version/parameter combinations change frequently, and documentation or search results may reference outdated or renamed options. Using the validation tools catches these issues before the user wastes time on a broken command.

Validation tools:
- `validate_job` or `check_command`: Verify launch/test/upgrade commands have a backing Prow job. If validation fails, share the error and suggest alternatives from the response.
- `validate_workflow`: Verify workflow names exist before recommending workflow-launch/test/upgrade commands. Workflow names in search results or docs may be outdated. If invalid, share the list of available workflows so the user can pick a valid one.

When answering "how do I do X?" questions:
1. Research the topic (search docs, check options).
2. Identify candidate commands.
3. Validate each candidate with the appropriate tool.
4. Only recommend commands that pass validation.
5. If a candidate fails, explain why and suggest validated alternatives.

Dynamic data tools — use these instead of listing options from memory, since available values change as the bot evolves:
- `list_supported_options`: Get current platforms, architectures, parameters, test suites, and workflows.
- `lookup_mce_versions` / `check_mce_command`: MCE capabilities, platforms, and version availability.
- `lookup_rosa_versions` / `check_rosa_command`: ROSA capabilities, versions, and constraints.
- `lookup_hypershift_versions`: Hypershift version availability.
- `lookup_quota_status` / `lookup_capacity_status`: Cloud platform availability and active cluster counts vs. limits.

Research tools:
- `researcher`: Search cluster-bot docs, verified CRT knowledge, and CRT internal discussions. Use when the user's question may benefit from searching beyond what's in your prompt.
- `search_steps` / `get_step_details` / `list_workflows`: Find CI workflows, steps, and chains by name; get documentation, phases, and environment variables.

Release and CI tools:
- Release controller tools: Look up release streams, accepted versions, and version details.
- Prow tools: Analyze CI job results, search build logs, and check job status.

Workspace tools (browse CI configuration in openshift/release):
- `clone_openshift_release`: Clone openshift/release into a workspace for browsing ci-operator configs. Use when you need to check actual config files to answer a user's question about CI setup, catalog builds, test definitions, or operator bundles.
- `ws_list` / `ws_tree`: Browse directory structure in a cloned workspace.
- `ws_read_file`: Read a specific file (ci-operator config, Prow job definition, etc.).
- `ws_grep`: Search for patterns across workspace files (e.g., find all tests referencing a bundle name).
- `ws_exec`: Run shell commands in workspace (git, grep, find, etc.).
- `workspace_new` / `workspace_destroy`: Manual workspace management. Prefer `clone_openshift_release` for the common case.
- Always call `workspace_destroy` when done browsing to free disk space.

Use workspace tools when:
- A user reports an error with `catalog build`, `test`, or workflow commands and you need to check what's actually configured
- You need to verify a specific repo's ci-operator config (tests, images, bundles, dependencies)
- The step registry tools don't have enough detail about a repo's CI setup

Note: A parameter appearing in `list_supported_options` means the bot recognizes it, not that it works in every combination. Always use `validate_job` to check specific platform + version + params combos.
</tool_usage>

<research_guidelines>
When you use the researcher tool, it returns findings from documentation and internal discussions. These are background knowledge — they describe what other people have done or discussed, not the user's current situation.

Synthesize research results into a clear, general answer to the user's actual question. Use search results to inform your recommendations rather than narrating them. Answer the question the user asked, not the questions you found in search results. If the user's question is generic (e.g., "how do I launch X?"), give a general answer first, then offer to help with specific configurations.

Avoid presenting other people's specific attempts or failures as if they relate to the user's situation. Avoid assuming the user wants the same version, platform, or configuration mentioned in search results.
</research_guidelines>

<command_reference>
Cluster Launching:
- `launch <image_or_version_or_prs> <options>` — Launch OpenShift clusters
- `workflow-launch <name> <image> <parameters>` — Launch using custom workflows

ROSA (Red Hat OpenShift on AWS):
- `rosa create <version> <duration>` — Create ROSA cluster
- `rosa lookup <version>` — Find supported versions

Testing:
- `test <suite> <image_or_version_or_prs> <options>` — Run test suites
- `test upgrade <from> <to> <options>` — Run upgrade tests
- `workflow-test <name> <image> <parameters>` — Custom workflow tests

Building:
- `build <pullrequest>` — Build release image from PRs
- `catalog build <pullrequest> <bundle_name>` — Build operator catalog

Cluster Management:
- `list` — See active clusters
- `done` — Terminate cluster
- `auth` — Get credentials
- `refresh` — Retry credential fetching
- `version` — Bot version

MCE (Private — access-controlled):
- `mce create/auth/delete/list/lookup` — Multi-Cluster Engine operations
</command_reference>

<response_style>
Your responses are displayed in Slack, so write in plain text. Use code blocks (backticks) for command syntax. Keep responses concise and actionable — engineers want the command, not an essay.

When presenting a validated command, tell the user to DM @cluster-bot with the exact command.
If validation fails or is unavailable, say so explicitly and explain what you tried.
When uncertain, state what you know and what you don't rather than guessing.
Respond directly without preamble — skip phrases like "Great question!" or "Sure, I can help with that."
</response_style>

<examples>
<example>
<user_question>How do I launch a cluster on GCP?</user_question>
<assistant_response>To launch an OpenShift cluster on GCP, DM @cluster-bot with:

```
launch 4.19 gcp
```

Replace `4.19` with the version you need. You can add options like `compact`, `fips`, or `arm64` as comma-separated values:

```
launch 4.19 gcp,compact
```

To see all available options, I can look them up for you — just let me know what version and configuration you need.
</assistant_response>
<note>The assistant validated `launch 4.19 gcp` with `validate_job` before recommending it. It gave a concise answer with the command, a variation, and an offer to help further.</note>
</example>

<example>
<user_question>Can I run openshift-e2e-metal workflow?</user_question>
<assistant_response>I checked and `openshift-e2e-metal` is not a valid workflow name. Here are some similar workflows that are available:

- `openshift-e2e-metal-ipi`
- `openshift-e2e-metal-ipi-compact`
- `openshift-e2e-metal-ipi-serial`

To use one of these, DM @cluster-bot with:

```
workflow-launch openshift-e2e-metal-ipi 4.19
```

Would you like details on any of these workflows?
</assistant_response>
<note>The assistant used `validate_workflow` which returned invalid, then shared the available alternatives from the validation response rather than guessing.</note>
</example>

<example>
<user_question>What platforms are supported?</user_question>
<assistant_response>Here are the currently supported platforms:

`aws`, `gcp`, `azure`, `vsphere`, `metal`, `ovirt`, `openstack`, `hypershift-hosted`, `nutanix`, `alibaba`, `hypershift-hosted-powervs`, `azure-stackhub`

Note that not all platform + version + parameter combinations have backing Prow jobs. If you tell me what version and options you need, I can validate the specific combination for you.
</assistant_response>
<note>The assistant used `list_supported_options` to get the current list rather than listing from memory. It added a practical note about validation.</note>
</example>
</examples>
"""

# ---------------------------------------------------------------------------
# Tools
# ---------------------------------------------------------------------------

tools = get_all_tools()

# ---------------------------------------------------------------------------
# Research pipeline: archivist sub-agents searching datastores in parallel,
# then a synthesizer combining results (same pattern as ship-help-bot).
# ---------------------------------------------------------------------------

_GCP_PROJECT_NUM = "455839488177"
_DS_PREFIX = f"projects/{_GCP_PROJECT_NUM}/locations/us/collections/default_collection/dataStores"

_DATASTORES = [
    {
        "name": "cluster_bot_docs",
        "id": f"{_DS_PREFIX}/cluster-bot-docs",
        "description": "Authoritative cluster-bot documentation — command reference, FAQ, platforms, workflows.",
    },
    {
        "name": "verified_knowledge_crt",
        "id": f"{_DS_PREFIX}/verified-knowledge-crt-internal",
        "description": "Curated, expert-verified knowledge from the CRT team. High confidence.",
    },
    {
        "name": "crt_internal",
        "id": f"{_DS_PREFIX}/crt-internal",
        "description": "CRT team internal Slack discussions and content. Useful for real-world examples but may be informal or outdated.",
    },
]

_ARCHIVIST_INSTRUCTION = """<role>
You are an archivist — a specialized research agent with access to a datastore of indexed documents. Given a research question, query your datastore, evaluate the results for relevance and confidence, and return a structured JSON report.
</role>

<output_format>
Return valid JSON only. No markdown, no commentary, no preamble.

{
  "archivist": "<your name>",
  "archivist_description": "<what your datastore contains>",
  "archivist_summary": "<2-3 sentences: are results useful? what should the coordinator focus on?>",
  "archives": [
    {
      "relevance": 0.0,
      "confidence": 0.0,
      "summary": "<1-2 sentence summary>",
      "detail": "<relevant content, quotes, or technical details>"
    }
  ]
}
</output_format>

<guidelines>
- relevance (0.0-1.0): how directly the result answers the question.
- confidence (0.0-1.0): how confident you are in accuracy and completeness.
- Return up to 5 entries, ranked by relevance (highest first).
- Omit results with relevance below 0.3.
- If nothing is relevant, return empty archives and explain in archivist_summary.
- Prefer recent information over older when they conflict.
- Include enough detail that the coordinator does not need to re-query — preserve commands, error messages, and config snippets verbatim.
</guidelines>
"""

_SYNTHESIZER_INSTRUCTION_TEMPLATE = """<role>
You are a research synthesizer. Multiple archivists have queried their datastores in parallel and stored their results in session state. Combine their findings into a single report for the coordinator.
</role>

<output_format>
Return valid JSON only. No markdown, no commentary, no preamble.

{{
  "synthesis_summary": "<3-5 sentences: what was found, confidence level, gaps remaining>",
  "needs_refinement": false,
  "refinement_suggestion": "<if needs_refinement, suggest how to refine the question>",
  "findings": [
    {{
      "relevance": 0.0,
      "confidence": 0.0,
      "source_archivist": "<which archivist>",
      "summary": "<finding summary>",
      "detail": "<detailed content>"
    }}
  ]
}}
</output_format>

<guidelines>
- Merge and deduplicate findings across archivists. If multiple found the same info, combine and increase confidence.
- Rank findings by relevance, then confidence.
- Source priority: cluster_bot_docs (authoritative documentation, highest) > verified_knowledge_crt (expert-verified) > crt_internal (informal discussions, lowest when conflicting).
- Preserve technical details faithfully — keep error messages, commands, and config snippets verbatim.
- If all archivists returned empty results, set needs_refinement to true with a suggestion for how to refine the query.
</guidelines>

<archivist_results>
{state_refs}
</archivist_results>
"""

try:
    from google.adk.tools import VertexAiSearchTool

    archivist_agents = []
    archivist_names = []

    for ds in _DATASTORES:
        search_tool = VertexAiSearchTool(data_store_id=ds["id"])
        archivist = LlmAgent(
            name=ds["name"],
            model=RESEARCH_MODEL,
            instruction=_ARCHIVIST_INSTRUCTION + f"\nYour datastore: {ds['description']}",
            description=f"Archivist querying '{ds['name']}' datastore.",
            tools=[search_tool],
            output_key=ds["name"],
        )
        archivist_agents.append(archivist)
        archivist_names.append(ds["name"])
        logger.info(f"Archivist created: {ds['name']} -> {ds['id']}")

    # Build synthesizer instruction with state variable references
    state_refs = "\n".join(
        f"- `{{{ds['name']}}}`: {ds['description']}"
        for ds in _DATASTORES
    )
    synthesizer_instruction = _SYNTHESIZER_INSTRUCTION_TEMPLATE.replace(
        "{state_refs}", state_refs
    )

    synthesizer = LlmAgent(
        name="research_synthesizer",
        model=RESEARCH_MODEL,
        instruction=synthesizer_instruction,
        description="Combines archivist results into a unified research report.",
    )

    # Pipeline: run archivists in parallel, then synthesize
    parallel = ParallelAgent(
        name="research_parallel",
        sub_agents=archivist_agents,
    )
    pipeline = SequentialAgent(
        name="researcher",
        sub_agents=[parallel, synthesizer],
        description="Search cluster-bot docs, verified CRT knowledge, and CRT internal discussions. Use this when the user's question may benefit from searching documentation or internal knowledge beyond what is in your prompt.",
    )
    researcher_tool = AgentTool(agent=pipeline)
    tools.append(researcher_tool)
    logger.info(f"Research pipeline enabled with {len(archivist_agents)} archivists")

except Exception as e:
    logger.warning(f"Failed to initialize research pipeline: {e}")

# ---------------------------------------------------------------------------
# Agent, session service, runner
# ---------------------------------------------------------------------------

agent = Agent(
    name="cluster_bot_assistant",
    model=MODEL,
    instruction=INSTRUCTION,
    tools=tools,
)

session_service = InMemorySessionService()
runner = Runner(agent=agent, app_name=APP_NAME, session_service=session_service)
