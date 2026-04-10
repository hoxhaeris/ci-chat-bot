# AI Assistant

ADK-based AI assistant for cluster-bot. Runs as a sidecar container alongside the Go bot, providing natural-language help for cluster-bot commands, CI workflows, and OpenShift release information.

## Architecture

```
User -> @cluster-bot "ask ..." -> Go bot (port 8080)
                                     | HTTP POST localhost:3000/ask
                                     v
                                  AI service (ADK agent + FastAPI, port 3000)
                                     | tool calls to localhost:8080/api/v1/*
                                     v
                                  Go bot API (job validation, MCE, ROSA, etc.)
                                     | response
                                     v
                                  Go bot -> Slack
```

Both services run in the same Kubernetes pod as separate containers (sidecar pattern). They communicate over localhost — no external networking needed.

- **Go bot** listens on `:8080` for Slack events and exposes an internal API for tool calls
- **AI service** listens on `:3000` for `/ask` requests from the Go bot

### How requests flow

1. User sends a question via Slack (DM, mention, modal, or error help button)
2. Go bot forwards the question to `POST localhost:3000/ask`
3. AI service creates/resumes an ADK session (keyed by Slack thread ID)
4. ADK agent processes the question, calling tools as needed
5. Tools call back to the Go bot API (`localhost:8080`) for dynamic data
6. AI service returns the answer; Go bot posts it to Slack

### Stack

