"""The `salary` capability's prompt (C6-1), ported verbatim from
`apps/api/internal/salary/application/service.go`'s `llmInfer`.
"""

from __future__ import annotations

SYSTEM_PROMPT = (
    "You are a compensation analyst. Estimate realistic salary ranges based on job "
    "title, company, and location. Look up comparable bands and read the full "
    "posting before answering — an estimate made without checking is a guess."
)


def build_user_prompt(
    *,
    job_id: str,
    title: str,
    company: str,
    location: str | None,
    remote: bool,
    description: str,
) -> str:
    """Verbatim port of the prompt built in `llmInfer`, truncating the
    description at 4000 characters exactly as the Go source does."""
    loc = location if location else "n/a"
    desc = description[:4000]
    return (
        "Estimate the annual salary range for this job in the local currency.\n\n"
        f"JOB:\nTitle: {title}\nCompany: {company}\nLocation: {loc} (remote: "
        f"{'true' if remote else 'false'})\nDescription:\n{desc}\n\n"
        "Return a single SalaryBand JSON object. Use the most likely currency for "
        "the location. Set confidence based on how certain you are (0.3-0.9). If "
        "you cannot make a reasonable estimate, set min=0, max=0, confidence=0.\n\n"
        f"The id of this posting is {job_id}."
    )


def schema_instruction(schema: str) -> str:
    """Verbatim port of the trailing turn `CompleteStructuredChat` appends to
    the final message (port.go)."""
    return "\n\nRespond with a single JSON object matching this JSON Schema:\n" + schema


def retry_instruction(schema: str, last_error: str) -> str:
    """Verbatim port of the retry turn `CompleteStructuredChat` appends when
    the previous attempt failed to parse or validate."""
    return (
        schema_instruction(schema)
        + "\nYour previous answer was invalid: "
        + last_error
        + "\nFix it and answer again with valid JSON only."
    )
