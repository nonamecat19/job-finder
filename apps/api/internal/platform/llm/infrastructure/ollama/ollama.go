// Package ollama implements domain.Provider against an Ollama server (chat +
// embeddings), either local or Ollama Cloud (https://ollama.com).
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/job-finder/api/internal/platform/llm/domain"
	"github.com/job-finder/api/internal/strutil"
)

// Provider talks to an Ollama server. When apiKey is set it authenticates
// with an Authorization: Bearer header. Mirrors ollama.provider.ts.
type Provider struct {
	http       *http.Client
	baseURL    string
	apiKey     string
	embedURL   string
	modelName  string
	embedModel string
}

// IsHosted reports whether this Ollama server is a hosted/remote instance
// (e.g. Ollama Cloud) rather than a local one, per research.md R2: hosted
// when an API key is set, or when the base URL host is not loopback/private.
// Used by the LLM Router to resolve the admission-gate provider class
// (019-ai-job-throughput).
func (o *Provider) IsHosted() bool {
	if o.apiKey != "" {
		return true
	}
	u, err := url.Parse(o.baseURL)
	if err != nil {
		return true
	}
	return !isLoopbackOrPrivateHost(u.Hostname())
}

func isLoopbackOrPrivateHost(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate()
	}
	return false
}

// New builds a provider. embedURL empty falls back to baseURL; apiKey empty
// means no auth header (local server). Chat and embeddings share apiKey.
func New(baseURL, apiKey, modelName, embedModel, embedURL string) *Provider {
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
	return &Provider{
		http:       &http.Client{Timeout: 300 * time.Second}, // local models are slow
		baseURL:    baseURL,
		apiKey:     apiKey,
		embedURL:   embedURL,
		modelName:  modelName,
		embedModel: embedModel,
	}
}

func (o *Provider) ModelName() string { return o.modelName }

// setHeaders applies the JSON content type and, when configured, Bearer auth.
func (o *Provider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Stream   bool          `json:"stream"`
	Format   string        `json:"format,omitempty"`
	Messages []chatMessage `json:"messages"`
	Options  chatOptions   `json:"options"`
}

type chatOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  *int    `json:"num_predict,omitempty"`
}

type chatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

func (o *Provider) chat(ctx context.Context, req chatRequest) (string, error) {
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
		return "", fmt.Errorf("ollama: chat request failed: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("ollama: chat returned %d: %s", res.StatusCode, string(data))
	}
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("ollama: invalid chat response: %w", err)
	}
	return parsed.Message.Content, nil
}

func (o *Provider) Complete(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	messages := []chatMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, chatMessage{Role: "system", Content: sys})
	}
	messages = append(messages, chatMessage{Role: "user", Content: prompt})

	chatOpts := chatOptions{Temperature: opts.Temp(0.3)}
	if opts != nil && opts.MaxTokens != nil {
		chatOpts.NumPredict = opts.MaxTokens
	}
	return o.chat(ctx, chatRequest{Model: opts.ModelOr(o.modelName), Stream: false, Messages: messages, Options: chatOpts})
}

func (o *Provider) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	messages := []chatMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, chatMessage{Role: "system", Content: sys})
	}
	messages = append(messages, chatMessage{Role: "user", Content: prompt})

	return o.chat(ctx, chatRequest{
		Model:    opts.ModelOr(o.modelName),
		Stream:   false,
		Format:   "json",
		Messages: messages,
		Options:  chatOptions{Temperature: opts.Temp(0.1)},
	})
}

type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

func (o *Provider) Embed(ctx context.Context, text string) ([]float32, error) {
	text = strutil.Truncate(text, 8000)
	body, err := json.Marshal(embedRequest{Model: o.embedModel, Prompt: text})
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
		return nil, fmt.Errorf("ollama: embeddings request failed: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama: embeddings returned %d: %s", res.StatusCode, string(data))
	}
	var parsed embedResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("ollama: invalid embeddings response: %w", err)
	}
	if len(parsed.Embedding) == 0 {
		return nil, errors.New("ollama returned empty embedding")
	}
	return parsed.Embedding, nil
}

var _ domain.Provider = (*Provider)(nil)
