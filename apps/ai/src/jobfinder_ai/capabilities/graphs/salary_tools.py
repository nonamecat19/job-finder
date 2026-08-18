"""The `salary` capability's tools (C5-2, T087), ported from
`apps/api/internal/salary/application/tools.go`'s `newSalaryToolset`.

Both lookups are **reads**, and in the Go original that guarantee has a
mechanism behind it: `apps/api/internal/toolfence_test.go` resolves the
package's transitive dependency closure and fails the build if it can reach a
write path. This service has no such closure to police because FR-008 removes
the thing that would make one necessary: it holds no database credentials at
all. Both lookups here run purely over `SalarySnapshot`, data the backend
already gathered and put on the event — there is no live query for a tool
call to escalate into, by construction rather than by fence.

This is the one place the Go and Python tool behaviour necessarily diverges:
`lookup_comparable_bands` in Go can look up *any* title/location the model
asks for, because it queries Postgres per call. Here it can only serve the
one bucket the backend pre-fetched into the snapshot (the posting's own
title/location) — a title/location argument that resolves to a different
bucket gets the same "nothing on file" empty result Go returns for an actual
cache miss, which is what a caller reading only the tool's declared contract
would expect either way.
"""

from __future__ import annotations

import json

from langchain_core.tools import StructuredTool
from pydantic import BaseModel, ConfigDict, Field


def _slugify(value: str) -> str:
    """Verbatim port of `slugify` (service.go)."""
    if not value:
        return ""
    return value.strip().lower().replace(" ", "-").replace(",", "")


def make_bucket(title: str, location: str, company_size: str = "") -> str:
    """Verbatim port of `makeBucket` (service.go)."""
    size_slug = _slugify(company_size) or "unknown"
    return f"{_slugify(title)}|{_slugify(location)}|{size_slug}"


class ComparableBand(BaseModel):
    """One row the backend pre-fetched from `salary_cache` for the posting's
    own bucket, mirroring the `entry` shape `lookup_comparable_bands`
    marshals in Go."""

    model_config = ConfigDict(extra="forbid")

    source: str
    min: int
    max: int
    currency: str


class SalarySnapshot(BaseModel):
    """The grounding data `llmInfer` (Go) read from Postgres and from the
    tool closures over `s`, carried on the event instead since FR-008 denies
    this service database access — the same move `GhostSnapshot` makes for
    `ghost`. `comparable_bands` is whatever rows `GetSalaryCacheByBucket`
    returned for this posting's own bucket at publish time."""

    model_config = ConfigDict(extra="forbid")

    job_id: str
    title: str
    company: str
    location: str | None = None
    remote: bool = False
    description: str
    comparable_bands: list[ComparableBand] = Field(default_factory=list)


class LookupComparableBandsArgs(BaseModel):
    title: str = Field(
        description="Job title to find comparable salary bands for, e.g. 'Senior Backend Engineer'"
    )
    location: str = Field(
        description="City or region for the role, e.g. 'Berlin'. Use 'unknown' if not stated."
    )


class GetPostingDetailsArgs(BaseModel):
    job_id: str = Field(description="The id of the job posting to read in full")


def _lookup_comparable_bands(snapshot: SalarySnapshot, title: str, location: str) -> str:
    bucket = make_bucket(snapshot.title, snapshot.location or "")
    payload = {
        "bucket": bucket,
        "bands": [b.model_dump() for b in snapshot.comparable_bands],
    }
    return json.dumps(payload)


def _get_posting_details(snapshot: SalarySnapshot, job_id: str) -> str:
    if job_id != snapshot.job_id:
        raise ValueError(f"job_id {job_id!r} is not a valid id")
    payload = {
        "title": snapshot.title,
        "company": snapshot.company,
        "location": snapshot.location or "",
        "remote": snapshot.remote,
        "description": snapshot.description,
    }
    return json.dumps(payload)


def build_salary_tools(snapshot: SalarySnapshot) -> list[StructuredTool]:
    """Builds the exchange's tools bound to one snapshot — the Python
    equivalent of `newSalaryToolset` being a method closing over `s`."""
    comparable = StructuredTool.from_function(
        name="lookup_comparable_bands",
        description=(
            "Look up salary bands already recorded for comparable roles. Returns an "
            "empty result when nothing comparable is on file — that is normal and not "
            "an error."
        ),
        func=lambda title, location: _lookup_comparable_bands(snapshot, title, location),
        args_schema=LookupComparableBandsArgs,
    )
    details = StructuredTool.from_function(
        name="get_posting_details",
        description=(
            "Read the full text of a job posting, including the part truncated from "
            "the initial summary."
        ),
        func=lambda job_id: _get_posting_details(snapshot, job_id),
        args_schema=GetPostingDetailsArgs,
    )
    return [comparable, details]
