// Package gateway implements domain.Provider against a LiteLLM proxy's
// OpenAI-compatible /chat/completions endpoint (029-litellm-proxy-gateway).
// The proxy maps each task key (sent as the model field) to a configured
// provider+model, so a model change is a config.yaml edit, not a redeploy.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/job-finder/api/internal/platform/llm/domain"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/shared"
)

// safetyNetTimeout is a backstop for a caller that supplies no deadline, not
// the operating limit. The per-call budget belongs to the caller's context:
// resume generation allows 15 minutes, fit analysis 5, and an http.Client
// timeout is absolute — it overrides the context rather than deferring to it,
// so a value below the caller's budget silently truncates every long task.
//
// A 120s cap here previously did exactly that: generation runs with a 900s
// budget died at 120.0s reporting "Client.Timeout exceeded while awaiting
// headers" while the proxy was still working. This value therefore sits above
// the proxy's own worst-case chain (see request_timeout in
// gateway/config.yaml, whose arithmetic is bounded to 600s) so the timeout
// that fires first is always the proxy's — which returns a real upstream
// error — and never this one, which can only report a local deadline.
const safetyNetTimeout = 15 * time.Minute

// Provider talks to a LiteLLM proxy. The proxy has no embeddings endpoint, so
// Embed delegates to an injected Ollama provider (FR-006: embeddings always
// stay on Ollama). Unlike the Cerebras adapter there is deliberately no
// circuit breaker here: the proxy already aggregates upstream quotas and is
// expected to handle provider fallback itself (FR-005 via config.yaml).
type Provider struct {
	http    *http.Client
	baseURL string
	apiKey  string
	ollama  domain.Provider
}

// New builds a provider. baseURL and apiKey are required — the caller
// (llm.NewProviders) only constructs a Provider when config.GatewayURL and
// config.LiteLLMMasterKey are set. apiKey is the proxy's master key; the
// model each request sends is the task key, resolved by the proxy to a real
// provider+model.
func New(baseURL, apiKey string, ollama domain.Provider) (*Provider, error) {
	if baseURL == "" {
		return nil, errors.New("gateway: baseURL is required")
	}
	if apiKey == "" {
		return nil, errors.New("gateway: apiKey is required")
	}
	return &Provider{
		http:    &http.Client{Timeout: safetyNetTimeout},
		baseURL: baseURL,
		apiKey:  apiKey,
		ollama:  ollama,
	}, nil
}

// ModelName is the fallback model identifier; the effective model is always
// the task key supplied via CompleteOptions.Model by the Router (the proxy
// resolves it to a real provider+model). This fallback intentionally names
// the provider rather than a proxy model, so a miswired call that reaches
// the proxy with it fails loudly as an unknown model.
func (g *Provider) ModelName() string { return "gateway" }

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
	ResponseFormat      *responseFormat   `json:"response_format,omitempty"`
}

// responseFormat expresses the OpenAI response_format parameter in both its
// forms: the legacy json_object mode and the strict json_schema mode (033
// FR-005). The pointer is nil when no format is sent (plain-text Complete calls).
type responseFormat struct {
	Type       string      `json:"type"`                 // "json_object" | "json_schema"
	JSONSchema *jsonSchema `json:"json_schema,omitempty"` // present only for json_schema
}

// jsonSchema is the strict-schema payload sent under response_format. The
// schema is a JSON object (not a string) so the provider validates field
// names and types at the API level. Strict=true means additionalProperties
// is false and every required field is enforced.
type jsonSchema struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// servedModel picks the resolved deployment model for FR-012 logging: the
// x-litellm-model-name response header is authoritative on both the primary
// and fallback paths (verified 2026-07-31 — the body's model field echoes
// the *requested* group name on a primary-tier hit, only becoming the
// resolved model once a fallback fires; see research.md R4). Falls back to
// the body field, then "unknown". Never errors — an unparsable/absent field
// is a logging gap, not a request failure.
func servedModel(headers http.Header, body chatResponse) string {
	if v := headers.Get("x-litellm-model-name"); v != "" {
		return v
	}
	if body.Model != "" {
		return body.Model
	}
	return "unknown"
}

