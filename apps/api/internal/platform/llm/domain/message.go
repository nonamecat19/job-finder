package domain

import "encoding/json"

// Conversation types (037). Before this, the LLM seam could express exactly one
// shape: a prompt string in, a completion string out. Everything the platform
// asks a model to do had to be flattened into that, which is why nothing could
// look anything up and every "conversation" was a re-sent transcript.
//
// The shape below is the one the routing proxy already speaks, deliberately.
// A neutral intermediate representation would need translating on the path that
// carries almost every call, and translation layers drift. The Ollama adapter
// translates to and from its own /api/chat shape at its own edge; the domain
// type does not pretend to be even-handed between the two.

// Role is who produced a turn. Exactly four values exist.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one turn of a conversation.
//
// Ordering is significant and preserved exactly (FR-005): nothing in this stack
// may merge adjacent turns, rewrite a role, or drop an assistant turn whose
// content is empty because it only requested tools. That last one is the
// tempting mistake — an empty-content assistant turn looks like nothing, and
// dropping it strands the tool results that answer it.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ToolCalls is present only on an assistant turn.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID is present only on a tool turn: which request this answers.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Name is present only on a tool turn: the lookup that produced it.
	Name string `json:"name,omitempty"`
}

// ToolCall is one lookup the model asked for.
type ToolCall struct {
	// ID ties a later tool message back to this request. Provider-assigned on
	// the gateway path; adapter-assigned on the Ollama path, whose native
	// format has no id field at all.
	ID string
	// Name is the declared lookup being requested.
	Name string
	// Arguments holds the **decoded** argument object.
	//
	// An OpenAI-compatible response encodes this as a JSON *string containing
	// JSON* — `"arguments": "{\"bucket\":\"berlin\"}"`. Unmarshalled straight
	// into a RawMessage that is a quoted string, and validating a quoted string
	// against an object schema refuses every well-formed call while looking
	// exactly like the refusal path working correctly. The adapter therefore
	// unquotes before storing (FR-009a).
	//
	// When unquoting fails the adapter stores the raw bytes unchanged, so
	// genuinely malformed input still reaches the registry's refusal path.
	Arguments json.RawMessage
}

// ChatResult is what CompleteChat returns.
//
// A result may carry content and tool calls at the same time. That is not
// final: the lookups are honoured and the exchange continues. Prose is never an
// answer (FR-023b).
type ChatResult struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}
