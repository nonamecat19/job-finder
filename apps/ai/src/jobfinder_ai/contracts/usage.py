from __future__ import annotations

from pydantic import BaseModel, ConfigDict


class Usage(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    input_tokens: int | None = None
    output_tokens: int | None = None
    cost_usd: float | None = None