func (g *Provider) chat(ctx context.Context, req chatRequest) (string, error) {
	requestedGroup := req.Model
	start := time.Now()

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)
	res, err := g.http.Do(httpReq)
	if err != nil {
		g.logServed(requestedGroup, "unknown", time.Since(start), "error", "")
		return "", fmt.Errorf("%w: gateway: request failed: %s", shared.ErrProviderUnavailable, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		g.logServed(requestedGroup, "unknown", time.Since(start), "error", "")
		return "", fmt.Errorf("%w: gateway: read response: %s", shared.ErrProviderUnavailable, err)
	}
	modelID := res.Header.Get("x-litellm-model-id")
	if res.StatusCode >= 400 {
		g.logServed(requestedGroup, servedModel(res.Header, chatResponse{}), time.Since(start), "error", modelID)
		return "", shared.ClassifyProviderError("gateway", res.StatusCode, data)
	}
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		g.logServed(requestedGroup, servedModel(res.Header, parsed), time.Since(start), "error", modelID)
		return "", fmt.Errorf("%w: gateway: invalid response: %s", shared.ErrInvalidResponse, err)
	}
	if len(parsed.Choices) == 0 {
		g.logServed(requestedGroup, servedModel(res.Header, parsed), time.Since(start), "error", modelID)
		return "", fmt.Errorf("%w: gateway: no choices returned", shared.ErrInvalidResponse)
	}
	served := servedModel(res.Header, parsed)
	g.logServed(requestedGroup, served, time.Since(start), "ok", modelID)
	domain.ReportServedModel(ctx, served)
	return parsed.Choices[0].Message.Content, nil
}

// logServed emits one structured line per gateway request (FR-012): task key,
// requested group (the same value — the Router always sends the task key as
// the model), served model, duration, outcome. Logging never changes error
// classification or introduces a new failure mode.
func (g *Provider) logServed(requestedGroup, served string, dur time.Duration, outcome, modelID string) {
	attrs := []any{
		"task", requestedGroup,
		"requested_group", requestedGroup,
		"served_model", served,
		"duration_ms", dur.Milliseconds(),
		"outcome", outcome,
	}
	if modelID != "" {
		attrs = append(attrs, "litellm_model_id", modelID)
	}
	slog.Info("gateway request", attrs...)
}

func (g *Provider) Complete(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	messages := []chatMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, chatMessage{Role: "system", Content: sys})
	}
	messages = append(messages, chatMessage{Role: "user", Content: prompt})

	req := chatRequest{Model: opts.ModelOr(g.ModelName()), Stream: false, Messages: messages, Temperature: opts.Temp(0.3)}
	if opts != nil && opts.MaxTokens != nil {
		req.MaxCompletionTokens = opts.MaxTokens
	}
	return g.chat(ctx, req)
}

func (g *Provider) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	messages := []chatMessage{}
	if sys := opts.SystemPrompt(); sys != "" {
		messages = append(messages, chatMessage{Role: "system", Content: sys})
	}
	messages = append(messages, chatMessage{Role: "user", Content: prompt})

	req := chatRequest{
		Model:       opts.ModelOr(g.ModelName()),
		Stream:      false,
		Messages:    messages,
		Temperature: opts.Temp(0.1),
	}
	if opts != nil && opts.MaxTokens != nil {
		req.MaxCompletionTokens = opts.MaxTokens
	}

	strictMode := opts != nil && opts.ResponseMode == domain.ResponseModeStrict && opts.JSONSchema != ""
	if strictMode {
		var schemaMap map[string]any
		if err := json.Unmarshal([]byte(opts.JSONSchema), &schemaMap); err == nil {
			req.ResponseFormat = &responseFormat{
				Type: "json_schema",
				JSONSchema: &jsonSchema{
					Name:   "tailored_output",
					Schema: schemaMap,
					Strict: true,
				},
			}
		} else {
			// Schema parse failure — fall back to json_object so the call
			// still goes through with the prompt-appended schema text.
			strictMode = false
			req.ResponseFormat = &responseFormat{Type: "json_object"}
		}
	} else {
		req.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	text, err := g.chat(ctx, req)
	if err != nil {
		// 033 FR-006: if the provider rejected the strict schema (a 400/422
		// that often signals response_format is unsupported or dropped),
		// retry once with json_object mode before propagating the error.
		// This prevents the 030-C5 capability trap at runtime — the
		// existing JSON-parse retry loop in CompleteStructured handles
		// any malformed output from the fallback.
		if strictMode && isResponseFormatRejection(err) {
			req.ResponseFormat = &responseFormat{Type: "json_object"}
			return g.chat(ctx, req)
		}
	}
	return text, err
}

// isResponseFormatRejection returns true when the error suggests the provider
// did not accept the response_format parameter (a 400/422 from the proxy
// classifying as an unavailable/unsupported model or param). This is the
// runtime guard for the 030-C5 capability trap: drop_params:true would
// silently drop an unsupported json_schema, degrading into prose the app
// cannot parse. Rather than rely on config-time verification alone, this
// fallback catches a provider that rejects the param loudly.
func isResponseFormatRejection(err error) bool {
	return errors.Is(err, shared.ErrModelUnavailable) || errors.Is(err, shared.ErrInvalidResponse)
}

// Embed delegates to the local Ollama embedder — the proxy offers no
// embeddings API (FR-006: embeddings always stay on Ollama).
func (g *Provider) Embed(ctx context.Context, text string) ([]float32, error) {
	return g.ollama.Embed(ctx, text)
}

var _ domain.Provider = (*Provider)(nil)
