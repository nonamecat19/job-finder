package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/job-finder/api/internal/strutil"
)

// OllamaProvider talks to an Ollama server (chat + embeddings), either local
// or Ollama Cloud (https://ollama.com). When apiKey is set it authenticates
// with an Authorization: Bearer header. Mirrors ollama.provider.ts.
type OllamaProvider struct {
	http       *http.Client
	baseURL    string
	apiKey     string
	embedURL   string
	modelName  string
	embedModel string
	// breaker guards Ollama Cloud's per-plan quota. A local server never
	// returns 429, so it never trips there.
	breaker rateLimitBreaker
}

// NewOllama builds a provider. embedURL empty falls back to baseURL; apiKey
// empty means no auth header (local server). Chat and embeddings share apiKey.
func NewOllama(baseURL, apiKey, modelName, embedModel, embedURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if embedURL == "" {
		embedURL = baseURL
	}
	if modelName == "" {
		modelName = "qwen2.5:14b"
	}
	if embedModel == "" {
		embedModel = "nomic-embed-text"
	}
	return &OllamaProvider{
		http:       &http.Client{Timeout: 300 * time.Second}, // local models are slow
		baseURL:    baseURL,
		apiKey:     apiKey,
		embedURL:   embedURL,
		modelName:  modelName,
		embedModel: embedModel,
	}
}

func (o *OllamaProvider) ModelName() string { return o.modelName }

// setHeaders applies the JSON content type and, when configured, Bearer auth.
func (o *OllamaProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Stream   bool                `json:"stream"`
	Format   string              `json:"format,omitempty"`
	Messages []ollamaChatMessage `json:"messages"`
	Options  ollamaChatOptions   `json:"options"`
}

type ollamaChatOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  *int    `json:"num_predict,omitempty"`
}

type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

func (o *OllamaProvider) chat(ctx context.Context, req ollamaChatRequest) (string, error) {
	if o.breaker.tripped() {
		return "", fmt.Errorf("%w: ollama: quota still cooling down", ErrRateLimited)
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	o.setHeaders(httpReq)
	res, err := o.http.Do(httpReq)
	if err != nil {
		// Server down / unreachable: retryable, not a permanent failure.
		return "", fmt.Errorf("%w: ollama: chat request failed: %s", ErrProviderUnavailable, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("%w: ollama: reading chat response: %s", ErrProviderUnavailable, err)
	}
	if res.StatusCode == http.StatusTooManyRequests {
		o.breaker.tripFor(retryAfter(res.Header))
	}
	if res.StatusCode >= 400 {
		return "", classifyProviderError("ollama", res.StatusCode, data)
	}
	var parsed ollamaChatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("%w: ollama: %s", ErrInvalidResponse, err)
	}
	if parsed.Message.Content == "" {
		return "", fmt.Errorf("%w: ollama: empty chat response", ErrInvalidResponse)
	}
	return parsed.Message.Content, nil
}

func (o *OllamaProvider) Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	messages := []ollamaChatMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, ollamaChatMessage{Role: "system", Content: sys})
	}
	messages = append(messages, ollamaChatMessage{Role: "user", Content: prompt})

	chatOpts := ollamaChatOptions{Temperature: opts.Temp(0.3)}
	if opts != nil && opts.MaxTokens != nil {
		chatOpts.NumPredict = opts.MaxTokens
	}
	return o.chat(ctx, ollamaChatRequest{Model: opts.ModelOr(o.modelName), Stream: false, Messages: messages, Options: chatOpts})
}

func (o *OllamaProvider) CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	messages := []ollamaChatMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, ollamaChatMessage{Role: "system", Content: sys})
	}
	messages = append(messages, ollamaChatMessage{Role: "user", Content: prompt})

	return o.chat(ctx, ollamaChatRequest{
		Model:    opts.ModelOr(o.modelName),
		Stream:   false,
		Format:   "json",
		Messages: messages,
		Options:  ollamaChatOptions{Temperature: opts.Temp(0.1)},
	})
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

func (o *OllamaProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	text = strutil.Truncate(text, 8000)
	body, err := json.Marshal(ollamaEmbedRequest{Model: o.embedModel, Prompt: text})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.embedURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	o.setHeaders(req)
	res, err := o.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: ollama: embeddings request failed: %s", ErrProviderUnavailable, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: ollama: reading embeddings response: %s", ErrProviderUnavailable, err)
	}
	if res.StatusCode == http.StatusTooManyRequests {
		o.breaker.tripFor(retryAfter(res.Header))
	}
	if res.StatusCode >= 400 {
		return nil, classifyProviderError("ollama", res.StatusCode, data)
	}
	var parsed ollamaEmbedResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("%w: ollama: %s", ErrInvalidResponse, err)
	}
	if len(parsed.Embedding) == 0 {
		return nil, fmt.Errorf("%w: ollama returned empty embedding", ErrInvalidResponse)
	}
	return parsed.Embedding, nil
}
