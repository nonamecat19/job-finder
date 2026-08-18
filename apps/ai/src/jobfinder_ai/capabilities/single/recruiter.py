"""The `recruiter` capability (C2-1, FR-039): a single LangChain call with
Pydantic structured output, served over the interactive HTTP surface
(H1-1). Ported from
`apps/api/internal/recruiter/application/{posting,companypage,linkedin}.go`'s
`Extract{Posting,CompanyPage,LinkedIn}Contact(s)` — three Go call sites that
share one extraction shape and one retry mechanism
(`llm.CompleteStructured`'s `structuredRetries`, ported here as
`MAX_EXTRA_ATTEMPTS`, same constant `ghost.py` ports).

`source` selects which of the three source-specific instructions
`prompts/recruiter.py` builds (C1-3: one capability, one task key, for all
three scraped-text sources). Field-level grounding — verifying a reported
name/title/email/phone/URL actually appears verbatim in the source text
(`groundContact` in Go) — is **not** ported here; it stays a caller-side
check in Go for all three sources, same as today.

Input/output contract for the Go caller:
    input:  {"source": "posting" | "company_page" | "linkedin", "text": str}
    result: {"contacts": [{"name": str, "title": str, "email": str,
                            "phone": str, "linkedin_url": str}, ...]}
Every field of a contact defaults to `""` when the model does not report
it — mirroring Go's `extractedContact`/`extractedContactList` JSON shape
exactly. `posting` asks the model to name at most one contact (the
requisition owner); `company_page` and `linkedin` ask for every named team
member; the caller (T107 territory) decides what to do with an empty or
multi-entry list.
"""

from __future__ import annotations

import json

from langchain_core.messages import AIMessage, HumanMessage, SystemMessage
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from jobfinder_ai import gateway, tracing
from jobfinder_ai.capabilities.registry import Capability, CapabilityBounds
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import CapabilityError, classify_provider_error
from jobfinder_ai.prompts import recruiter as prompts

TASK_KEY = "recruiter"

# Ported from port.go's `structuredRetries`: max 2 EXTRA attempts after the
# first (three attempts total) before failing with category `internal`
# (C3-4) — the same constant every `CompleteStructured` caller in Go shares.
MAX_EXTRA_ATTEMPTS = 2


class ExtractedContact(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: str = ""
    title: str = ""
    email: str = ""
    phone: str = ""
    linkedin_url: str = ""


class RecruiterInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    source: prompts.Source
    text: str


class RecruiterResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contacts: list[ExtractedContact] = Field(default_factory=list)


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


async def run(
    data: RecruiterInput, *, trace_id: str | None = None
) -> tuple[RecruiterResult, Usage]:
    """Runs the recruiter capability: builds the source-specific prompt,
    calls the gateway, and parses/validates the structured result, retrying
    on malformed JSON up to `MAX_EXTRA_ATTEMPTS` extra times. Raises
    `CapabilityError` (E5) on any classified failure; never returns a
    partially populated result (FR-003, C3-2)."""
    user_prompt = prompts.build_user_prompt(source=data.source, text=data.text)

    bind_kwargs: dict[str, object] = {"response_format": {"type": "json_object"}}
    if trace_id is not None:
        bind_kwargs["extra_body"] = tracing.gateway_call_metadata(trace_id)
    model = gateway.chat_model(TASK_KEY).bind(**bind_kwargs)

    schema = json.dumps(RecruiterResult.model_json_schema())
    turn = _schema_instruction(schema)
    last_error: str | None = None
    result: RecruiterResult | None = None
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
            result = RecruiterResult.model_validate(payload)
        except (json.JSONDecodeError, ValidationError) as exc:
            last_error = str(exc)
            continue
        break

    if result is None:
        raise CapabilityError(
            "internal",
            f"recruiter: structured output failed after {MAX_EXTRA_ATTEMPTS + 1} "
            f"attempts: {last_error}",
            failed_step=TASK_KEY,
        )

    return result, usage


CAPABILITY = Capability(
    name="recruiter",
    task_key=TASK_KEY,
    layer="chain",
    input_model="jobfinder_ai.capabilities.single.recruiter:RecruiterInput",
    output_model="jobfinder_ai.capabilities.single.recruiter:RecruiterResult",
    bounds=CapabilityBounds(max_model_calls=1),
    prompt_module="jobfinder_ai.prompts.recruiter",
    transport="http",
)
