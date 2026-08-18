"""The `rephrase` capability (C2-1, C2-2a, FR-039): a single LangChain call
with plain-text output, served over the interactive HTTP surface (H1-1).

Ported from two Go callers that share one model adapter today —
`apps/api/internal/keyword/application.Suggester.Suggest` (the keyword path)
and `apps/api/internal/coach/application.Service.generateGroundedRephrase`
(coach chat) — both of which build a prompt, call
`rephraseadapter.ProviderRephraseModel.Rephrase` once, and verify the
result's grounding themselves. This capability is only that one model call
(C4-1: `max_model_calls=1`); prompt assembly for both callers lives in
`prompts/rephrase.py`, but the retry-on-grounding-violation loop and the
grounding check itself (`verifyRephraseGrounding` / `domain.VerifyRephraseGrounding`)
stay in Go on the caller side — T107 repoints the Go callers at this
capability without moving that logic (C1-3, C6-1).

Input/output contract for the Go caller (T107):
    input:  {"term": str, "canonical": str, "source_bullet": str,
              "source_label": str | null, "prior_violations": [str, ...]}
    result: {"rephrase": str}
`canonical` and `prior_violations` default to `""`/`[]` when absent;
`source_label` is optional and, when omitted, the prompt drops the SOURCE
LABEL section and its two seniority/duration rules — this is exactly the
keyword caller's shape today. A caller wanting another retry attempt (with
grounding feedback) issues another `/invoke` call with `prior_violations`
populated; this capability does not loop internally.
"""

from __future__ import annotations

from langchain_core.messages import AIMessage, HumanMessage, SystemMessage
from pydantic import BaseModel, ConfigDict, Field

from jobfinder_ai import gateway, tracing
from jobfinder_ai.capabilities.registry import Capability, CapabilityBounds
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import classify_provider_error
from jobfinder_ai.prompts import rephrase as prompts

TASK_KEY = "rephrase"


class RephraseInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    term: str
    canonical: str = ""
    source_bullet: str
    source_label: str | None = None
    prior_violations: list[str] = Field(default_factory=list)


class RephraseResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    rephrase: str


def _usage_from_message(message: AIMessage) -> Usage:
    meta = message.usage_metadata
    if not meta:
        return Usage()
    return Usage(
        input_tokens=meta.get("input_tokens"),
        output_tokens=meta.get("output_tokens"),
        cost_usd=None,
    )


async def run(data: RephraseInput, *, trace_id: str | None = None) -> tuple[RephraseResult, Usage]:
    """Runs the rephrase capability: one model call, no retries, no grounding
    check (that stays with the caller). Raises `CapabilityError` (E5) on any
    classified failure (FR-003, C3-2)."""
    user_prompt = prompts.build_user_prompt(
        term=data.term,
        canonical=data.canonical,
        source_bullet=data.source_bullet,
        source_label=data.source_label,
        prior_violations=data.prior_violations,
    )

    bind_kwargs: dict[str, object] = {}
    if trace_id is not None:
        bind_kwargs["extra_body"] = tracing.gateway_call_metadata(trace_id)
    model = gateway.chat_model(TASK_KEY).bind(**bind_kwargs)

    try:
        message = await model.ainvoke(
            [
                SystemMessage(content=prompts.SYSTEM_PROMPT),
                HumanMessage(content=user_prompt),
            ],
            config={"callbacks": [tracing.callback_handler()]},
        )
    except Exception as exc:  # noqa: BLE001 - reclassified below (E5)
        raise classify_provider_error(exc, failed_step=TASK_KEY) from exc

    assert isinstance(message, AIMessage)  # ChatOpenAI always returns AIMessage
    usage = _usage_from_message(message)
    content = message.content if isinstance(message.content, str) else str(message.content)
    return RephraseResult(rephrase=content.strip()), usage


CAPABILITY = Capability(
    name="rephrase",
    task_key=TASK_KEY,
    layer="chain",
    input_model="jobfinder_ai.capabilities.single.rephrase:RephraseInput",
    output_model="jobfinder_ai.capabilities.single.rephrase:RephraseResult",
    bounds=CapabilityBounds(max_model_calls=1),
    prompt_module="jobfinder_ai.prompts.rephrase",
    transport="http",
)
