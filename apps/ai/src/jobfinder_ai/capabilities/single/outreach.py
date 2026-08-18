"""The `outreach` capability (C2-1, FR-039): a single LangChain call with
Pydantic structured output, served over the interactive HTTP surface
(H1-1). Ported from
`apps/api/internal/outreach/application/generate.go`'s `generateGrounded`
and `apps/api/internal/outreach/domain.DraftOutput`/`BuildPrompt`.

`generateGrounded` loops up to `maxAttempts` (3) in Go, rebuilding the
prompt each time with the previous violation and falling back to a generic
opener if every attempt fails grounding — none of that loop, the grounding
check (`GroundClaims`), or the fallback live here (T107 territory). This
capability is the one model call an attempt makes, with the same
malformed-JSON retry (`MAX_EXTRA_ATTEMPTS`, ghost.py's constant) every
`CompleteStructured` caller in Go shares — that retry is about a broken
response, not a grounding violation.

Input/output contract for the Go caller:
    input:  {"tone": "warm" | "direct" | "formal", "contact_name": str,
              "company_name": str, "facts": [{"kind": str, "value": str}, ...],
              "last_violation": str}
    result: {"text": str, "specific_claims": [str, ...]}
`contact_name`, `company_name`, and `last_violation` default to `""`;
`facts` defaults to `[]`. A caller retrying after a grounding violation
issues another `/invoke` call with `last_violation` populated; this
capability does not loop or fall back internally.
"""

from __future__ import annotations

import json

from langchain_core.messages import AIMessage, HumanMessage, SystemMessage
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from jobfinder_ai import gateway, tracing
from jobfinder_ai.capabilities.registry import Capability, CapabilityBounds
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import CapabilityError, classify_provider_error
from jobfinder_ai.prompts import outreach as prompts

TASK_KEY = "outreach"

# Ported from port.go's `structuredRetries`: max 2 EXTRA attempts after the
# first (three attempts total) before failing with category `internal`
# (C3-4) — the same constant every `CompleteStructured` caller in Go shares.
MAX_EXTRA_ATTEMPTS = 2


class Fact(BaseModel):
    model_config = ConfigDict(extra="forbid")

    kind: str
    value: str


class OutreachInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    tone: prompts.Tone
    contact_name: str = ""
    company_name: str = ""
    facts: list[Fact] = Field(default_factory=list)
    last_violation: str = ""


class OutreachResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    text: str
    specific_claims: list[str] = Field(default_factory=list)


def _usage_from_message(message: AIMessage) -> Usage:
    meta = message.usage_metadata
    if not meta:
        return Usage()
    return Usage(
        input_tokens=meta.get("input_tokens"),
        output_tokens=meta.get("output_tokens"),
        cost_usd=None,
    )


def _schema_instruction(schema: str) -> str:
    return "\n\nRespond with a single JSON object matching this JSON Schema:\n" + schema


def _retry_instruction(schema: str, last_error: str) -> str:
    return (
        _schema_instruction(schema)
        + "\nYour previous answer was invalid: "
        + last_error
        + "\nFix it and answer again with valid JSON only."
    )


async def run(data: OutreachInput, *, trace_id: str | None = None) -> tuple[OutreachResult, Usage]:
    """Runs the outreach capability: builds the prompt, calls the gateway,
    and parses/validates the structured result, retrying on malformed JSON
    up to `MAX_EXTRA_ATTEMPTS` extra times. Raises `CapabilityError` (E5) on
    any classified failure; never returns a partially populated result
    (FR-003, C3-2)."""
    user_prompt = prompts.build_user_prompt(
        tone=data.tone,
        contact_name=data.contact_name,
        company_name=data.company_name,
        facts=[(f.kind, f.value) for f in data.facts],
        last_violation=data.last_violation,
    )

    bind_kwargs: dict[str, object] = {"response_format": {"type": "json_object"}}
    if trace_id is not None:
        bind_kwargs["extra_body"] = tracing.gateway_call_metadata(trace_id)
    model = gateway.chat_model(TASK_KEY).bind(**bind_kwargs)

    schema = json.dumps(OutreachResult.model_json_schema())
    turn = _schema_instruction(schema)
    last_error: str | None = None
    result: OutreachResult | None = None
    usage = Usage()

    for _attempt in range(MAX_EXTRA_ATTEMPTS + 1):
        if last_error is not None:
            turn = _retry_instruction(schema, last_error)
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

        assert isinstance(message, AIMessage)  # ChatOpenAI always returns AIMessage
        usage = _usage_from_message(message)
        content = message.content if isinstance(message.content, str) else str(message.content)
        try:
            payload = json.loads(content)
            result = OutreachResult.model_validate(payload)
        except (json.JSONDecodeError, ValidationError) as exc:
            last_error = str(exc)
            continue
        break

    if result is None:
        raise CapabilityError(
            "internal",
            f"outreach: structured output failed after {MAX_EXTRA_ATTEMPTS + 1} "
            f"attempts: {last_error}",
            failed_step=TASK_KEY,
        )

    return result, usage


CAPABILITY = Capability(
    name="outreach",
    task_key=TASK_KEY,
    layer="chain",
    input_model="jobfinder_ai.capabilities.single.outreach:OutreachInput",
    output_model="jobfinder_ai.capabilities.single.outreach:OutreachResult",
    bounds=CapabilityBounds(max_model_calls=1),
    prompt_module="jobfinder_ai.prompts.outreach",
    transport="http",
)
