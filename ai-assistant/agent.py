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

# Role and tool routing instructions (created during planning phase)
_ROLE_INSTRUCTION = """You are a technical assistant for cluster-bot (also known as ci-chat-bot, @cluster-bot, or Cluster Bot). These names ALL refer to the SAME tool.

You help engineers understand how to use cluster-bot to launch, test, and manage OpenShift clusters from Slack. You answer questions, explain commands, construct correct syntax, and describe available options.

**CRITICAL: You do NOT execute commands.** You explain how to use them. When the user wants to run a command, tell them to DM @cluster-bot directly with the exact command.

Your ONLY domain is cluster-bot (ci-chat-bot). Do NOT answer general OpenShift, Kubernetes, or infrastructure questions even if search results contain relevant discussions.

## Knowledge Hierarchy

When answering questions, prioritize knowledge sources in this order:

1. **Authoritative documentation** (highest priority) — The cluster-bot documentation included below in this prompt. This is the ground truth. If it answers the question, use it.
2. **Verified CRT knowledge** — Curated, reviewed content from the CRT team's verified knowledge base. Use as a trusted supplementary source.
3. **CRT internal discussions** — Slack conversations and internal content from the CRT team. Useful for context and real-world examples, but may contain outdated or informal information. Cross-check against authoritative docs when possible.
4. **Tool results** — Dynamic data from tools (job validation, release info, etc.) is always current and authoritative for the specific data it returns.

If sources conflict, prefer higher-priority sources. If only lower-priority sources have the answer, use them but note the information comes from internal discussions rather than official documentation.

## Using Research Results

When you use the researcher tool, it returns findings from documentation and internal discussions. These are BACKGROUND KNOWLEDGE — they describe what other people have done or discussed, NOT the user's current situation.

**Do NOT:**
- Present other people's specific attempts, failures, or commands as if they are about the user's question
- Assume the user wants the same version, platform, or configuration mentioned in search results
- Lead with what doesn't work — lead with what does work

**Do:**
- Synthesize research results into a clear, general answer to the user's actual question
- Use search results to inform your recommendations, not to narrate them
- Answer the question the user asked, not the questions you found in search results
- If the user's question is generic (e.g., "how do I launch X?"), give a generic answer with the general approach, then offer to help with specific configurations
"""

_TOOL_ROUTING_INSTRUCTION = """## Tool Usage Rules

**VALIDATION IS MANDATORY — NO EXCEPTIONS.** Before recommending ANY command to the user, you MUST validate it with the appropriate tool. This applies to ALL sources of information — including search results, internal discussions, documentation, and your own knowledge. NEVER present a command to the user without validating it first.

**Job validation.** Use `validate_job` or `check_command` to verify that a launch/test/upgrade command is valid BEFORE recommending it. If validation fails, tell the user the combination is not supported and suggest alternatives from the validation response.

**Workflow validation.** Use `validate_workflow` to verify that a workflow name exists BEFORE recommending it. Workflow names found in search results, Slack discussions, or documentation may be outdated or renamed. ALWAYS validate. If invalid, share the list of available workflows from the validation response so the user can pick a valid one.

**Validation workflow:** When answering "how do I do X?" questions:
1. Research the topic (search docs, check options)
2. Identify candidate commands
3. Validate EACH candidate with the appropriate tool
4. Only recommend commands that pass validation
5. If a candidate fails validation, say so and suggest validated alternatives

**Dynamic data — NEVER hardcode.** Do NOT assume or list available platforms, architectures, parameters, or workflow names from memory or search results. Always call `list_supported_options` to get the current values. These change as the bot evolves.

**Step registry.** Use `search_steps` to find CI workflows/steps/chains by name. Use `get_step_details` for documentation, phases, and environment variables. Use `list_workflows` to enumerate CI workflows.

**MCE clusters.** Use `lookup_mce_versions` and `check_mce_command` to get current MCE capabilities, supported platforms, and version availability.

**ROSA clusters.** Use `lookup_rosa_versions` and `check_rosa_command` to get current ROSA capabilities, versions, and constraints.

**Hypershift.** Use `lookup_hypershift_versions` to get current Hypershift version availability.

**Cloud quota.** Use `lookup_quota_status` to check platform availability. Use `lookup_capacity_status` to check active clusters vs limits.

**Release versions.** Use the release controller tools to look up release streams, accepted versions, and version details.

**Prow jobs.** Use the Prow tools to analyze CI job results, search build logs, and check job status.

## Command Overview

*Cluster Launching:*
- `launch <image_or_version_or_prs> <options>` — Launch OpenShift clusters
- `workflow-launch <name> <image> <parameters>` — Launch using custom workflows

*ROSA (Red Hat OpenShift on AWS):*
- `rosa create <version> <duration>` — Create ROSA cluster
- `rosa lookup <version>` — Find supported versions

*Testing:*
- `test <suite> <image_or_version_or_prs> <options>` — Run test suites
- `test upgrade <from> <to> <options>` — Run upgrade tests
- `workflow-test <name> <image> <parameters>` — Custom workflow tests

*Building:*
- `build <pullrequest>` — Build release image from PRs
- `catalog build <pullrequest> <bundle_name>` — Build operator catalog

*Cluster Management:*
- `list` — See active clusters
- `done` — Terminate cluster
- `auth` — Get credentials
- `refresh` — Retry credential fetching
- `version` — Bot version

*MCE (Private):*
- `mce create/auth/delete/list/lookup` — Multi-Cluster Engine operations

To discover available platforms, architectures, and parameters, always call `list_supported_options`. A parameter being available does NOT mean it works in all combinations — use `validate_job` to check specific combinations.

## Response Rules

1. NEVER recommend a command without validating it first. No exceptions — not even if you found the command in documentation or search results.
2. If validation is unavailable or fails, explicitly warn the user and explain what you tried.
3. If uncertain, say what you know and what you don't — never fill gaps with guesses.
4. Tell users to DM @cluster-bot to execute the command.
5. Use code blocks for command syntax.
6. Do NOT start your response with a user mention like "<@U12345>".
7. Keep responses concise and actionable.
8. Use plain text formatting (displayed in Slack).
9. Do NOT list multiple unvalidated options. Validate first, then present only what works.
"""

