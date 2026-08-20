"""The `generation-analyze` stage prompt (C6-1), ported **verbatim** from
`buildAnalyzePrompt` in
`apps/api/internal/generation/application/rendercv_llm.go` — same wording,
same structure, same field order (C8-1, C8-2, FR-021).

Hints are the caller's prior guess at the vacancy's requirements, not
ground truth: when any are present the prompt asks the model to validate
and correct them against the vacancy text rather than accept them, which is
why the whole hint block collapses to nothing when the caller has none.
"""

from __future__ import annotations

from jobfinder_ai.prompts.composition import PromptBuilder, truncate

__all__ = ["SYSTEM_PROMPT", "VACANCY_TRUNCATE_RUNES", "build_prompt"]

SYSTEM_PROMPT = (
    "You are a job-market analyst who extracts structured requirements from "
    "vacancy descriptions. Be precise and concise."
)

VACANCY_TRUNCATE_RUNES = 6000

_TASK = "Analyze this job vacancy and extract structured requirements."

_HINTS_HEADER = "PROVIDED HINTS (validate and refine these):"
_HINTS_INSTRUCTION = (
    "Verify the hints against the vacancy text. Add any missing "
    "required/nice-to-have skills. Correct the experience level if the "
    "hints seem wrong."
)

_OUTPUT_HEADER = "Return a VacancyAnalysis with:"
_OUTPUT_FIELDS = (
    "requiredSkills: skills explicitly listed as required/mandatory",
    "niceToHaveSkills: preferred but not required",
    "experienceLevel: one of junior|mid|senior|lead|staff|principal",
    "keyResponsibilities: top 8-10 responsibilities",
    "industryKeywords: domain terms (e.g. fintech, healthcare, SaaS, e-commerce)",
    ("seniorityKeywords: leadership indicators (e.g. mentor, lead team, architecture decisions)"),
)

_HINT_INDENT = "  "


def build_prompt(
    vacancy: str,
    *,
    required_skills: list[str] | None = None,
    nice_to_have: list[str] | None = None,
    experience_level: str | None = None,
) -> str:
    """Verbatim port of `buildAnalyzePrompt`. `required_skills`/`nice_to_have`/
    `experience_level` mirror `domain.VacancyHints`; a caller with no hints
    passes none of them, matching the Go `hints == nil` branch."""
    prompt = PromptBuilder()
    prompt.paragraph(_TASK)
    prompt.line("VACANCY TEXT:")
    prompt.line(truncate(vacancy, VACANCY_TRUNCATE_RUNES))

    if required_skills or nice_to_have or experience_level:
        prompt.blank().line(_HINTS_HEADER)
        if required_skills:
            prompt.field(f"{_HINT_INDENT}Required skills (provided)", ", ".join(required_skills))
        if nice_to_have:
            prompt.field(f"{_HINT_INDENT}Nice-to-have skills (provided)", ", ".join(nice_to_have))
        if experience_level:
            prompt.field(f"{_HINT_INDENT}Experience level (provided)", experience_level)
        prompt.blank().line(_HINTS_INSTRUCTION)

    prompt.blank().line(_OUTPUT_HEADER)
    # The final field carries no trailing newline: the Go original ends the
    # prompt on it, and a stray newline here would be a parity drift.
    *leading, last = _OUTPUT_FIELDS
    prompt.bullets(leading).text(f"- {last}")

    return prompt.render()
