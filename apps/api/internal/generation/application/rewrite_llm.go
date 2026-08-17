package application

import (
	"context"
	"strings"
	"time"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
)

// rewriteMaxTokens covers 2-3 short phrasings of one bullet.
const rewriteMaxTokens = 512

// rewriteStageTimeout bounds the one call RewriteItem makes — a single
// bullet is a small ask, so this stays well under the other stages'
// deadlines (analyzeStageTimeout etc., service.go).
const rewriteStageTimeout = 45 * time.Second

// buildRewritePrompt asks for 2-3 alternate phrasings of one bullet,
// mirroring the rewording rules buildSelectPrompt already states for its own
// "rephrased" field: same facts, no merged bullets, no new numbers.
func buildRewritePrompt(source string) string {
	var b strings.Builder
	b.WriteString("Reword this one resume bullet 2-3 different ways.\n\n")
	b.WriteString("BULLET:\n")
	b.WriteString(source)
	b.WriteString("\n\nRULES:\n")
	b.WriteString("- Each variant must describe the exact same accomplishment — same employer, same numbers, same scope.\n")
	b.WriteString("- Never add a number, metric, technology or claim that is not already in the bullet above.\n")
	b.WriteString("- Never remove a number or metric that is in the bullet above.\n")
	b.WriteString("- Vary sentence structure and word choice, not content.\n")
	b.WriteString("- Keep each variant to one concise sentence, in the same voice as the original (no \"I\").\n")
	return b.String()
}

// rewriteBullet is the one LLM call the rewrite-variants endpoint makes. Its
// output is untrusted until domain.FilterGroundedRewordings checks it — the
// system prompt asks the model not to fabricate, but the grounding filter is
// what actually enforces it.
func rewriteBullet(ctx context.Context, lc llm.Provider, model, source string) (domain.RewriteVariants, error) {
	ctx, cancel := context.WithTimeout(ctx, rewriteStageTimeout)
	defer cancel()
	maxT := rewriteMaxTokens
	return llm.CompleteStructured[domain.RewriteVariants](ctx, lc, buildRewritePrompt(source), &llm.CompleteOptions{
		System:       "You are an expert resume writer who never fabricates information. You reword a single existing bullet without changing what it claims.",
		Model:        model,
		MaxTokens:    &maxT,
		ResponseMode: llm.ResponseModeStrict,
	})
}
