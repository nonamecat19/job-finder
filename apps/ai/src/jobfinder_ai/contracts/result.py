from __future__ import annotations

from typing import Any

from pydantic import BaseModel, ConfigDict


class Failure(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    category: str
    retryable: bool
    message: str
    failed_step: str | None = None


class Usage(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    input_tokens: int | None = None
    output_tokens: int | None = None
    cost_usd: float | None = None


class Result(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    status: str
    result: dict[str, Any] | None = None
    failure: Failure | None = None
    trace_id: str | None = None
    snapshot_hash: str | None = None
    usage: Usage | None = None
