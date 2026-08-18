"""The `outreach` capability's prompt (C6-1), ported **verbatim** from
`apps/api/internal/outreach/domain.BuildPrompt` and the system message in
`apps/api/internal/outreach/application/generate.go`'s `generateGrounded`.

The retry-with-feedback loop, the grounding verification (`GroundClaims`),
length enforcement (`EnforceLength`), and the generic-opener fallback all
stay in Go — this capability makes the one model call `generateGrounded`
makes per attempt; the caller decides whether to retry with a new
`last_violation` (T107 territory, not this capability's).
"""

from __future__ import annotations

from typing import Literal

Tone = Literal["warm", "direct", "formal"]

SYSTEM_PROMPT = (
    "You write brief, honest outreach messages. You never state a specific fact "
    "about a company or team that is not explicitly given to you as an allowed "
    "fact. Vagueness is always preferred to invention."
)

MAX_DRAFT_CHARS = 500

_TONE_INSTRUCTION: dict[Tone, str] = {
    "warm": "warm, friendly, and enthusiastic, while staying professional",
    "direct": (
        "direct and concise — get to the point in as few words as possible, minimal pleasantries"
    ),
    "formal": "formal and polished, traditional business register",
}


def build_user_prompt(
    *,
    tone: Tone,
    contact_name: str,
    company_name: str,
    facts: list[tuple[str, str]],
    last_violation: str,
) -> str:
    """Verbatim port of `domain.BuildPrompt` (outreach/domain/grounding.go)."""
    parts: list[str] = []
    parts.append(
        "Write a single short outreach message to a hiring contact after the sender "
        "has just applied to a job at their company.\n\n"
    )

    if contact_name:
        parts.append(f"Address it to: {contact_name}\n")
    else:
        parts.append(
            'No named contact is known — use a neutral salutation such as "Hi '
            'there" and never invent a name.\n'
        )
    if company_name:
        parts.append(f"Company: {company_name}\n")
    parts.append(f"Tone: {_TONE_INSTRUCTION[tone]}\n\n")

    parts.append(
        "ALLOWED FACTS (the ONLY specific things you may state about the team, "
        "company, or role — copy any you use into specificClaims VERBATIM, "
        "unaltered):\n"
    )
    for kind, value in facts:
        parts.append(f"- {kind}: {value}\n")
    parts.append(
        f"\nUse at most one or two of these facts, only if they fit naturally. Keep "
        f"the whole message under {MAX_DRAFT_CHARS} characters. Never state a "
        "specific technology, funding figure, headcount, rating, or any other "
        "detail that is not one of the ALLOWED FACTS above — if you are not sure "
        "something is allowed, leave it out. This is a draft the user will read and "
        "send themselves, so it must contain no send/apply action, just the message "
        "body.\n"
    )

    if last_violation:
        parts.append(
            f"\nYour previous attempt was rejected: {last_violation}. Fix this and answer again.\n"
        )

    return "".join(parts)
