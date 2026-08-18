"""The `generation` capability (C1-1, C2-3, FR-039, FR-040): a LangGraph
`graph_state` capability ported from
`apps/api/internal/generation/application/service.go`'s
`tailorRendercvResume` — analyze -> select (economy, escalating to premium on
a completeness shortfall) -> summarize -> assemble.

Each stage is bound to its own task key so per-stage model routing stays a
`gateway/config.yaml` edit, never a code change (C2-3, 030-FR-005):

    generation-analyze          -> node_analyze
    generation-select            -> node_select (economy)
    generation-select-premium    -> node_select_premium (escalation only)
    generation-summary*          -> node_summarize (option-routed, 034)

`assemble` makes no model call — it deterministically applies the select
stage's `{sourceIndex, rephrased}` references onto the master snapshot,
mirroring `domain.MergeTailored`'s highlight resolution without a task key of
its own.

Grounding (C6-3, Constitution II): every stage's prompt is built ONLY from
`GenerationSnapshot` — the profile/job data the event carries. No stage makes
an HTTP call, a database read, or a filesystem read beyond the snapshot.

Bounds (C4-1, C4-3, C4-4): `MAX_NODES` caps the graph's node executions,
`NODE_TIMEOUT_SECONDS` bounds each node, `RUN_TIMEOUT_SECONDS` bounds the
whole run; all three are recorded in the trace metadata. Hitting any of them
raises a `bound_exceeded` `CapabilityError` naming the bound — never a
truncated result (E5-3).

Also carries the cover-letter branch (`_run_cover_letter`), ported from
`writeCoverLetter` (service.go line 871): a single LLM call that never
enters the pipeline above, routed on `GenerateWork.type ==
COVER_LETTER_DOC_TYPE`. It shares the `generation` task key — there is no
separate cover-letter task key among the fourteen (contracts/capabilities.md
§ C2) — and data-model.md § 8's `generation` row covers both outputs
("resume/cover-letter sections").
"""

from __future__ import annotations

import asyncio
import json
import re
from typing import Any, TypedDict

from langchain_core.messages import AIMessage, HumanMessage, SystemMessage
from langgraph.errors import GraphRecursionError
from langgraph.graph import END, START, StateGraph
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from jobfinder_ai import gateway, tracing
from jobfinder_ai.capabilities.registry import Capability, CapabilityBounds
from jobfinder_ai.contracts.generate_work import GenerateWork
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import CapabilityError, classify_provider_error
from jobfinder_ai.prompts.generation import analyze as analyze_prompts
from jobfinder_ai.prompts.generation import cover_letter as cover_letter_prompts
from jobfinder_ai.prompts.generation import select as select_prompts
from jobfinder_ai.prompts.generation import summary as summary_prompts

MAX_NODES = 5
NODE_TIMEOUT_SECONDS = 240.0
RUN_TIMEOUT_SECONDS = 600.0

MAX_EXTRA_ATTEMPTS = 2

_FENCE_RE = re.compile(r"^```(?:json)?\s*(.*?)\s*```$", re.DOTALL)


def _strip_fences(text: str) -> str:
    stripped = text.strip()
    match = _FENCE_RE.match(stripped)
    return match.group(1) if match else stripped


class VacancyHintsIn(BaseModel):
    model_config = ConfigDict(extra="forbid")

    requiredSkills: list[str] = Field(default_factory=list)
    niceToHave: list[str] = Field(default_factory=list)
    experienceLevel: str = ""


class ShapeConfigIn(BaseModel):
    """Mirrors the fields of `domain.ShapeConfig` the generation stages
    actually read. Defaults mirror `domain.DefaultShapeConfig()`."""

    model_config = ConfigDict(extra="forbid")

    summaryLines: int = 4
    skillsEnabled: bool = True
    experienceBulletsMin: int = 8
    experienceBulletsMax: int = 10
    targetPages: int = 2
    projectsMax: int = 0
    projectBulletsMax: int = 0


