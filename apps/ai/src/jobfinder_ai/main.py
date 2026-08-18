"""The service entry point: one process, two transports (research R9).

FastAPI serves the interactive capabilities (contracts/http.md); FastStream's
RabbitMQ router serves the queued ones. Both share the same capability
registry and the same Langfuse client, so a capability behaves identically —
and traces identically — regardless of which transport carried the request.
"""

from __future__ import annotations

import importlib
import secrets
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

from fastapi import Depends, FastAPI, Header, HTTPException
from fastapi.responses import JSONResponse
from faststream.rabbit.fastapi import RabbitRouter
from pydantic import BaseModel, ValidationError

from jobfinder_ai import tracing
from jobfinder_ai.capabilities.graphs import generation as generation_capability
from jobfinder_ai.capabilities.graphs import salary as salary_capability
from jobfinder_ai.capabilities.registry import registry
from jobfinder_ai.capabilities.single import embed as embed_capability
from jobfinder_ai.capabilities.single import ghost as ghost_capability
from jobfinder_ai.capabilities.single import match as match_capability
from jobfinder_ai.capabilities.single import outreach as outreach_capability
from jobfinder_ai.capabilities.single import recruiter as recruiter_capability
from jobfinder_ai.capabilities.single import rephrase as rephrase_capability
from jobfinder_ai.contracts.result import Failure, Result
from jobfinder_ai.contracts.result import Usage as ResultUsage
from jobfinder_ai.failures import CATEGORY_RETRYABLE, CapabilityError
from jobfinder_ai.messaging import consumers
from jobfinder_ai.settings import get_settings

settings = get_settings()

broker_router = RabbitRouter(settings.rabbitmq_url)

# Every capability registers itself at import time (C1-4): a process that is
# running has already proven its definition valid, so /health/ready never
# has to re-check it (H8-1).
registry.register(ghost_capability.CAPABILITY)
registry.register(match_capability.CAPABILITY)
registry.register(generation_capability.CAPABILITY)
registry.register(salary_capability.CAPABILITY)
registry.register(embed_capability.CAPABILITY)
registry.register(rephrase_capability.CAPABILITY)
registry.register(recruiter_capability.CAPABILITY)
registry.register(outreach_capability.CAPABILITY)

# Queued capabilities attach their FastStream subscriber/publisher to the
# one shared router (research R9) — same process, same broker connection.
consumers.register(broker_router)

# The interactive HTTP surface's dispatch table (H2-1): every `transport="http"`
# capability's module, keyed by capability name, each exposing `Input` (or an
# equivalently named Pydantic model matching `Capability.input_model`) and
# `run(input, *, trace_id=...) -> (output, Usage)` (H1-1).
_HTTP_CAPABILITY_MODULES: dict[str, Any] = {
    rephrase_capability.CAPABILITY.name: rephrase_capability,
    recruiter_capability.CAPABILITY.name: recruiter_capability,
    outreach_capability.CAPABILITY.name: outreach_capability,
    embed_capability.CAPABILITY.name: embed_capability,
}

# H4-2: 429 rate_limited without a real provider-supplied backoff signal uses
# this conservative constant (E5 does not carry one through today).
RATE_LIMITED_RETRY_AFTER_SECONDS = 1

# H4-2's status-code mapping. A category absent from H4-2's explicit list
# (`credential_rejected`, `insufficient_credits`) falls back to `internal`'s
# 500 — both are server-side configuration problems, not the caller's fault.
_STATUS_FOR_CATEGORY: dict[str, int] = {
    "invalid_input": 422,
    "rate_limited": 429,
    "provider_unavailable": 503,
    "model_unavailable": 503,
    "timeout": 504,
    "internal": 500,
    "bound_exceeded": 500,
    "credential_rejected": 500,
    "insufficient_credits": 500,
}


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    tracing.bootstrap(settings=settings)
    # The consumers declare nothing (M1-1) — they assert topology the backend
    # owns — so connecting before that topology exists is a startup failure
    # rather than something the broker fixes on its own. On a cold stack the
    # backend may simply not have got there yet, so wait for it.
    await consumers.wait_for_topology(settings.rabbitmq_url)
    async with broker_router.lifespan_context(app):
        try:
            yield
        finally:
            tracing.shutdown()


app = FastAPI(title="jobfinder-ai", lifespan=lifespan)
app.include_router(broker_router)


class InvokeContext(BaseModel):
    user_id: str
    work_id: str
    activity_id: str | None = None


class InvokeRequest(BaseModel):
    input: dict[str, Any]
    context: InvokeContext


@app.get("/health/live")
async def health_live() -> dict[str, str]:
    """The process is up. No dependency is consulted (H2)."""
    return {"status": "ok"}


