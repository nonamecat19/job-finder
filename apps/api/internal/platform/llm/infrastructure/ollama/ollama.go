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

type Provider struct {
	http       *http.Client
	baseURL    string
	apiKey     string
	embedURL   string
	modelName  string
	embedModel string
}

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
		http:       &http.Client{Timeout: 300 * time.Second},
		baseURL:    baseURL,
		apiKey:     apiKey,
		embedURL:   embedURL,
		modelName:  modelName,
		embedModel: embedModel,
	}
}

func (o *Provider) ModelName() string { return o.modelName }

func (o *Provider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Ollama's native format carries tool calls on the message and tool
	// results as a plain `tool` role message. It has no id field anywhere,
	// which is why ids are synthesised locally and never sent (C2-10).
	ToolCalls []wireToolCall `json:"tool_calls,omitempty"`
	Name      string         `json:"name,omitempty"`
}

// wireToolCall is Ollama's shape: arguments are an **object**, not a string —
// the opposite of the OpenAI-compatible encoding, and the reason each adapter
// owns its own decoding.
type wireToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
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
	Model    string        `json:"model"`
	Stream   bool          `json:"stream"`
	Format   string        `json:"format,omitempty"`
	Messages []chatMessage `json:"messages"`
	Tools    []wireTool    `json:"tools,omitempty"`
	Options  chatOptions   `json:"options"`
}

type chatOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  *int    `json:"num_predict,omitempty"`
}

type chatResponse struct {
	Message struct {
		Content   string         `json:"content"`
		ToolCalls []wireToolCall `json:"tool_calls"`
	} `json:"message"`
	DoneReason string `json:"done_reason"`
}

// Same as the gateway adapter: no content-only wrapper survives 037. Every
// path reaches the model through CompleteChat, so the num_predict and `format`
// rules that distinguish the two entry points live in one place.

func (o *Provider) send(ctx context.Context, req chatRequest) (chatResponse, error) {
	var parsed chatResponse
	body, err := json.Marshal(req)
	if err != nil {
		return parsed, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return parsed, err
	}
	o.setHeaders(httpReq)
	res, err := o.http.Do(httpReq)
	if err != nil {
		return parsed, fmt.Errorf("ollama: chat request failed: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return parsed, err
	}
	if res.StatusCode >= 400 {
		return parsed, fmt.Errorf("ollama: chat returned %d: %s", res.StatusCode, string(data))
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return parsed, fmt.Errorf("ollama: invalid chat response: %w", err)
	}
	domain.ReportServedModel(ctx, req.Model)
	return parsed, nil
}

// CompleteChat sends a conversation in Ollama's native /api/chat shape.
//
// Two things differ from the gateway path and both are properties of Ollama
// rather than choices: arguments arrive as an object (no unquoting step), and
// tool calls carry no id at all, so ids are synthesised here (C2-10).
func (o *Provider) CompleteChat(ctx context.Context, msgs []domain.Message, opts *domain.CompleteOptions) (domain.ChatResult, error) {
	req := chatRequest{
		Model:    opts.ModelOr(o.modelName),
		Stream:   false,
		Messages: o.wireMessages(msgs),
		Options:  chatOptions{Temperature: opts.Temp(0.3)},
	}
	if opts != nil && opts.JSONOutput {
		req.Format = "json"
	}
	// MaxTokens is forwarded as num_predict on plain completions and
	// deliberately **dropped** on JSON ones.
	//
	// This asymmetry is not an oversight to tidy up. Every structured call in
	// the platform ends up here on the terminal tier of every routing chain,
	// and a num_predict cap on a structured generation truncates JSON
	// mid-object: the model returns invalid output, the retry loop re-prompts,
	// and the run burns three attempts before failing for a reason nothing
	// reports. The rule lives in the adapter rather than in CompleteJSON so the
	// tool loop's structured terminal inherits it too. Pinned by
	// golden_request_test.go.
	if opts != nil && opts.MaxTokens != nil && !opts.JSONOutput {
		req.Options.NumPredict = opts.MaxTokens
	}
	if opts != nil {
		req.Tools = wireTools(opts.Tools)
	}

	parsed, err := o.send(ctx, req)
	if err != nil {
		return domain.ChatResult{}, err
	}
	return domain.ChatResult{
		Content:      parsed.Message.Content,
		ToolCalls:    o.toolCallsFrom(msgs, parsed.Message.ToolCalls),
		FinishReason: parsed.DoneReason,
	}, nil
}

func (o *Provider) wireMessages(msgs []domain.Message) []chatMessage {
	out := make([]chatMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := chatMessage{Role: m.Role, Content: m.Content, Name: m.Name}
		// ToolCallID is deliberately not forwarded: Ollama has no field for it
		// and does not read one. It exists in the domain type so the loop can
		// match a result to its request, which is a platform-side concern.
		for _, tc := range m.ToolCalls {
			var w wireToolCall
			w.Function.Name = tc.Name
			w.Function.Arguments = tc.Arguments
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

// toolCallsFrom synthesises the ids Ollama does not supply.
//
// The id has to be unique within one exchange and stable for the request that
// carries the result back. Deriving it from the conversation's current length
// gives both: the conversation only grows, so `call_<len>_<index>` never
// repeats within an exchange, and the value is fixed at the moment the call is
// read. These ids are never sent to Ollama, which has no field for them.
func (o *Provider) toolCallsFrom(msgs []domain.Message, wire []wireToolCall) []domain.ToolCall {
	if len(wire) == 0 {
		return nil
	}
	out := make([]domain.ToolCall, 0, len(wire))
	for i, w := range wire {
		out = append(out, domain.ToolCall{
			ID:        fmt.Sprintf("call_%d_%d", len(msgs), i),
			Name:      w.Function.Name,
			Arguments: w.Function.Arguments,
		})
	}
	return out
}

// Complete is a shim onto CompleteChat: temperature 0.3, no `format`, and
// MaxTokens forwarded as num_predict.
func (o *Provider) Complete(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	res, err := o.CompleteChat(ctx, domain.PromptMessages(opts.SystemPrompt(), prompt), opts.ShimOptions(0.3, false))
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

// CompleteJSON is a shim onto CompleteChat at temperature 0.1 with
// `format: "json"`. The num_predict omission that distinguishes it from
// Complete lives in CompleteChat, keyed off JSONOutput.
func (o *Provider) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	res, err := o.CompleteChat(ctx, domain.PromptMessages(opts.SystemPrompt(), prompt), opts.ShimOptions(0.1, true))
	if err != nil {
		return "", err
	}
	return res.Content, nil
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
