from __future__ import annotations

import asyncio
import json
from collections.abc import Callable
from typing import Any

import pytest
from langchain_core.messages import AIMessage

from jobfinder_ai.capabilities.graphs import generation
from jobfinder_ai.contracts.generate_work import GenerateWork
from jobfinder_ai.failures import CapabilityError


class _FakeBoundModel:
    def __init__(self, respond: Callable[[str], AIMessage]) -> None:
        self._respond = respond
        self.calls: list[list[Any]] = []

    async def ainvoke(self, messages: list[Any], config: dict[str, Any] | None = None) -> AIMessage:
        self.calls.append(messages)
        return self._respond(messages[-1].content)


class _FakeChatModel:
    def __init__(self, respond: Callable[[str], AIMessage]) -> None:
        self._respond = respond
        self.bind_kwargs: dict[str, Any] | None = None
        self.bound: _FakeBoundModel | None = None

    def bind(self, **kwargs: Any) -> _FakeBoundModel:
        self.bind_kwargs = kwargs
        self.bound = _FakeBoundModel(self._respond)
        return self.bound


def _ai_message(payload: dict[str, Any]) -> AIMessage:
    return AIMessage(
        content=json.dumps(payload),
        usage_metadata={"input_tokens": 50, "output_tokens": 10, "total_tokens": 60},
    )


MASTER = {
    "cv": {
        "sections": {
            "skills": [{"label": "Languages", "details": "Python, Go"}],
            "experience": [
                {
                    "company": "Acme",
                    "position": "Engineer",
                    "start_date": "2019-01",
                    "end_date": "present",
                    "highlights": ["Built X", "Shipped Y", "Led Z"],
                },
            ],
            "projects": [],
        }
    }
}

ANALYSIS_RESULT = {
    "requiredSkills": ["Python"],
    "niceToHaveSkills": [],
    "experienceLevel": "senior",
    "keyResponsibilities": ["build things"],
    "industryKeywords": ["saas"],
    "seniorityKeywords": ["lead"],
}

FULL_SELECTION = {
    "experience": [
        {
            "company": "Acme",
            "highlights": [{"sourceIndex": 0}, {"sourceIndex": 1}, {"sourceIndex": 2}],
        }
    ],
    "projects": [],
}

SHORT_SELECTION = {
    "experience": [{"company": "Acme", "highlights": [{"sourceIndex": 0}]}],
    "projects": [],
}

SUMMARY_RESULT = {"summary": "5+ years of experience building things."}


def _work(*, shape: dict[str, Any] | None = None) -> GenerateWork:
    snapshot = {
        "master": MASTER,
        "vacancy": "We need a senior Python engineer.",
        "level": "moderate",
        "shape": shape or {"experienceBulletsMin": 2, "experienceBulletsMax": 3},
    }
    return GenerateWork(jobId="job_1", type="resume", snapshot=snapshot, snapshot_hash="sha256:x")


def _router(
    responses_by_task_key: dict[str, list[dict[str, Any]]],
) -> Callable[[str], _FakeChatModel]:
    """Builds a `gateway.chat_model` stand-in keyed by task key, each
    returning its queued responses in order."""
    queues = {k: list(v) for k, v in responses_by_task_key.items()}

    def chat_model(task_key: str) -> _FakeChatModel:
        queue = queues[task_key]

        def respond(_prompt: str) -> AIMessage:
            return _ai_message(queue.pop(0))

        return _FakeChatModel(respond)

    return chat_model


def test_run_assembles_result_without_escalation(monkeypatch: pytest.MonkeyPatch) -> None:
    chat_model = _router(
        {
            "generation-analyze": [ANALYSIS_RESULT],
            "generation-select": [FULL_SELECTION],
            "generation-summary": [SUMMARY_RESULT],
        }
    )
    monkeypatch.setattr(generation.gateway, "chat_model", chat_model)

    result, usage = asyncio.run(generation.run(_work()))

    assert result.analysis.requiredSkills == ["Python"]
    assert result.summary == SUMMARY_RESULT["summary"]
    assert result.experience[0]["highlights"] == ["Built X", "Shipped Y", "Led Z"]
    assert result.selectionEscalated is False
    assert usage.input_tokens == 150  # three stages x 50


