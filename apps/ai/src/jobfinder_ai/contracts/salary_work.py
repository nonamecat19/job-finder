from __future__ import annotations

from typing import Any

from pydantic import BaseModel, ConfigDict


class SalaryWork(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    jobId: str
    activityId: str | None = None
    snapshot: dict[str, Any]
    snapshot_hash: str
