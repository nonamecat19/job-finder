"""The `recruiter` capability's prompts (C6-1), ported **verbatim** from
`apps/api/internal/recruiter/application/{posting,companypage,linkedin}.go`.
All three Go call sites share one system message and one truncation length;
they differ only in the source-specific instruction text, kept here as one
function per source rather than three near-duplicate capability modules
(C1-3: one capability, one task key, for all three scraped-text sources).

Grounding (`groundContact` in Go — verifying a reported field actually
appears verbatim in the source text) is **not** ported here: it stays a
caller-side check in Go, same as it is today for all three sources.
"""

from __future__ import annotations

from typing import Literal

Source = Literal["posting", "company_page", "linkedin"]

SYSTEM_PROMPT = (
    "You extract only what is explicitly written in the given text; you never "
    "fabricate names, titles, or contact details."
)

MAX_TEXT_CHARS = 4000


def build_user_prompt(*, source: Source, text: str) -> str:
    truncated = text[:MAX_TEXT_CHARS]
    if source == "posting":
        return (
            "Read this job posting and identify, if named, the specific human being "
            "who owns this requisition (a recruiter, hiring manager, or similar) — "
            "someone a candidate could reach out to.\n\n"
            "Only report a person, title, email, phone, or LinkedIn URL if it is "
            "EXPLICITLY present in the text below. Never guess or invent a name. A "
            "generic mailbox like jobs@company.com or careers@company.com with no "
            "named person is NOT a contact — leave it out in that case.\n\n"
            f"POSTING TEXT:\n{truncated}\n\n"
            'Return a single JSON object with a "contacts" array containing at most '
            "one entry — the named requisition owner, if any (empty array if none)."
        )
    if source == "company_page":
        return (
            "Read this company About/Team page and list every named human team "
            "member who could plausibly own hiring for a role — a recruiter, talent "
            "acquisition specialist, HR/People team member, or hiring manager.\n\n"
            "Only report a person, title, email, phone, or LinkedIn URL if it is "
            "EXPLICITLY present in the text below. Never guess or invent a name.\n\n"
            f"PAGE TEXT:\n{truncated}\n\n"
            'Return a single JSON object with a "contacts" array (empty if none).'
        )
    return (
        "Read this LinkedIn company People-section page and list every named human "
        "team member who could plausibly own hiring for a role — a recruiter, talent "
        "acquisition specialist, HR/People team member, or hiring manager.\n\n"
        "Only report a person, title, email, phone, or LinkedIn URL if it is "
        "EXPLICITLY present in the text below. Never guess or invent a name.\n\n"
        f"PAGE TEXT:\n{truncated}\n\n"
        'Return a single JSON object with a "contacts" array (empty if none).'
    )