def test_run_escalates_to_premium_on_bullet_shortfall(monkeypatch: pytest.MonkeyPatch) -> None:
    chat_model = _router(
        {
            "generation-analyze": [ANALYSIS_RESULT],
            "generation-select": [SHORT_SELECTION],
            "generation-select-premium": [FULL_SELECTION],
            "generation-summary": [SUMMARY_RESULT],
        }
    )
    monkeypatch.setattr(generation.gateway, "chat_model", chat_model)

    result, _usage = asyncio.run(generation.run(_work()))

    assert result.selectionEscalated is True
    assert result.experience[0]["highlights"] == ["Built X", "Shipped Y", "Led Z"]


def test_each_stage_gets_its_own_span_with_input_and_output(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    chat_model = _router(
        {
            "generation-analyze": [ANALYSIS_RESULT],
            "generation-select": [FULL_SELECTION],
            "generation-summary": [SUMMARY_RESULT],
        }
    )
    monkeypatch.setattr(generation.gateway, "chat_model", chat_model)

    spans: list[dict[str, Any]] = []

    class _FakeSpan:
        def __init__(self, name: str) -> None:
            self.name = name
            self.updates: list[dict[str, Any]] = []

        def update(self, **kwargs: Any) -> None:
            self.updates.append(kwargs)

        def __enter__(self) -> _FakeSpan:
            return self

        def __exit__(self, *exc_info: Any) -> None:
            spans.append({"name": self.name, "updates": self.updates})

    class _FakeClient:
        def start_as_current_observation(self, *, name: str, as_type: str) -> _FakeSpan:
            return _FakeSpan(name)

    monkeypatch.setattr(generation.tracing, "bootstrap", lambda: _FakeClient())

    asyncio.run(generation.run(_work()))

    names = [s["name"] for s in spans]
    assert names == ["generation-analyze", "generation-select", "generation-summary", "assemble"]
    for span in spans:
        keys = {k for update in span["updates"] for k in update}
        assert "input" in keys
        assert "output" in keys


def test_a_stage_failure_fails_the_run_naming_that_stage(monkeypatch: pytest.MonkeyPatch) -> None:
    def chat_model(task_key: str) -> _FakeChatModel:
        assert task_key == "generation-analyze"
        return _FakeChatModel(lambda _prompt: AIMessage(content="not json"))

    monkeypatch.setattr(generation.gateway, "chat_model", chat_model)

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(generation.run(_work()))

    assert exc_info.value.failed_step == "generation-analyze"
    assert exc_info.value.category == "internal"


def test_bound_exceeded_when_max_nodes_is_hit(monkeypatch: pytest.MonkeyPatch) -> None:
    chat_model = _router(
        {
            "generation-analyze": [ANALYSIS_RESULT],
            "generation-select": [FULL_SELECTION],
            "generation-summary": [SUMMARY_RESULT],
        }
    )
    monkeypatch.setattr(generation.gateway, "chat_model", chat_model)
    monkeypatch.setattr(generation, "MAX_NODES", 2)

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(generation.run(_work()))

    assert exc_info.value.category == "bound_exceeded"
    assert "max_nodes" in exc_info.value.message


def test_bound_exceeded_when_run_timeout_is_hit(monkeypatch: pytest.MonkeyPatch) -> None:
    async def _hang(*_args: Any, **_kwargs: Any) -> Any:
        await asyncio.sleep(10)

    monkeypatch.setattr(generation, "RUN_TIMEOUT_SECONDS", 0.01)
    monkeypatch.setattr(generation, "_structured_call", _hang)

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(generation.run(_work()))

    assert exc_info.value.category == "bound_exceeded"
    assert exc_info.value.failed_step == "generation"


def test_run_rejects_a_malformed_snapshot_as_invalid_input() -> None:
    work = GenerateWork(
        jobId="job_1", type="resume", snapshot={"vacancy": "x"}, snapshot_hash="sha256:x"
    )

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(generation.run(work))

    assert exc_info.value.category == "invalid_input"


def test_capability_declares_graph_state_layer_and_bounds() -> None:
    assert generation.CAPABILITY.layer == "graph_state"
    assert generation.CAPABILITY.task_key == "generation"
    assert generation.CAPABILITY.transport == "event"
    assert generation.CAPABILITY.bounds.max_nodes == generation.MAX_NODES


# ---------------------------------------------------------------------------
# Cover-letter branch (single call, service.go's `writeCoverLetter` ported)
# ---------------------------------------------------------------------------

COVER_LETTER_TEXT = "Hello,\n\nI'm excited about the Senior Engineer role at Acme.\n\nBest regards."


def _cover_letter_work(*, extra_notes: str | None = None) -> GenerateWork:
    snapshot: dict[str, Any] = {
        "profileText": "Senior engineer with 8 years building backend systems.",
        "company": "Acme",
        "title": "Senior Engineer",
        "vacancyText": "We need a senior backend engineer.",
    }
    if extra_notes is not None:
        snapshot["extraNotes"] = extra_notes
    return GenerateWork(
        jobId="job_1", type="cover_letter", snapshot=snapshot, snapshot_hash="sha256:x"
    )


def test_cover_letter_branch_produces_structured_letter(monkeypatch: pytest.MonkeyPatch) -> None:
    chat_model = _router({"generation": [{"letter": COVER_LETTER_TEXT}]})
    monkeypatch.setattr(generation.gateway, "chat_model", chat_model)

    result, usage = asyncio.run(generation.run(_cover_letter_work()))

    assert isinstance(result, generation.CoverLetterResult)
    paragraphs = [p for p in result.letter.split("\n\n") if p]
    assert len(paragraphs) == 3
    assert len(result.letter.split()) <= 150
    assert usage.input_tokens == 50


def test_cover_letter_branch_never_enters_the_resume_pipeline(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def chat_model(task_key: str) -> _FakeChatModel:
        assert task_key == "generation"
        return _FakeChatModel(lambda _prompt: _ai_message({"letter": COVER_LETTER_TEXT}))

    monkeypatch.setattr(generation.gateway, "chat_model", chat_model)

    asyncio.run(generation.run(_cover_letter_work()))


def test_cover_letter_branch_gets_its_own_span_with_input_and_output(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    chat_model = _router({"generation": [{"letter": COVER_LETTER_TEXT}]})
    monkeypatch.setattr(generation.gateway, "chat_model", chat_model)

    spans: list[dict[str, Any]] = []

    class _FakeSpan:
        def __init__(self, name: str) -> None:
            self.name = name
            self.updates: list[dict[str, Any]] = []

        def update(self, **kwargs: Any) -> None:
            self.updates.append(kwargs)

        def __enter__(self) -> _FakeSpan:
            return self

        def __exit__(self, *exc_info: Any) -> None:
            spans.append({"name": self.name, "updates": self.updates})

    class _FakeClient:
        def start_as_current_observation(self, *, name: str, as_type: str) -> _FakeSpan:
            return _FakeSpan(name)

    monkeypatch.setattr(generation.tracing, "bootstrap", lambda: _FakeClient())

    asyncio.run(generation.run(_cover_letter_work()))

    assert [s["name"] for s in spans] == ["generation-cover-letter"]
    keys = {k for update in spans[0]["updates"] for k in update}
    assert "input" in keys
    assert "output" in keys


def test_cover_letter_branch_fails_the_run_naming_its_own_stage(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def chat_model(task_key: str) -> _FakeChatModel:
        assert task_key == "generation"
        return _FakeChatModel(lambda _prompt: AIMessage(content="not json"))

    monkeypatch.setattr(generation.gateway, "chat_model", chat_model)

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(generation.run(_cover_letter_work()))

    assert exc_info.value.failed_step == "generation-cover-letter"
    assert exc_info.value.category == "internal"


def test_cover_letter_branch_rejects_a_malformed_snapshot_as_invalid_input() -> None:
    work = GenerateWork(
        jobId="job_1", type="cover_letter", snapshot={"company": "Acme"}, snapshot_hash="sha256:x"
    )

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(generation.run(work))

    assert exc_info.value.category == "invalid_input"


def test_cover_letter_prompt_matches_go_service_verbatim() -> None:
    prompt = generation.cover_letter_prompts.build_prompt(
        profile_text="Profile text.",
        extra_notes="Some notes.",
        company="Acme",
        title="Senior Engineer",
        vacancy_text="Vacancy text.",
    )

    assert prompt == (
        "Write a short cover letter (maximum 150 words, exactly 3 paragraphs separated by "
        "blank lines) for this application.\n\n"
        "Structure: (1) hook referencing the company and role, (2) 2-3 concrete matching "
        "experiences from the candidate's real background, (3) brief close.\n"
        "STRICT RULES: mention only experience present in the profile below; no invented "
        'facts, no clichés like "I am writing to express". Plain text, no salutation '
        'placeholders like [Hiring Manager] — use "Hello," if needed.\n\n'
        "CANDIDATE PROFILE:\nProfile text.\n\n"
        "EXTRA NOTES:\nSome notes.\n\n"
        "JOB:\nTitle: Senior Engineer\nCompany: Acme\nDescription:\nVacancy text."
    )
