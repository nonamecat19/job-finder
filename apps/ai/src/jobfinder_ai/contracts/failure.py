from __future__ import annotations

from pydantic import BaseModel, ConfigDict


class Failure(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    category: str
    retryable: bool
    message: str
    failed_step: str | None = None
