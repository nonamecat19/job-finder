"""The `rephrase` capability's prompt (C6-1), ported **verbatim** from
`apps/api/internal/coach/application/service.go`'s `buildRephrasePrompt` (the
superset of the two callers — see module docstring on
`capabilities/single/rephrase.py`) and the system message in
`apps/api/internal/keyword/infrastructure/rephraseadapter.ProviderRephraseModel.Rephrase`,
which both the keyword path and coach chat share today (C2-2a).

The prompt has two shapes, not two variants: the coach caller supplies a
`source_label` (seniority, employer, dates) and the keyword caller does not.
The label unlocks three extra anti-inflation rules — they are only meaningful
when the model can see the seniority and dates they forbid it from
overstating — so the rules and the label section are emitted together,
gated on the same condition, and the closing feedback sentence names the
label only when one is present.
"""

from __future__ import annotations

from jobfinder_ai.prompts.composition import PromptBuilder

__all__ = ["SYSTEM_PROMPT", "build_user_prompt"]

SYSTEM_PROMPT = (
    "You reframe existing resume bullets truthfully. You never invent skills, "
    "technologies, employers, job titles, dates, or metrics. If the source does not "
    "support the target term, you return the bullet unchanged."
)

_TASK = (
    "Reframe the candidate's EXISTING resume bullet so that it honestly surfaces "
    "the target skill/term the job wants."
)

_RULES_HEADER = "STRICT RULES:"

_RULE_ONLY_SOURCE = "Rephrase ONLY the experience shown in the source bullet below."
_RULE_NO_INVENTION = (
    "Do NOT add any skill, technology, employer, job title, date, or metric "
    "that is not already in the source bullet."
)

# Only emitted alongside a SOURCE LABEL: each forbids overstating something
# the label is what makes visible.
_LABEL_RULES = (
    "Do NOT inflate seniority. If the source label says 'Junior', do not call it 'Senior'.",
    "Do NOT inflate duration. If the source label says '2022–2024', do not claim '10+ years'.",
    "Do NOT borrow technologies from other entries. Only use what is in the source bullet.",
)

_RULE_NO_STRETCH = (
    "If the source bullet does not genuinely support the target term, do not "
    "stretch it — return the bullet unchanged."
)
_RULE_PLAIN_OUTPUT = (
    "Output the single reframed bullet as plain text. No preamble, no quotes, no bullet marker."
)

_VIOLATION_HEADER = "Your previous attempt violated the no-invention rule:"
_VIOLATION_FOOTER = "Regenerate using only what the source bullet {subject}.\n"
_SUBJECT_WITH_LABEL = "and label contain"
_SUBJECT_WITHOUT_LABEL = "contains"


def build_user_prompt(
    *,
    term: str,
    canonical: str,
    source_bullet: str,
    source_label: str | None,
    prior_violations: list[str],
) -> str:
    """Verbatim port of `buildRephrasePrompt` (coach/application/service.go),
    with the SOURCE LABEL section and its three rules only emitted when
    `source_label` is given (keyword's caller never supplies one).

    `canonical` is the term's normalized form and wins over the raw `term`
    when present — the model is asked for the canonical spelling so the
    keyword match downstream lines up.
    """
    want = canonical if canonical else term

    prompt = PromptBuilder()
    prompt.paragraph(_TASK)
    prompt.line(_RULES_HEADER)
    prompt.bullet(_RULE_ONLY_SOURCE)
    prompt.bullet(_RULE_NO_INVENTION)
    if source_label:
        prompt.bullets(_LABEL_RULES)
    prompt.bullet(_RULE_NO_STRETCH)
    prompt.bullet(_RULE_PLAIN_OUTPUT)
    prompt.blank()

    prompt.field("TARGET TERM", want).blank()
    if source_label:
        prompt.line("SOURCE LABEL (seniority, employer, dates):").paragraph(source_label)
    prompt.line("SOURCE BULLET:").line(source_bullet)

    if prior_violations:
        subject = _SUBJECT_WITH_LABEL if source_label else _SUBJECT_WITHOUT_LABEL
        prompt.blank().line(_VIOLATION_HEADER)
        prompt.text("- ").joined(prior_violations, separator="\n- ")
        prompt.line().text(_VIOLATION_FOOTER.format(subject=subject))

    return prompt.render()
