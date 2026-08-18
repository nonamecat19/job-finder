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
	"github.com/job-finder/api/internal/strutil"
)

const safetyNetTimeout = 15 * time.Minute

const embedMaxChars = 8000

type Provider struct {
	http      *http.Client
	baseURL   string
	apiKey    string
	embedDims int
}

func New(baseURL, apiKey string, embedDims int) (*Provider, error) {
	if baseURL == "" {
		return nil, errors.New("gateway: baseURL is required")
	}
	if apiKey == "" {
		return nil, errors.New("gateway: apiKey is required")
	}
	return &Provider{
		http:      &http.Client{Timeout: safetyNetTimeout},
		baseURL:   baseURL,
		apiKey:    apiKey,
		embedDims: embedDims,
	}, nil
}

func (g *Provider) ModelName() string { return "gateway" }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`

	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters,omitempty"`
	} `json:"function"`
}

type chatRequest struct {
	Model               string          `json:"model"`
	Stream              bool            `json:"stream"`
	Messages            []chatMessage   `json:"messages"`
	Temperature         float64         `json:"temperature"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	ResponseFormat      *responseFormat `json:"response_format,omitempty"`

	Tools      []wireTool `json:"tools,omitempty"`
	ToolChoice string     `json:"tool_choice,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

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

func embedObservabilityMetadata(ctx context.Context) map[string]any {
	trace := domain.TraceIDFrom(ctx)
	if trace == "" {
		return nil
	}
	return map[string]any{
		"existing_trace_id": trace,
		"generation_name":   "embed",
		"tags":              []string{"embed"},
	}
}

type responseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *jsonSchema `json:"json_schema,omitempty"`
}

type jsonSchema struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict"`
}

type chatChoiceMessage struct {
	Content   string         `json:"content"`
	ToolCalls []wireToolCall `json:"tool_calls"`
}

type chatChoice struct {
	Message      chatChoiceMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type chatResponse struct {
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   struct {
		Cost             float64 `json:"cost"`
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
	} `json:"usage"`
}

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

func (g *Provider) send(ctx context.Context, req chatRequest) (chatResponse, error) {
	requestedGroup := req.Model
	start := time.Now()

	var parsed chatResponse
	body, err := json.Marshal(req)
	if err != nil {
		return parsed, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return parsed, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)
	res, err := g.http.Do(httpReq)
	if err != nil {
		g.logServed(requestedGroup, "unknown", time.Since(start), "error", "")
		return parsed, fmt.Errorf("%w: gateway: request failed: %s", shared.ErrProviderUnavailable, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		g.logServed(requestedGroup, "unknown", time.Since(start), "error", "")
		return parsed, fmt.Errorf("%w: gateway: read response: %s", shared.ErrProviderUnavailable, err)
	}
	modelID := res.Header.Get("x-litellm-model-id")
	if res.StatusCode >= 400 {
		g.logServed(requestedGroup, servedModel(res.Header, chatResponse{}), time.Since(start), "error", modelID)
		return parsed, shared.ClassifyProviderError("gateway", res.StatusCode, data)
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		g.logServed(requestedGroup, servedModel(res.Header, parsed), time.Since(start), "error", modelID)
		return parsed, fmt.Errorf("%w: gateway: invalid response: %s", shared.ErrInvalidResponse, err)
	}
	if len(parsed.Choices) == 0 {
		g.logServed(requestedGroup, servedModel(res.Header, parsed), time.Since(start), "error", modelID)
		return parsed, fmt.Errorf("%w: gateway: no choices returned", shared.ErrInvalidResponse)
	}
	served := servedModel(res.Header, parsed)
	g.logServed(requestedGroup, served, time.Since(start), "ok", modelID)
	domain.ReportServedModel(ctx, served)
	domain.ReportUsage(ctx, usageFrom(res.Header, parsed))
	return parsed, nil
}

func decodeArguments(raw string) json.RawMessage {
	var inner string
	if err := json.Unmarshal([]byte(raw), &inner); err == nil {
		return json.RawMessage(inner)
	}
	return json.RawMessage(raw)
}

func toolCallsFrom(wire []wireToolCall) []domain.ToolCall {
	if len(wire) == 0 {
		return nil
	}
	out := make([]domain.ToolCall, 0, len(wire))
	for _, w := range wire {
		out = append(out, domain.ToolCall{
			ID:   w.ID,
			Name: w.Function.Name,

			Arguments: decodeArguments(w.Function.Arguments),
		})
	}
	return out
}

func wireMessages(msgs []domain.Message) []chatMessage {
	out := make([]chatMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := chatMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, tc := range m.ToolCalls {
			var w wireToolCall
			w.ID = tc.ID
			w.Type = "function"
			w.Function.Name = tc.Name
			w.Function.Arguments = string(tc.Arguments)
			wm.ToolCalls = append(wm.ToolCalls, w)
		}
		out = append(out, wm)
	}
	return out
}

func wireTools(tools []domain.ToolDef) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]wireTool, 0, len(tools))
	for _, t := range tools {
		var w wireTool
		w.Type = "function"
		w.Function.Name = t.Name
		w.Function.Description = t.Description

		var params map[string]any
		if err := json.Unmarshal([]byte(t.ArgsSchema), &params); err == nil {
			w.Function.Parameters = params
		}
		out = append(out, w)
	}
	return out
}

