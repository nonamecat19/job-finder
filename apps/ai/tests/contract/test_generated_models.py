"""Round-trip every JSON Schema fixture in apps/api/internal/events/schema
against its generated Pydantic model in jobfinder_ai.contracts."""

from __future__ import annotations

import importlib
import json
from pathlib import Path
from typing import Any

import pytest
from pydantic import ValidationError

REPO_ROOT = Path(__file__).resolve().parents[4]
SCHEMA_DIR = REPO_ROOT / "apps" / "api" / "internal" / "events" / "schema"

SAMPLE_BY_TYPE: dict[str, Any] = {
    "string": "sample",
    "integer": 1,
    "number": 1.5,
    "boolean": True,
    "object": {"k": "v"},
    "array": ["sample"],
}


def _schema_fixtures() -> list[Path]:
    paths = sorted(SCHEMA_DIR.glob("*.schema.json"))
    assert paths, f"no schema fixtures found in {SCHEMA_DIR}"
    return paths


def _pascal_case(name: str) -> str:
    return "".join(part.capitalize() for part in name.split("_"))


def _sample_value(prop_schema: dict[str, Any]) -> Any:
    if "format" in prop_schema and prop_schema["format"] == "date-time":
        return "2026-08-18T00:00:00Z"
    return SAMPLE_BY_TYPE[prop_schema["type"]]


@pytest.mark.parametrize("schema_path", _schema_fixtures(), ids=lambda p: p.stem)
def test_generated_model_round_trips_schema(schema_path: Path) -> None:
    schema = json.loads(schema_path.read_text())
    title = schema["title"]
    module_name = title
    class_name = _pascal_case(title)

    module = importlib.import_module(f"jobfinder_ai.contracts.{module_name}")
    model_cls = getattr(module, class_name)

    schema_props = set(schema.get("properties", {}))
    model_fields = set(model_cls.model_fields)
    assert model_fields == schema_props, (
        f"{class_name}: model fields {model_fields} != schema properties {schema_props}"
    )

    required = set(schema.get("required", []))
    model_required = {name for name, field in model_cls.model_fields.items() if field.is_required()}
    assert model_required == required, (
        f"{class_name}: required fields {model_required} != schema required {required}"
    )

    payload = {
        name: _sample_value(schema["properties"][name])
        for name in required
        if schema["properties"][name].get("type") is not None
    }
    instance = model_cls.model_validate(payload)
    dumped = instance.model_dump(mode="json", exclude_none=True)
    assert set(dumped) == set(payload)

    assert schema.get("additionalProperties") is False
    with pytest.raises(ValidationError):
        model_cls.model_validate({**payload, "unexpected_extra_field": "nope"})


def test_contracts_package_exports_every_schema_title() -> None:
    contracts = importlib.import_module("jobfinder_ai.contracts")
    for schema_path in _schema_fixtures():
        title = json.loads(schema_path.read_text())["title"]
        class_name = _pascal_case(title)
        assert hasattr(contracts, class_name), (
            f"jobfinder_ai.contracts does not export {class_name} (from {schema_path.name})"
        )
