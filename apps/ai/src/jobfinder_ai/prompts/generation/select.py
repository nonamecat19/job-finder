"""The `generation-select` / `generation-select-premium` stage prompt
(C6-1), ported **verbatim** from `buildSelectPrompt` and `domain.LevelRules`
in `apps/api/internal/generation/application/rendercv_llm.go` and
`apps/api/internal/generation/domain/rendercv.go` — same wording, same
structure, same field order (C8-1, C8-2, FR-021). Both the economy and the
premium selection call share this one prompt builder, exactly as the Go
`selectContent`/escalation ladder shares `buildSelectPrompt`.
"""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any, TypedDict

SYSTEM_PROMPT = (
    "You are an expert resume writer who never fabricates information. "
    "You select, reorder and rephrase existing content to match a specific vacancy."
)

LEVEL_RULES: dict[str, str] = {
    "strict": (
        "GROUNDING = STRICT. Use ONLY skills and facts already present in the master profile. "
        "You may reorder, trim and rephrase, but you must NOT introduce any technology, tool or "
        "skill token that does not already appear in the master. Do not invent achievements."
    ),
    "moderate": (
        "GROUNDING = MODERATE. You may reorder, trim and rephrase freely, and you MAY add a skill "
        "or reframe a highlight for technology that is directly ADJACENT to the existing stack "
        "(e.g. the vacancy asks for Terraform and the master already lists AWS, Docker and "
        "Kubernetes). Never add technology unrelated to the demonstrated experience, and never "
        "invent employers, dates, projects or metrics."
    ),
    "aggressive": (
        "GROUNDING = AGGRESSIVE. Maximize keyword match with the vacancy: you may add any skills "
        "the vacancy requires and frame highlights toward them. Still never invent employers, "
        "dates, degrees or numeric metrics that are not in the master."
    ),
}


class VacancyAnalysisLike(TypedDict, total=False):
    requiredSkills: list[str]
    niceToHaveSkills: list[str]
    experienceLevel: str
    keyResponsibilities: list[str]
    industryKeywords: list[str]
    seniorityKeywords: list[str]


class ShapeConfigLike(TypedDict, total=False):
    skillsEnabled: bool
    experienceBulletsMin: int
    experienceBulletsMax: int
    projectsMax: int
    projectBulletsMax: int


def _at_least_one(n: int) -> int:
    return n if n >= 1 else 1


def _projects_limited(cfg: Mapping[str, Any]) -> bool:
    return bool(cfg.get("projectsMax", 0) > 0 or cfg.get("projectBulletsMax", 0) > 0)


def render_analysis_lines(analysis: Mapping[str, Any]) -> list[str]:
    """Verbatim port of `renderAnalysisLines`."""
    lines = ["REQUIRED SKILLS: " + ", ".join(analysis.get("requiredSkills") or [])]
    nice = analysis.get("niceToHaveSkills") or []
    if nice:
        lines.append("NICE-TO-HAVE: " + ", ".join(nice))
    lines.append("EXPERIENCE LEVEL: " + (analysis.get("experienceLevel") or ""))
    responsibilities = analysis.get("keyResponsibilities") or []
    if responsibilities:
        lines.append("KEY RESPONSIBILITIES:")
        lines.extend("  - " + r for r in responsibilities)
    industry = analysis.get("industryKeywords") or []
    if industry:
        lines.append("INDUSTRY: " + ", ".join(industry))
    seniority = analysis.get("seniorityKeywords") or []
    if seniority:
        lines.append("SENIORITY SIGNALS: " + ", ".join(seniority))
    return lines


def render_skill_group_lines(skills: list[dict[str, Any]]) -> list[str]:
    """Verbatim port of `renderSkillGroupLines`."""
    return [f"  [{i}] {s.get('label', '')}: {s.get('details', '')}" for i, s in enumerate(skills)]


def render_experience_entry_lines(entry: dict[str, Any]) -> list[str]:
    """Verbatim port of `renderExperienceEntryLines`."""
    line = "  - company: " + str(entry.get("company", ""))
    position = entry.get("position")
    if position:
        line += f" ({position})"
    location = entry.get("location")
    if location:
        line += " | " + str(location)
    lines = [line]
    highlights = entry.get("highlights") or []
    lines.extend(f"      [{i}] {h}" for i, h in enumerate(highlights))
    return lines