func (g *Provider) CompleteChat(ctx context.Context, msgs []domain.Message, opts *domain.CompleteOptions) (domain.ChatResult, error) {
	req := chatRequest{
		Model:       opts.ModelOr(g.ModelName()),
		Stream:      false,
		Messages:    wireMessages(msgs),
		Temperature: opts.Temp(0.3),
	}
	if opts != nil && opts.MaxTokens != nil {
		req.MaxCompletionTokens = opts.MaxTokens
	}
	req.Metadata = observabilityMetadata(ctx, opts)
	if opts != nil {
		req.Tools = wireTools(opts.Tools)
		if len(req.Tools) > 0 {
			req.ToolChoice = opts.ToolChoice
		}
	}

	strictMode := false
	if opts != nil && opts.JSONOutput {
		strictMode = opts.ResponseMode == domain.ResponseModeStrict && opts.JSONSchema != ""
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

				strictMode = false
				req.ResponseFormat = &responseFormat{Type: "json_object"}
			}
		} else {
			req.ResponseFormat = &responseFormat{Type: "json_object"}
		}
	}

	parsed, err := g.send(ctx, req)
	if err != nil && strictMode && isResponseFormatRejection(err) {

		req.ResponseFormat = &responseFormat{Type: "json_object"}
		parsed, err = g.send(ctx, req)
	}
	if err != nil {
		return domain.ChatResult{}, err
	}

	choice := parsed.Choices[0]
	return domain.ChatResult{
		Content:      choice.Message.Content,
		ToolCalls:    toolCallsFrom(choice.Message.ToolCalls),
		FinishReason: choice.FinishReason,
	}, nil
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
	res, err := g.CompleteChat(ctx, domain.PromptMessages(opts.SystemPrompt(), prompt), opts.ShimOptions(0.3, false))
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

func (g *Provider) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	res, err := g.CompleteChat(ctx, domain.PromptMessages(opts.SystemPrompt(), prompt), opts.ShimOptions(0.1, true))
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

func isResponseFormatRejection(err error) bool {
	return errors.Is(err, shared.ErrModelUnavailable) || errors.Is(err, shared.ErrInvalidResponse)
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type embedResponse struct {
	Model string      `json:"model"`
	Data  []embedData `json:"data"`
	Usage struct {
		Cost         float64 `json:"cost"`
		PromptTokens int     `json:"prompt_tokens"`
		TotalTokens  int     `json:"total_tokens"`
	} `json:"usage"`
}

func (g *Provider) Embed(ctx context.Context, text string) ([]float32, error) {
	const scenario = "embed"
	start := time.Now()
	text = strutil.Truncate(text, embedMaxChars)

	req := embedRequest{Model: scenario, Input: []string{text}}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)
	if md := embedObservabilityMetadata(ctx); md != nil {

		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err == nil {
			raw["metadata"] = md
			if withMD, err := json.Marshal(raw); err == nil {
				httpReq.Body = io.NopCloser(bytes.NewReader(withMD))
				httpReq.ContentLength = int64(len(withMD))
			}
		}
	}

	res, err := g.http.Do(httpReq)
	if err != nil {
		g.logServed(scenario, "unknown", time.Since(start), "error", "")
		return nil, fmt.Errorf("%w: gateway: embed request failed: %s", shared.ErrProviderUnavailable, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		g.logServed(scenario, "unknown", time.Since(start), "error", "")
		return nil, fmt.Errorf("%w: gateway: read embed response: %s", shared.ErrProviderUnavailable, err)
	}
	modelID := res.Header.Get("x-litellm-model-id")
	if res.StatusCode >= 400 {
		g.logServed(scenario, "unknown", time.Since(start), "error", modelID)
		return nil, shared.ClassifyProviderError("gateway", res.StatusCode, data)
	}
	var parsed embedResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		g.logServed(scenario, "unknown", time.Since(start), "error", modelID)
		return nil, fmt.Errorf("%w: gateway: invalid embed response: %s", shared.ErrInvalidResponse, err)
	}
	if len(parsed.Data) == 0 {
		g.logServed(scenario, embedServedModel(res.Header, parsed), time.Since(start), "error", modelID)
		return nil, fmt.Errorf("%w: gateway: no embedding data returned", shared.ErrInvalidResponse)
	}
	vec := parsed.Data[0].Embedding
	served := embedServedModel(res.Header, parsed)
	if g.embedDims > 0 && len(vec) != g.embedDims {
		g.logServed(scenario, served, time.Since(start), "error", modelID)
		return nil, fmt.Errorf("%w: gateway: embedding length %d does not match configured EMBED_DIMS %d",
			shared.ErrInvalidResponse, len(vec), g.embedDims)
	}
	g.logServed(scenario, served, time.Since(start), "ok", modelID)
	domain.ReportServedModel(ctx, served)
	domain.ReportUsage(ctx, embedUsageFrom(res.Header, parsed))
	return vec, nil
}

func embedServedModel(headers http.Header, body embedResponse) string {
	if v := headers.Get("x-litellm-model-name"); v != "" {
		return v
	}
	if body.Model != "" {
		return body.Model
	}
	return "unknown"
}

func embedUsageFrom(headers http.Header, body embedResponse) domain.Usage {
	u := domain.Usage{
		CostUSD:      body.Usage.Cost,
		PromptTokens: body.Usage.PromptTokens,
		ServedGroup:  headers.Get("x-litellm-model-group"),
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

var _ domain.Provider = (*Provider)(nil)
