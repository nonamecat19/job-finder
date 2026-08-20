"""The `generation-summary*` stage prompt (C6-1), ported **verbatim** from
`buildSummaryPrompt` in
`apps/api/internal/generation/application/rendercv_llm.go` — same wording,
same structure, same field order (C8-1, C8-2, FR-021). The one prompt builder
serves the `standard`, `premium` and `fast` summary options alike; only the
task key (and therefore the model) they are bound to differs (034/035).
"""

from __future__ import annotations

from jobfinder_ai.prompts.composition import PromptBuilder

__all__ = ["SYSTEM_PROMPT", "build_prompt"]

SYSTEM_PROMPT = (
    "You are an expert resume writer who never fabricates information. You write a concise "
    "professional summary using only the facts you are given."
)

_TASK = "Write a professional summary about the candidate."

_SKILL_AREAS_LABEL = "CANDIDATE SKILL AREAS"
_ACHIEVEMENTS_HEADER = "CANDIDATE ACHIEVEMENTS (the only achievements you may reference):"
_ACHIEVEMENT_INDENT = "  "

_INSTRUCTION_HEADER = "Write {sentence_min}-{sentence_max} sentences that:"

_YEARS_RULE = (
    'Open with "{total_years}+ years of experience" (derived from the candidate\'s dates; '
    "use it verbatim) and domain expertise"
)
_BACKGROUND_RULE = (
    "Summarize the candidate's background and strengths, drawing from the skill areas and "
    "achievements above"
)
_NO_SENIORITY_LABEL_RULE = (
    "Never use a seniority label (e.g. 'mid-level', 'senior') in place of the years figure"
)
_NO_INVENTION_RULE = "Introduce no skill, employer, credential or metric that does not appear above"

_VIOLATION_HEADER = "Your previous attempt violated these grounding rules:"
_VIOLATION_FOOTER = "Rewrite without them."


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
    handful of facts a summary can legitimately reference (035 FR-004), and
    the rules below forbid reaching past them."""
    prompt = PromptBuilder()
    prompt.paragraph(_TASK)

    if skill_group_labels:
        prompt.field(_SKILL_AREAS_LABEL, ", ".join(skill_group_labels))
    if highlights:
        prompt.blank().line(_ACHIEVEMENTS_HEADER)
        prompt.bullets(highlights, indent=_ACHIEVEMENT_INDENT)

    prompt.blank()
    prompt.line(_INSTRUCTION_HEADER.format(sentence_min=sentence_min, sentence_max=sentence_max))
    prompt.bullet(_YEARS_RULE.format(total_years=total_years))
    prompt.bullet(_BACKGROUND_RULE)
    prompt.bullet(_NO_SENIORITY_LABEL_RULE)
    prompt.bullet(_NO_INVENTION_RULE)

    if previous_violations:
        prompt.blank().line(_VIOLATION_HEADER)
        prompt.text("- ").joined(previous_violations, separator="\n- ")
        prompt.line().text(_VIOLATION_FOOTER)

    return prompt.render()
