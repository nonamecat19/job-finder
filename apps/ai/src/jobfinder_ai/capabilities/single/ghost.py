"""The `ghost` capability (C2-1, FR-039, FR-003): a single LangChain call
with Pydantic structured output, ported from
`apps/api/internal/ghostjob/application/service.go`'s `ScoreJob`/
`buildPrompt` (C6-1, C6-4, T062, T063). Runs entirely off the input
snapshot — no database access (FR-008): the signals `ScoreJob` measured
from Postgres now ride on the event as part of the snapshot (data-model.md
§ 2, C6-3).
"""

from __future__ import annotations

import json
import re

from langchain_core.messages import AIMessage, HumanMessage, SystemMessage
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from jobfinder_ai import gateway, tracing
from jobfinder_ai.capabilities.registry import Capability, CapabilityBounds
from jobfinder_ai.contracts.ghost_work import GhostWork
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import CapabilityError, classify_provider_error
from jobfinder_ai.prompts import ghost as prompts

TASK_KEY = "ghost"

# Ported from service.go: confidence is capped when any measured signal was
# unknown at scoring time (`signals.HasUnknownSignal()`).
CONFIDENCE_CAP_WHEN_UNKNOWN = 0.6

# Ported from port.go's `structuredRetries`: max 2 EXTRA attempts after the
# first (three attempts total) before failing with category `internal`
# (C3-4).
MAX_EXTRA_ATTEMPTS = 2

_FENCE_RE = re.compile(r"^```(?:json)?\s*(.*?)\s*```$", re.DOTALL)


def _strip_fences(text: str) -> str:
    """Verbatim port of Go's `stripFences`."""
    stripped = text.strip()
    match = _FENCE_RE.match(stripped)
    return match.group(1) if match else stripped


class GhostSnapshot(BaseModel):
    """The grounding data `buildPrompt` (Go) read from Postgres, carried on
    the event instead since FR-008 denies this service database access
    (E3-2, E3-3, C6-3). Field names and JSON casing mirror
    `application.GhostSnapshot` in
    `apps/api/internal/ghostjob/application/publish.go` (T065) exactly —
    snake_case, unlike `domain.GhostSignals`'s Go field names."""

    model_config = ConfigDict(extra="forbid")

    job_id: str
    title: str
    company: str
    repost_count: int
    days_open: int | None = None
    cross_board_count: int | None = None
    always_hiring_count: int | None = None
    notes: dict[str, str] | None = None


class GhostResult(BaseModel):
    """Mirrors `domain.GhostJobResult` — same 0-100/0-1 ranges (C3-3)."""

    model_config = ConfigDict(extra="forbid")

    score: float = Field(ge=0, le=100)
    confidence: float = Field(ge=0, le=1)
    explanation: str
    topSignals: list[str]


def _usage_from_message(message: AIMessage) -> Usage:
    meta = message.usage_metadata
    if not meta:
        return Usage()
    return Usage(
        input_tokens=meta.get("input_tokens"),
        output_tokens=meta.get("output_tokens"),
        # Best-effort (E4-4): the gateway does not surface cost inline on
        # the chat completion; cost is recorded via the Langfuse generation
        # span (T068), not duplicated here.
        cost_usd=None,
    )


async def run(work: GhostWork, *, trace_id: str | None = None) -> tuple[GhostResult, Usage]:
    """Runs the ghost capability end to end: builds the prompt from the
    snapshot, calls the gateway by task key, parses and validates the
    structured result, and applies the confidence cap `ScoreJob` applies
    after the call.

    Raises `CapabilityError` (E5) on any classified failure; never returns a
    partially populated result (FR-003, C3-2).
    """
    try:
        snapshot = GhostSnapshot.model_validate(work.snapshot)
    except ValidationError as exc:
        raise CapabilityError(
            "invalid_input", f"ghost: malformed snapshot: {exc}", failed_step=TASK_KEY
        ) from exc

    user_prompt = prompts.build_user_prompt(
        title=snapshot.title,
        company=snapshot.company,
        repost_count=snapshot.repost_count,
        days_open=snapshot.days_open,
        cross_board_count=snapshot.cross_board_count,
        always_hiring_count=snapshot.always_hiring_count,
    )

    # C6-4: response_format stays json_object, never strict json_schema —
    # `drop_params: true` at the gateway silently drops json_schema mode.
    # Matches ghostjob.Service.ScoreJob, which never sets ResponseModeStrict.
    bind_kwargs: dict[str, object] = {"response_format": {"type": "json_object"}}
    if trace_id is not None:
        # T070/FR-017/research R8: correlate this run's trace with
        # LiteLLM's own Langfuse record.
        bind_kwargs["extra_body"] = tracing.gateway_call_metadata(trace_id)
    model = gateway.chat_model(TASK_KEY).bind(**bind_kwargs)

    schema = json.dumps(GhostResult.model_json_schema())
    turn = prompts.schema_instruction(schema)
    last_error: str | None = None
    raw: GhostResult | None = None
    usage = Usage()

    for _attempt in range(MAX_EXTRA_ATTEMPTS + 1):
        if last_error is not None:
            turn = prompts.retry_instruction(schema, last_error)
        try:
            # T068/T069/research R8: the Langfuse LangChain callback handler
            # turns this call into its own generation span (input, output,
            # duration, model, tokens, cost) nested under the current trace —
            # one span per attempt, so a retry is visible as a repeated span
            # rather than folded into the trace root.
            message = await model.ainvoke(
                [
                    SystemMessage(content=prompts.SYSTEM_PROMPT),
                    HumanMessage(content=user_prompt + turn),
                ],
                config={"callbacks": [tracing.callback_handler()]},
            )
        except Exception as exc:  # noqa: BLE001 - reclassified below (E5)
            raise classify_provider_error(exc, failed_step=TASK_KEY) from exc

        assert isinstance(message, AIMessage)  # ChatOpenAI always returns AIMessage
        usage = _usage_from_message(message)
        content = message.content if isinstance(message.content, str) else str(message.content)
        try:
            payload = json.loads(_strip_fences(content))
            raw = GhostResult.model_validate(payload)
        except (json.JSONDecodeError, ValidationError) as exc:
            last_error = str(exc)
            continue
        break

    if raw is None:
        raise CapabilityError(
            "internal",
            f"ghost: structured output failed after {MAX_EXTRA_ATTEMPTS + 1} "
            f"attempts: {last_error}",
            failed_step=TASK_KEY,
        )

    confidence = raw.confidence
    has_unknown_signal = (
        snapshot.days_open is None
        or snapshot.cross_board_count is None
        or snapshot.always_hiring_count is None
    )
    if has_unknown_signal:
        confidence = min(confidence, CONFIDENCE_CAP_WHEN_UNKNOWN)

    return raw.model_copy(update={"confidence": confidence}), usage


CAPABILITY = Capability(
    name="ghost",
    task_key=TASK_KEY,
    layer="chain",
    input_model="jobfinder_ai.contracts.ghost_work:GhostWork",
    output_model="jobfinder_ai.capabilities.single.ghost:GhostResult",
    bounds=CapabilityBounds(max_model_calls=1),
    prompt_module="jobfinder_ai.prompts.ghost",
    transport="event",
)
