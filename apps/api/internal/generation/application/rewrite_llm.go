package application

import (
	"context"
	"strings"
	"time"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
)

const rewriteMaxTokens = 512

const rewriteStageTimeout = 45 * time.Second

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
