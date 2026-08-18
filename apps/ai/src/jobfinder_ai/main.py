"""The service entry point: one process, two transports (research R9).

FastAPI serves the interactive capabilities (contracts/http.md); FastStream's
RabbitMQ router serves the queued ones. Both share the same capability
registry and the same Langfuse client, so a capability behaves identically —
and traces identically — regardless of which transport carried the request.
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

from fastapi import FastAPI, HTTPException
from faststream.rabbit.fastapi import RabbitRouter
from pydantic import BaseModel

from jobfinder_ai import tracing
from jobfinder_ai.capabilities.registry import registry
from jobfinder_ai.capabilities.single import ghost as ghost_capability
from jobfinder_ai.messaging import consumers
from jobfinder_ai.settings import get_settings

settings = get_settings()

broker_router = RabbitRouter(settings.rabbitmq_url)

# Every capability registers itself at import time (C1-4): a process that is
# running has already proven its definition valid, so /health/ready never
# has to re-check it (H8-1).
registry.register(ghost_capability.CAPABILITY)

# Queued capabilities attach their FastStream subscriber/publisher to the
# one shared router (research R9) — same process, same broker connection.
consumers.register(broker_router)


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    tracing.bootstrap(settings=settings)
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


@app.post("/v1/capabilities/{name}/invoke")
async def invoke_capability(name: str, request: InvokeRequest) -> dict[str, Any]:
    """H2. A capability whose transport is `event` does not exist here (H1-2, T131) —
    same 404 as a name that was never registered."""
    capability = registry.resolve_for_invoke(name)
    if capability is None:
        raise HTTPException(status_code=404, detail=f"capability {name!r} not found")
    raise HTTPException(
        status_code=501,
        detail=f"capability {capability.name!r} execution not yet implemented",
    )


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)  # noqa: S104 — container-internal only
