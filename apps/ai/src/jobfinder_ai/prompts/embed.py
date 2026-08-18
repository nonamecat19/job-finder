"""The `embed` capability has no prompt (C1-1 still requires an importable
`prompt_module`): it is a direct embeddings-endpoint call with no system or
user prompt text, no structured-output instruction and no retry turn — the
opposite end of the capability spectrum from `ghost`'s prompt-and-parse
shape.
"""

from __future__ import annotations