def build_prompt(
    *,
    skills: list[dict[str, Any]],
    experience: list[dict[str, Any]],
    projects: list[dict[str, Any]],
    analysis: Mapping[str, Any],
    level: str,
    prev_violations: list[str] | None,
    cfg: Mapping[str, Any],
) -> str:
    """Verbatim port of `buildSelectPrompt`."""
    analysis_lines = render_analysis_lines(analysis)
    skill_lines = render_skill_group_lines(skills)

    exp_lines: list[str] = []
    for e in experience:
        exp_lines.extend(render_experience_entry_lines(e))

    bullets_min = cfg.get("experienceBulletsMin", 0)
    bullets_max = _at_least_one(cfg.get("experienceBulletsMax", 0))

    parts: list[str] = []
    parts.append("Given this vacancy analysis, tailor the candidate's resume content.\n\n")
    parts.append("VACANCY ANALYSIS:\n")
    parts.append("\n".join(analysis_lines))
    parts.append("\n\n")
    parts.append(LEVEL_RULES[level])
    parts.append("\n\nHARD RULES (all levels):\n")
    parts.append(
        "- Return experience keyed by the EXACT company name shown below; do not add companies.\n"
    )
    parts.append(
        f"- For each experience entry, select the TOP {bullets_min}-{bullets_max} most relevant "
        "bullets by their [index], in the order they should appear.\n"
    )
    parts.append(
        "- A highlight is {sourceIndex, rephrased}. sourceIndex is the [index] of the bullet as "
        "shown. rephrased is optional: set it only to reword THAT bullet for this vacancy, and "
        "omit it to keep the master's wording.\n"
    )
    parts.append(
        "- A rewording never merges two bullets, never borrows from another entry, and never "
        "changes a number. A rewording that does is discarded and the original bullet is used.\n"
    )
    parts.append("- Keep every experience entry; never set drop to true. Do not omit any job.\n")
    parts.append(
        "- Keep experience entries in the EXACT order shown in the master; do not reorder.\n"
    )
    parts.append(
        "- Do NOT return skills. Skill selection and ordering are computed from the analysis "
        "above, not chosen here.\n"
    )
    parts.append("- Keep highlights concise, one achievement each.\n")
    parts.append(
        "- Do not drop, add, rename, or reorder any resume section. Keep the master's section set "
        "and order exactly as given.\n"
    )
    parts.append("- Do NOT write a summary. A separate step writes it.\n\n")

    if cfg.get("skillsEnabled", False):
        parts.append("SKILL GROUPS (master, reference only):\n")
        parts.append("\n".join(skill_lines))
        parts.append("\n")

    parts.append("\nEXPERIENCE (master):\n")
    parts.append("\n".join(exp_lines))

    if _projects_limited(cfg) and projects:
        parts.append("\n\nPROJECTS (master):\n")
        for p in projects:
            parts.append("  - name: " + str(p.get("name", "")) + "\n")
            for i, h in enumerate(p.get("highlights") or []):
                parts.append(f"      [{i}] {h}\n")
        projects_max = cfg.get("projectsMax", 0)
        if projects_max > 0:
            parts.append(f"Return the {projects_max} most vacancy-relevant projects")
        else:
            parts.append("Return the most vacancy-relevant projects")
        parts.append(", with each name copied EXACTLY as shown above.\n")
        project_bullets_max = cfg.get("projectBulletsMax", 0)
        if project_bullets_max > 0:
            parts.append(
                f"For each returned project, keep at most {project_bullets_max} highlights.\n"
            )
        parts.append(
            "Project highlights are indices into that same project's own bullet list above; an "
            "index cannot reach another project's bullets.\n"
        )

    if prev_violations:
        parts.append("\n\nYour previous attempt violated grounding rules:\n- ")
        parts.append("\n- ".join(prev_violations))
        parts.append("\nRegenerate without these violations.")

    return "".join(parts)
