"""Failure classification (contracts/events.md E5), mirroring
`apps/api/internal/platform/llm/infrastructure/shared/errors.go`'s
`ClassifyProviderError` so retry decisions carry over unchanged from the Go
implementation (E5-1, FR-004). `retryable` is fixed by the producer here,
never re-derived by the consumer (E5-2).
"""

from __future__ import annotations

import openai

CATEGORY_RETRYABLE: dict[str, bool] = {
    "rate_limited": True,
    "credential_rejected": False,
    "insufficient_credits": False,
    "model_unavailable": False,
    "provider_unavailable": True,
    "invalid_input": False,
    "bound_exceeded": False,
    "timeout": True,
    "internal": True,
}


class CapabilityError(Exception):
    """A classified capability failure, carrying exactly what a `Failure`
    result event needs (category, retryable, message, failed_step)."""

    def __init__(
        self,
        category: str,
        message: str,
        *,
        failed_step: str | None = None,
        retryable: bool | None = None,
    ) -> None:
        super().__init__(message)
        self.category = category
        self.message = message
        self.failed_step = failed_step
        self.retryable = CATEGORY_RETRYABLE[category] if retryable is None else retryable


def classify_provider_error(exc: Exception, *, failed_step: str | None = None) -> CapabilityError:
    """Classify a gateway-call exception (E5-1), mirroring Go's
    `ClassifyProviderError` status-code switch:
    <=0 -> provider_unavailable, 429 -> rate_limited, 401/403 ->
    credential_rejected, 402 -> insufficient_credits,
    400/404/422 -> model_unavailable, else -> provider_unavailable.
    """
    if isinstance(exc, openai.APITimeoutError):
        return CapabilityError("timeout", str(exc), failed_step=failed_step)
    if isinstance(exc, openai.APIConnectionError):
        return CapabilityError("provider_unavailable", str(exc), failed_step=failed_step)
    if isinstance(exc, openai.RateLimitError):
        return CapabilityError("rate_limited", str(exc), failed_step=failed_step)
    if isinstance(exc, openai.AuthenticationError | openai.PermissionDeniedError):
        return CapabilityError("credential_rejected", str(exc), failed_step=failed_step)
    if isinstance(exc, openai.APIStatusError):
        status = exc.status_code
        if status == 402:
            return CapabilityError("insufficient_credits", str(exc), failed_step=failed_step)
        if status in (400, 404, 422):
            return CapabilityError("model_unavailable", str(exc), failed_step=failed_step)
        return CapabilityError("provider_unavailable", str(exc), failed_step=failed_step)
    return CapabilityError("internal", str(exc), failed_step=failed_step)
