from __future__ import annotations

from pydantic import BaseModel, ConfigDict


class IngestWork(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    searchId: str
    subscriptionId: str | None = None
    sourceKey: str
    activityId: str | None = None
