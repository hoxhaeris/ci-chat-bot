"""FastAPI HTTP API for the cluster-bot AI assistant.

Provides a simple HTTP interface:
- POST /ask — Ask the AI assistant a question
- GET /health/live, /health/ready — Health checks
"""

import logging
import time
from typing import Optional

from fastapi import FastAPI, HTTPException
from google.adk.events import Event
from google.genai import types
from pydantic import BaseModel

from agent import APP_NAME, runner, session_service

logger = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")

# Quiet noisy third-party loggers
for _lib in ("httpx", "httpcore", "urllib3", "google", "grpc", "asyncio"):
    logging.getLogger(_lib).setLevel(logging.WARNING)

app = FastAPI(title="cluster-bot AI assistant", version="2.0.0")


class AskRequest(BaseModel):
    """Request body for the /ask endpoint."""

    question: str
    user_id: str
    thread_id: str
    context: Optional[str] = None
    channel_id: Optional[str] = None


class AskResponse(BaseModel):
    """Response body for the /ask endpoint."""

    answer: str


def _extract_answer(events: list) -> str:
    """Extract the final text answer from ADK runner events."""
    for event in reversed(events):
        if hasattr(event, "content") and event.content and hasattr(event.content, "parts"):
            for part in event.content.parts:
                if hasattr(part, "text") and part.text:
                    return part.text
    return "I was unable to generate a response. Please try again."


@app.post("/ask", response_model=AskResponse)
async def ask(req: AskRequest):
    """Ask the AI assistant a question."""
    start_time = time.time()

    # Get or create session using thread_id for conversation continuity
    session = await session_service.get_session(
        app_name=APP_NAME, user_id=req.user_id, session_id=req.thread_id
    )
    if not session:
        session = await session_service.create_session(
            app_name=APP_NAME, user_id=req.user_id, session_id=req.thread_id
        )

    question = req.question
    if req.context:
        question = f"Context from a failed command:\n{req.context}\n\nUser question: {req.question}"

    logger.info(
        f"Processing question from user={req.user_id} "
        f"thread={req.thread_id} len={len(question)}"
    )

    try:
        events = []
        async for event in runner.run_async(
            user_id=req.user_id,
            session_id=session.id,
            new_message=types.Content(role="user", parts=[types.Part(text=question)]),
        ):
            events.append(event)

        answer = _extract_answer(events)

        duration_ms = (time.time() - start_time) * 1000
        logger.info(
            f"Answered for user={req.user_id} thread={req.thread_id} "
            f"duration={duration_ms:.0f}ms answer_len={len(answer)}"
        )

        return AskResponse(answer=answer)

    except Exception as e:
        duration_ms = (time.time() - start_time) * 1000
        logger.error(
            f"Error for user={req.user_id} thread={req.thread_id} "
            f"duration={duration_ms:.0f}ms error={e}",
            exc_info=True,
        )
        # Delete the corrupted session so the next request starts fresh.
        # When a sub-agent crashes mid-tool-call, the session retains an
        # orphaned tool_use without a matching tool_result, which causes
        # every subsequent request on this thread to fail with a 400.
        try:
            await session_service.delete_session(
                app_name=APP_NAME, user_id=req.user_id, session_id=req.thread_id,
            )
            logger.info(
                f"Deleted corrupted session for user={req.user_id} "
                f"thread={req.thread_id}"
            )
        except Exception:
            logger.warning(
                f"Failed to delete session for thread={req.thread_id}",
                exc_info=True,
            )
        raise HTTPException(
            status_code=500,
            detail="An internal error occurred while processing your question.",
        )


@app.get("/health/live")
async def health_live():
    """Kubernetes liveness probe."""
    return {"status": "live"}


@app.get("/health/ready")
async def health_ready():
    """Kubernetes readiness probe."""
    return {"status": "ready"}
