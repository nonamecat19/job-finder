package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CerebrasProvider talks to Cerebras's OpenAI-compatible /chat/completions
// API. Cerebras has no embeddings endpoint, so Embed delegates to an Ollama
// provider (001-cerebras-model-toggle: embeddings always stay on Ollama).
type CerebrasProvider struct {
	http      *http.Client
	baseURL   string
	apiKey    string
	modelName string
	ollama    *OllamaProvider
}

// NewCerebras builds a provider. apiKey is required — the caller (factory.go)
// only constructs a CerebrasProvider when config.CerebrasAPIKey is set.
func NewCerebras(baseURL, apiKey, modelName string, ollama *OllamaProvider) (*CerebrasProvider, error) {
	if apiKey == "" {
		return nil, errors.New("cerebras: apiKey is required")
	}
	if baseURL == "" {
		baseURL = "https://api.cerebras.ai/v1"
	}
	if modelName == "" {
		modelName = DefaultCerebrasModel
	}
	return &CerebrasProvider{
		http:      &http.Client{Timeout: 120 * time.Second},
		baseURL:   baseURL,
		apiKey:    apiKey,
		modelName: modelName,
		ollama:    ollama,
	}, nil
}

func (c *CerebrasProvider) ModelName() string { return c.modelName }

type cerebrasMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type cerebrasRequest struct {
	Model               string            `json:"model"`
	Stream              bool              `json:"stream"`
	Messages            []cerebrasMessage `json:"messages"`
	Temperature         float64           `json:"temperature"`
	MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
	ResponseFormat      map[string]string `json:"response_format,omitempty"`
}

type cerebrasResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// cerebrasErrMessage extracts a human-readable message from a Cerebras error
// body when possible, falling back to the raw body. Never includes the
// request (so the API key, sent only in the Authorization header, cannot
// leak into an error string derived from the response).
func cerebrasErrMessage(status int, body []byte) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		switch {
		case status == http.StatusTooManyRequests:
			return fmt.Sprintf("cerebras: rate limit or quota exceeded: %s", parsed.Error.Message)
		case status == http.StatusUnauthorized || status == http.StatusForbidden:
			return fmt.Sprintf("cerebras: credential rejected: %s", parsed.Error.Message)
		default:
			return fmt.Sprintf("cerebras: returned %d: %s", status, parsed.Error.Message)
		}
	}
	return fmt.Sprintf("cerebras: returned %d: %s", status, string(body))
}

func (c *CerebrasProvider) chat(ctx context.Context, req cerebrasRequest) (string, error) {
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
	if res.StatusCode >= 400 {
		return "", errors.New(cerebrasErrMessage(res.StatusCode, data))
	}
	var parsed cerebrasResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("cerebras: invalid response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("cerebras: no choices returned")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (c *CerebrasProvider) Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	messages := []cerebrasMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, cerebrasMessage{Role: "system", Content: sys})
	}
	messages = append(messages, cerebrasMessage{Role: "user", Content: prompt})

	req := cerebrasRequest{Model: opts.ModelOr(c.modelName), Stream: false, Messages: messages, Temperature: opts.Temp(0.3)}
	if opts != nil && opts.MaxTokens != nil {
		req.MaxCompletionTokens = opts.MaxTokens
	}
	return c.chat(ctx, req)
}

func (c *CerebrasProvider) CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	messages := []cerebrasMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, cerebrasMessage{Role: "system", Content: sys})
	}
	messages = append(messages, cerebrasMessage{Role: "user", Content: prompt})

	return c.chat(ctx, cerebrasRequest{
		Model:          opts.ModelOr(c.modelName),
		Stream:         false,
		Messages:       messages,
		Temperature:    opts.Temp(0.1),
		ResponseFormat: map[string]string{"type": "json_object"},
	})
}

// Embed delegates to the local Ollama embedder — Cerebras offers no
// embeddings API (FR-006: embeddings always stay on Ollama).
func (c *CerebrasProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return c.ollama.Embed(ctx, text)
}
