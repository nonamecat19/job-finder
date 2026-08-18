package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/platform/llm"
)

type smokeSchema struct {
	Language   string `json:"language"`
	IsCompiled bool   `json:"isCompiled"`
}

func main() {
	task := flag.String("task", "match", "task key to route one chat request through")
	embedText := flag.String("embed", "", "run one embed request for this text and print its vector length")
	embedCheck := flag.Bool("embed-check", false, "asymmetry sanity check (quickstart.md step 5): identical text -> identical vector; a related pair scores above an unrelated pair")
	flag.Parse()

	if err := run(*task, *embedText, *embedCheck); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(task, embedText string, embedCheck bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	gw, err := llm.NewGateway(cfg.GatewayURL, cfg.LiteLLMMasterKey, cfg.EmbedDims)
	if err != nil {
		return err
	}
	ctx := context.Background()

	if embedCheck {
		return runEmbedCheck(ctx, gw)
	}
	if embedText != "" {
		return runEmbed(ctx, gw, embedText)
	}
	return runChat(ctx, gw, task)
}

func runChat(ctx context.Context, gw llm.Provider, task string) error {
	router := llm.NewRouter(task, gw)

	fmt.Println("task:", task)

	text, err := router.Complete(ctx, "Reply with the single word: pong", nil)
	if err != nil {
		return fmt.Errorf("complete[%s]: %w", task, err)
	}
	fmt.Printf("complete[%s]: %s\n", task, text)

	structured, err := llm.CompleteStructured[smokeSchema](ctx, router,
		"TypeScript: what language does it compile to, and is it a compiled language?", nil)
	if err != nil {
		return fmt.Errorf("completeStructured: %w", err)
	}
	fmt.Printf("structured: %+v\n", structured)
	return nil
}

func runEmbed(ctx context.Context, gw llm.Provider, text string) error {
	ctx, served := llm.WithServedModelCapture(ctx)

	vec, err := gw.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	fmt.Println("embedding dims:", len(vec))
	fmt.Println("served model:", *served)
	return nil
}

func runEmbedCheck(ctx context.Context, gw llm.Provider) error {
	const query = "golang backend engineer"
	const related = "go developer, backend"
	const unrelated = "pastry chef, Lyon"

	a1, err := gw.Embed(ctx, query)
	if err != nil {
		return fmt.Errorf("embed(query) 1st call: %w", err)
	}
	a2, err := gw.Embed(ctx, query)
	if err != nil {
		return fmt.Errorf("embed(query) 2nd call: %w", err)
	}
	if !identical(a1, a2) {
		return fmt.Errorf("embed-check: identical text produced different vectors — " +
			"the deployment is not deterministic for repeated calls")
	}
	fmt.Println("identical text -> identical vector: OK")

	relatedVec, err := gw.Embed(ctx, related)
	if err != nil {
		return fmt.Errorf("embed(related): %w", err)
	}
	unrelatedVec, err := gw.Embed(ctx, unrelated)
	if err != nil {
		return fmt.Errorf("embed(unrelated): %w", err)
	}

	relatedScore := cosineSimilarity(a1, relatedVec)
	unrelatedScore := cosineSimilarity(a1, unrelatedVec)
	fmt.Printf("related pair score: %.4f\n", relatedScore)
	fmt.Printf("unrelated pair score: %.4f\n", unrelatedScore)

	if relatedScore <= unrelatedScore {
		return fmt.Errorf("embed-check: related pair (%.4f) did not score above the unrelated pair (%.4f) — "+
			"check that the embed deployment declares input_type: search_document (research.md R2) "+
			"before touching the match threshold", relatedScore, unrelatedScore)
	}
	fmt.Println("related pair scores above unrelated pair: OK")
	return nil
}

func identical(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
