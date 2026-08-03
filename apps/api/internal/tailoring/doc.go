// Package tailoring implements the constrained AI resume tailoring flow
// described in specs/020-ai-resume-tailoring: it owns the edit-proposal
// lifecycle (propose -> review -> accept/reject -> finalize), baseline
// resolution (the persisted RendercvMaster snapshot each draft diffs
// against, updated in place as proposals are accepted), and triggers the
// single-page PDF export once a draft is finalized. It does not itself
// talk to the LLM (that stays in internal/generation) or render PDFs (that
// lives in internal/generation/singlepage); this package is the
// review-gated state machine that sits between the two.
package tailoring