@app.get("/health/ready")
async def health_ready() -> dict[str, Any]:
    """Registry validity, broker connection, gateway configuration — no model call (H8-1–H8-3).

    Registry validity and gateway configuration presence are both enforced at
    process startup (C1-4, K3-1): a process that is running has already
    proven both, so there is nothing left to re-probe for them here. Broker
    connectivity is the one thing that can change after boot, so it is the
    one thing actually checked.
    """
    if not settings.gateway_url or not settings.litellm_master_key:
        raise HTTPException(status_code=503, detail="gateway not configured")
    connected = await broker_router.broker.ping(timeout=2.0)
    if not connected:
        raise HTTPException(status_code=503, detail="broker not connected")
    return {"status": "ok", "capabilities": len(registry)}


def require_shared_secret(authorization: str | None = Header(default=None)) -> None:
    """H7-1: an unauthenticated (or wrongly authenticated) request is 401.
    This is a shared secret between the backend and this service, never an
    end-user credential or session token (H7-2) — the backend is the only
    caller (H1-3) and has already authorized the end user before this call
    is ever made.
    """
    expected = f"Bearer {settings.ai_service_token}"
    if authorization is None or not secrets.compare_digest(authorization, expected):
        raise HTTPException(status_code=401, detail="unauthorized")


def _resolve_input_type(dotted: str) -> type[BaseModel]:
    """Imports a `Capability.input_model` dotted path
    (`"module.path:AttrName"`) into a live Pydantic model type.
    Registration already proved the path importable (C1-4), so this never
    fails for a registered capability."""
    module_path, _, attr_name = dotted.partition(":")
    model_type = getattr(importlib.import_module(module_path), attr_name)
    if not (isinstance(model_type, type) and issubclass(model_type, BaseModel)):
        raise TypeError(f"{dotted!r} is not a Pydantic model type")
    return model_type


def _failure_result(
    *, trace_id: str | None, category: str, message: str, failed_step: str | None
) -> Result:
    return Result(
        status="failed",
        result=None,
        failure=Failure(
            category=category,
            retryable=CATEGORY_RETRYABLE.get(category, False),
            message=message,
            failed_step=failed_step,
        ),
        trace_id=trace_id,
        usage=None,
    )


@app.post("/v1/capabilities/{name}/invoke")
async def invoke_capability(
    name: str, request: InvokeRequest, _auth: None = Depends(require_shared_secret)
) -> JSONResponse:
    """H2. A capability whose transport is `event` does not exist here (H1-2, T131) —
    same 404 as a name that was never registered.

    Resolves the capability, validates `request.input` against its declared
    input model (H3-1: a mismatch is 422 naming the offending field, never a
    best-effort run), runs it, and maps the outcome to an HTTP response per
    H4: 200 with the result on success, the matching failure status
    otherwise, `trace_id` on both (H4-3), and never profile/posting content
    in a failure body (H4-4 — a `Failure` carries only category/message/
    failed_step).
    """
    capability = registry.resolve_for_invoke(name)
    if capability is None:
        raise HTTPException(status_code=404, detail=f"capability {name!r} not found")

    module = _HTTP_CAPABILITY_MODULES[capability.name]
    input_type = _resolve_input_type(capability.input_model)

    with tracing.run_trace(
        name=capability.name,
        user_id=request.context.user_id,
        session_id=request.context.work_id,
        metadata={
            "task_key": capability.task_key,
            "workflow_version": tracing.resolve_workflow_version(),
            "activity_id": request.context.activity_id,
        },
        tags=[capability.name, "http"],
    ) as span:
        trace_id = tracing.bootstrap().get_current_trace_id()

        try:
            parsed_input = input_type.model_validate(request.input)
        except ValidationError as exc:
            tracing.mark_run_failed(span, failed_step=capability.name, error=str(exc))
            body = _failure_result(
                trace_id=trace_id,
                category="invalid_input",
                message=f"{capability.name}: invalid input: {exc}",
                failed_step=capability.name,
            )
            return JSONResponse(status_code=422, content=body.model_dump(exclude={"snapshot_hash"}))

        try:
            result, usage = await module.run(parsed_input, trace_id=trace_id)
        except CapabilityError as exc:
            tracing.mark_run_failed(
                span, failed_step=exc.failed_step or capability.name, error=exc.message
            )
            body = _failure_result(
                trace_id=trace_id,
                category=exc.category,
                message=exc.message,
                failed_step=exc.failed_step,
            )
            status_code = _STATUS_FOR_CATEGORY.get(exc.category, 500)
            response = JSONResponse(
                status_code=status_code, content=body.model_dump(exclude={"snapshot_hash"})
            )
            if exc.category == "rate_limited":
                response.headers["Retry-After"] = str(RATE_LIMITED_RETRY_AFTER_SECONDS)
            return response

        body = Result(
            status="succeeded",
            result=result.model_dump(),
            failure=None,
            trace_id=trace_id,
            usage=ResultUsage(
                input_tokens=usage.input_tokens,
                output_tokens=usage.output_tokens,
                cost_usd=usage.cost_usd,
            ),
        )

    return JSONResponse(status_code=200, content=body.model_dump(exclude={"snapshot_hash"}))


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)  # noqa: S104 — container-internal only
