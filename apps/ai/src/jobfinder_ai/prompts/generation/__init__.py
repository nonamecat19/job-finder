"""Prompts for the `generation` capability (C6-1), ported verbatim from
`apps/api/internal/generation/application/{rendercv_llm,service}.go`.

One module per stage of the state graph — `analyze`, `select`, `summary` —
plus `cover_letter`, the single-call branch that bypasses the graph
entirely. All four are built on `jobfinder_ai.prompts.composition`, and all
four are pinned byte-for-byte by the goldens in `tests/golden/prompts`.
"""

from __future__ import annotations
