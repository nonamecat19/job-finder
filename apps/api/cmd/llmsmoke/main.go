// Command llmsmoke is the Go port of apps/api/scripts/llm-smoke.ts — run with
// the ollama container up:
//
//	go run ./cmd/llmsmoke
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/llm"
)

type smokeSchema struct {
	Language   string `json:"language"`
	IsCompiled bool   `json:"isCompiled"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	provider, err := llm.New(cfg)
	if err != nil {
		return err
	}
	ctx := context.Background()

	fmt.Println("model:", provider.ModelName())

	text, err := provider.Complete(ctx, "Reply with the single word: pong", nil)
	if err != nil {
		return fmt.Errorf("complete: %w", err)
	}
	fmt.Println("complete:", text)

	// Exercise each per-task model so both tiers are validated in one run.
	for label, model := range map[string]string{
		"match":      cfg.ModelOr(cfg.LLMModelMatch),
		"generation": cfg.ModelOr(cfg.LLMModelGeneration),
	} {
		out, err := provider.Complete(ctx, "Reply with the single word: pong", &llm.CompleteOptions{Model: model})
		if err != nil {
			return fmt.Errorf("complete[%s=%s]: %w", label, model, err)
		}
		fmt.Printf("complete[%s=%s]: %s\n", label, model, out)
	}

	structured, err := llm.CompleteStructured[smokeSchema](ctx, provider,
		"TypeScript: what language does it compile to, and is it a compiled language?", nil)
	if err != nil {
		return fmt.Errorf("completeStructured: %w", err)
	}
	fmt.Printf("structured: %+v\n", structured)

	emb, err := provider.Embed(ctx, "senior backend engineer, node.js, postgres")
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	fmt.Println("embedding dims:", len(emb))
	return nil
}
