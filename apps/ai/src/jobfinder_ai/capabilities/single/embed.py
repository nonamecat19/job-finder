"""The `embed` capability (C2-1, C2-2): a direct embeddings-endpoint call,
ported from `apps/api/internal/platform/llm/infrastructure/gateway/gateway.go`'s
`Provider.Embed`. No prompt, no structured-output parsing, no retry loop —
just text in, vector out.

Invoked over HTTP (contracts/http.md H1-1), not as a queued work event: its
callers (profile indexing, retrieval) are synchronous request paths (C2-2).
It is migrated last (research R11) because it is the highest-volume,
lowest-benefit path to move — the added HTTP hop is measured against SC-005
before its cutover (T129), not assumed acceptable.
"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field

from jobfinder_ai import gateway
from jobfinder_ai.capabilities.registry import Capability, CapabilityBounds
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import CapabilityError, classify_provider_error

TASK_KEY = "embed"

EMBED_DIMS = 1024

EMBED_MAX_CHARS = 8000


class EmbedInput(BaseModel):
    """The capability's HTTP input model (contracts/http.md H3-1)."""

    model_config = ConfigDict(extra="forbid")

    text: str


class EmbedResult(BaseModel):
    """Mirrors Go's `Provider.Embed` return shape: one vector, dimensionality
    asserted against `EMBED_DIMS` rather than trusted (E2-2)."""

    model_config = ConfigDict(extra="forbid")

    vector: list[float] = Field(min_length=EMBED_DIMS, max_length=EMBED_DIMS)


def _truncate(text: str, limit: int) -> str:
    return text if len(text) <= limit else text[:limit]


async def run(work: EmbedInput, *, trace_id: str | None = None) -> tuple[EmbedResult, Usage]:
    """Runs the `embed` capability end to end: truncates the input as a
    backstop, calls the gateway by task key, and asserts the returned
    vector's dimensionality before returning it.

    Raises `CapabilityError` (E5) on any classified failure; a
    dimensionality mismatch is `internal` — the same posture Go's
    `ErrInvalidResponse` takes, since it is a response-shape defect, not a
    provider-status failure (FR-003, C3-2).
    """
    text = _truncate(work.text, EMBED_MAX_CHARS)
    model = gateway.embeddings_model(TASK_KEY)

    try:
        vector = await model.aembed_query(text)
    except Exception as exc:  # noqa: BLE001 - reclassified below (E5)
        raise classify_provider_error(exc, failed_step=TASK_KEY) from exc

    if len(vector) != EMBED_DIMS:
        raise CapabilityError(
            "internal",
            f"embed: embedding length {len(vector)} does not match configured "
            f"EMBED_DIMS {EMBED_DIMS}",
            failed_step=TASK_KEY,
        )

    return EmbedResult(vector=vector), Usage()


CAPABILITY = Capability(
    name="embed",
    task_key=TASK_KEY,
    layer="chain",
    input_model="jobfinder_ai.capabilities.single.embed:EmbedInput",
    output_model="jobfinder_ai.capabilities.single.embed:EmbedResult",
    bounds=CapabilityBounds(max_model_calls=1),
    prompt_module="jobfinder_ai.prompts.embed",
    transport="http",
)
