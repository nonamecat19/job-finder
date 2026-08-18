from __future__ import annotations

from jobfinder_ai.prompts import match as prompts


def test_system_prompt_matches_go_verbatim() -> None:
    assert prompts.SYSTEM_PROMPT == (
        "You are a precise technical recruiter. Judge only from the given profile and job text."
    )


def test_user_prompt_matches_go_shape() -> None:
    text = prompts.build_user_prompt(
        profile_text="Senior Backend Engineer.",
        title="Staff Backend Engineer",
        company="Acme Corp",
        location="Remote",
        remote=True,
        description="Build distributed systems.",
    )
    assert text == (
        "Rate how well this candidate fits this job.\n\n"
        "CANDIDATE PROFILE:\nSenior Backend Engineer.\n\n"
        "JOB POSTING:\nTitle: Staff Backend Engineer\nCompany: Acme Corp\n"
        "Location: Remote (remote: true)\n"
        "Description:\nBuild distributed systems.\n\n"
        "Scoring guide: 90-100 near-perfect fit; 70-89 strong fit, minor gaps; "
        "50-69 partial fit, notable gaps; below 50 poor fit. "
        "matchedSkills/missingSkills = concrete skills from the job description. "
        "redFlags = concerns like seniority mismatch, hard requirements the candidate lacks, "
        "suspicious posting. summary = 2-3 sentences."
    )


def test_user_prompt_with_remote_false() -> None:
    text = prompts.build_user_prompt(
        profile_text="p",
        title="t",
        company="c",
        location="n/a",
        remote=False,
        description="d",
    )
    assert "Location: n/a (remote: false)" in text


def test_schema_instruction_and_retry_instruction_shape() -> None:
    schema = '{"type": "object"}'
    instruction = prompts.schema_instruction(schema)
    expected = "\n\nRespond with a single JSON object matching this JSON Schema:\n" + schema
    assert instruction == expected

    retry = prompts.retry_instruction(schema, "not valid JSON")
    assert retry.startswith(instruction)
    assert "Your previous answer was invalid: not valid JSON" in retry
    assert retry.endswith("Fix it and answer again with valid JSON only.")
