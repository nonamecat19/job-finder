package aiclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/platform/llm"
)

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
