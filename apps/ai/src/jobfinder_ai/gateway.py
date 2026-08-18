"""The one path a capability may reach a model through (FR-009, FR-010, FR-010a).

`langchain-openai` is the adapter, not a provider integration: LiteLLM presents
an OpenAI-compatible endpoint (029-FR-011), so pointing the OpenAI-shaped
client at `GATEWAY_URL` with the task key as `model` is the whole mechanism —
no provider-specific package is installed, and no code path can reach a
provider credential (research R3). Retries are disabled here on purpose: the
gateway already runs its own failover chain, and a client-side retry would
turn one chain exhaustion into several and multiply spend (C7-2).
"""

from __future__ import annotations

from langchain_openai import ChatOpenAI, OpenAIEmbeddings
from pydantic import SecretStr

from jobfinder_ai.settings import Settings, get_settings

# gateway/config.yaml pins `request_timeout: 60` per attempt. Ours must clear
# that with margin (C7-4) so the client never abandons a call the gateway is
# still serving.
REQUEST_TIMEOUT_SECONDS: float = 90.0


def chat_model(task_key: str, *, settings: Settings | None = None) -> ChatOpenAI:
    """A chat client scoped to one task key, with no bypass path (FR-010a)."""
    settings = settings or get_settings()
    return ChatOpenAI(
        model=task_key,
        base_url=settings.gateway_url,
        api_key=SecretStr(settings.litellm_master_key),
        max_retries=0,
        timeout=REQUEST_TIMEOUT_SECONDS,
    )


def embeddings_model(task_key: str, *, settings: Settings | None = None) -> OpenAIEmbeddings:
    """An embeddings client scoped to one task key, with no bypass path (FR-010a)."""
    settings = settings or get_settings()
    return OpenAIEmbeddings(
        model=task_key,
        base_url=settings.gateway_url,
        api_key=SecretStr(settings.litellm_master_key),
        max_retries=0,
        timeout=REQUEST_TIMEOUT_SECONDS,
    )
