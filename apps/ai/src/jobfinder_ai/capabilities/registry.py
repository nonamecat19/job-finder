"""The capability registry — the service-side home of prompt assembly, step
sequencing, tool loops, validation and retry (FR-001).

Every capability is validated at registration time: name, task key, layer,
models and prompt module all importable, bounds all positive (C1-1–C1-4,
FR-007). A capability's definition is not readable code until it clears every
check here — an invalid one is a boot failure naming it, never a
request-time surprise.
"""

from __future__ import annotations

import importlib
from collections.abc import Iterator
from dataclasses import dataclass
from typing import Literal

Layer = Literal["chain", "graph_loop", "graph_state"]
Transport = Literal["event", "http"]

# The fourteen task keys declared in gateway/config.yaml (C1-2, C1-3). Kept as
# a literal set here — not read from the YAML at runtime — because the
# registry validates *against* the gateway's contract, it does not derive one.
TASK_KEYS: frozenset[str] = frozenset(
    {
        "match",
        "ghost",
        "rephrase",
        "recruiter",
        "outreach",
        "salary",
        "generation",
        "generation-analyze",
        "generation-select",
        "generation-select-premium",
        "generation-summary",
        "generation-summary-premium",
        "generation-summary-fast",
        "embed",
    }
)


class CapabilityDefinitionError(RuntimeError):
    """A capability definition failed startup validation (C1-4, FR-007)."""


@dataclass(frozen=True, slots=True)
class CapabilityBounds:
    """Bounds enforced by the runtime, not by counters in capability code (C4-2)."""

    max_model_calls: int
    max_nodes: int | None = None
    max_tool_rounds: int | None = None

    def check(self, capability_name: str) -> None:
        for field_name, value in (
            ("max_model_calls", self.max_model_calls),
            ("max_nodes", self.max_nodes),
            ("max_tool_rounds", self.max_tool_rounds),
        ):
            if value is not None and value <= 0:
                raise CapabilityDefinitionError(
                    f"capability {capability_name!r}: bound {field_name!r} must be "
                    f"positive, got {value!r}"
                )


@dataclass(frozen=True, slots=True)
class Capability:
    """A named unit with typed input/output, a declared layer and a task key (C1-1).

    `input_model`, `output_model` and `prompt_module` are dotted import paths
    (`"package.module:AttrName"` for the two models, `"package.module"` for
    the prompt module) rather than live objects, so registration can prove
    each one is actually importable (C1-4) instead of assuming the caller
    already managed to import it.
    """

    name: str
    task_key: str
    layer: Layer
    input_model: str
    output_model: str
    bounds: CapabilityBounds
    prompt_module: str
    transport: Transport


def _import_model(path: str, *, capability_name: str, field_name: str) -> type:
    if ":" not in path:
        raise CapabilityDefinitionError(
            f"capability {capability_name!r}: {field_name} {path!r} must be 'module.path:AttrName'"
        )
    module_path, _, attr_name = path.partition(":")
    try:
        module = importlib.import_module(module_path)
    except ImportError as exc:
        raise CapabilityDefinitionError(
            f"capability {capability_name!r}: {field_name} module {module_path!r} is not importable"
        ) from exc
    try:
        attr = getattr(module, attr_name)
    except AttributeError as exc:
        raise CapabilityDefinitionError(
            f"capability {capability_name!r}: {field_name} {path!r} has no attribute {attr_name!r}"
        ) from exc
    if not isinstance(attr, type):
        raise CapabilityDefinitionError(
            f"capability {capability_name!r}: {field_name} {path!r} is not a type"
        )
    return attr


class CapabilityRegistry:
    """Validates every capability at registration; serves lookups after."""

    def __init__(self) -> None:
        self._capabilities: dict[str, Capability] = {}
        self._task_keys_used: dict[str, str] = {}

    def register(self, capability: Capability) -> None:
        self._validate(capability)
        self._capabilities[capability.name] = capability
        self._task_keys_used[capability.task_key] = capability.name

    def _validate(self, capability: Capability) -> None:
        name = capability.name
        if not name:
            raise CapabilityDefinitionError("capability name must not be empty")
        if name in self._capabilities:
            raise CapabilityDefinitionError(f"capability {name!r} is already registered")

        if capability.task_key not in TASK_KEYS:
            raise CapabilityDefinitionError(
                f"capability {name!r}: task key {capability.task_key!r} is not one of "
                "the fourteen keys declared in gateway/config.yaml (C1-2)"
            )
        existing = self._task_keys_used.get(capability.task_key)
        if existing is not None and existing != name:
            raise CapabilityDefinitionError(
                f"task key {capability.task_key!r} is already claimed by capability "
                f"{existing!r}; C1-3 requires exactly one capability per task key"
            )

        capability.bounds.check(name)

        try:
            importlib.import_module(capability.prompt_module)
        except ImportError as exc:
            raise CapabilityDefinitionError(
                f"capability {name!r}: prompt module {capability.prompt_module!r} is not importable"
            ) from exc

        _import_model(capability.input_model, capability_name=name, field_name="input_model")
        _import_model(capability.output_model, capability_name=name, field_name="output_model")

    def get(self, name: str) -> Capability | None:
        return self._capabilities.get(name)

    def resolve_for_invoke(self, name: str) -> Capability | None:
        """H1-2/T131: a capability whose transport is `event` does not exist through
        `/invoke` — the same 404 as a name that was never registered at all."""
        capability = self._capabilities.get(name)
        if capability is None or capability.transport != "http":
            return None
        return capability

    def __iter__(self) -> Iterator[Capability]:
        return iter(self._capabilities.values())

    def __len__(self) -> int:
        return len(self._capabilities)


# Process-wide registry shared by the FastAPI app and the FastStream consumers
# (research R9). Empty until later tasks register capabilities against it.
registry = CapabilityRegistry()
