"""The `recruiter` capability's prompts (C6-1), ported **verbatim** from
`apps/api/internal/recruiter/application/{posting,companypage,linkedin}.go`.
All three Go call sites share one system message and one truncation length;
they differ only in the source-specific instruction text, kept here as one
`_SourceTemplate` per source rather than three near-duplicate capability
modules (C1-3: one capability, one task key, for all three scraped-text
sources).

Modelling the three as data rather than as three `if` branches keeps the
shared spine — the never-fabricate rule, the labelled text block, the
`contacts`-array output contract — written once, so a source cannot quietly
lose one of them. The parts that genuinely differ (the task sentence, the
label on the text block, the output line) stay per-source fields.

Grounding (`groundContact` in Go — verifying a reported field actually
appears verbatim in the source text) is **not** ported here: it stays a
caller-side check in Go, same as it is today for all three sources.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

from jobfinder_ai.prompts.composition import PromptBuilder, truncate

__all__ = ["MAX_TEXT_CHARS", "SYSTEM_PROMPT", "Source", "build_user_prompt"]

Source = Literal["posting", "company_page", "linkedin"]

SYSTEM_PROMPT = (
    "You extract only what is explicitly written in the given text; you never "
    "fabricate names, titles, or contact details."
)

MAX_TEXT_CHARS = 4000

_NEVER_FABRICATE = (
    "Only report a person, title, email, phone, or LinkedIn URL if it is "
    "EXPLICITLY present in the text below. Never guess or invent a name."
)

_GENERIC_MAILBOX_RULE = (
    " A generic mailbox like jobs@company.com or careers@company.com with no "
    "named person is NOT a contact — leave it out in that case."
)

_TEAM_MEMBER_TASK = (
    "list every named human "
    "team member who could plausibly own hiring for a role — a recruiter, talent "
    "acquisition specialist, HR/People team member, or hiring manager."
)

_CONTACTS_ARRAY = 'Return a single JSON object with a "contacts" array (empty if none).'


@dataclass(frozen=True, slots=True)
class _SourceTemplate:
    """The three scraped-text sources differ only in these four fragments."""

    task: str
    rules: str
    text_label: str
    output: str


_TEMPLATES: dict[Source, _SourceTemplate] = {
    "posting": _SourceTemplate(
        task=(
            "Read this job posting and identify, if named, the specific human being "
            "who owns this requisition (a recruiter, hiring manager, or similar) — "
            "someone a candidate could reach out to."
        ),
        rules=_NEVER_FABRICATE + _GENERIC_MAILBOX_RULE,
        text_label="POSTING TEXT:",
        output=(
            'Return a single JSON object with a "contacts" array containing at most '
            "one entry — the named requisition owner, if any (empty array if none)."
        ),
    ),
    "company_page": _SourceTemplate(
        task="Read this company About/Team page and " + _TEAM_MEMBER_TASK,
        rules=_NEVER_FABRICATE,
        text_label="PAGE TEXT:",
        output=_CONTACTS_ARRAY,
    ),
    "linkedin": _SourceTemplate(
        task="Read this LinkedIn company People-section page and " + _TEAM_MEMBER_TASK,
        rules=_NEVER_FABRICATE,
        text_label="PAGE TEXT:",
        output=_CONTACTS_ARRAY,
    ),
}


def build_user_prompt(*, source: Source, text: str) -> str:
    """Verbatim port of the three Go call sites' prompts, truncating the
    scraped text at 4000 characters exactly as they do."""
    template = _TEMPLATES[source]
    return (
        PromptBuilder()
        .paragraph(template.task)
        .paragraph(template.rules)
        .line(template.text_label)
        .paragraph(truncate(text, MAX_TEXT_CHARS))
        .text(template.output)
        .render()
    )
