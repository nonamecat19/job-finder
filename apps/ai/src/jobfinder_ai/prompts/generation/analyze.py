"""The `generation-analyze` stage prompt (C6-1), ported **verbatim** from
`buildAnalyzePrompt` in
`apps/api/internal/generation/application/rendercv_llm.go` — same wording,
same structure, same field order (C8-1, C8-2, FR-021).
"""

from __future__ import annotations

SYSTEM_PROMPT = (
    "You are a job-market analyst who extracts structured requirements from "
    "vacancy descriptions. Be precise and concise."
)

# Ported from rendercv_llm.go's `strutil.Truncate(vacancy, 6000)`.
VACANCY_TRUNCATE_RUNES = 6000


def _truncate(text: str, limit: int) -> str:
    """Verbatim port of `strutil.Truncate`: truncates by rune count, not byte
    count, and returns the text unchanged when it is already short enough."""
    if limit <= 0:
        return ""
    if len(text) <= limit:
        return text
    return text[:limit]


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
    vac = _truncate(vacancy, VACANCY_TRUNCATE_RUNES)

    parts: list[str] = []
    parts.append("Analyze this job vacancy and extract structured requirements.\n\n")
    parts.append("VACANCY TEXT:\n")
    parts.append(vac)
    parts.append("\n")

    has_hints = bool(required_skills) or bool(nice_to_have) or bool(experience_level)
    if has_hints:
        parts.append("\nPROVIDED HINTS (validate and refine these):\n")
        if required_skills:
            parts.append("  Required skills (provided): ")
            parts.append(", ".join(required_skills))
            parts.append("\n")
        if nice_to_have:
            parts.append("  Nice-to-have skills (provided): ")
            parts.append(", ".join(nice_to_have))
            parts.append("\n")
        if experience_level:
            parts.append("  Experience level (provided): ")
            parts.append(experience_level)
            parts.append("\n")
        parts.append(
            "\nVerify the hints against the vacancy text. Add any missing "
            "required/nice-to-have skills. Correct the experience level if the "
            "hints seem wrong.\n"
        )

    parts.append("\nReturn a VacancyAnalysis with:\n")
    parts.append("- requiredSkills: skills explicitly listed as required/mandatory\n")
    parts.append("- niceToHaveSkills: preferred but not required\n")
    parts.append("- experienceLevel: one of junior|mid|senior|lead|staff|principal\n")
    parts.append("- keyResponsibilities: top 8-10 responsibilities\n")
    parts.append("- industryKeywords: domain terms (e.g. fintech, healthcare, SaaS, e-commerce)\n")
    parts.append(
        "- seniorityKeywords: leadership indicators (e.g. mentor, lead team, architecture "
        "decisions)"
    )

    return "".join(parts)
