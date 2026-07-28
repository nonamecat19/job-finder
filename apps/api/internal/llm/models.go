package llm

// CerebrasModel is a curated free-tier chat model the operator may select in
// dashboard Settings (001-cerebras-model-toggle). The list is code-defined
// rather than fetched live, so Settings never depends on reaching Cerebras
// just to render its options (research.md R2).
type CerebrasModel struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	IsDefault bool   `json:"isDefault"`
}

// DefaultCerebrasModel is used whenever a task's persisted model is empty.
const DefaultCerebrasModel = "gpt-oss-120b"

// CerebrasModels is the curated free-tier model list surfaced to the
// dashboard. Exactly one entry MUST have IsDefault set.
var CerebrasModels = []CerebrasModel{
	{ID: "gpt-oss-120b", Label: "GPT-OSS 120B", IsDefault: true},
	{ID: "llama-3.3-70b", Label: "Llama 3.3 70B", IsDefault: false},
	{ID: "llama3.1-8b", Label: "Llama 3.1 8B", IsDefault: false},
	{ID: "qwen-3-32b", Label: "Qwen 3 32B", IsDefault: false},
}

// IsSupportedCerebrasModel reports whether id is a known curated model, or
// empty (meaning "use the default").
func IsSupportedCerebrasModel(id string) bool {
	if id == "" {
		return true
	}
	for _, m := range CerebrasModels {
		if m.ID == id {
			return true
		}
	}
	return false
}
