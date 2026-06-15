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

	"github.com/job-finder/api/internal/strutil"
)

// OllamaProvider talks to a local Ollama server (chat + embeddings).
// Mirrors ollama.provider.ts.
type OllamaProvider struct {
	http       *http.Client
	baseURL    string
	modelName  string
	embedModel string
}

func NewOllama(baseURL, modelName, embedModel string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
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
		modelName:  modelName,
		embedModel: embedModel,
	}
}

func (o *OllamaProvider) ModelName() string { return o.modelName }

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
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
	var parsed ollamaChatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("ollama: invalid chat response: %w", err)
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
	return o.chat(ctx, ollamaChatRequest{Model: o.modelName, Stream: false, Messages: messages, Options: chatOpts})
}

func (o *OllamaProvider) CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	messages := []ollamaChatMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, ollamaChatMessage{Role: "system", Content: sys})
	}
	messages = append(messages, ollamaChatMessage{Role: "user", Content: prompt})

	return o.chat(ctx, ollamaChatRequest{
		Model:    o.modelName,
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
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
	var parsed ollamaEmbedResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("ollama: invalid embeddings response: %w", err)
	}
	if len(parsed.Embedding) == 0 {
		return nil, errors.New("ollama returned empty embedding")
	}
	return parsed.Embedding, nil
}
