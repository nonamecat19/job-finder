"""The `salary` capability (C2-3, FR-039, FR-041, T086): a bounded tool-call
loop implemented as a LangGraph state graph, ported from
`apps/api/internal/salary/application/service.go`'s `llmInfer` and
`apps/api/internal/platform/llm/application/toolloop`'s `Run`.

The Go loop is a `for round := 1; round <= MaxRounds` over one provider call
per round: round 1 forces a tool call (`tool_choice: required`, the
not-tool-capable detector), later rounds allow one, and the first round with
no tool call hands the accumulated conversation to a separate typed terminal
call. Nothing in that shape enforces the round bound by counting in Python —
the `for` loop itself is the bound. Here the equivalent bound is LangGraph's
`recursion_limit` (C4-2): the graph has no round counter that *stops*
anything, only bookkeeping (`round`, used to pick `tool_choice` and to label
spans) that keeps running until either the model stops asking for tools or
the runtime refuses to take another step and raises `GraphRecursionError`,
which is translated into `bound_exceeded` (C4-3) naming the bound.

Graph shape: `agent` (one model call) -> conditional -> `tools` (dispatch
every call from that round, one call span and one result span each per
C5-3) -> back to `agent`, or -> `finalize` (the typed terminal call,
mirroring the Go loop's post-loop `CompleteStructuredChat`) -> END. Each
round costs exactly two graph steps (`agent` + `tools`); a round that ends
the loop costs one (`agent` alone) plus the `finalize` step. Setting
`recursion_limit = 2 * MAX_TOOL_ROUNDS` therefore allows exactly
`MAX_TOOL_ROUNDS` full rounds — a model that still wants a tool after the
last permitted round trips the limit attempting round `MAX_TOOL_ROUNDS + 1`,
exactly where Go's `for` loop would have exited with `StopMaxRounds`.
"""

from __future__ import annotations

import asyncio
import functools
import json
import re
import time
from typing import Annotated, Any, Literal, TypedDict

from langchain_core.messages import AIMessage, BaseMessage, HumanMessage, SystemMessage, ToolMessage
from langgraph.errors import GraphRecursionError
from langgraph.graph import END, START, StateGraph
from langgraph.graph.message import add_messages
from pydantic import BaseModel, ConfigDict, Field, ValidationError, model_validator

from jobfinder_ai import gateway, tracing
from jobfinder_ai.capabilities.graphs import untrusted
from jobfinder_ai.capabilities.graphs.salary_tools import SalarySnapshot, build_salary_tools
from jobfinder_ai.capabilities.registry import Capability, CapabilityBounds
from jobfinder_ai.contracts.salary_work import SalaryWork
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import CapabilityError, classify_provider_error
from jobfinder_ai.prompts import salary as prompts

TASK_KEY = "salary"

MAX_TOOL_ROUNDS = 4
PER_TOOL_TIMEOUT_SECONDS = 10.0
MAX_RESULT_BYTES = 32 * 1024
WHOLE_RUN_TIMEOUT_SECONDS = 4 * 60.0
MAX_NODES = 2 * MAX_TOOL_ROUNDS
MAX_MODEL_CALLS = MAX_TOOL_ROUNDS + 1
MAX_EXTRA_ATTEMPTS = 2

_FENCE_RE = re.compile(r"^```(?:json)?\s*(.*?)\s*```$", re.DOTALL)


def _strip_fences(text: str) -> str:
    stripped = text.strip()
    match = _FENCE_RE.match(stripped)
    return match.group(1) if match else stripped


class SalaryBand(BaseModel):
    """Mirrors `domain.SalaryBand` (port.go) — same fields, same ranges,
    same enum (C3-3)."""

    model_config = ConfigDict(extra="forbid")

    min: int = Field(ge=0)
    max: int = Field(ge=0)
    currency: str
    confidence: float = Field(ge=0, le=1)
    source: Literal["llm", "levels-fyi", "ingested-cache", "blended"]

    @model_validator(mode="after")
    def _min_le_max(self) -> SalaryBand:
        if self.min > self.max:
            raise ValueError(f"min ({self.min}) must be <= max ({self.max})")
        return self


class _SalaryState(TypedDict):
    messages: Annotated[list[BaseMessage], add_messages]
    round: int
    input_tokens: int
    output_tokens: int
    result: dict[str, Any] | None


def _usage_metadata(message: AIMessage) -> dict[str, Any]:
    return dict(message.usage_metadata or {})


