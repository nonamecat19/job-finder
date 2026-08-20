"""The structured-output turns the Go gateway appends to a chat's final
message (`CompleteStructured` / `CompleteStructuredChat` in `port.go`).

Three capabilities — `ghost`, `match` and `salary` — run the same legacy
`response_format: {"type": "json_object"}` path (C6-4), where the schema
travels as prompt text rather than as a strict `json_schema` response
format. They carried three byte-identical copies of these two functions;
one copy here keeps a wording change from silently applying to two of the
three capabilities and not the third.
"""

from __future__ import annotations

from jobfinder_ai.prompts.composition import PromptBuilder

__all__ = ["retry_instruction", "schema_instruction"]

_SCHEMA_PREAMBLE = "Respond with a single JSON object matching this JSON Schema:"
_RETRY_PREAMBLE = "Your previous answer was invalid:"
_RETRY_DIRECTIVE = "Fix it and answer again with valid JSON only."


def schema_instruction(schema: str) -> str:
    """The trailing turn appended to the final message: the schema-in-prompt
    instruction that makes `response_format: json_object` reliable without
    strict schema mode."""
    return PromptBuilder().blank(2).line(_SCHEMA_PREAMBLE).text(schema).render()


def retry_instruction(schema: str, last_error: str) -> str:
    """The retry turn appended when the previous attempt failed to parse or
    validate: the schema instruction plus what went wrong."""
    return (
        PromptBuilder()
        .text(schema_instruction(schema))
        .line()
        .line(f"{_RETRY_PREAMBLE} {last_error}")
        .text(_RETRY_DIRECTIVE)
        .render()
    )
