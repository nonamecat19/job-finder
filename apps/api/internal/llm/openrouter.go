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

// OpenRouterProvider talks to OpenRouter's OpenAI-compatible
// /chat/completions API. Like Cerebras it is chat-only — OpenRouter has no
// embeddings endpoint, so Embed delegates to an Ollama provider.
//
// It shares Cerebras's rateLimitBreaker: OpenRouter's free-tier models are
// quota-limited per day/minute, so the first 429 trips a process-wide
// cooldown and every other queued task fails fast with ErrRateLimited
// instead of burning a request against the same exhausted quota.
type OpenRouterProvider struct {
	http      *http.Client
	baseURL   string
	apiKey    string
	modelName string
	siteURL   string
	appName   string
	ollama    *OllamaProvider
	breaker   rateLimitBreaker
}

// NewOpenRouter builds a provider. apiKey is required — the caller
// (factory.go) only constructs one when config.OpenRouterAPIKey is set.
// siteURL/appName are optional attribution headers; empty values are omitted.
func NewOpenRouter(baseURL, apiKey, modelName, siteURL, appName string, ollama *OllamaProvider) (*OpenRouterProvider, error) {
	if apiKey == "" {
		return nil, errors.New("openrouter: apiKey is required")
	}
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	if modelName == "" {
		modelName = DefaultOpenRouterModel
	}
	return &OpenRouterProvider{
		http:      &http.Client{Timeout: 120 * time.Second},
		baseURL:   baseURL,
		apiKey:    apiKey,
		modelName: modelName,
		siteURL:   siteURL,
		appName:   appName,
		ollama:    ollama,
	}, nil
}

func (c *OpenRouterProvider) ModelName() string { return c.modelName }

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterRequest struct {
	Model          string              `json:"model"`
	Stream         bool                `json:"stream"`
	Messages       []openRouterMessage `json:"messages"`
	Temperature    float64             `json:"temperature"`
	MaxTokens      *int                `json:"max_tokens,omitempty"`
	ResponseFormat map[string]string   `json:"response_format,omitempty"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	// OpenRouter returns upstream failures as a 200 body with an `error`
	// object rather than an HTTP error status, so it is parsed here too.
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *OpenRouterProvider) chat(ctx context.Context, req openRouterRequest) (string, error) {
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
	if c.siteURL != "" {
		httpReq.Header.Set("HTTP-Referer", c.siteURL)
	}
	if c.appName != "" {
		httpReq.Header.Set("X-Title", c.appName)
	}
	res, err := c.http.Do(httpReq)
	if err != nil {
		// Transport failure (DNS, refused, timeout): the provider may well
		// be back on the next attempt, so this is retryable.
		return "", fmt.Errorf("%w: openrouter: request failed: %s", ErrProviderUnavailable, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("%w: openrouter: reading response: %s", ErrProviderUnavailable, err)
	}
	if res.StatusCode == http.StatusTooManyRequests {
		// Honour the provider's own reset hint when it sends one, so the
		// breaker reopens as soon as the quota actually refills.
		c.breaker.tripFor(retryAfter(res.Header))
	}
	if res.StatusCode >= 400 {
		return "", classifyProviderError("openrouter", res.StatusCode, data)
	}
	var parsed openRouterResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("%w: openrouter: %s", ErrInvalidResponse, err)
	}
	// OpenRouter reports upstream (model-side) failures as a 200 carrying an
	// error object, so the same classification runs over the body's own code
	// — a 429 there is the exhausted-quota signal an HTTP 429 would be.
	if parsed.Error != nil && parsed.Error.Message != "" {
		if parsed.Error.Code == http.StatusTooManyRequests {
			c.breaker.tripFor(retryAfter(res.Header))
		}
		return "", classifyProviderError("openrouter", parsed.Error.Code, data)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("%w: openrouter: no choices returned", ErrInvalidResponse)
	}
	return parsed.Choices[0].Message.Content, nil
}

func (c *OpenRouterProvider) Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	req := openRouterRequest{
		Model:       opts.ModelOr(c.modelName),
		Stream:      false,
		Messages:    openRouterMessages(prompt, opts),
		Temperature: opts.Temp(0.3),
	}
	if opts != nil && opts.MaxTokens != nil {
		req.MaxTokens = opts.MaxTokens
	}
	return c.chat(ctx, req)
}

func (c *OpenRouterProvider) CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	return c.chat(ctx, openRouterRequest{
		Model:          opts.ModelOr(c.modelName),
		Stream:         false,
		Messages:       openRouterMessages(prompt, opts),
		Temperature:    opts.Temp(0.1),
		ResponseFormat: map[string]string{"type": "json_object"},
	})
}

func openRouterMessages(prompt string, opts *CompleteOptions) []openRouterMessage {
	messages := []openRouterMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, openRouterMessage{Role: "system", Content: sys})
	}
	return append(messages, openRouterMessage{Role: "user", Content: prompt})
}

// Embed delegates to the local Ollama embedder — OpenRouter offers no
// embeddings API, so embeddings always stay on Ollama.
func (c *OpenRouterProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return c.ollama.Embed(ctx, text)
}

var _ Provider = (*OpenRouterProvider)(nil)
