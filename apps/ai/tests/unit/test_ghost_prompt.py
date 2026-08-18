from __future__ import annotations

from jobfinder_ai.prompts import ghost as prompts


def test_system_prompt_matches_go_verbatim() -> None:
    assert prompts.SYSTEM_PROMPT == (
        "You are a skeptical but fair job-market analyst. You judge only from the "
        "measured numbers given to you. You never assert anything about the employer, "
        "the role, or hiring intent that the numbers do not support."
    )


def test_user_prompt_with_every_signal_known() -> None:
    text = prompts.build_user_prompt(
        title="Senior Backend Engineer",
        company="Acme Corp",
        repost_count=2,
        days_open=10,
        cross_board_count=1,
        always_hiring_count=3,
    )
    assert "JOB:\nTitle: Senior Backend Engineer\nCompany: Acme Corp\n\n" in text
    assert "Repost count (times this exact posting has reappeared across " in text
    assert "- Days open: 10 (a posting older than 45 days" in text
    assert "- Cross-board duplicates (same description on other sources, last " in text
    assert "- Always-hiring count (other postings from this company in the " in text
    assert "unknown" not in text.split("Report your confidence")[0].lower()


def test_user_prompt_with_every_optional_signal_unknown() -> None:
    text = prompts.build_user_prompt(
        title="Role",
        company="Co",
        repost_count=1,
        days_open=None,
        cross_board_count=None,
        always_hiring_count=None,
    )
    assert "- Days open: unknown (no posting date available)" in text
    assert "- Cross-board duplicates: unknown (description too short/empty)" in text
    assert "- Always-hiring count: unknown (company name unparseable)" in text


def test_schema_instruction_and_retry_instruction_shape() -> None:
    schema = '{"type": "object"}'
    instruction = prompts.schema_instruction(schema)
    expected = "\n\nRespond with a single JSON object matching this JSON Schema:\n" + schema
    assert instruction == expected

    retry = prompts.retry_instruction(schema, "not valid JSON")
    assert retry.startswith(instruction)
    assert "Your previous answer was invalid: not valid JSON" in retry
    assert retry.endswith("Fix it and answer again with valid JSON only.")