async def _dispatch_one(tool: Any, args: dict[str, Any]) -> tuple[str, str, float, bool]:
    """Runs one lookup and turns every outcome into text the model can react
    to, mirroring `dispatchOne` (loop.go). Never raises: a refused, failed
    or timed-out call is something a model can recover from by asking
    differently (C5-4)."""
    started = time.monotonic()
    try:
        raw = await asyncio.wait_for(
            asyncio.to_thread(tool.func, **args), timeout=PER_TOOL_TIMEOUT_SECONDS
        )
    except TimeoutError:
        dur = time.monotonic() - started
        return (
            untrusted.wrap_result(
                f"TIMED OUT after {PER_TOOL_TIMEOUT_SECONDS:.0f}s. Try a narrower "
                "request or answer without this lookup."
            ),
            "timeout",
            dur,
            False,
        )
    except Exception as exc:  # noqa: BLE001 - turned into model-visible text, not raised
        dur = time.monotonic() - started
        return untrusted.wrap_result(f"FAILED: {exc}"), "failed", dur, False

    dur = time.monotonic() - started
    suspect = untrusted.looks_injected(raw)
    raw_bytes = raw.encode()
    if len(raw_bytes) > MAX_RESULT_BYTES:
        truncated = raw_bytes[:MAX_RESULT_BYTES].decode(errors="ignore")
        content = untrusted.wrap_result(
            f"{truncated}\n\n[TRUNCATED: this result was {len(raw_bytes)} bytes and "
            f"was cut to {MAX_RESULT_BYTES}. You are seeing part of it.]"
        )
        return content, "truncated", dur, suspect
    return untrusted.wrap_result(raw), "ok", dur, suspect


async def _agent_node(
    state: _SalaryState, *, tools: list[Any], trace_id: str | None
) -> dict[str, Any]:
    round_ = state["round"]
    choice = "required" if round_ == 1 else "auto"
    model = gateway.chat_model(TASK_KEY)
    bind_kwargs: dict[str, Any] = {"tool_choice": choice}
    if trace_id is not None:
        bind_kwargs["extra_body"] = tracing.gateway_call_metadata(trace_id)
    bound = model.bind_tools(tools, **bind_kwargs)
    reply = await bound.ainvoke(
        state["messages"], config={"callbacks": [tracing.callback_handler()]}
    )
    assert isinstance(reply, AIMessage)

    usage = _usage_metadata(reply)
    new_input = state["input_tokens"] + (usage.get("input_tokens") or 0)
    new_output = state["output_tokens"] + (usage.get("output_tokens") or 0)

    if not reply.tool_calls and round_ == 1:
        raise CapabilityError(
            "model_unavailable",
            "salary: the model answered without calling a tool on a round that "
            "required one — the serving tier cannot call tools, or the tools were "
            "dropped in transit; no answer produced",
            failed_step=TASK_KEY,
        )
    return {"messages": [reply], "input_tokens": new_input, "output_tokens": new_output}


async def _tools_node(state: _SalaryState, *, tool_map: dict[str, Any]) -> dict[str, Any]:
    last = state["messages"][-1]
    assert isinstance(last, AIMessage)
    round_ = state["round"]
    client = tracing.bootstrap()
    tool_messages: list[ToolMessage] = []

    for call in last.tool_calls:
        name = call["name"]
        args = call.get("args", {})
        call_id = call["id"]
        tool = tool_map.get(name)

        with client.start_as_current_observation(
            name=f"salary.tool_call.{name}", as_type="span", input=args, metadata={"round": round_}
        ) as call_span:
            if tool is None:
                content, outcome, dur, suspect = (
                    untrusted.wrap_result(f"FAILED: unknown tool {name!r}"),
                    "failed",
                    0.0,
                    False,
                )
            else:
                content, outcome, dur, suspect = await _dispatch_one(tool, args)
            call_span.update(
                metadata={"round": round_, "outcome": outcome, "duration_ms": dur * 1000}
            )

        with client.start_as_current_observation(
            name=f"salary.tool_result.{name}",
            as_type="span",
            output=content,
            metadata={"round": round_, "outcome": outcome, "suspected_injection": suspect},
        ):
            pass

        tool_messages.append(ToolMessage(content=content, tool_call_id=call_id, name=name))

    return {"messages": tool_messages, "round": round_ + 1}


async def _finalize_node(state: _SalaryState) -> dict[str, Any]:
    """The typed terminal call: the model's prose from prior rounds is
    discarded, and the answer comes from a schema-constrained call over the
    accumulated conversation — mirroring `CompleteStructuredChat[T]` in
    loop.go's post-loop terminal step. Tools are not bound here (C3-4,
    C4-4): this turn may only answer."""
    model = gateway.chat_model(TASK_KEY)
    schema = json.dumps(SalaryBand.model_json_schema())
    last_error: str | None = None
    parsed: SalaryBand | None = None
    input_tokens = state["input_tokens"]
    output_tokens = state["output_tokens"]

    for _attempt in range(MAX_EXTRA_ATTEMPTS + 1):
        turn = (
            prompts.retry_instruction(schema, last_error)
            if last_error is not None
            else prompts.schema_instruction(schema)
        )
        messages = [*state["messages"], HumanMessage(content=turn)]
        try:
            reply = await model.ainvoke(
                messages, config={"callbacks": [tracing.callback_handler()]}
            )
        except Exception as exc:  # noqa: BLE001 - reclassified below (E5)
            raise classify_provider_error(exc, failed_step=TASK_KEY) from exc

        assert isinstance(reply, AIMessage)
        usage = _usage_metadata(reply)
        input_tokens += usage.get("input_tokens") or 0
        output_tokens += usage.get("output_tokens") or 0
        content = reply.content if isinstance(reply.content, str) else str(reply.content)
        try:
            payload = json.loads(_strip_fences(content))
            parsed = SalaryBand.model_validate(payload)
        except (json.JSONDecodeError, ValidationError) as exc:
            last_error = str(exc)
            continue
        break

    if parsed is None:
        raise CapabilityError(
            "internal",
            f"salary: terminal step failed after {MAX_EXTRA_ATTEMPTS + 1} attempts: {last_error}",
            failed_step=TASK_KEY,
        )

    return {
        "result": parsed.model_dump(),
        "input_tokens": input_tokens,
        "output_tokens": output_tokens,
    }


