"""Byte-parity guard over every prompt this service sends (C8-1, C8-2,
FR-021).

The prompts are ports of the Go originals, so their text is a contract, not
an implementation detail: a reworded rule or a moved newline changes the
model's answers and breaks parity with the recorded baseline. Reviewing that
by reading a builder is unreliable — reviewing it as a diff on the rendered
prompt is not.

Every file in `tests/golden/prompts` is the exact output of one builder for
one input shape from `prompt_cases`. They were rendered from the pre-refactor
builders, so they also pin what the composition layer was refactored away
from. A failure here is never "update the golden and move on": either the Go
side changed too and the new text is the intended port, or the change is a
regression.

Regenerating (only alongside a matching Go-side change):

    uv run python -c "import pathlib; from tests.unit import prompt_cases; \
        [pathlib.Path('tests/golden/prompts', n + '.txt').write_text(t()) \
         for n, t in prompt_cases.all_cases()]"
"""

from __future__ import annotations

import pathlib
from collections.abc import Callable

import pytest

from jobfinder_ai.prompts import recruiter, salary
from jobfinder_ai.prompts.generation import analyze, cover_letter
from tests.unit import prompt_cases

GOLDEN_DIR = pathlib.Path(__file__).parent.parent / "golden" / "prompts"

CASES = prompt_cases.all_cases()


@pytest.mark.parametrize("name,build", CASES, ids=[name for name, _ in CASES])
def test_prompt_matches_golden(name: str, build: Callable[[], str]) -> None:
    golden = GOLDEN_DIR / f"{name}.txt"
    assert golden.exists(), f"missing golden for {name} — render it before asserting on it"
    assert build() == golden.read_text()


def test_every_golden_has_a_case() -> None:
    """A golden with no case left behind by a deleted builder would silently
    stop being asserted on."""
    rendered = {name for name, _ in CASES}
    orphans = {path.stem for path in GOLDEN_DIR.glob("*.txt")} - rendered
    assert not orphans, f"golden files with no case: {sorted(orphans)}"


def test_salary_truncates_description() -> None:
    filler = "x" * (salary.DESCRIPTION_TRUNCATE_CHARS + 50)
    prompt = salary.build_user_prompt(
        job_id="job-1",
        title="Engineer",
        company="Acme",
        location="Berlin",
        remote=False,
        description=filler,
    )
    assert "x" * salary.DESCRIPTION_TRUNCATE_CHARS in prompt
    assert "x" * (salary.DESCRIPTION_TRUNCATE_CHARS + 1) not in prompt


@pytest.mark.parametrize("source", ["posting", "company_page", "linkedin"])
def test_recruiter_truncates_scraped_text(source: recruiter.Source) -> None:
    filler = "y" * (recruiter.MAX_TEXT_CHARS + 50)
    prompt = recruiter.build_user_prompt(source=source, text=filler)
    assert "y" * recruiter.MAX_TEXT_CHARS in prompt
    assert "y" * (recruiter.MAX_TEXT_CHARS + 1) not in prompt


def test_analyze_truncates_vacancy() -> None:
    filler = "z" * (analyze.VACANCY_TRUNCATE_RUNES + 50)
    prompt = analyze.build_prompt(filler)
    assert "z" * analyze.VACANCY_TRUNCATE_RUNES in prompt
    assert "z" * (analyze.VACANCY_TRUNCATE_RUNES + 1) not in prompt


def test_analyze_truncates_by_rune_not_byte() -> None:
    """Go truncates by rune count; a byte-based cut would both shorten the
    text and risk splitting a multi-byte character."""
    prompt = analyze.build_prompt("é" * (analyze.VACANCY_TRUNCATE_RUNES + 10))
    assert "é" * analyze.VACANCY_TRUNCATE_RUNES in prompt


def test_cover_letter_truncates_each_input_to_its_own_budget() -> None:
    prompt = cover_letter.build_prompt(
        profile_text="p" * (cover_letter.PROFILE_TRUNCATE_CHARS + 50),
        extra_notes="n" * (cover_letter.EXTRA_NOTES_TRUNCATE_CHARS + 50),
        company="Acme",
        title="Engineer",
        vacancy_text="v" * (cover_letter.VACANCY_TRUNCATE_CHARS + 50),
    )
    for char, limit in (
        ("p", cover_letter.PROFILE_TRUNCATE_CHARS),
        ("n", cover_letter.EXTRA_NOTES_TRUNCATE_CHARS),
        ("v", cover_letter.VACANCY_TRUNCATE_CHARS),
    ):
        assert char * limit in prompt
        assert char * (limit + 1) not in prompt