class GenerationSnapshot(BaseModel):
    """The grounding data this run consumes (C6-3, E3-2, E3-3): the master
    profile (RenderCV shape: `{"cv": {"sections": {...}}}`), the vacancy
    text, the tailoring level and the resume-shape config. Nothing else is
    read at runtime."""

    model_config = ConfigDict(extra="forbid")

    master: dict[str, Any]
    vacancy: str
    level: str = "moderate"
    shape: ShapeConfigIn = Field(default_factory=ShapeConfigIn)
    hints: VacancyHintsIn | None = None
    summaryOption: str = "standard"


class CoverLetterSnapshot(BaseModel):
    """The grounding data the cover-letter branch consumes (C6-3, E3-2,
    E3-3): mirrors the arguments `writeCoverLetter` (service.go) takes —
    the candidate's rendered profile text, optional extra notes, and the
    target job's company/title/description. Nothing else is read at
    runtime; this branch never touches the RenderCV `master` shape the
    resume pipeline needs."""

    model_config = ConfigDict(extra="forbid")

    profileText: str
    extraNotes: str | None = None
    company: str
    title: str
    vacancyText: str


class CoverLetterResult(BaseModel):
    """Mirrors Go's `coverLetterResult` — the cover-letter branch's
    terminal output (C3-2, C3-3)."""

    model_config = ConfigDict(extra="forbid")

    letter: str


class VacancyAnalysisOut(BaseModel):
    model_config = ConfigDict(extra="forbid")

    requiredSkills: list[str] = Field(default_factory=list)
    niceToHaveSkills: list[str] = Field(default_factory=list)
    experienceLevel: str = ""
    keyResponsibilities: list[str] = Field(default_factory=list)
    industryKeywords: list[str] = Field(default_factory=list)
    seniorityKeywords: list[str] = Field(default_factory=list)


class HighlightRefOut(BaseModel):
    model_config = ConfigDict(extra="forbid")

    sourceIndex: int
    rephrased: str | None = None


class TailoredExperienceOut(BaseModel):
    model_config = ConfigDict(extra="forbid")

    company: str
    highlights: list[HighlightRefOut] = Field(default_factory=list)


