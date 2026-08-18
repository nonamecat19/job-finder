"""The `generation-summary*` stage prompt (C6-1), ported **verbatim** from
`buildSummaryPrompt` in
`apps/api/internal/generation/application/rendercv_llm.go` — same wording,
same structure, same field order (C8-1, C8-2, FR-021). The one prompt builder
serves the `standard`, `premium` and `fast` summary options alike; only the
task key (and therefore the model) they are bound to differs (034/035).
"""

from __future__ import annotations

SYSTEM_PROMPT = (
    "You are an expert resume writer who never fabricates information. You write a concise "
    "professional summary using only the facts you are given."
)


def build_prompt(
    *,
    skill_group_labels: list[str],
    highlights: list[str],
    sentence_min: int,
    sentence_max: int,
    total_years: int,
    previous_violations: list[str] | None = None,
) -> str:
    """Verbatim port of `buildSummaryPrompt`. The master profile is
    deliberately absent from the arguments — the brief carries only the
    handful of facts a summary can legitimately reference (035 FR-004)."""
    parts: list[str] = []
    parts.append("Write a professional summary about the candidate.\n\n")
    if skill_group_labels:
        parts.append("CANDIDATE SKILL AREAS: " + ", ".join(skill_group_labels) + "\n")
    if highlights:
        parts.append("\nCANDIDATE ACHIEVEMENTS (the only achievements you may reference):\n")
        for h in highlights:
            parts.append("  - " + h + "\n")
    parts.append(f"\nWrite {sentence_min}-{sentence_max} sentences that:\n")
    parts.append(
        f'- Open with "{total_years}+ years of experience" (derived from the candidate\'s dates; '
        "use it verbatim) and domain expertise\n"
    )
    parts.append(
        "- Summarize the candidate's background and strengths, drawing from the skill areas and "
        "achievements above\n"
    )
    parts.append(
        "- Never use a seniority label (e.g. 'mid-level', 'senior') in place of the years figure\n"
    )
    parts.append(
        "- Introduce no skill, employer, credential or metric that does not appear above\n"
    )
    if previous_violations:
        parts.append("\nYour previous attempt violated these grounding rules:\n- ")
        parts.append("\n- ".join(previous_violations))
        parts.append("\nRewrite without them.")
    return "".join(parts)
