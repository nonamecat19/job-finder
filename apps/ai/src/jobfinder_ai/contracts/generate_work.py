from __future__ import annotations

from typing import Any

from pydantic import BaseModel, ConfigDict


class GenerateWork(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    jobId: str
    type: str
    profileId: str | None = None
    activityId: str | None = None
    generationRunId: str | None = None
    isRerun: bool | None = None
    rerunSections: list[str] | None = None
    snapshot: dict[str, Any]
    snapshot_hash: str