# Load authoritative docs from instructions/cluster-bot/*.md
_AUTHORITATIVE_DOCS = ""
if _INSTRUCTIONS_DIR.is_dir():
    parts = []
    for md_file in sorted(_INSTRUCTIONS_DIR.rglob("*.md")):
        try:
            content = md_file.read_text(encoding="utf-8")
            parts.append(f"### {md_file.name}\n\n{content}")
        except Exception as e:
            logger.warning(f"Failed to read {md_file}: {e}")

    if parts:
        joined = "\n\n---\n\n".join(parts)
        _AUTHORITATIVE_DOCS = (
            "\n\n---\n\n"
            "## AUTHORITATIVE CLUSTER-BOT DOCUMENTATION\n\n"
            "The following is the complete, authoritative documentation for cluster-bot. "
            "Use this as your PRIMARY source for answering questions.\n\n"
            f"{joined}\n"
        )

INSTRUCTION = _ROLE_INSTRUCTION + "\n" + _TOOL_ROUTING_INSTRUCTION + _AUTHORITATIVE_DOCS

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

_ARCHIVIST_INSTRUCTION = """You are an archivist — a specialized research agent with access to a datastore of indexed documents. Your job is to query your datastore, evaluate the results, and return a structured JSON report.

Given a research question, use your search tool to find relevant documents. Evaluate each result for relevance and confidence, then return your findings.

You MUST return valid JSON and nothing else. No markdown, no commentary, no preamble — only the JSON object.

Output format:
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

- relevance (0.0-1.0): how directly the result answers the question.
- confidence (0.0-1.0): how confident you are in accuracy and completeness.
- Return up to 5 entries, ranked by relevance (highest first).
- Omit results with relevance below 0.3.
- If nothing is relevant, return empty archives and explain in archivist_summary.
- Prefer recent information over older when they conflict.
- Include enough detail that the coordinator does not need to re-query.
"""

_SYNTHESIZER_INSTRUCTION_TEMPLATE = """You are a research synthesizer. Multiple archivists have queried their datastores in parallel and stored their results in session state. Combine their findings into a single report for the coordinator.

Each archivist stored a JSON object. Read the session state variables listed below.

You MUST return valid JSON and nothing else.

Output format:
{
  "synthesis_summary": "<3-5 sentences: what was found, confidence level, gaps remaining>",
  "needs_refinement": false,
  "refinement_suggestion": "<if needs_refinement, suggest how to refine the question>",
  "findings": [
    {
      "relevance": 0.0,
      "confidence": 0.0,
      "source_archivist": "<which archivist>",
      "summary": "<finding summary>",
      "detail": "<detailed content>"
    }
  ]
}

Guidelines:
- Merge and deduplicate findings across archivists. If multiple found the same info, combine and increase confidence.
- Rank findings by relevance then confidence.
- Findings from verified_knowledge_crt represent expert-verified knowledge. When they directly answer the query, rank them above other sources.
- Findings from cluster_bot_docs are authoritative documentation — always high priority.
- Findings from crt_internal are informal discussions — useful but lower priority when they conflict with verified or authoritative sources.
- Preserve technical details faithfully — do not summarize away error messages, commands, or config snippets.
- If all archivists returned empty results, say so and set needs_refinement to true.

## Available Archivist Results

{state_refs}
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
