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
	"strconv"
	"time"

	"github.com/job-finder/api/internal/platform/llm/domain"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/shared"
)

const safetyNetTimeout = 15 * time.Minute

type Provider struct {
	http    *http.Client
	baseURL string
	apiKey  string
	ollama  domain.Provider
}

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

func (g *Provider) ModelName() string { return "gateway" }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model               string          `json:"model"`
	Stream              bool            `json:"stream"`
	Messages            []chatMessage   `json:"messages"`
	Temperature         float64         `json:"temperature"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	ResponseFormat      *responseFormat `json:"response_format,omitempty"`
	// Metadata carries observability grouping keys to the proxy's collector
	// callbacks (036). omitempty on a nil map keeps the body byte-identical to
	// the pre-036 request when nothing is set — absent, not null, not {}.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// observabilityMetadata builds the proxy metadata object from the call options,
// returning nil when there is nothing to send.
//
// Two details are load-bearing and easy to get wrong (036 research R5/R6):
//
//   - The correlation key is existing_trace_id, NOT trace_id. trace_id creates
//     a trace and rewrites its name, input, output and tags on every call, so a
//     multi-call run would end up described by whichever call finished last.
//     existing_trace_id appends without overwriting.
//   - generation_name must carry the requested task key. The collector records
//     `model` as the *served deployment*, so without this two stages served by
//     the same model (generation-summary and generation-select-premium both
//     resolve to the same one) collapse into a single reporting bucket —
//     erasing exactly the per-stage distinction 035 exists to create.
//
// The trace id normally arrives on the context (domain.WithTraceID), stamped
// once per run; CompleteOptions.TraceID is an explicit per-call override for
// the rare caller that needs one.
func observabilityMetadata(ctx context.Context, opts *domain.CompleteOptions) map[string]any {
	trace, task := opts.Trace(), opts.Task()
	if trace == "" {
		trace = domain.TraceIDFrom(ctx)
	}
	if trace == "" && task == "" {
		return nil
	}
	md := map[string]any{}
	if trace != "" {
		md["existing_trace_id"] = trace
	}
	if task != "" {
		md["generation_name"] = task
		md["tags"] = []string{task}
	}
	return md
}

// responseFormat expresses the OpenAI response_format parameter in both its
// forms: the legacy json_object mode and the strict json_schema mode (033
// FR-005). The pointer is nil when no format is sent (plain-text Complete calls).
type responseFormat struct {
	Type       string      `json:"type"`                  // "json_object" | "json_schema"
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
	Usage struct {
		Cost             float64 `json:"cost"`
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
	} `json:"usage"`
}

// usageFrom reads the economics of a call from the proxy's response headers,
// falling back to the body's usage block for cost. Every field is optional: a
// header the proxy did not send, or one it sent malformed, leaves its field
// zero rather than failing a call that otherwise succeeded.
func usageFrom(headers http.Header, body chatResponse) domain.Usage {
	u := domain.Usage{
		CostUSD:          body.Usage.Cost,
		PromptTokens:     body.Usage.PromptTokens,
		CompletionTokens: body.Usage.CompletionTokens,
		ServedGroup:      headers.Get("x-litellm-model-group"),
	}
	if u.CostUSD == 0 {
		if c, err := strconv.ParseFloat(headers.Get("x-litellm-response-cost"), 64); err == nil {
			u.CostUSD = c
		}
	}
	if n, err := strconv.Atoi(headers.Get("x-litellm-attempted-fallbacks")); err == nil && n > 0 {
		u.AttemptedFallbacks = n
		u.Substituted = true
	}
	return u
}

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
	domain.ReportUsage(ctx, usageFrom(res.Header, parsed))
	return parsed.Choices[0].Message.Content, nil
}

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
	req.Metadata = observabilityMetadata(ctx, opts)
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
	req.Metadata = observabilityMetadata(ctx, opts)

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

func (g *Provider) Embed(ctx context.Context, text string) ([]float32, error) {
	return g.ollama.Embed(ctx, text)
}

var _ domain.Provider = (*Provider)(nil)
