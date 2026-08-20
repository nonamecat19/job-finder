"""The `generation` capability's cover-letter branch prompt (C6-1), ported
**verbatim** from `apps/api/internal/generation/application/service.go`'s
`writeCoverLetter` — same wording, same structure, same truncation lengths
(C8-1, C8-2, FR-021). Unlike `analyze`/`select`/`summary`, this branch is a
single LLM call that never enters the analyze -> select -> summarize ->
assemble pipeline (service.go line 788).

Each input has its own truncation budget, and they are not
interchangeable: the profile is the only sanctioned source of the concrete
experiences the letter may cite, so it gets the largest budget, while the
vacancy and the user's notes are context.
"""

from __future__ import annotations

from jobfinder_ai.prompts.composition import PromptBuilder, truncate

__all__ = [
    "EXTRA_NOTES_TRUNCATE_CHARS",
    "MAX_TOKENS",
    "PROFILE_TRUNCATE_CHARS",
    "SYSTEM_PROMPT",
    "VACANCY_TRUNCATE_CHARS",
    "build_prompt",
]

SYSTEM_PROMPT = "You write concise, concrete, honest cover letters."

PROFILE_TRUNCATE_CHARS = 8000
EXTRA_NOTES_TRUNCATE_CHARS = 1500
VACANCY_TRUNCATE_CHARS = 4000

MAX_TOKENS = 1024

_TASK = (
    "Write a short cover letter (maximum 150 words, exactly 3 paragraphs separated by "
    "blank lines) for this application."
)

_STRUCTURE = (
    "Structure: (1) hook referencing the company and role, (2) 2-3 concrete matching "
    "experiences from the candidate's real background, (3) brief close."
)

_RULES = (
    "STRICT RULES: mention only experience present in the profile below; no invented "
    'facts, no clichés like "I am writing to express". Plain text, no salutation '
    'placeholders like [Hiring Manager] — use "Hello," if needed.'
)


def build_prompt(
    *,
    profile_text: str,
    extra_notes: str | None,
    company: str,
    title: str,
    vacancy_text: str,
) -> str:
    """Verbatim port of `writeCoverLetter`'s prompt assembly. Every literal
    string in this module MUST match the Go source byte for byte."""
    prompt = PromptBuilder()
    prompt.paragraph(_TASK)
    prompt.line(_STRUCTURE).paragraph(_RULES)
    prompt.line("CANDIDATE PROFILE:")
    prompt.paragraph(truncate(profile_text, PROFILE_TRUNCATE_CHARS))

    if extra_notes:
        prompt.line("EXTRA NOTES:")
        prompt.paragraph(truncate(extra_notes, EXTRA_NOTES_TRUNCATE_CHARS))

    prompt.line("JOB:").field("Title", title).field("Company", company)
    prompt.line("Description:")
    prompt.text(truncate(vacancy_text, VACANCY_TRUNCATE_CHARS))
    return prompt.render()
