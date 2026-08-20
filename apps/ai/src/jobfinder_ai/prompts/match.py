"""The `match` capability's prompt (C6-1), ported **verbatim** from
`apps/api/internal/matching/application/service.go`'s inline prompt
construction in `Service.MatchJob` — same wording, same structure, same
field order. This is a migration, not a rewrite (C8-1, C8-2, FR-021): any
drift here changes the model's answers and breaks parity with the recorded
baseline.

Retains `response_format: {"type": "json_object"}` (C6-4) — the schema is
sent as an appended instruction on the final message rather than as a
strict `json_schema` response format, exactly matching the Go
`CompleteStructured` legacy path that `matching.Service.MatchJob` uses
today. Those two instruction builders live in `prompts.structured` and are
re-exported here so each capability keeps one prompt module to import.
"""

from __future__ import annotations

from jobfinder_ai.prompts.composition import PromptBuilder
from jobfinder_ai.prompts.structured import retry_instruction, schema_instruction

__all__ = [
    "SYSTEM_PROMPT",
    "build_user_prompt",
    "retry_instruction",
    "schema_instruction",
]

SYSTEM_PROMPT = (
    "You are a precise technical recruiter. Judge only from the given profile and job text."
)

_TASK = "Rate how well this candidate fits this job."

_SCORING_GUIDE = (
    "Scoring guide: 90-100 near-perfect fit; 70-89 strong fit, minor gaps; "
    "50-69 partial fit, notable gaps; below 50 poor fit. "
    "matchedSkills/missingSkills = concrete skills from the job description. "
    "redFlags = concerns like seniority mismatch, hard requirements the candidate lacks, "
    "suspicious posting. summary = 2-3 sentences."
)


def _go_bool(value: bool) -> str:
    """Go renders a `bool` into the prompt as `true`/`false`; Python's
    `str(True)` would send `True` and drift from the baseline."""
    return "true" if value else "false"


def build_user_prompt(
    *,
    profile_text: str,
    title: str,
    company: str,
    location: str,
    remote: bool,
    description: str,
) -> str:
    """Verbatim port of the inline `fmt.Sprintf` prompt in `service.go`'s
    `MatchJob`. Every literal string in this module MUST match the Go source
    byte for byte."""
    prompt = PromptBuilder()
    prompt.paragraph(_TASK)
    prompt.line("CANDIDATE PROFILE:").paragraph(profile_text)
    prompt.line("JOB POSTING:")
    prompt.field("Title", title).field("Company", company)
    prompt.field("Location", f"{location} (remote: {_go_bool(remote)})")
    prompt.line("Description:").paragraph(description)
    prompt.text(_SCORING_GUIDE)
    return prompt.render()
