package toolloop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

// C4-13: a duplicate name fails at construction, not at call time. Discovered
// at call time it is discovered mid-exchange, in production, on whichever of
// the two the map happened to keep.
func TestDuplicateToolNameFailsAtConstruction(t *testing.T) {
	tool := domain.NewTool("lookup", "first", func(ctx context.Context, a lookupArgs) (string, error) { return "1", nil })
	other := domain.NewTool("lookup", "second", func(ctx context.Context, a lookupArgs) (string, error) { return "2", nil })

	if _, err := NewToolset(tool, other); err == nil {
		t.Fatal("NewToolset accepted two tools with the same name")
	}
	if _, err := NewToolset(domain.ToolDef{Name: "no-handler"}); err == nil {
		t.Fatal("NewToolset accepted a tool with no handler")
	}
}

// C4-11 / FR-009 / SC-006: an unknown name is refused **without dispatch**, and
// the refusal comes back as a tool message so the exchange continues.
func TestUnknownToolNameIsRefusedWithoutDispatch(t *testing.T) {
	ts, seen := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{call("c1", "admin_delete_everything", map[string]any{"target": "all"})}},
		{content: "that tool does not exist, answering anyway"},
		{content: `{"min":1,"max":2}`},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, Bounds{})
	if err != nil {
		t.Fatalf("a refused call must not abort the exchange: %v", err)
	}
	if len(*seen) != 0 {
		t.Errorf("the declared handler ran %d times on a call for a different tool", len(*seen))
	}
	if res.Rounds[0].Calls[0].Outcome != OutcomeRefused {
		t.Errorf("outcome = %q, want refused", res.Rounds[0].Calls[0].Outcome)
	}
	delivered := p.seen[1][len(p.seen[1])-1]
	if delivered.Role != string(domain.RoleTool) {
		t.Errorf("the refusal came back as role %q, want tool", delivered.Role)
	}
	if !strings.Contains(delivered.Content, "no such tool") {
		t.Errorf("the refusal does not say what was wrong: %q", delivered.Content)
	}
	if !strings.Contains(delivered.Content, "lookup_comparable_bands") {
		t.Errorf("the refusal does not tell the model what it may call instead: %q", delivered.Content)
	}
}

// C4-12: arguments that do not match the declared schema are refused without
// dispatch, with the reason in the message.
func TestSchemaInvalidArgumentsAreRefusedWithoutDispatch(t *testing.T) {
	ts, seen := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{{
			ID:        "c1",
			Name:      "lookup_comparable_bands",
			Arguments: json.RawMessage(`"berlin"`), // a bare string, not an object
		}}},
		{content: "retrying without it"},
		{content: `{"min":1,"max":2}`},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, Bounds{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(*seen) != 0 {
		t.Errorf("the handler ran on arguments that failed validation")
	}
	if res.Rounds[0].Calls[0].Outcome != OutcomeRefused {
		t.Errorf("outcome = %q, want refused", res.Rounds[0].Calls[0].Outcome)
	}
	if !strings.Contains(p.seen[1][len(p.seen[1])-1].Content, "JSON object") {
		t.Errorf("the refusal does not state the validation reason: %q", p.seen[1][len(p.seen[1])-1].Content)
	}
}

// The decoding trap, stated as its own test (FR-009a, C2-12).
//
// This is the case that makes the two above worth having. If arguments are
// validated against the *wire* encoding — a JSON string containing JSON — then
// every well-formed call is refused, the exchange fills with refusals, and the
// refusal path looks like it is working perfectly. A test that only checks that
// bad input is refused cannot tell the difference.
func TestWellFormedArgumentsAreNotRefused(t *testing.T) {
	ts, seen := testToolset(t, func(args lookupArgs) (string, error) {
		if args.Bucket != "senior-backend|berlin" {
			t.Errorf("handler received bucket %q, want the decoded value", args.Bucket)
		}
		return `{"p50":120000}`, nil
	})
	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{call("c1", "lookup_comparable_bands", lookupArgs{Bucket: "senior-backend|berlin"})}},
		{content: "done"},
		{content: `{"min":1,"max":2}`},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, Bounds{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("handler ran %d times, want 1 — a well-formed call was refused", len(*seen))
	}
	if res.Rounds[0].Calls[0].Outcome != OutcomeOK {
		t.Errorf("outcome = %q on a well-formed call, want ok", res.Rounds[0].Calls[0].Outcome)
	}
}