class TailoredProjectOut(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: str
    highlights: list[HighlightRefOut] = Field(default_factory=list)


class TailoredSelectionOut(BaseModel):
    model_config = ConfigDict(extra="forbid")

    experience: list[TailoredExperienceOut] = Field(default_factory=list)
    projects: list[TailoredProjectOut] = Field(default_factory=list)


class TailoredSummaryOut(BaseModel):
    model_config = ConfigDict(extra="forbid")

    summary: str


class GenerationResult(BaseModel):
    """Assembled resume sections — the graph's terminal output (C3-2, C3-3)."""

    model_config = ConfigDict(extra="forbid")

    analysis: VacancyAnalysisOut
    summary: str
    experience: list[dict[str, Any]]
    skills: list[dict[str, Any]]
    projects: list[dict[str, Any]]
    selectionEscalated: bool
    summaryOption: str


class GenerationState(TypedDict, total=False):
    snapshot: GenerationSnapshot
    trace_id: str | None
    node_count: int
    usage_by_stage: dict[str, Usage]
    analysis: VacancyAnalysisOut
    selection: TailoredSelectionOut
    selection_tier: str
    summary: str
    result: GenerationResult


def _cv_sections(master: dict[str, Any]) -> dict[str, Any]:
    cv = master.get("cv")
    if not isinstance(cv, dict):
        return {}
    sections = cv.get("sections")
    return sections if isinstance(sections, dict) else {}


def _norm(s: str) -> str:
    return s.strip().lower()


def _at_least_one(n: int) -> int:
    return n if n >= 1 else 1


def _summary_select_range(shape: ShapeConfigIn) -> tuple[int, int]:
    """Verbatim port of `summarySelectRange`."""
    return _at_least_one(shape.summaryLines - 1), _at_least_one(shape.summaryLines)


_YEAR_RE = re.compile(r"(\d{4})")


def _parse_year(value: str) -> int | None:
    match = _YEAR_RE.search(value or "")
    return int(match.group(1)) if match else None


def _derive_total_experience_years(master: dict[str, Any], *, now_year: int | None = None) -> int:
    """Simplified port of `DeriveTotalExperienceYearsAsOf`: sums each
    experience entry's start/end year span, treating an unparsable or
    "present" end date as the current year."""
    import datetime as _dt

    now = now_year if now_year is not None else _dt.datetime.now(tz=_dt.UTC).year
    total = 0
    for entry in _cv_sections(master).get("experience") or []:
        start_year = _parse_year(str(entry.get("start_date", "")))
        if start_year is None:
            continue
        end_raw = str(entry.get("end_date", ""))
        end_year = _parse_year(end_raw)
        if end_year is None or end_raw.strip().lower() == "present":
            end_year = now
        total += max(0, end_year - start_year)
    return total


def _skill_group_labels(master: dict[str, Any]) -> list[str]:
    groups = _cv_sections(master).get("skills") or []
    return [g.get("label", "") for g in groups if g.get("label")]


def _resolve_highlights(source: list[str], refs: list[HighlightRefOut]) -> list[str]:
    """Simplified port of `ResolveHighlights`: an out-of-range index is
    dropped; a rewording is used when present, the master's own wording
    otherwise. (Drift/duplicate-bullet verification is out of scope for this
    graph — it stays a Go-side render-time concern.)"""
    out: list[str] = []
    for ref in refs:
        if 0 <= ref.sourceIndex < len(source):
            out.append(ref.rephrased or source[ref.sourceIndex])
    return out


def _selected_highlights(master: dict[str, Any], selection: TailoredSelectionOut) -> list[str]:
    """Port of `SelectedHighlights`."""
    by_company = {
        _norm(str(e.get("company", ""))): list(e.get("highlights") or [])
        for e in (_cv_sections(master).get("experience") or [])
    }
    by_project = {
        _norm(str(p.get("name", ""))): list(p.get("highlights") or [])
        for p in (_cv_sections(master).get("projects") or [])
    }
    out: list[str] = []
    for exp in selection.experience:
        out.extend(_resolve_highlights(by_company.get(_norm(exp.company), []), exp.highlights))
    for proj in selection.projects:
        out.extend(_resolve_highlights(by_project.get(_norm(proj.name), []), proj.highlights))
    return out


def _summary_task_key(option: str) -> str:
    return {
        "premium": "generation-summary-premium",
        "fast": "generation-summary-fast",
    }.get(option, "generation-summary")


def _selection_shortfall(snapshot: GenerationSnapshot, selection: TailoredSelectionOut) -> bool:
    """Simplified port of `bulletShortfalls`: a company the master could
    have supplied at least `experienceBulletsMin` bullets for, but the
    selection stage returned fewer than that for, is a shortfall the
    economy model could have avoided (035 FR-006/FR-007)."""
    min_bullets = snapshot.shape.experienceBulletsMin
    if min_bullets <= 0:
        return False
    master_counts = {
        _norm(str(e.get("company", ""))): len(e.get("highlights") or [])
        for e in (_cv_sections(snapshot.master).get("experience") or [])
    }
    for exp in selection.experience:
        available = master_counts.get(_norm(exp.company))
        if available is None or available < min_bullets:
            continue
        if len(exp.highlights) < min_bullets:
            return True
    return False


def _assemble_experience(
    master: dict[str, Any], selection: TailoredSelectionOut
) -> list[dict[str, Any]]:
    by_company = {_norm(e.company): e for e in selection.experience}
    assembled: list[dict[str, Any]] = []
    for entry in _cv_sections(master).get("experience") or []:
        master_highlights = [str(h) for h in (entry.get("highlights") or [])]
        selected = by_company.get(_norm(str(entry.get("company", ""))))
        if selected is not None:
            highlights = _resolve_highlights(master_highlights, selected.highlights)
        else:
            highlights = master_highlights
        assembled.append({**entry, "highlights": highlights})
    return assembled


def _assemble_projects(
    master: dict[str, Any], selection: TailoredSelectionOut
) -> list[dict[str, Any]]:
    master_projects = _cv_sections(master).get("projects") or []
    if not selection.projects:
        return list(master_projects)
    by_name = {p.get("name", ""): p for p in master_projects}
    out: list[dict[str, Any]] = []
    for proj in selection.projects:
        master_project = by_name.get(proj.name)
        if master_project is None:
            continue
        master_highlights = [str(h) for h in (master_project.get("highlights") or [])]
        resolved = _resolve_highlights(master_highlights, proj.highlights)
        out.append({**master_project, "highlights": resolved})
    return out


def _usage_from_message(message: AIMessage) -> Usage:
    meta = message.usage_metadata
    if not meta:
        return Usage()
    return Usage(
        input_tokens=meta.get("input_tokens"),
        output_tokens=meta.get("output_tokens"),
        cost_usd=None,
    )


def _schema_instruction(schema: str) -> str:
    return "\n\nRespond with a single JSON object matching this JSON Schema:\n" + schema


def _retry_instruction(schema: str, last_error: str) -> str:
    return (
        _schema_instruction(schema)
        + "\nYour previous answer was invalid: "
        + last_error
        + "\nFix it and answer again with valid JSON only."
    )


async def _structured_call(
    *,
    task_key: str,
    system_prompt: str,
    user_prompt: str,
    model_cls: type[BaseModel],
    trace_id: str | None,
) -> tuple[BaseModel, Usage]:
    """Calls the gateway by task key and parses/validates structured JSON
    output, retrying on a parse/validation failure — the same ladder
    `ghost.run` uses (C3-4)."""
    bind_kwargs: dict[str, object] = {"response_format": {"type": "json_object"}}
    if trace_id is not None:
        bind_kwargs["extra_body"] = tracing.gateway_call_metadata(trace_id)
    model = gateway.chat_model(task_key).bind(**bind_kwargs)

    schema = json.dumps(model_cls.model_json_schema())
    turn = _schema_instruction(schema)
    last_error: str | None = None
    parsed: BaseModel | None = None
    usage = Usage()

    for _attempt in range(MAX_EXTRA_ATTEMPTS + 1):
        if last_error is not None:
            turn = _retry_instruction(schema, last_error)
        try:
            message = await model.ainvoke(
                [SystemMessage(content=system_prompt), HumanMessage(content=user_prompt + turn)],
                config={"callbacks": [tracing.callback_handler()]},
            )
        except Exception as exc:  # noqa: BLE001 - reclassified below (E5)
            raise classify_provider_error(exc, failed_step=task_key) from exc

        assert isinstance(message, AIMessage)
        usage = _usage_from_message(message)
        content = message.content if isinstance(message.content, str) else str(message.content)
        try:
            payload = json.loads(_strip_fences(content))
            parsed = model_cls.model_validate(payload)
        except (json.JSONDecodeError, ValidationError) as exc:
            last_error = str(exc)
            continue
        break

    if parsed is None:
        raise CapabilityError(
            "internal",
            f"generation: {task_key} structured output failed after "
            f"{MAX_EXTRA_ATTEMPTS + 1} attempts: {last_error}",
            failed_step=task_key,
        )
    return parsed, usage


async def _run_stage(
    state: GenerationState,
    *,
    name: str,
    task_key: str,
    system_prompt: str,
    user_prompt: str,
    model_cls: type[BaseModel],
) -> tuple[Any, Usage, int]:
    """One graph node's worth of work: a span of its own (US3 scenario 1) and
    the structured gateway call. `max_nodes` itself is enforced by the graph
    runtime's `recursion_limit` (C4-2) at the `run()` call site, not here —
    `node_count` is bookkeeping only, for usage-by-stage and span labels.

    Every stage is its own span with its own input/output, and a bound
    hit or provider failure marks the trace failed naming this stage
    (E5-4, FR-040) rather than the run in general.
    """
    node_count = state.get("node_count", 0) + 1

    trace_id = state.get("trace_id")
    with tracing.bootstrap().start_as_current_observation(name=name, as_type="span") as span:
        span.update(input={"task_key": task_key, "prompt": user_prompt})
        try:
            result, usage = await asyncio.wait_for(
                _structured_call(
                    task_key=task_key,
                    system_prompt=system_prompt,
                    user_prompt=user_prompt,
                    model_cls=model_cls,
                    trace_id=trace_id,
                ),
                timeout=NODE_TIMEOUT_SECONDS,
            )
        except TimeoutError as exc:
            err = CapabilityError(
                "bound_exceeded",
                f"generation: stage {name!r} exceeded per-node timeout of {NODE_TIMEOUT_SECONDS}s",
                failed_step=name,
            )
            tracing.mark_run_failed(span, failed_step=name, error=err.message)
            raise err from exc
        except CapabilityError as exc:
            tracing.mark_run_failed(span, failed_step=exc.failed_step or name, error=exc.message)
            raise
        span.update(output=result.model_dump())
    return result, usage, node_count


def _with_usage(
    state: GenerationState, node_count: int, stage: str, usage: Usage, **updates: Any
) -> dict[str, Any]:
    usage_by_stage = {**state.get("usage_by_stage", {}), stage: usage}
    return {"node_count": node_count, "usage_by_stage": usage_by_stage, **updates}


async def node_analyze(state: GenerationState) -> dict[str, Any]:
    snapshot = state["snapshot"]
    hints = snapshot.hints
    user_prompt = analyze_prompts.build_prompt(
        snapshot.vacancy,
        required_skills=hints.requiredSkills if hints else None,
        nice_to_have=hints.niceToHave if hints else None,
        experience_level=hints.experienceLevel if hints else None,
    )
    analysis, usage, node_count = await _run_stage(
        state,
        name="generation-analyze",
        task_key="generation-analyze",
        system_prompt=analyze_prompts.SYSTEM_PROMPT,
        user_prompt=user_prompt,
        model_cls=VacancyAnalysisOut,
    )
    return _with_usage(state, node_count, "generation-analyze", usage, analysis=analysis)


def _select_prompt(state: GenerationState, *, prev_violations: list[str] | None = None) -> str:
    snapshot = state["snapshot"]
    sections = _cv_sections(snapshot.master)
    return select_prompts.build_prompt(
        skills=sections.get("skills") or [],
        experience=sections.get("experience") or [],
        projects=sections.get("projects") or [],
        analysis=state["analysis"].model_dump(),
        level=snapshot.level,
        prev_violations=prev_violations,
        cfg=snapshot.shape.model_dump(),
    )


async def node_select(state: GenerationState) -> dict[str, Any]:
    selection, usage, node_count = await _run_stage(
        state,
        name="generation-select",
        task_key="generation-select",
        system_prompt=select_prompts.SYSTEM_PROMPT,
        user_prompt=_select_prompt(state),
        model_cls=TailoredSelectionOut,
    )
    return _with_usage(
        state, node_count, "generation-select", usage, selection=selection, selection_tier="economy"
    )


async def node_select_premium(state: GenerationState) -> dict[str, Any]:
    selection, usage, node_count = await _run_stage(
        state,
        name="generation-select-premium",
        task_key="generation-select-premium",
        system_prompt=select_prompts.SYSTEM_PROMPT,
        user_prompt=_select_prompt(state),
        model_cls=TailoredSelectionOut,
    )
    return _with_usage(
        state,
        node_count,
        "generation-select-premium",
        usage,
        selection=selection,
        selection_tier="premium",
    )


def route_after_select(state: GenerationState) -> str:
    """C2-3's select branch: escalate to the premium model only when the
    economy model's own output falls short of the configured bullet minimum
    (035 FR-006/FR-007) — never on a fixed schedule."""
    if state.get("selection_tier") == "premium":
        return "summarize"
    if _selection_shortfall(state["snapshot"], state["selection"]):
        return "select_premium"
    return "summarize"


async def node_summarize(state: GenerationState) -> dict[str, Any]:
    snapshot = state["snapshot"]
    selection = state["selection"]
    sentence_min, sentence_max = _summary_select_range(snapshot.shape)
    user_prompt = summary_prompts.build_prompt(
        skill_group_labels=_skill_group_labels(snapshot.master),
        highlights=_selected_highlights(snapshot.master, selection),
        sentence_min=sentence_min,
        sentence_max=sentence_max,
        total_years=_derive_total_experience_years(snapshot.master),
    )
    task_key = _summary_task_key(snapshot.summaryOption)
    summary_out, usage, node_count = await _run_stage(
        state,
        name=task_key,
        task_key=task_key,
        system_prompt=summary_prompts.SYSTEM_PROMPT,
        user_prompt=user_prompt,
        model_cls=TailoredSummaryOut,
    )
    return _with_usage(state, node_count, task_key, usage, summary=summary_out.summary)


async def node_assemble(state: GenerationState) -> dict[str, Any]:
    """Deterministic — no model call, no task key. Applies the select
    stage's `{sourceIndex, rephrased}` references onto the master snapshot
    (simplified port of `domain.MergeTailored`'s highlight resolution)."""
    node_count = state.get("node_count", 0) + 1
    snapshot = state["snapshot"]
    selection = state["selection"]
    with tracing.bootstrap().start_as_current_observation(name="assemble", as_type="span") as span:
        span.update(
            input={
                "experience_entries": len(selection.experience),
                "project_entries": len(selection.projects),
            }
        )
        experience = _assemble_experience(snapshot.master, selection)
        projects = _assemble_projects(snapshot.master, selection)
        skills = list(_cv_sections(snapshot.master).get("skills") or [])
        result = GenerationResult(
            analysis=state["analysis"],
            summary=state["summary"],
            experience=experience,
            skills=skills,
            projects=projects,
            selectionEscalated=state.get("selection_tier") == "premium",
            summaryOption=snapshot.summaryOption,
        )
        span.update(output={"experience_entries": len(experience), "skill_groups": len(skills)})
    return {"node_count": node_count, "result": result}


def _build_graph() -> Any:
    graph = StateGraph(GenerationState)
    graph.add_node("analyze", node_analyze)
    graph.add_node("select", node_select)
    graph.add_node("select_premium", node_select_premium)
    graph.add_node("summarize", node_summarize)
    graph.add_node("assemble", node_assemble)

    graph.add_edge(START, "analyze")
    graph.add_edge("analyze", "select")
    graph.add_conditional_edges(
        "select", route_after_select, {"select_premium": "select_premium", "summarize": "summarize"}
    )
    graph.add_edge("select_premium", "summarize")
    graph.add_edge("summarize", "assemble")
    graph.add_edge("assemble", END)
    return graph.compile()


def _aggregate_usage(usage_by_stage: dict[str, Usage]) -> Usage:
    input_tokens = sum(u.input_tokens or 0 for u in usage_by_stage.values()) or None
    output_tokens = sum(u.output_tokens or 0 for u in usage_by_stage.values()) or None
    return Usage(input_tokens=input_tokens, output_tokens=output_tokens, cost_usd=None)


COVER_LETTER_DOC_TYPE = "cover_letter"

_COVER_LETTER_STAGE = "generation-cover-letter"


async def _run_cover_letter(
    work: GenerateWork, *, trace_id: str | None
) -> tuple[CoverLetterResult, Usage]:
    """The cover-letter branch (service.go line 788): a single LLM call that
    never enters the analyze -> select -> summarize -> assemble pipeline,
    reachable through the same `generation` task key (there is no separate
    cover-letter task key among the fourteen) and traced the same way as the
    resume path — its own span, marked failed with its own `failed_step` on
    error (E5-4)."""
    try:
        snapshot = CoverLetterSnapshot.model_validate(work.snapshot)
    except ValidationError as exc:
        raise CapabilityError(
            "invalid_input",
            f"generation: malformed cover-letter snapshot: {exc}",
            failed_step="generation",
        ) from exc

    user_prompt = cover_letter_prompts.build_prompt(
        profile_text=snapshot.profileText,
        extra_notes=snapshot.extraNotes,
        company=snapshot.company,
        title=snapshot.title,
        vacancy_text=snapshot.vacancyText,
    )

    with tracing.bootstrap().start_as_current_observation(
        name=_COVER_LETTER_STAGE, as_type="span"
    ) as span:
        span.update(input={"task_key": "generation", "prompt": user_prompt})
        try:
            parsed, usage = await asyncio.wait_for(
                _structured_call(
                    task_key="generation",
                    system_prompt=cover_letter_prompts.SYSTEM_PROMPT,
                    user_prompt=user_prompt,
                    model_cls=CoverLetterResult,
                    trace_id=trace_id,
                ),
                timeout=NODE_TIMEOUT_SECONDS,
            )
        except TimeoutError as exc:
            err = CapabilityError(
                "bound_exceeded",
                f"generation: cover-letter branch exceeded per-node timeout of "
                f"{NODE_TIMEOUT_SECONDS}s",
                failed_step=_COVER_LETTER_STAGE,
            )
            tracing.mark_run_failed(span, failed_step=_COVER_LETTER_STAGE, error=err.message)
            raise err from exc
        except CapabilityError as exc:
            tracing.mark_run_failed(span, failed_step=_COVER_LETTER_STAGE, error=exc.message)
            raise CapabilityError(
                exc.category, exc.message, failed_step=_COVER_LETTER_STAGE
            ) from exc
        assert isinstance(parsed, CoverLetterResult)
        result = parsed.model_copy(update={"letter": parsed.letter.strip()})
        span.update(output=result.model_dump())
    return result, usage


async def run(
    work: GenerateWork, *, trace_id: str | None = None
) -> tuple[GenerationResult | CoverLetterResult, Usage]:
    """Runs the `generation` capability end to end from a `GenerateWork`
    event payload, routing on `work.type` (data-model.md § 8): the
    cover-letter branch (`_run_cover_letter`) for `"cover_letter"`, the
    analyze -> select -> summarize -> assemble graph otherwise. Raises
    `CapabilityError` (E5) on any classified failure, including
    `bound_exceeded` when `MAX_NODES`/`NODE_TIMEOUT_SECONDS`/
    `RUN_TIMEOUT_SECONDS` is hit; never returns a partial result (C3-2)."""
    if work.type == COVER_LETTER_DOC_TYPE:
        return await _run_cover_letter(work, trace_id=trace_id)

    try:
        snapshot = GenerationSnapshot.model_validate(work.snapshot)
    except ValidationError as exc:
        raise CapabilityError(
            "invalid_input", f"generation: malformed snapshot: {exc}", failed_step="generation"
        ) from exc

    graph = _build_graph()
    initial_state: GenerationState = {
        "snapshot": snapshot,
        "trace_id": trace_id,
        "node_count": 0,
        "usage_by_stage": {},
    }
    try:
        final_state = await asyncio.wait_for(
            graph.ainvoke(initial_state, config={"recursion_limit": MAX_NODES + 1}),
            timeout=RUN_TIMEOUT_SECONDS,
        )
    except TimeoutError as exc:
        raise CapabilityError(
            "bound_exceeded",
            f"generation: exceeded whole-run timeout of {RUN_TIMEOUT_SECONDS}s",
            failed_step="generation",
        ) from exc
    except GraphRecursionError as exc:
        raise CapabilityError(
            "bound_exceeded",
            f"generation: exceeded max_nodes bound of {MAX_NODES}",
            failed_step="generation",
        ) from exc

    result = final_state["result"]
    usage = _aggregate_usage(final_state.get("usage_by_stage", {}))
    return result, usage


def trace_bounds_metadata() -> dict[str, float | int]:
    """The graph's bounds, recorded on the trace (C4-4, T095)."""
    return {
        "max_nodes": MAX_NODES,
        "node_timeout_seconds": NODE_TIMEOUT_SECONDS,
        "run_timeout_seconds": RUN_TIMEOUT_SECONDS,
    }


CAPABILITY = Capability(
    name="generation",
    task_key="generation",
    layer="graph_state",
    input_model="jobfinder_ai.contracts.generate_work:GenerateWork",
    output_model="jobfinder_ai.capabilities.graphs.generation:GenerationResult",
    bounds=CapabilityBounds(max_model_calls=4, max_nodes=MAX_NODES),
    prompt_module="jobfinder_ai.prompts.generation",
    transport="event",
)
