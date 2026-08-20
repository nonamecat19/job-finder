"""The input matrix the prompt golden files (tests/golden/prompts) are
rendered from.

Every case here is a *shape* of a prompt, not an interesting value: the
branch where a signal is unknown, the branch where a caller supplies no
hints, the branch where a previous attempt was rejected. Adding a case means
adding a golden file; changing one means the golden it renders must change
with it, which is exactly the review moment these files exist to force.

Inputs are kept short on purpose — truncation limits are asserted directly in
`test_prompt_parity.py` rather than baked into multi-kilobyte goldens.
"""

from __future__ import annotations

from collections.abc import Callable
from typing import Any

SCHEMA = '{"type":"object","properties":{"score":{"type":"number"}}}'
LAST_ERROR = "expected object, got array"

GHOST_CASES: dict[str, dict[str, Any]] = {
    "ghost__all_signals_known": dict(
        title="Senior Backend Engineer",
        company="Acme Corp",
        repost_count=2,
        days_open=10,
        cross_board_count=1,
        always_hiring_count=3,
    ),
    "ghost__all_optional_signals_unknown": dict(
        title="Role",
        company="Co",
        repost_count=0,
        days_open=None,
        cross_board_count=None,
        always_hiring_count=None,
    ),
    "ghost__mixed_signals": dict(
        title="Data Scientist",
        company="Globex",
        repost_count=7,
        days_open=90,
        cross_board_count=None,
        always_hiring_count=1,
    ),
}

MATCH_CASES: dict[str, dict[str, Any]] = {
    "match__remote": dict(
        profile_text="10 years Go and Python.",
        title="Staff Engineer",
        company="Initech",
        location="Berlin",
        remote=True,
        description="Build distributed systems.",
    ),
    "match__onsite_all_fields_empty": dict(
        profile_text="",
        title="",
        company="",
        location="",
        remote=False,
        description="",
    ),
}

SALARY_CASES: dict[str, dict[str, Any]] = {
    "salary__located_remote": dict(
        job_id="job-123",
        title="Backend Engineer",
        company="Acme",
        location="Amsterdam",
        remote=True,
        description="Own the payments service.",
    ),
    "salary__location_missing": dict(
        job_id="job-9",
        title="SRE",
        company="Globex",
        location=None,
        remote=False,
        description="Short description.",
    ),
    "salary__location_empty_string": dict(
        job_id="job-0",
        title="QA",
        company="Initech",
        location="",
        remote=False,
        description="",
    ),
}

OUTREACH_CASES: dict[str, dict[str, Any]] = {
    "outreach__warm_named_contact": dict(
        tone="warm",
        contact_name="Dana Scully",
        company_name="Acme Corp",
        facts=[("tech", "Go"), ("funding", "Series B")],
        last_violation="",
    ),
    "outreach__direct_anonymous_after_violation": dict(
        tone="direct",
        contact_name="",
        company_name="",
        facts=[],
        last_violation="invented a headcount",
    ),
    "outreach__formal": dict(
        tone="formal",
        contact_name="Fox Mulder",
        company_name="Globex",
        facts=[("rating", "4.5")],
        last_violation="",
    ),
}

RECRUITER_CASES: dict[str, dict[str, Any]] = {
    "recruiter__posting": dict(source="posting", text="Reach out to Jane at jane@acme.com."),
    "recruiter__company_page": dict(source="company_page", text="Our team page."),
    "recruiter__linkedin_empty_text": dict(source="linkedin", text=""),
}

REPHRASE_CASES: dict[str, dict[str, Any]] = {
    "rephrase__labelled_after_violations": dict(
        term="terraform",
        canonical="Terraform",
        source_bullet="Automated AWS infrastructure with Docker and Kubernetes.",
        source_label="Junior DevOps, Acme, 2022-2024",
        prior_violations=["invented Terraform", "inflated seniority"],
    ),
    "rephrase__unlabelled": dict(
        term="kafka",
        canonical="",
        source_bullet="Built event pipelines with RabbitMQ.",
        source_label=None,
        prior_violations=[],
    ),
    "rephrase__unlabelled_after_violations": dict(
        term="k8s",
        canonical="Kubernetes",
        source_bullet="Deployed containers.",
        source_label=None,
        prior_violations=["invented Kubernetes"],
    ),
}

ANALYZE_CASES: dict[str, dict[str, Any]] = {
    "analyze__no_hints": dict(vacancy="We need a backend engineer."),
    "analyze__all_hints": dict(
        vacancy="Backend role, Go and Postgres.",
        required_skills=["Go", "Postgres"],
        nice_to_have=["Kafka"],
        experience_level="senior",
    ),
    "analyze__partial_hints": dict(
        vacancy="Short vacancy.",
        required_skills=[],
        nice_to_have=["Terraform"],
        experience_level=None,
    ),
}

COVER_LETTER_CASES: dict[str, dict[str, Any]] = {
    "cover_letter__with_extra_notes": dict(
        profile_text="Backend engineer, 10 years.",
        extra_notes="Prefers remote.",
        company="Acme",
        title="Engineer",
        vacancy_text="Own the payments service.",
    ),
    "cover_letter__without_extra_notes": dict(
        profile_text="Short profile.",
        extra_notes=None,
        company="Globex",
        title="SRE",
        vacancy_text="Vacancy.",
    ),
}

_ANALYSIS = {
    "requiredSkills": ["Go", "Kubernetes"],
    "niceToHaveSkills": ["Terraform"],
    "experienceLevel": "senior",
    "keyResponsibilities": ["Own services", "Mentor"],
    "industryKeywords": ["fintech"],
    "seniorityKeywords": ["mentor"],
}

