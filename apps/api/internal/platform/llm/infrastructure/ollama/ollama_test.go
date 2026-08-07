package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

func TestIsHosted(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		apiKey  string
		want    bool
	}{
		{"local no key", "http://localhost:11434", "", false},
		{"loopback IP no key", "http://127.0.0.1:11434", "", false},
		{"loopback IP with key", "http://127.0.0.1:11434", "some-key", true},
		{"private network no key", "http://192.168.1.50:11434", "", false},
		{"cloud URL no key", "https://ollama.com", "", true},
		{"cloud URL with key", "https://ollama.com", "cloud-key", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.baseURL, tt.apiKey, "", "", "")
			if got := p.IsHosted(); got != tt.want {
				t.Errorf("IsHosted() = %v, want %v", got, tt.want)
			}
		})
	}
}

// C2-8 / C6-3 / FR-019: when the local model cannot call tools, the adapter
// must surface that as a distinguishable condition rather than mask it.
//
// "Distinguishable" here means one specific thing: the adapter must not invent
// a tool call to make the exchange look normal. It returns exactly what the
// model produced — prose, no calls — and the loop's required first round is
// what turns that into an explicit not_tool_capable failure. The local tier is
// the terminal tier of every chain, so a silent degradation here would mean a
// confident answer built without any of the lookups it was supposed to make.
func TestCompleteChatDoesNotFabricateToolCallsOnANonToolCapableModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A model that ignored the tools array: prose, no tool_calls field.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"The salary is probably around 120k."},"done_reason":"stop"}`))
	}))
	t.Cleanup(srv.Close)

	p := New(srv.URL, "", "test-model", "", "")
	res, err := p.CompleteChat(context.Background(), []domain.Message{{Role: "user", Content: "estimate"}},
		&domain.CompleteOptions{
			Tools:      []domain.ToolDef{{Name: "lookup", Description: "d", ArgsSchema: `{"type":"object"}`}},
			ToolChoice: "required",
		})
	if err != nil {
		t.Fatalf("CompleteChat: %v", err)
	}
	if len(res.ToolCalls) != 0 {
		t.Errorf("the adapter fabricated %d tool call(s) for a model that made none; that would hide the one signal the loop has", len(res.ToolCalls))
	}
	if res.Content == "" {
		t.Error("the prose was discarded; the caller cannot tell an empty response from a non-tool-capable one")
	}
	if res.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want the model's done_reason", res.FinishReason)
	}
}

// C2-10 / FR-015a: Ollama supplies no tool-call ids, so the adapter assigns
// them. They must be unique within an exchange and must never be sent back.
func TestOllamaSynthesisesToolCallIDs(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"","tool_calls":[
			{"function":{"name":"lookup","arguments":{"a":1}}},
			{"function":{"name":"lookup","arguments":{"a":2}}}]}}`))
	}))
	t.Cleanup(srv.Close)

	p := New(srv.URL, "", "test-model", "", "")
	res, err := p.CompleteChat(context.Background(), []domain.Message{{Role: "user", Content: "look it up"}}, &domain.CompleteOptions{})
	if err != nil {
		t.Fatalf("CompleteChat: %v", err)
	}
	if len(res.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(res.ToolCalls))
	}
	if res.ToolCalls[0].ID == "" || res.ToolCalls[1].ID == "" {
		t.Fatal("a tool call has no id; the tool message answering it would have nothing to quote")
	}
	if res.ToolCalls[0].ID == res.ToolCalls[1].ID {
		t.Errorf("both calls got id %q; ids must be unique within an exchange", res.ToolCalls[0].ID)
	}
	// Arguments arrive as an object on this path — no unquoting step, unlike
	// the OpenAI-compatible encoding.
	if string(res.ToolCalls[0].Arguments) != `{"a":1}` {
		t.Errorf("arguments = %s, want the object as sent", res.ToolCalls[0].Arguments)
	}

	// Sending a synthesised id back would be sending Ollama a field it does not
	// read, on a request it has to parse.
	msgs := []domain.Message{
		{Role: "assistant", ToolCalls: res.ToolCalls},
		{Role: "tool", ToolCallID: res.ToolCalls[0].ID, Name: "lookup", Content: "42"},
	}
	if _, err := p.CompleteChat(context.Background(), msgs, &domain.CompleteOptions{}); err != nil {
		t.Fatalf("CompleteChat: %v", err)
	}
	raw, _ := json.Marshal(sent)
	if strings.Contains(string(raw), res.ToolCalls[0].ID) {
		t.Errorf("the synthesised id %q was sent to Ollama, which has no field for it: %s", res.ToolCalls[0].ID, raw)
	}
}
