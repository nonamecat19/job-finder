"""The `generation` capability's cover-letter branch prompt (C6-1), ported
**verbatim** from `apps/api/internal/generation/application/service.go`'s
`writeCoverLetter` — same wording, same structure, same truncation lengths
(C8-1, C8-2, FR-021). Unlike `analyze`/`select`/`summary`, this branch is a
single LLM call that never enters the analyze -> select -> summarize ->
assemble pipeline (service.go line 788).
"""

from __future__ import annotations

SYSTEM_PROMPT = "You write concise, concrete, honest cover letters."

# Verbatim port of service.go's strutil.Truncate call sites.
PROFILE_TRUNCATE_CHARS = 8000
EXTRA_NOTES_TRUNCATE_CHARS = 1500
VACANCY_TRUNCATE_CHARS = 4000

# Verbatim port of service.go's `coverLetterMaxTokens` (033 FR-012).
MAX_TOKENS = 1024


def _truncate(text: str, n: int) -> str:
    """Verbatim port of Go's `strutil.Truncate`: a plain prefix cut, no
    ellipsis, no-op when `text` already fits."""
    return text[:n]


def build_prompt(
    *,
    profile_text: str,
    extra_notes: str | None,
    company: str,
    title: str,
    vacancy_text: str,
) -> str:
    """Verbatim port of `writeCoverLetter`'s prompt assembly. Every literal
    string below MUST match the Go source byte for byte."""
    prompt = (
        "Write a short cover letter (maximum 150 words, exactly 3 paragraphs separated by "
        "blank lines) for this application.\n\n"
        "Structure: (1) hook referencing the company and role, (2) 2-3 concrete matching "
        "experiences from the candidate's real background, (3) brief close.\n"
        "STRICT RULES: mention only experience present in the profile below; no invented "
        'facts, no clichés like "I am writing to express". Plain text, no salutation '
        'placeholders like [Hiring Manager] — use "Hello," if needed.' + "\n\n"
        "CANDIDATE PROFILE:\n" + _truncate(profile_text, PROFILE_TRUNCATE_CHARS) + "\n\n"
    )
    if extra_notes:
        prompt += "EXTRA NOTES:\n" + _truncate(extra_notes, EXTRA_NOTES_TRUNCATE_CHARS) + "\n\n"
    prompt += (
        f"JOB:\nTitle: {title}\nCompany: {company}\n"
        f"Description:\n{_truncate(vacancy_text, VACANCY_TRUNCATE_CHARS)}"
    )
    return prompt
