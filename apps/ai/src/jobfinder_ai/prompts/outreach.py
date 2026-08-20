"""The `outreach` capability's prompt (C6-1), ported **verbatim** from
`apps/api/internal/outreach/domain.BuildPrompt` and the system message in
`apps/api/internal/outreach/application/generate.go`'s `generateGrounded`.

The retry-with-feedback loop, the grounding verification (`GroundClaims`),
length enforcement (`EnforceLength`), and the generic-opener fallback all
stay in Go — this capability makes the one model call `generateGrounded`
makes per attempt; the caller decides whether to retry with a new
`last_violation` (T107 territory, not this capability's).

`facts` is the grounding allowlist: it is the *only* channel through which a
specific claim about the company may reach the model, and the rules block
below is what makes that allowlist binding. Facts are emitted as
`- kind: value` bullets so the model can copy a used one back verbatim into
`specificClaims`, which is what the Go-side `GroundClaims` check verifies.
"""

from __future__ import annotations

from typing import Literal

from jobfinder_ai.prompts.composition import PromptBuilder

__all__ = [
    "MAX_DRAFT_CHARS",
    "SYSTEM_PROMPT",
    "Tone",
    "build_user_prompt",
]

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

_TASK = (
    "Write a single short outreach message to a hiring contact after the sender "
    "has just applied to a job at their company."
)

_ANONYMOUS_SALUTATION = (
    'No named contact is known — use a neutral salutation such as "Hi there" and '
    "never invent a name."
)

_FACTS_HEADER = (
    "ALLOWED FACTS (the ONLY specific things you may state about the team, "
    "company, or role — copy any you use into specificClaims VERBATIM, "
    "unaltered):"
)

_GROUNDING_RULES = (
    "Use at most one or two of these facts, only if they fit naturally. Keep "
    "the whole message under {max_chars} characters. Never state a "
    "specific technology, funding figure, headcount, rating, or any other "
    "detail that is not one of the ALLOWED FACTS above — if you are not sure "
    "something is allowed, leave it out. This is a draft the user will read and "
    "send themselves, so it must contain no send/apply action, just the message "
    "body."
)

_VIOLATION_FEEDBACK = "Your previous attempt was rejected: {violation}. Fix this and answer again."


def build_user_prompt(
    *,
    tone: Tone,
    contact_name: str,
    company_name: str,
    facts: list[tuple[str, str]],
    last_violation: str,
) -> str:
    """Verbatim port of `domain.BuildPrompt` (outreach/domain/grounding.go).

    An empty `contact_name` or `company_name` is not a missing field to skip
    silently: the contact line is replaced by an explicit instruction never
    to invent a name, and the company line is simply left out.
    """
    prompt = PromptBuilder()
    prompt.paragraph(_TASK)

    if contact_name:
        prompt.field("Address it to", contact_name)
    else:
        prompt.line(_ANONYMOUS_SALUTATION)
    if company_name:
        prompt.field("Company", company_name)
    prompt.field("Tone", _TONE_INSTRUCTION[tone]).blank()

    prompt.line(_FACTS_HEADER)
    prompt.bullets(f"{kind}: {value}" for kind, value in facts)
    prompt.blank().line(_GROUNDING_RULES.format(max_chars=MAX_DRAFT_CHARS))

    if last_violation:
        prompt.blank().line(_VIOLATION_FEEDBACK.format(violation=last_violation))

    return prompt.render()
