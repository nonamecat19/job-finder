"""The `match` capability (C2-1, FR-039, FR-003): a single LangChain call
with Pydantic structured output, ported from
`apps/api/internal/matching/application/service.go`'s `MatchJob` (the LLM
fit-analysis step only — embedding, similarity prefiltering and persistence
stay in Go; see data-model.md § 2). Runs entirely off the input snapshot —
no database access (FR-008): the candidate profile text and job posting
`MatchJob` read from Postgres now ride on the event as part of the snapshot
(data-model.md § 2, C6-3).
"""

from __future__ import annotations

import json
import re

from langchain_core.messages import AIMessage, HumanMessage, SystemMessage
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from jobfinder_ai import gateway, tracing
from jobfinder_ai.capabilities.registry import Capability, CapabilityBounds
from jobfinder_ai.contracts.match_work import MatchWork
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import CapabilityError, classify_provider_error
from jobfinder_ai.prompts import match as prompts

TASK_KEY = "match"

MAX_EXTRA_ATTEMPTS = 2

_FENCE_RE = re.compile(r"^```(?:json)?\s*(.*?)\s*```$", re.DOTALL)


def _strip_fences(text: str) -> str:
    """Verbatim port of Go's `stripFences`."""
    stripped = text.strip()
    match = _FENCE_RE.match(stripped)
    return match.group(1) if match else stripped


class MatchSnapshot(BaseModel):
    """The grounding data `MatchJob` (Go) read from Postgres — the
    candidate's rendered profile text (rendercv + extra notes, truncated to
    6000 chars) and the job posting — carried on the event instead since
    FR-008 denies this service database access (E3-2, E3-3, C6-3). Field
    names and JSON casing mirror the Go publisher's snapshot struct exactly
    — snake_case."""

    model_config = ConfigDict(extra="forbid")

    profile_text: str
    title: str
    company: str
    location: str
    remote: bool
    description: str


class MatchResult(BaseModel):
    """Mirrors `domain.FitResult` — same 0-100 score range (C3-3)."""

    model_config = ConfigDict(extra="forbid")

    score: float = Field(ge=0, le=100)
    matchedSkills: list[str]
    missingSkills: list[str]
    summary: str
    redFlags: list[str]


def _usage_from_message(message: AIMessage) -> Usage:
    meta = message.usage_metadata
    if not meta:
        return Usage()
    return Usage(
        input_tokens=meta.get("input_tokens"),
        output_tokens=meta.get("output_tokens"),
        cost_usd=None,
    )


async def run(work: MatchWork, *, trace_id: str | None = None) -> tuple[MatchResult, Usage]:
    """Runs the match capability end to end: builds the prompt from the
    snapshot, calls the gateway by task key, parses and validates the
    structured result.

    Raises `CapabilityError` (E5) on any classified failure; never returns a
    partially populated result (FR-003, C3-2).
    """
    try:
        snapshot = MatchSnapshot.model_validate(work.snapshot)
    except ValidationError as exc:
        raise CapabilityError(
            "invalid_input", f"match: malformed snapshot: {exc}", failed_step=TASK_KEY
        ) from exc

    user_prompt = prompts.build_user_prompt(
        profile_text=snapshot.profile_text,
        title=snapshot.title,
        company=snapshot.company,
        location=snapshot.location,
        remote=snapshot.remote,
        description=snapshot.description,
    )

    bind_kwargs: dict[str, object] = {"response_format": {"type": "json_object"}}
    if trace_id is not None:
        bind_kwargs["extra_body"] = tracing.gateway_call_metadata(trace_id)
    model = gateway.chat_model(TASK_KEY).bind(**bind_kwargs)

    schema = json.dumps(MatchResult.model_json_schema())
    turn = prompts.schema_instruction(schema)
    last_error: str | None = None
    raw: MatchResult | None = None
    usage = Usage()

    for _attempt in range(MAX_EXTRA_ATTEMPTS + 1):
        if last_error is not None:
            turn = prompts.retry_instruction(schema, last_error)
        try:
            message = await model.ainvoke(
                [
                    SystemMessage(content=prompts.SYSTEM_PROMPT),
                    HumanMessage(content=user_prompt + turn),
                ],
                config={"callbacks": [tracing.callback_handler()]},
            )
        except Exception as exc:  # noqa: BLE001 - reclassified below (E5)
            raise classify_provider_error(exc, failed_step=TASK_KEY) from exc

        assert isinstance(message, AIMessage)
        usage = _usage_from_message(message)
        content = message.content if isinstance(message.content, str) else str(message.content)
        try:
            payload = json.loads(_strip_fences(content))
            raw = MatchResult.model_validate(payload)
        except (json.JSONDecodeError, ValidationError) as exc:
            last_error = str(exc)
            continue
        break

    if raw is None:
        raise CapabilityError(
            "internal",
            f"match: structured output failed after {MAX_EXTRA_ATTEMPTS + 1} "
            f"attempts: {last_error}",
            failed_step=TASK_KEY,
        )

    return raw, usage


CAPABILITY = Capability(
    name="match",
    task_key=TASK_KEY,
    layer="chain",
    input_model="jobfinder_ai.contracts.match_work:MatchWork",
    output_model="jobfinder_ai.capabilities.single.match:MatchResult",
    bounds=CapabilityBounds(max_model_calls=1),
    prompt_module="jobfinder_ai.prompts.match",
    transport="event",
)
