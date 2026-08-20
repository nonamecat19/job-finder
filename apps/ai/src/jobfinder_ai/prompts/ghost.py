"""The `ghost` capability's prompt (C6-1), ported **verbatim** from
`apps/api/internal/ghostjob/application/service.go`'s `buildPrompt` and the
system message in `Service.ScoreJob` — same wording, same structure, same
field order. This is a migration, not a rewrite (C8-1, C8-2, FR-021): any
drift here changes the model's answers and breaks parity with the recorded
baseline.

Retains `response_format: {"type": "json_object"}` (C6-4) — the schema is
sent as an appended instruction on the final message rather than as a
strict `json_schema` response format, exactly matching the Go
`CompleteStructuredChat` legacy path (`ResponseModeJSON`) that
`ghostjob.Service.ScoreJob` uses today (it never sets `ResponseModeStrict`).
The two instruction builders it needs live in `prompts.structured` and are
re-exported here so each capability keeps one prompt module to import.

Every signal the scorer measures is modelled as a `_Signal`: a known value
and the "unknown" line to emit in its place. That pairing is the whole
point of the prompt — the model is told to lower its confidence for each
missing signal, so a signal whose unknown branch went missing would silently
overstate confidence rather than fail loudly.
"""

from __future__ import annotations

from dataclasses import dataclass

from jobfinder_ai.prompts.composition import PromptBuilder
from jobfinder_ai.prompts.structured import retry_instruction, schema_instruction

__all__ = [
    "SYSTEM_PROMPT",
    "build_user_prompt",
    "retry_instruction",
    "schema_instruction",
]

SYSTEM_PROMPT = (
    "You are a skeptical but fair job-market analyst. You judge only from the "
    "measured numbers given to you. You never assert anything about the employer, "
    "the role, or hiring intent that the numbers do not support."
)

_TASK = (
    'Rate how likely this job posting is a "ghost job" — a posting the '
    "employer has no real intent to fill. Score 0-100, where higher means more "
    "suspicious. Base your score ONLY on the measured signals below; do not invent "
    "facts about the employer, the role, or their hiring intent."
)

_SIGNALS_HEADER = "MEASURED SIGNALS:"

_REPOST_COUNT = (
    "Repost count (times this exact posting has reappeared across ingestion runs): {value}"
)

_DAYS_OPEN_KNOWN = (
    "Days open: {value} (a posting older than 45 days with no user "
    "progression contributes to suspicion, but age alone is never sufficient for "
    "a high score)"
)
_DAYS_OPEN_UNKNOWN = (
    "Days open: unknown (no posting date available) — treat this as "
    "missing information, not as zero or infinite, and lower your confidence"
)

_CROSS_BOARD_KNOWN = (
    "Cross-board duplicates (same description on other sources, last "
    "60 days): {value}. IMPORTANT: a legitimate recruiter or agency "
    "often cross-posts one JD to several boards — this signal alone, even if "
    "nonzero, MUST NOT push the score into the 80-100 band. It only matters when "
    "it compounds with repost count or the always-hiring count."
)
_CROSS_BOARD_UNKNOWN = (
    "Cross-board duplicates: unknown (description too short/empty) — lower your confidence"
)

_ALWAYS_HIRING_KNOWN = (
    "Always-hiring count (other postings from this company in the "
    "last 90 days that never progressed past initial discovery, including this "
    "one): {value}. IMPORTANT: a value of 1 means this is the ONLY "
    "posting seen from this company — that is the normal case, not a red flag, "
    "and MUST NOT be treated as evidence of ghosting."
)
_ALWAYS_HIRING_UNKNOWN = (
    "Always-hiring count: unknown (company name unparseable) — lower your confidence"
)

_CLOSING = (
    "Report your confidence (0-1) in the score, lowering it for every "
    "signal marked unknown above. Explain, in plain English, which of the measured "
    "signals drove your score — reference only the numbers given."
)


@dataclass(frozen=True, slots=True)
class _Signal:
    """One measured signal and the line that stands in for it when the
    measurement could not be taken."""

    known: str
    unknown: str

    def render(self, value: int | None) -> str:
        if value is None:
            return self.unknown
        return self.known.format(value=value)


_DAYS_OPEN = _Signal(known=_DAYS_OPEN_KNOWN, unknown=_DAYS_OPEN_UNKNOWN)
_CROSS_BOARD = _Signal(known=_CROSS_BOARD_KNOWN, unknown=_CROSS_BOARD_UNKNOWN)
_ALWAYS_HIRING = _Signal(known=_ALWAYS_HIRING_KNOWN, unknown=_ALWAYS_HIRING_UNKNOWN)


def build_user_prompt(
    *,
    title: str,
    company: str,
    repost_count: int,
    days_open: int | None,
    cross_board_count: int | None,
    always_hiring_count: int | None,
) -> str:
    """Verbatim port of `buildPrompt` (service.go). Every literal string in
    this module MUST match the Go source byte for byte."""
    prompt = PromptBuilder()
    prompt.paragraph(_TASK)
    prompt.line("JOB:").field("Title", title).field("Company", company).blank()
    prompt.line(_SIGNALS_HEADER)
    prompt.bullet(_REPOST_COUNT.format(value=repost_count))
    prompt.bullet(_DAYS_OPEN.render(days_open))
    prompt.bullet(_CROSS_BOARD.render(cross_board_count))
    prompt.bullet(_ALWAYS_HIRING.render(always_hiring_count))
    prompt.blank().text(_CLOSING)
    return prompt.render()
