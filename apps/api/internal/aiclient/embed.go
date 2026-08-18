package aiclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/platform/llm"
)

// EmbedRoutedProvider wraps an llm.Provider, diverting only Embed calls to
// the AI service's `embed` capability once AI_CAPABILITY_ROUTING routes it
// to python (C2-2, research R11 — embed is migrated last). Every other
// method passes through to Provider untouched, including Complete/
// CompleteChat calls made through the same Router an Embed call goes
// through (matching's llmc calls both).
type EmbedRoutedProvider struct {
	llm.Provider
	Client  *Client
	Routing func(capability string) string
}

type embedInput struct {
	Text string `json:"text"`
}

type embedResult struct {
	Vector []float32 `json:"vector"`
}

// Embed implements llm.Provider, overriding only the routed case.
func (p *EmbedRoutedProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if p.Routing == nil || p.Routing("embed") != "python" {
		return p.Provider.Embed(ctx, text)
	}

	resp, err := p.Client.Invoke(ctx, "embed", embedInput{Text: text}, RequestContext{})
	if err != nil {
		return nil, fmt.Errorf("aiclient: embed: %w", err)
	}
	if resp.Status != events.ResultSucceeded {
		msg := ""
		if resp.Failure != nil {
			msg = resp.Failure.Message
		}
		return nil, fmt.Errorf("aiclient: embed failed: %s", msg)
	}

	var out embedResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		return nil, fmt.Errorf("aiclient: embed: unmarshal result: %w", err)
	}
	return out.Vector, nil
}