| Component | Technology |
|---|---|
| Agent framework | [Google ADK](https://google.github.io/adk-docs/) |
| LLM | Claude Sonnet via Vertex AI |
| HTTP server | FastAPI + Uvicorn |
| Session storage | In-memory (resets on pod restart) |
| Doc retrieval | Vertex AI Search (optional) |

## Tools

The agent has 22 tool functions across 4 modules. All tools are plain Python functions — ADK wraps them automatically.

### Cluster Bot API (`tools/cluster_bot_api.py`)

Calls the Go bot's internal API on `localhost:8080`. These tools provide dynamic data about supported platforms, job validation, and cluster capabilities.

| Tool | Description |
|---|---|
| `validate_job` | Validate a launch/test/upgrade job configuration |
| `check_command` | Parse a raw command string and validate it |
| `list_supported_options` | List available platforms, architectures, and parameters |
| `validate_workflow` | Validate a workflow name exists |
| `lookup_mce_versions` | Get available MCE versions and platforms |
| `check_mce_command` | Validate an MCE command |
| `lookup_rosa_versions` | Get available ROSA versions |
| `check_rosa_command` | Validate a ROSA command |
| `lookup_hypershift_versions` | Get Hypershift version availability |
| `lookup_quota_status` | Check cloud platform quota/availability |
| `lookup_capacity_status` | Check active clusters vs limits |

### Step Registry (`tools/step_registry.py`)

Scrapes [steps.ci.openshift.org](https://steps.ci.openshift.org) for CI workflow and step information.

| Tool | Description |
|---|---|
| `search_steps` | Search for CI steps/chains/workflows by name |
| `get_step_details` | Get documentation, phases, and env vars for a step |
| `list_workflows` | List available CI workflows |

### Prow (`tools/prow.py`)

Analyzes Prow CI job results and build logs. Self-contained, stdlib-only.

| Tool | Description |
|---|---|
| `get_prow_url_info` | Parse a Prow URL and extract job metadata |
| `get_prowjob_status` | Get the status and results of a Prow job |
| `search_build_log` | Search build logs for a pattern |
| `get_build_log_excerpt` | Get a section of a build log |

### Release Controller (`tools/release_controller.py`)

Queries OpenShift release controller endpoints across all architectures. Self-contained, stdlib-only.

| Tool | Description |
|---|---|
| `get_release_streams` | List release streams for an architecture |
| `get_release_stream_tags` | Get tags/versions in a release stream |
| `get_release_info` | Get details for a specific release version |
| `get_rejected_releases` | Get rejected releases and failure reasons |

## Running Locally

### Prerequisites

- Python 3.12+
- [uv](https://docs.astral.sh/uv/) package manager
- GCP credentials with Vertex AI access (for Claude model)

### Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `GOOGLE_CLOUD_PROJECT` | Yes | — | GCP project ID |
| `GOOGLE_CLOUD_LOCATION` | No | `us-east5` | Vertex AI region |
| `AI_MODEL` | No | `claude-sonnet-4-5-20250514` | Model ID |
| `CI_CHAT_BOT_URL` | No | `http://localhost:8080` | Go bot API URL |
| `VERTEX_SEARCH_DATASTORE_ID` | No | — | Vertex AI Search datastore for doc retrieval |
| `LOG_LEVEL` | No | `INFO` | Logging level |

### Start the service

```bash
# Install dependencies
make install

# Run (requires GOOGLE_CLOUD_PROJECT to be set)
make run

# Run with debug logging
make run-debug
```

The service starts on `http://localhost:3000`.

### Test it

```bash
curl -X POST http://localhost:3000/ask \
  -H "Content-Type: application/json" \
  -d '{"question": "How do I launch a cluster?", "user_id": "test", "thread_id": "t1"}'
```

### Running with the Go bot

1. Start the AI service on port 3000 (as above)
2. Start the Go bot with `--ai-service-url=http://localhost:3000`
3. Both services communicate over localhost

## Configuration

### Agent instructions

The agent's behavior is defined by:

1. **Inline instructions** in `agent.py` — role definition and tool routing rules
2. **Authoritative docs** in `instructions/cluster-bot/*.md` — loaded at startup and appended to the instruction prompt

The instruction files are:

| File | Content |
|---|---|
| `command-reference.md` | Complete command syntax reference |
| `faq.md` | Frequently asked questions |
| `help-categories.md` | Help topic categories |
| `overview.md` | Cluster-bot overview and architecture |
| `platforms-parameters.md` | Supported platforms and parameters |
| `workflow-guide.md` | Workflow launch guide |

### Vertex AI Search (optional)

If `VERTEX_SEARCH_DATASTORE_ID` is set, the agent gets an additional `VertexAiSearchTool` for supplementary document retrieval. The same instruction docs are uploaded to a Vertex AI Search datastore for semantic search.

## Deployment

### Docker

```bash
make docker-build
make docker-run  # requires .env file with credentials
```

### Kubernetes (sidecar)

The AI service runs as a second container in the ci-chat-bot pod:

```yaml
containers:
  - name: ci-chat-bot
    image: ci-chat-bot:latest
    ports:
      - containerPort: 8080
  - name: ai-assistant
    image: ai-assistant:latest
    ports:
      - containerPort: 3000
    env:
      - name: GOOGLE_CLOUD_PROJECT
        value: "your-gcp-project"
      - name: CI_CHAT_BOT_URL
        value: "http://localhost:8080"
```

Both containers share the pod's network namespace, so they reach each other via `localhost`.

## Project Structure

```
ai-assistant/
├── agent.py                    # ADK Agent + Runner definition
├── app.py                      # FastAPI HTTP endpoint (POST /ask, health checks)
├── tools/
│   ├── __init__.py             # Tool registry (get_all_tools())
│   ├── cluster_bot_api.py      # 11 tools calling Go bot API
│   ├── step_registry.py        # 3 tools querying steps.ci.openshift.org
│   ├── prow.py                 # 4 tools for Prow job analysis
│   └── release_controller.py   # 4 tools for release info
├── instructions/
│   └── cluster-bot/            # Authoritative docs (loaded into agent prompt)
├── pyproject.toml              # Dependencies (google-adk, fastapi, uvicorn, anthropic)
├── Dockerfile
└── Makefile
```

## Development

```bash
# Install dev dependencies
make install-dev

# Run tests
make test

# Clean caches
make clean
```

### Adding a new tool

1. Add a Python function with a descriptive docstring to the appropriate module in `tools/`
2. Import and add it to the list in `tools/__init__.py`
3. If the tool needs routing instructions, update the tool routing section in `agent.py`
