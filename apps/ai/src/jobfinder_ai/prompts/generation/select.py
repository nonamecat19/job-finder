"""The `generation-select` / `generation-select-premium` stage prompt
(C6-1), ported **verbatim** from `buildSelectPrompt` and `domain.LevelRules`
in `apps/api/internal/generation/application/rendercv_llm.go` and
`apps/api/internal/generation/domain/rendercv.go` — same wording, same
structure, same field order (C8-1, C8-2, FR-021). Both the economy and the
premium selection call share this one prompt builder, exactly as the Go
`selectContent`/escalation ladder shares `buildSelectPrompt`.

Two things carry the safety of this stage. `LEVEL_RULES` is the grounding
contract, picked per request and stated before anything else the model
reads. The `[index]` numbering on every master bullet is the second: the
model answers with indices into the lists rendered here, so a selection can
only ever point back at content the candidate actually wrote — the renderers
below are what make that addressing scheme hold.
"""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any, TypedDict

from jobfinder_ai.prompts.composition import PromptBuilder

__all__ = [
    "LEVEL_RULES",
    "SYSTEM_PROMPT",
    "ShapeConfigLike",
    "VacancyAnalysisLike",
    "build_prompt",
    "render_analysis_lines",
    "render_experience_entry_lines",
    "render_skill_group_lines",
]

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

_TASK = "Given this vacancy analysis, tailor the candidate's resume content."

_HARD_RULES_HEADER = "HARD RULES (all levels):"

_RULE_EXACT_COMPANY = (
    "Return experience keyed by the EXACT company name shown below; do not add companies."
)
_RULE_BULLET_BUDGET = (
    "For each experience entry, select the TOP {bullets_min}-{bullets_max} most relevant "
    "bullets by their [index], in the order they should appear."
)
_RULE_HIGHLIGHT_SHAPE = (
    "A highlight is {sourceIndex, rephrased}. sourceIndex is the [index] of the bullet as "
    "shown. rephrased is optional: set it only to reword THAT bullet for this vacancy, and "
    "omit it to keep the master's wording."
)
_RULE_REWORDING_LIMITS = (
    "A rewording never merges two bullets, never borrows from another entry, and never "
    "changes a number. A rewording that does is discarded and the original bullet is used."
)
_RULE_KEEP_ENTRIES = "Keep every experience entry; never set drop to true. Do not omit any job."
_RULE_KEEP_ORDER = "Keep experience entries in the EXACT order shown in the master; do not reorder."
_RULE_NO_SKILLS = (
    "Do NOT return skills. Skill selection and ordering are computed from the analysis "
    "above, not chosen here."
)
_RULE_CONCISE = "Keep highlights concise, one achievement each."
_RULE_SECTIONS = (
    "Do not drop, add, rename, or reorder any resume section. Keep the master's section set "
    "and order exactly as given."
)
_RULE_NO_SUMMARY = "Do NOT write a summary. A separate step writes it."

_PROJECTS_LIMITED = "Return the {projects_max} most vacancy-relevant projects"
_PROJECTS_UNLIMITED = "Return the most vacancy-relevant projects"
_PROJECTS_NAME_RULE = ", with each name copied EXACTLY as shown above."
_PROJECT_BULLET_BUDGET = "For each returned project, keep at most {max_highlights} highlights."
_PROJECT_INDEX_SCOPE = (
    "Project highlights are indices into that same project's own bullet list above; an "
    "index cannot reach another project's bullets."
)

_VIOLATION_HEADER = "Your previous attempt violated grounding rules:"
_VIOLATION_FOOTER = "Regenerate without these violations."

_ENTRY_INDENT = "  "
_HIGHLIGHT_INDENT = "      "


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
    """The upper bullet bound is stated to the model as a range end, so a
    configured 0 would ask for `0-0` bullets; Go clamps it to 1."""
    return n if n >= 1 else 1


def _projects_limited(cfg: Mapping[str, Any]) -> bool:
    """Projects reach the prompt only when the shape config puts a cap on
    them — an uncapped shape leaves the whole PROJECTS block out."""
    return bool(cfg.get("projectsMax", 0) > 0 or cfg.get("projectBulletsMax", 0) > 0)


def render_analysis_lines(analysis: Mapping[str, Any]) -> list[str]:
    """Verbatim port of `renderAnalysisLines`. Empty optional sections are
    omitted rather than rendered as an empty label."""
    lines = [f"REQUIRED SKILLS: {', '.join(analysis.get('requiredSkills') or [])}"]
    if nice := analysis.get("niceToHaveSkills"):
        lines.append(f"NICE-TO-HAVE: {', '.join(nice)}")
    lines.append(f"EXPERIENCE LEVEL: {analysis.get('experienceLevel') or ''}")
    if responsibilities := analysis.get("keyResponsibilities"):
        lines.append("KEY RESPONSIBILITIES:")
        lines.extend(f"{_ENTRY_INDENT}- {item}" for item in responsibilities)
    if industry := analysis.get("industryKeywords"):
        lines.append(f"INDUSTRY: {', '.join(industry)}")
    if seniority := analysis.get("seniorityKeywords"):
        lines.append(f"SENIORITY SIGNALS: {', '.join(seniority)}")
    return lines


