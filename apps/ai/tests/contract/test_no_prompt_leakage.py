"""FR-002: no work-event payload and no HTTP request body carries prompt text,
a step sequence, or a model/provider name.

This is a shape assertion on the transports themselves, not on any one
capability's data: the request/response models the HTTP surface exposes
(contracts/http.md H3, H4) and the envelope + payload fields the messaging
layer exposes (data-model.md) are inspected for any field that would let a
prompt, a step name, or a model/provider identifier leak across the
boundary.
"""

from __future__ import annotations

import jobfinder_ai.main as main

FORBIDDEN_FIELD_SUBSTRINGS = (
    "prompt",
    "step",
    "model_name",
    "provider",
    "system_message",
    "messages",
)

ALLOWED_FIELDS = {"task_key"}


def _field_names(model: type) -> set[str]:
    return set(model.model_fields.keys())


def test_invoke_request_carries_no_prompt_or_step_or_model_fields() -> None:
    fields = _field_names(main.InvokeRequest) | _field_names(main.InvokeContext)
    _assert_no_forbidden_fields(fields, where="InvokeRequest")


def test_invoke_request_input_is_opaque_capability_data() -> None:
    """H3-3: the request carries only the capability's own input — a dict the
    service does not interpret at the transport layer, so it cannot itself
    name a prompt, step or model."""
    assert main.InvokeRequest.model_fields["input"].annotation is not None


def _assert_no_forbidden_fields(fields: set[str], *, where: str) -> None:
    offending = {
        field
        for field in fields
        if field not in ALLOWED_FIELDS
        and any(term in field.lower() for term in FORBIDDEN_FIELD_SUBSTRINGS)
    }
    assert not offending, f"{where} exposes forbidden field(s): {offending}"