_ANALYSIS_MINIMAL = {"requiredSkills": [], "experienceLevel": ""}

_SKILLS = [
    {"label": "Languages", "details": "Go, Python"},
    {"label": "Cloud", "details": "AWS"},
]

_EXPERIENCE = [
    {
        "company": "Acme",
        "position": "Senior Engineer",
        "location": "Berlin",
        "highlights": ["Built X", "Scaled Y"],
    },
    {"company": "Globex", "highlights": []},
]

_PROJECTS = [
    {"name": "Job Finder", "highlights": ["Wrote the graph", "Shipped it"]},
    {"name": "Bare", "highlights": []},
]

SELECT_CASES: dict[str, dict[str, Any]] = {
    "select__strict_full_shape": dict(
        skills=_SKILLS,
        experience=_EXPERIENCE,
        projects=_PROJECTS,
        analysis=_ANALYSIS,
        level="strict",
        prev_violations=["added Rust"],
        cfg={
            "skillsEnabled": True,
            "experienceBulletsMin": 3,
            "experienceBulletsMax": 6,
            "projectsMax": 2,
            "projectBulletsMax": 3,
        },
    ),
    "select__moderate_without_projects": dict(
        skills=_SKILLS,
        experience=_EXPERIENCE,
        projects=[],
        analysis=_ANALYSIS,
        level="moderate",
        prev_violations=None,
        cfg={"skillsEnabled": True, "experienceBulletsMin": 2, "experienceBulletsMax": 4},
    ),
    "select__aggressive_minimal": dict(
        skills=[],
        experience=[],
        projects=_PROJECTS,
        analysis=_ANALYSIS_MINIMAL,
        level="aggressive",
        prev_violations=[],
        cfg={"skillsEnabled": False, "experienceBulletsMin": 0, "experienceBulletsMax": 0},
    ),
    "select__projects_capped_by_bullets_only": dict(
        skills=_SKILLS,
        experience=_EXPERIENCE,
        projects=_PROJECTS,
        analysis=_ANALYSIS,
        level="strict",
        prev_violations=None,
        cfg={
            "skillsEnabled": False,
            "experienceBulletsMin": 1,
            "experienceBulletsMax": 5,
            "projectsMax": 0,
            "projectBulletsMax": 2,
        },
    ),
}

SUMMARY_CASES: dict[str, dict[str, Any]] = {
    "summary__full_after_violations": dict(
        skill_group_labels=["Languages", "Cloud"],
        highlights=["Built X", "Scaled Y"],
        sentence_min=2,
        sentence_max=4,
        total_years=10,
        previous_violations=["invented a degree"],
    ),
    "summary__no_skills_no_highlights": dict(
        skill_group_labels=[],
        highlights=[],
        sentence_min=1,
        sentence_max=2,
        total_years=0,
        previous_violations=None,
    ),
}


def all_cases() -> list[tuple[str, Callable[[], str]]]:
    """Every prompt fragment under golden protection, as (name, thunk).

    Imports are function-local so the same matrix can also be rendered
    against a checkout of the pre-refactor modules when regenerating.
    """
    from jobfinder_ai.prompts import ghost, match, outreach, recruiter, rephrase, salary
    from jobfinder_ai.prompts.generation import analyze, cover_letter, select, summary

    cases: list[tuple[str, Callable[[], str]]] = []

    def add(matrix: dict[str, dict[str, Any]], build: Callable[..., str]) -> None:
        for name, kwargs in matrix.items():
            cases.append((name, lambda build=build, kwargs=kwargs: build(**kwargs)))

    add(GHOST_CASES, ghost.build_user_prompt)
    add(MATCH_CASES, match.build_user_prompt)
    add(SALARY_CASES, salary.build_user_prompt)
    add(OUTREACH_CASES, outreach.build_user_prompt)
    add(RECRUITER_CASES, recruiter.build_user_prompt)
    add(REPHRASE_CASES, rephrase.build_user_prompt)
    add(SUMMARY_CASES, summary.build_prompt)
    add(SELECT_CASES, select.build_prompt)
    add(COVER_LETTER_CASES, cover_letter.build_prompt)

    for name, kwargs in ANALYZE_CASES.items():
        vacancy = kwargs["vacancy"]
        hints = {key: value for key, value in kwargs.items() if key != "vacancy"}
        cases.append((name, lambda v=vacancy, h=hints: analyze.build_prompt(v, **h)))

    # The three json_object capabilities each re-export the shared
    # structured-output turns; all three stay under golden protection so a
    # future divergence between them shows up as a failing file.
    for module_name, module in (("ghost", ghost), ("match", match), ("salary", salary)):
        cases.append(
            (f"{module_name}__schema_instruction", lambda m=module: m.schema_instruction(SCHEMA))
        )
        cases.append(
            (
                f"{module_name}__retry_instruction",
                lambda m=module: m.retry_instruction(SCHEMA, LAST_ERROR),
            )
        )

    for module_name, module in (
        ("ghost", ghost),
        ("match", match),
        ("salary", salary),
        ("outreach", outreach),
        ("recruiter", recruiter),
        ("rephrase", rephrase),
        ("analyze", analyze),
        ("select", select),
        ("summary", summary),
        ("cover_letter", cover_letter),
    ):
        cases.append((f"{module_name}__system_prompt", lambda m=module: m.SYSTEM_PROMPT))

    return cases
