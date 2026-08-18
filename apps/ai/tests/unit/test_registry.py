from __future__ import annotations

import inspect

import pytest

from jobfinder_ai.capabilities import registry as registry_module
from jobfinder_ai.capabilities.registry import (
    Capability,
    CapabilityBounds,
    CapabilityDefinitionError,
    CapabilityRegistry,
)


class DummyInput:
    pass


class DummyOutput:
    pass


def _capability(**overrides: object) -> Capability:
    defaults: dict[str, object] = dict(
        name="ghost",
        task_key="ghost",
        layer="chain",
        input_model="tests.unit.test_registry:DummyInput",
        output_model="tests.unit.test_registry:DummyOutput",
        bounds=CapabilityBounds(max_model_calls=1),
        prompt_module="jobfinder_ai.capabilities",
        transport="event",
    )
    defaults.update(overrides)
    return Capability(**defaults)  # type: ignore[arg-type]


def test_register_accepts_a_well_formed_capability() -> None:
    registry = CapabilityRegistry()
    registry.register(_capability())
    assert len(registry) == 1
    assert registry.get("ghost") is not None


def test_register_rejects_undeclared_task_key() -> None:
    registry = CapabilityRegistry()
    with pytest.raises(CapabilityDefinitionError, match="task key"):
        registry.register(_capability(task_key="not-a-real-key"))


def test_register_rejects_a_second_capability_for_the_same_task_key() -> None:
    registry = CapabilityRegistry()
    registry.register(_capability())
    with pytest.raises(CapabilityDefinitionError, match="already claimed"):
        registry.register(_capability(name="ghost-2"))


def test_register_rejects_non_positive_bounds() -> None:
    registry = CapabilityRegistry()
    with pytest.raises(CapabilityDefinitionError, match="max_model_calls"):
        registry.register(_capability(bounds=CapabilityBounds(max_model_calls=0)))


def test_register_rejects_missing_prompt_module() -> None:
    registry = CapabilityRegistry()
    with pytest.raises(CapabilityDefinitionError, match="prompt module"):
        registry.register(_capability(prompt_module="jobfinder_ai.does_not_exist"))


def test_register_rejects_unimportable_input_model() -> None:
    registry = CapabilityRegistry()
    with pytest.raises(CapabilityDefinitionError, match="input_model"):
        registry.register(_capability(input_model="jobfinder_ai.does_not_exist:Foo"))


def test_register_rejects_input_model_missing_colon() -> None:
    registry = CapabilityRegistry()
    with pytest.raises(CapabilityDefinitionError, match="input_model"):
        registry.register(_capability(input_model="jobfinder_ai.capabilities"))


def test_register_rejects_duplicate_name() -> None:
    registry = CapabilityRegistry()
    registry.register(_capability())
    with pytest.raises(CapabilityDefinitionError, match="already registered"):
        registry.register(_capability(task_key="match"))


# --- T131: a queued (event-transport) capability does not exist through /invoke ---


def test_resolve_for_invoke_returns_none_for_event_transport_capability() -> None:
    registry = CapabilityRegistry()
    registry.register(_capability(transport="event"))
    assert registry.resolve_for_invoke("ghost") is None


def test_resolve_for_invoke_returns_the_capability_for_http_transport() -> None:
    registry = CapabilityRegistry()
    registry.register(_capability(name="rephrase", task_key="rephrase", transport="http"))
    resolved = registry.resolve_for_invoke("rephrase")
    assert resolved is not None
    assert resolved.name == "rephrase"


def test_resolve_for_invoke_returns_none_for_unknown_name() -> None:
    registry = CapabilityRegistry()
    assert registry.resolve_for_invoke("does-not-exist") is None


# --- T080: definitions are in-repo only, never fetched at runtime (FR-015a, C6-2) ---


def test_registry_module_has_no_network_or_database_imports() -> None:
    """register()'s only I/O is importlib against already-installed local
    modules — no HTTP client, no DB driver, nothing that could reach a
    remote registry or database for a capability's definition."""
    source = inspect.getsource(registry_module)
    forbidden = (
        "httpx",
        "requests",
        "aiohttp",
        "urllib",
        "socket",
        "psycopg",
        "asyncpg",
        "sqlalchemy",
        "boto3",
    )
    for name in forbidden:
        assert name not in source, f"registry.py must not import {name!r} — in-repo only"


# --- T081/T083 (US2 scenario 3): an invalid definition fails at registration
# (startup), never at request time — every test_register_rejects_* test above
# already proves this: each calls register() directly and asserts it raises
# before any capability could ever be looked up or invoked. ---
