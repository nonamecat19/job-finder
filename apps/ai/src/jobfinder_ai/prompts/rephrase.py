"""The `rephrase` capability's prompt (C6-1), ported **verbatim** from
`apps/api/internal/coach/application/service.go`'s `buildRephrasePrompt` (the
superset of the two callers — see module docstring on
`capabilities/single/rephrase.py`) and the system message in
`apps/api/internal/keyword/infrastructure/rephraseadapter.ProviderRephraseModel.Rephrase`,
which both the keyword path and coach chat share today (C2-2a). The
`source_label` section and its two seniority/duration rules are ported from
coach's superset; they are simply never emitted when `source_label` is
absent, which is exactly the keyword caller's case.
"""

from __future__ import annotations

SYSTEM_PROMPT = (
    "You reframe existing resume bullets truthfully. You never invent skills, "
    "technologies, employers, job titles, dates, or metrics. If the source does not "
    "support the target term, you return the bullet unchanged."
)


def build_user_prompt(
    *,
    term: str,
    canonical: str,
    source_bullet: str,
    source_label: str | None,
    prior_violations: list[str],
) -> str:
    """Verbatim port of `buildRephrasePrompt` (coach/application/service.go),
    with the SOURCE LABEL section and its two rules only emitted when
    `source_label` is given (keyword's caller never supplies one)."""
    want = canonical if canonical else term

    parts: list[str] = []
    parts.append(
        "Reframe the candidate's EXISTING resume bullet so that it honestly surfaces "
        "the target skill/term the job wants.\n\n"
    )
    parts.append("STRICT RULES:\n")
    parts.append("- Rephrase ONLY the experience shown in the source bullet below.\n")
    parts.append(
        "- Do NOT add any skill, technology, employer, job title, date, or metric "
        "that is not already in the source bullet.\n"
    )
    if source_label:
        parts.append(
            "- Do NOT inflate seniority. If the source label says 'Junior', do not "
            "call it 'Senior'.\n"
        )
        parts.append(
            "- Do NOT inflate duration. If the source label says '2022–2024', do not "
            "claim '10+ years'.\n"
        )
        parts.append(
            "- Do NOT borrow technologies from other entries. Only use what is in "
            "the source bullet.\n"
        )
    parts.append(
        "- If the source bullet does not genuinely support the target term, do not "
        "stretch it — return the bullet unchanged.\n"
    )
    parts.append(
        "- Output the single reframed bullet as plain text. No preamble, no quotes, "
        "no bullet marker.\n\n"
    )
    parts.append(f"TARGET TERM: {want}\n\n")
    if source_label:
        parts.append(f"SOURCE LABEL (seniority, employer, dates):\n{source_label}\n\n")
    parts.append(f"SOURCE BULLET:\n{source_bullet}\n")
    if prior_violations:
        parts.append("\nYour previous attempt violated the no-invention rule:\n- ")
        parts.append("\n- ".join(prior_violations))
        suffix = "and label contain" if source_label else "contains"
        parts.append(f"\nRegenerate using only what the source bullet {suffix}.\n")
    return "".join(parts)
