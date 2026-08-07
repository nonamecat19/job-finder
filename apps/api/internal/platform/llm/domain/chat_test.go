package domain

import (
	"context"
	"testing"
)

// recordingProvider captures the conversation each call received, so ordering
// and mutation can be asserted on what actually reached the provider rather
// than on what the caller believes it sent.
type recordingProvider struct {
	seen  [][]Message
	reply func(msgs []Message) ChatResult
}

func (r *recordingProvider) ModelName() string { return "recording" }

func (r *recordingProvider) CompleteChat(ctx context.Context, msgs []Message, opts *CompleteOptions) (ChatResult, error) {
	snapshot := make([]Message, len(msgs))
	copy(snapshot, msgs)
	r.seen = append(r.seen, snapshot)
	if r.reply != nil {
		return r.reply(msgs), nil
	}
	return ChatResult{Content: "ok"}, nil
}

func (r *recordingProvider) Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	res, err := r.CompleteChat(ctx, PromptMessages(opts.SystemPrompt(), prompt), opts.ShimOptions(0.3, false))
	return res.Content, err
}

func (r *recordingProvider) CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	res, err := r.CompleteChat(ctx, PromptMessages(opts.SystemPrompt(), prompt), opts.ShimOptions(0.1, true))
	return res.Content, err
}

func (r *recordingProvider) Embed(context.Context, string) ([]float32, error) { return nil, nil }

// 037 FR-001/FR-005, SC-002: a three-turn exchange arrives as four messages in
// order with roles intact. Nothing merges adjacent turns, drops one, or
// rewrites a role — the failure mode this guards against is a stack that
// "helpfully" collapses history and produces an answer that ignores turn one.
func TestThreeTurnConversationArrivesIntactAndInOrder(t *testing.T) {
	p := &recordingProvider{reply: func(msgs []Message) ChatResult {
		// The answer depends on turn one, so a dropped first turn cannot pass.
		for _, m := range msgs {
			if m.Role == string(RoleUser) && m.Content == "my name is Ada" {
				return ChatResult{Content: "Ada"}
			}
		}
		return ChatResult{Content: "I don't know"}
	}}

	convo := []Message{
		{Role: string(RoleUser), Content: "my name is Ada"},
		{Role: string(RoleAssistant), Content: "hello Ada"},
		{Role: string(RoleUser), Content: "what is my name?"},
	}
	res, err := p.CompleteChat(context.Background(), convo, nil)
	if err != nil {
		t.Fatalf("CompleteChat: %v", err)
	}
	if res.Content != "Ada" {
		t.Errorf("answer = %q, want %q — the final turn depends on turn one, so this means history was lost", res.Content, "Ada")
	}

	got := p.seen[0]
	if len(got) != 3 {
		t.Fatalf("provider saw %d messages, want 3", len(got))
	}
	wantRoles := []string{"user", "assistant", "user"}
	for i, want := range wantRoles {
		if got[i].Role != want {
			t.Errorf("message %d role = %q, want %q", i, got[i].Role, want)
		}
	}
	if got[0].Content != "my name is Ada" || got[2].Content != "what is my name?" {
		t.Errorf("message contents reordered or rewritten: %+v", got)
	}
}

// C1-6: the caller's slice is the caller's. A provider that appends to it in
// place corrupts any retry that reuses the same conversation — and the loop
// reuses it on every round.
func TestCompleteChatDoesNotMutateTheCallersMessages(t *testing.T) {
	convo := []Message{
		{Role: string(RoleUser), Content: "one"},
		{Role: string(RoleUser), Content: "two"},
	}
	// Spare capacity is what makes an in-place append silently succeed; without
	// it, an append would reallocate and the bug would hide.
	convo = append(make([]Message, 0, 8), convo...)
	before := make([]Message, len(convo))
	copy(before, convo)

	p := &recordingProvider{}
	if _, err := p.CompleteChat(context.Background(), convo, nil); err != nil {
		t.Fatalf("CompleteChat: %v", err)
	}

	if len(convo) != len(before) {
		t.Fatalf("caller's slice length changed from %d to %d", len(before), len(convo))
	}
	for i := range before {
		if convo[i].Role != before[i].Role || convo[i].Content != before[i].Content ||
			len(convo[i].ToolCalls) != len(before[i].ToolCalls) {
			t.Errorf("caller's message %d was mutated: %+v became %+v", i, before[i], convo[i])
		}
	}
}

// C1-3: no system prompt means no system message — not an empty one. An empty
// system turn is a real instruction to some models and a wasted token to all of
// them.
func TestPromptMessagesOmitsAnEmptySystemTurn(t *testing.T) {
	msgs := PromptMessages("", "hello")
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if msgs[0].Role != string(RoleUser) {
		t.Errorf("role = %q, want user", msgs[0].Role)
	}

	withSystem := PromptMessages("be terse", "hello")
	if len(withSystem) != 2 || withSystem[0].Role != string(RoleSystem) {
		t.Fatalf("expected [system, user], got %+v", withSystem)
	}
}

// C1-8: an assistant turn whose content is empty because it only requested
// tools must survive. It looks like nothing and dropping it strands the tool
// results that answer it.
func TestEmptyAssistantTurnWithToolCallsIsPreserved(t *testing.T) {
	convo := []Message{
		{Role: string(RoleUser), Content: "look it up"},
		{Role: string(RoleAssistant), ToolCalls: []ToolCall{{ID: "call_1", Name: "lookup"}}},
		{Role: string(RoleTool), ToolCallID: "call_1", Name: "lookup", Content: "42"},
	}
	p := &recordingProvider{}
	if _, err := p.CompleteChat(context.Background(), convo, nil); err != nil {
		t.Fatalf("CompleteChat: %v", err)
	}
	got := p.seen[0]
	if len(got) != 3 {
		t.Fatalf("provider saw %d messages, want 3 — the empty assistant turn was dropped", len(got))
	}
	if len(got[1].ToolCalls) != 1 || got[1].ToolCalls[0].ID != "call_1" {
		t.Errorf("assistant turn lost its tool calls: %+v", got[1])
	}
	if got[2].ToolCallID != "call_1" {
		t.Errorf("tool turn lost the id of the request it answers: %+v", got[2])
	}
}
