// Package cerebras implements domain.Provider against Cerebras's
// OpenAI-compatible /chat/completions API (001-cerebras-model-toggle).
package cerebras

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

// Provider talks to Cerebras's OpenAI-compatible /chat/completions API.
// Cerebras has no embeddings endpoint, so Embed delegates to an Ollama
// provider (001-cerebras-model-toggle: embeddings always stay on Ollama).
type Provider struct {
	http      *http.Client
	baseURL   string
	apiKey    string
	modelName string
	ollama    domain.Provider
	breaker   rateLimitBreaker
}

// New builds a provider. apiKey is required — the caller (llm.NewProviders)
// only constructs a Provider when config.CerebrasAPIKey is set.
func New(baseURL, apiKey, modelName string, ollama domain.Provider) (*Provider, error) {
	if apiKey == "" {
		return nil, errors.New("cerebras: apiKey is required")
	}
	if baseURL == "" {
		baseURL = "https://api.cerebras.ai/v1"
	}
	if modelName == "" {
		modelName = DefaultModel
	}
	return &Provider{
		http:      &http.Client{Timeout: 120 * time.Second},
		baseURL:   baseURL,
		apiKey:    apiKey,
		modelName: modelName,
		ollama:    ollama,
	}, nil
}

func (c *Provider) ModelName() string { return c.modelName }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model               string            `json:"model"`
	Stream              bool              `json:"stream"`
	Messages            []chatMessage     `json:"messages"`
	Temperature         float64           `json:"temperature"`
	MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
	ResponseFormat      map[string]string `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// errMessage extracts a human-readable message from a Cerebras error body
// when possible, falling back to the raw body. Never includes the request
// (so the API key, sent only in the Authorization header, cannot leak into
// an error string derived from the response).
func errMessage(status int, body []byte) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		switch status {
		case http.StatusTooManyRequests:
			return fmt.Sprintf("cerebras: rate limit or quota exceeded: %s", parsed.Error.Message)
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Sprintf("cerebras: credential rejected: %s", parsed.Error.Message)
		default:
			return fmt.Sprintf("cerebras: returned %d: %s", status, parsed.Error.Message)
		}
	}
	return fmt.Sprintf("cerebras: returned %d: %s", status, string(body))
}

func (c *Provider) chat(ctx context.Context, req chatRequest) (string, error) {
	if c.breaker.tripped() {
		return "", fmt.Errorf("%w: quota still cooling down", ErrRateLimited)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	res, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("cerebras: request failed: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode == http.StatusTooManyRequests {
		c.breaker.tripFor(rateLimitCooldown)
		return "", fmt.Errorf("%w: %s", ErrRateLimited, errMessage(res.StatusCode, data))
	}
	if res.StatusCode >= 400 {
		return "", errors.New(errMessage(res.StatusCode, data))
	}
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("cerebras: invalid response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("cerebras: no choices returned")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (c *Provider) Complete(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	messages := []chatMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, chatMessage{Role: "system", Content: sys})
	}
	messages = append(messages, chatMessage{Role: "user", Content: prompt})

	req := chatRequest{Model: opts.ModelOr(c.modelName), Stream: false, Messages: messages, Temperature: opts.Temp(0.3)}
	if opts != nil && opts.MaxTokens != nil {
		req.MaxCompletionTokens = opts.MaxTokens
	}
	return c.chat(ctx, req)
}

func (c *Provider) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	messages := []chatMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, chatMessage{Role: "system", Content: sys})
	}
	messages = append(messages, chatMessage{Role: "user", Content: prompt})

	return c.chat(ctx, chatRequest{
		Model:          opts.ModelOr(c.modelName),
		Stream:         false,
		Messages:       messages,
		Temperature:    opts.Temp(0.1),
		ResponseFormat: map[string]string{"type": "json_object"},
	})
}

// Embed delegates to the local Ollama embedder — Cerebras offers no
// embeddings API (FR-006: embeddings always stay on Ollama).
func (c *Provider) Embed(ctx context.Context, text string) ([]float32, error) {
	return c.ollama.Embed(ctx, text)
}

var _ domain.Provider = (*Provider)(nil)
