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
	// Tool-call plumbing (037). Every field is omitempty so a conversation
	// with no tools in it produces exactly the pre-037 message object.
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

// wireToolCall is the OpenAI-compatible encoding of a tool call. Note
// Function.Arguments is a **string** containing JSON, not an object — the
// encoding that makes C2-12's decoding step necessary in the other direction.
type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// wireTool is a tool declaration on the request.
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
	// Tools and ToolChoice are absent — not null, not [] — when no tools are
	// declared, so every pre-037 request stays byte-identical (C2-1).
	Tools      []wireTool `json:"tools,omitempty"`
	ToolChoice string     `json:"tool_choice,omitempty"`
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

// chatChoice and chatChoiceMessage are named rather than anonymous since 037:
// once the response carries tool calls and a finish reason, an anonymous struct
// has to be restated in full at every construction site, including tests.
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

// send performs one request and returns the parsed response.
//
// This is the lower half of what used to be `chat()`. Everything that must
// happen on *every* path lives here — error classification, the served-model
// log line, and both capture hooks — so that no caller can reach the proxy
// without them. That matters more than it looks: the strict-schema retry calls
// this twice for one logical CompleteJSON, and features 035 and 036 depend on
// seeing both reports.
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

// There is deliberately no `chat() (string, error)` wrapper left here. 037
// split it into send() plus CompleteChat, and Complete/CompleteJSON became
// shims onto CompleteChat rather than onto a second content-only path — so a
// wrapper would be a route to the proxy that skips the tool, response_format
// and retry handling every caller now goes through.

// decodeArguments turns the wire encoding of tool-call arguments into the
// object the registry validates.
//
// The wire form is a JSON *string containing JSON*. Left alone it unmarshals to
// a quoted string, and validating a quoted string against an object schema
// refuses every well-formed call — a false refusal that looks exactly like the
// refusal path working correctly (FR-009a, C2-12).
//
// When unquoting fails, the raw bytes are returned unchanged: genuinely
// malformed arguments must still reach the refusal path rather than being
// smoothed over here.
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
			// The provider-assigned id is preserved verbatim: it is what the
			// tool message carrying the result must quote back (C1-7).
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
		// The schema is already the strict dialect the structured path
		// produces; it is unmarshalled rather than embedded as a string
		// because `parameters` is an object on the wire.
		var params map[string]any
		if err := json.Unmarshal([]byte(t.ArgsSchema), &params); err == nil {
			w.Function.Parameters = params
		}
		out = append(out, w)
	}
	return out
}

// CompleteChat is the single implementation every entry point now goes through.
//
// Complete and CompleteJSON are shims onto it, which is why the JSON-output and
// strict-schema handling lives here rather than in CompleteJSON: the structured
// terminal of a tool exchange needs exactly the same treatment, and two copies
// of this logic would drift the way two schema generators would.
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
				// Schema parse failure — fall back to json_object so the call
				// still goes through with the prompt-appended schema text. The
				// retry below is skipped in this case: there is nothing to fall
				// back *from*, so repeating the request would repeat an
				// identical failure.
				strictMode = false
				req.ResponseFormat = &responseFormat{Type: "json_object"}
			}
		} else {
			req.ResponseFormat = &responseFormat{Type: "json_object"}
		}
	}

	parsed, err := g.send(ctx, req)
	if err != nil && strictMode && isResponseFormatRejection(err) {
		// 033 FR-006: the provider rejected the strict schema — often a sign
		// response_format was unsupported or dropped — so retry once in
		// json_object mode before propagating. This is the runtime guard for
		// the 030-C5 capability trap. It re-enters send(), so one logical call
		// deliberately logs and reports twice.
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

// Complete is a shim onto CompleteChat: [system?, user] at temperature 0.3 with
// no response_format. It has never set one and must not start.
func (g *Provider) Complete(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	res, err := g.CompleteChat(ctx, domain.PromptMessages(opts.SystemPrompt(), prompt), opts.ShimOptions(0.3, false))
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

// CompleteJSON is a shim onto CompleteChat with JSON output requested, at
// temperature 0.1. Requesting JSON is unconditional — including when opts is
// nil, which cmd/llmsmoke does — and the strict-schema handling and its retry
// live in CompleteChat so the loop's structured terminal gets them too.
func (g *Provider) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	res, err := g.CompleteChat(ctx, domain.PromptMessages(opts.SystemPrompt(), prompt), opts.ShimOptions(0.1, true))
	if err != nil {
		return "", err
	}
	return res.Content, nil
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
