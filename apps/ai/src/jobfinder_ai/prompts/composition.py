"""Composition primitives every prompt builder in this package is written on.

The prompts here are byte-for-byte ports of the Go originals (C8-1, C8-2,
FR-021), so the goal of this module is *not* to make the prompt text easier
to change — it is to make the assembly of that text auditable. A builder
that says `.field("Title", title)` or `.bullet(rule)` states which
structural role each fragment plays; the `parts.append("...\n\n")` style it
replaces stated only that some characters were added somewhere.

`PromptBuilder` is a mutable accumulator on purpose: prompt assembly is
full of `if` branches (an optional hints block, an optional violation
footer), and plain Python control flow around a mutable builder reads
better than folding conditionals into a chained expression. Every method
also returns `self`, so straight-line runs still chain.

Nothing here interpolates untrusted text into anything but a plain string —
these prompts carry scraped postings and user profiles, so no fragment is
ever `eval`'d, formatted through a caller-supplied format string, or
otherwise given a second layer of interpretation.
"""

from __future__ import annotations

from collections.abc import Iterable, Sequence
from dataclasses import dataclass, field
from typing import Self

__all__ = ["PromptBuilder", "truncate"]


def truncate(text: str, limit: int) -> str:
    """Port of Go's `strutil.Truncate`: a prefix cut with no ellipsis.

    Go truncates by rune count, which is what slicing a Python `str` already
    does — Python indexes code points, not bytes — so the two agree for every
    input. A non-positive `limit` yields the empty string, and text already
    within `limit` is returned unchanged.
    """
    if limit <= 0:
        return ""
    return text[:limit]


@dataclass(slots=True)
class PromptBuilder:
    """Accumulates prompt fragments and renders them in insertion order.

    The builder never inserts separators of its own: each method appends
    exactly the characters its name describes, so what a builder renders is
    still fully determined by the call sequence.
    """

    _parts: list[str] = field(default_factory=list)

    def text(self, value: str) -> Self:
        """Append `value` with no trailing newline (a mid-line fragment)."""
        self._parts.append(value)
        return self

    def line(self, value: str = "") -> Self:
        """Append `value` followed by a single newline."""
        self._parts.append(value + "\n")
        return self

    def lines(self, values: Iterable[str]) -> Self:
        """Append each value as its own line."""
        for value in values:
            self.line(value)
        return self

    def paragraph(self, value: str) -> Self:
        """Append `value` followed by a blank line."""
        self._parts.append(value + "\n\n")
        return self

    def blank(self, count: int = 1) -> Self:
        """Append `count` bare newlines."""
        self._parts.append("\n" * count)
        return self

    def field(self, label: str, value: object) -> Self:
        """Append a `Label: value` line — the labelled-fact shape the models
        see for job titles, companies, tones and measured signals."""
        self._parts.append(f"{label}: {value}\n")
        return self

    def bullet(self, value: str, *, marker: str = "- ", indent: str = "") -> Self:
        """Append one bullet line."""
        self._parts.append(f"{indent}{marker}{value}\n")
        return self

    def bullets(self, values: Iterable[str], *, marker: str = "- ", indent: str = "") -> Self:
        """Append one bullet line per value."""
        for value in values:
            self.bullet(value, marker=marker, indent=indent)
        return self

    def indexed(self, values: Iterable[str], *, indent: str = "") -> Self:
        """Append `[i] value` lines. The index is the model's handle on the
        item — selection prompts ask for bullets back by these indices, so
        the numbering must stay zero-based and gap-free."""
        for index, value in enumerate(values):
            self._parts.append(f"{indent}[{index}] {value}\n")
        return self

    def section(self, header: str, body: Iterable[str]) -> Self:
        """Append a header line followed by one line per body entry."""
        return self.line(header).lines(body)

    def joined(self, values: Sequence[str], *, separator: str = "\n") -> Self:
        """Append pre-rendered lines joined by `separator`, with no trailing
        separator — for blocks whose final newline the caller controls."""
        self._parts.append(separator.join(values))
        return self

    def render(self) -> str:
        """Render every fragment appended so far, in order."""
        return "".join(self._parts)

    def __str__(self) -> str:
        return self.render()