def render_skill_group_lines(skills: list[dict[str, Any]]) -> list[str]:
    """Verbatim port of `renderSkillGroupLines`."""
    return [
        f"{_ENTRY_INDENT}[{index}] {group.get('label', '')}: {group.get('details', '')}"
        for index, group in enumerate(skills)
    ]


def render_experience_entry_lines(entry: dict[str, Any]) -> list[str]:
    """Verbatim port of `renderExperienceEntryLines`: one header line naming
    the company (plus position and location when known), then the entry's
    highlights numbered from zero — those numbers are what a selection
    refers back to."""
    header = f"{_ENTRY_INDENT}- company: {entry.get('company', '')}"
    if position := entry.get("position"):
        header += f" ({position})"
    if location := entry.get("location"):
        header += f" | {location}"

    lines = [header]
    lines.extend(
        f"{_HIGHLIGHT_INDENT}[{index}] {highlight}"
        for index, highlight in enumerate(entry.get("highlights") or [])
    )
    return lines


def _append_hard_rules(prompt: PromptBuilder, *, bullets_min: int, bullets_max: int) -> None:
    prompt.line(_HARD_RULES_HEADER)
    prompt.bullet(_RULE_EXACT_COMPANY)
    prompt.bullet(_RULE_BULLET_BUDGET.format(bullets_min=bullets_min, bullets_max=bullets_max))
    prompt.bullet(_RULE_HIGHLIGHT_SHAPE)
    prompt.bullet(_RULE_REWORDING_LIMITS)
    prompt.bullet(_RULE_KEEP_ENTRIES)
    prompt.bullet(_RULE_KEEP_ORDER)
    prompt.bullet(_RULE_NO_SKILLS)
    prompt.bullet(_RULE_CONCISE)
    prompt.bullet(_RULE_SECTIONS)
    prompt.bullet(_RULE_NO_SUMMARY)
    prompt.blank()


def _append_projects(
    prompt: PromptBuilder, *, projects: list[dict[str, Any]], cfg: Mapping[str, Any]
) -> None:
    prompt.blank(2).line("PROJECTS (master):")
    for project in projects:
        prompt.line(f"{_ENTRY_INDENT}- name: {project.get('name', '')}")
        prompt.indexed(project.get("highlights") or [], indent=_HIGHLIGHT_INDENT)

    projects_max = cfg.get("projectsMax", 0)
    if projects_max > 0:
        prompt.text(_PROJECTS_LIMITED.format(projects_max=projects_max))
    else:
        prompt.text(_PROJECTS_UNLIMITED)
    prompt.line(_PROJECTS_NAME_RULE)

    project_bullets_max = cfg.get("projectBulletsMax", 0)
    if project_bullets_max > 0:
        prompt.line(_PROJECT_BULLET_BUDGET.format(max_highlights=project_bullets_max))
    prompt.line(_PROJECT_INDEX_SCOPE)


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
    """Verbatim port of `buildSelectPrompt`.

    `level` indexes `LEVEL_RULES` directly: an unknown level raises rather
    than falling back to a laxer rule set, matching the Go behaviour of only
    ever passing a validated `domain` level through.
    """
    exp_lines: list[str] = []
    for entry in experience:
        exp_lines.extend(render_experience_entry_lines(entry))

    prompt = PromptBuilder()
    prompt.paragraph(_TASK)
    prompt.line("VACANCY ANALYSIS:")
    prompt.joined(render_analysis_lines(analysis)).blank(2)
    prompt.text(LEVEL_RULES[level]).blank(2)

    _append_hard_rules(
        prompt,
        bullets_min=cfg.get("experienceBulletsMin", 0),
        bullets_max=_at_least_one(cfg.get("experienceBulletsMax", 0)),
    )

    if cfg.get("skillsEnabled", False):
        prompt.line("SKILL GROUPS (master, reference only):")
        prompt.joined(render_skill_group_lines(skills)).line()

    prompt.blank().line("EXPERIENCE (master):")
    prompt.joined(exp_lines)

    if _projects_limited(cfg) and projects:
        _append_projects(prompt, projects=projects, cfg=cfg)

    if prev_violations:
        prompt.blank(2).line(_VIOLATION_HEADER)
        prompt.text("- ").joined(prev_violations, separator="\n- ")
        prompt.line().text(_VIOLATION_FOOTER)

    return prompt.render()
