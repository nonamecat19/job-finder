"""The `salary` capability's prompt (C6-1), ported verbatim from
`apps/api/internal/salary/application/service.go`'s `llmInfer`.

The structured-output turns it appends (the legacy `json_object` path, C6-4)
live in `prompts.structured` and are re-exported here so each capability
keeps one prompt module to import.
"""

from __future__ import annotations

from jobfinder_ai.prompts.composition import PromptBuilder, truncate
from jobfinder_ai.prompts.structured import retry_instruction, schema_instruction

__all__ = [
    "DESCRIPTION_TRUNCATE_CHARS",
    "SYSTEM_PROMPT",
    "build_user_prompt",
    "retry_instruction",
    "schema_instruction",
]

SYSTEM_PROMPT = (
    "You are a compensation analyst. Estimate realistic salary ranges based on job "
    "title, company, and location. Look up comparable bands and read the full "
    "posting before answering — an estimate made without checking is a guess."
)

DESCRIPTION_TRUNCATE_CHARS = 4000

UNKNOWN_LOCATION = "n/a"

_TASK = "Estimate the annual salary range for this job in the local currency."

_OUTPUT_RULES = (
    "Return a single SalaryBand JSON object. Use the most likely currency for "
    "the location. Set confidence based on how certain you are (0.3-0.9). If "
    "you cannot make a reasonable estimate, set min=0, max=0, confidence=0."
)

_POSTING_ID = "The id of this posting is {job_id}."


def _go_bool(value: bool) -> str:
    """Go renders a `bool` into the prompt as `true`/`false`; Python's
    `str(True)` would send `True` and drift from the baseline."""
    return "true" if value else "false"


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
    description at 4000 characters exactly as the Go source does. A missing
    or empty location is reported as `n/a` rather than omitted, so the model
    is told the location is unknown instead of inferring one."""
    prompt = PromptBuilder()
    prompt.paragraph(_TASK)
    prompt.line("JOB:").field("Title", title).field("Company", company)
    prompt.field("Location", f"{location or UNKNOWN_LOCATION} (remote: {_go_bool(remote)})")
    prompt.line("Description:")
    prompt.paragraph(truncate(description, DESCRIPTION_TRUNCATE_CHARS))
    prompt.paragraph(_OUTPUT_RULES)
    prompt.text(_POSTING_ID.format(job_id=job_id))
    return prompt.render()