def _route_after_agent(state: _SalaryState) -> str:
    last = state["messages"][-1]
    tool_calls = getattr(last, "tool_calls", None)
    return "tools" if tool_calls else "finalize"


def _build_graph(*, tools: list[Any], tool_map: dict[str, Any], trace_id: str | None) -> Any:
    graph: StateGraph[_SalaryState] = StateGraph(_SalaryState)
    graph.add_node("agent", functools.partial(_agent_node, tools=tools, trace_id=trace_id))
    graph.add_node("tools", functools.partial(_tools_node, tool_map=tool_map))
    graph.add_node("finalize", _finalize_node)
    graph.add_edge(START, "agent")
    graph.add_conditional_edges(
        "agent", _route_after_agent, {"tools": "tools", "finalize": "finalize"}
    )
    graph.add_edge("tools", "agent")
    graph.add_edge("finalize", END)
    return graph.compile()


async def run(work: SalaryWork, *, trace_id: str | None = None) -> tuple[SalaryBand, Usage]:
    """Runs the salary capability end to end: builds the tool-call loop from
    the snapshot, drives it to a typed answer or a classified failure, and
    applies the same post-loop defaulting `llmInfer` applies (source forced
    to `llm`, confidence defaulted to 0.3 when the model left it at zero).

    Raises `CapabilityError` on any classified failure; never returns a
    partially populated result (C3-2)."""
    try:
        snapshot = SalarySnapshot.model_validate(work.snapshot)
    except ValidationError as exc:
        raise CapabilityError(
            "invalid_input", f"salary: malformed snapshot: {exc}", failed_step=TASK_KEY
        ) from exc

    tools = build_salary_tools(snapshot)
    tool_map = {tool.name: tool for tool in tools}

    user_prompt = prompts.build_user_prompt(
        job_id=snapshot.job_id,
        title=snapshot.title,
        company=snapshot.company,
        location=snapshot.location,
        remote=snapshot.remote,
        description=snapshot.description,
    )
    initial_messages: list[BaseMessage] = [
        SystemMessage(content=untrusted.SYSTEM_FRAMING),
        SystemMessage(content=prompts.SYSTEM_PROMPT),
        HumanMessage(content=user_prompt),
    ]
    initial_state: _SalaryState = {
        "messages": initial_messages,
        "round": 1,
        "input_tokens": 0,
        "output_tokens": 0,
        "result": None,
    }

    graph = _build_graph(tools=tools, tool_map=tool_map, trace_id=trace_id)
    try:
        final_state = await asyncio.wait_for(
            graph.ainvoke(initial_state, config={"recursion_limit": MAX_NODES}),
            timeout=WHOLE_RUN_TIMEOUT_SECONDS,
        )
    except GraphRecursionError as exc:
        raise CapabilityError(
            "bound_exceeded",
            f"salary: exceeded max_tool_rounds bound of {MAX_TOOL_ROUNDS}",
            failed_step=TASK_KEY,
        ) from exc
    except TimeoutError as exc:
        raise CapabilityError(
            "timeout",
            f"salary: run exceeded whole-run timeout of {WHOLE_RUN_TIMEOUT_SECONDS:.0f}s",
            failed_step=TASK_KEY,
        ) from exc
    except CapabilityError:
        raise
    except Exception as exc:  # noqa: BLE001 - reclassified below (E5)
        raise classify_provider_error(exc, failed_step=TASK_KEY) from exc

    band = SalaryBand.model_validate(final_state["result"])
    band = band.model_copy(update={"source": "llm", "confidence": band.confidence or 0.3})
    usage = Usage(
        input_tokens=final_state["input_tokens"] or None,
        output_tokens=final_state["output_tokens"] or None,
        cost_usd=None,
    )
    return band, usage


CAPABILITY = Capability(
    name="salary",
    task_key=TASK_KEY,
    layer="graph_loop",
    input_model="jobfinder_ai.contracts.salary_work:SalaryWork",
    output_model="jobfinder_ai.capabilities.graphs.salary:SalaryBand",
    bounds=CapabilityBounds(
        max_model_calls=MAX_MODEL_CALLS, max_nodes=MAX_NODES, max_tool_rounds=MAX_TOOL_ROUNDS
    ),
    prompt_module="jobfinder_ai.prompts.salary",
    transport="event",
)
