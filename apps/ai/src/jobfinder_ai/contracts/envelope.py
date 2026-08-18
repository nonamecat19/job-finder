from __future__ import annotations

from pydantic import AwareDatetime, BaseModel, ConfigDict


class Envelope(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    event_id: str
    event_type: str
    schema_version: int
    occurred_at: AwareDatetime
    work_id: str
    correlation_id: str
    idempotency_key: str
    run_id: str
    activity_id: str | None = None
    trace_id: str | None = None
