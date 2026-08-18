package domain

import "encoding/json"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`

	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	ToolCallID string `json:"tool_call_id,omitempty"`

	Name string `json:"name,omitempty"`
}

type ToolCall struct {
	ID string

	Name string

	Arguments json.RawMessage
}

type ChatResult struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}
