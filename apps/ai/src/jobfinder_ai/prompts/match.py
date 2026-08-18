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
today.
"""

from __future__ import annotations

SYSTEM_PROMPT = (
    "You are a precise technical recruiter. Judge only from the given profile and job text."
)


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
    `MatchJob`. Every literal string below MUST match the Go source byte
    for byte."""
    return (
        "Rate how well this candidate fits this job.\n\n"
        f"CANDIDATE PROFILE:\n{profile_text}\n\n"
        f"JOB POSTING:\nTitle: {title}\nCompany: {company}\n"
        f"Location: {location} (remote: {'true' if remote else 'false'})\n"
        f"Description:\n{description}\n\n"
        "Scoring guide: 90-100 near-perfect fit; 70-89 strong fit, minor gaps; "
        "50-69 partial fit, notable gaps; below 50 poor fit. "
        "matchedSkills/missingSkills = concrete skills from the job description. "
        "redFlags = concerns like seniority mismatch, hard requirements the candidate lacks, "
        "suspicious posting. summary = 2-3 sentences."
    )


def schema_instruction(schema: str) -> str:
    """Verbatim port of the trailing turn `CompleteStructured` appends to
    the final message (port.go): the schema-in-prompt instruction that makes
    `response_format: json_object` reliable without strict schema mode."""
    return "\n\nRespond with a single JSON object matching this JSON Schema:\n" + schema


def retry_instruction(schema: str, last_error: str) -> str:
    """Verbatim port of the retry turn `CompleteStructured` appends when the
    previous attempt failed to parse or validate."""
    return (
        schema_instruction(schema)
        + "\nYour previous answer was invalid: "
        + last_error
        + "\nFix it and answer again with valid JSON only."
    )
