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
"""

from __future__ import annotations

SYSTEM_PROMPT = (
    "You are a skeptical but fair job-market analyst. You judge only from the "
    "measured numbers given to you. You never assert anything about the employer, "
    "the role, or hiring intent that the numbers do not support."
)


def build_user_prompt(
    *,
    title: str,
    company: str,
    repost_count: int,
    days_open: int | None,
    cross_board_count: int | None,
    always_hiring_count: int | None,
) -> str:
    """Verbatim port of `buildPrompt` (service.go). Every literal string below
    MUST match the Go source byte for byte."""
    parts: list[str] = []

    parts.append(
        'Rate how likely this job posting is a "ghost job" — a posting the '
        "employer has no real intent to fill. Score 0-100, where higher means more "
        "suspicious. Base your score ONLY on the measured signals below; do not invent "
        "facts about the employer, the role, or their hiring intent.\n\n"
    )
    parts.append(f"JOB:\nTitle: {title}\nCompany: {company}\n\n")
    parts.append("MEASURED SIGNALS:\n")
    parts.append(
        "- Repost count (times this exact posting has reappeared across "
        f"ingestion runs): {repost_count}\n"
    )
    if days_open is not None:
        parts.append(
            f"- Days open: {days_open} (a posting older than 45 days with no user "
            "progression contributes to suspicion, but age alone is never sufficient for "
            "a high score)\n"
        )
    else:
        parts.append(
            "- Days open: unknown (no posting date available) — treat this as "
            "missing information, not as zero or infinite, and lower your confidence\n"
        )
    if cross_board_count is not None:
        parts.append(
            "- Cross-board duplicates (same description on other sources, last "
            f"60 days): {cross_board_count}. IMPORTANT: a legitimate recruiter or agency "
            "often cross-posts one JD to several boards — this signal alone, even if "
            "nonzero, MUST NOT push the score into the 80-100 band. It only matters when "
            "it compounds with repost count or the always-hiring count.\n"
        )
    else:
        parts.append(
            "- Cross-board duplicates: unknown (description too short/empty) — "
            "lower your confidence\n"
        )
    if always_hiring_count is not None:
        parts.append(
            "- Always-hiring count (other postings from this company in the "
            "last 90 days that never progressed past initial discovery, including this "
            f"one): {always_hiring_count}. IMPORTANT: a value of 1 means this is the ONLY "
            "posting seen from this company — that is the normal case, not a red flag, "
            "and MUST NOT be treated as evidence of ghosting.\n"
        )
    else:
        parts.append(
            "- Always-hiring count: unknown (company name unparseable) — lower "
            "your confidence\n"
        )
    parts.append(
        "\nReport your confidence (0-1) in the score, lowering it for every "
        "signal marked unknown above. Explain, in plain English, which of the measured "
        "signals drove your score — reference only the numbers given."
    )
    return "".join(parts)


def schema_instruction(schema: str) -> str:
    """Verbatim port of the trailing turn `CompleteStructuredChat` appends to
    the final message (port.go): the schema-in-prompt instruction that makes
    `response_format: json_object` reliable without strict schema mode."""
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
