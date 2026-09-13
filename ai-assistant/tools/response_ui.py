"""Response-UI tools for the cluster-bot assistant.

These tools do NOT post to Slack. They let the agent attach structured UI
hints (follow-up quick-action buttons) that the /ask response returns to the
Go bot, which renders them as Slack Block Kit. Mirrors ship-help-bot's
set_response_buttons, adapted to cluster-bot's "Go owns Slack" architecture:
the assistant produces a semantic payload, the Go bot renders and posts it.
"""

import logging

logger = logging.getLogger(__name__)


def set_followup_buttons(buttons: list[dict]) -> dict:
    """Offer the user up to 5 follow-up quick-action buttons for your answer.

    Call this AFTER composing your answer, to present obvious next steps as
    one-click buttons in Slack. When the user clicks a button, its ``message``
    is sent back to you as their next question. Only include genuinely useful
    next steps; if none apply, do not call this tool.

    Args:
        buttons: A list of up to 5 button objects. Each object has:
            - label:   short button text shown to the user (<= 75 chars),
                       e.g. "Validate on GCP" or "Show 4.19 options".
            - message: the exact follow-up question to send when clicked
                       (<= 2000 chars), e.g. "validate launch 4.19 gcp,fips".
            - style:   optional; "primary" or "danger" to emphasize a button.

    Returns:
        A confirmation dict, e.g. {"registered": 3}. The buttons themselves
        are read from this call and returned in the /ask response.
    """
    count = 0
    if isinstance(buttons, list):
        count = sum(
            1
            for b in buttons
            if isinstance(b, dict) and b.get("label") and b.get("message")
        )
    return {"registered": min(count, 5)}


def set_run_commands(commands: list[dict]) -> dict:
    """Offer validated cluster-bot commands as one-click ▶ Run buttons.

    Call this when you recommend a specific cluster-bot command that you have
    VALIDATED (e.g. with validate_job or validate_workflow). Each becomes a
    button that executes the command directly in Slack, so the user does not
    have to copy-paste it. Always also show the command text in your answer.

    Do NOT offer a run button for: commands you could not validate,
    access-controlled commands (e.g. mce), or destructive/irreversible actions
    the user did not explicitly ask for.

    Args:
        commands: up to 3 objects, each:
            - command: the exact cluster-bot command to run, e.g.
                       "launch 4.19 gcp" or "workflow-launch openshift-e2e-gcp 4.19".
            - label:   optional short button label (<= 75 chars); defaults to
                       the command text.

    Returns:
        A confirmation dict, e.g. {"registered": 2}.
    """
    count = 0
    if isinstance(commands, list):
        count = sum(
            1 for c in commands if isinstance(c, dict) and c.get("command")
        )
    return {"registered": min(count, 3)}
