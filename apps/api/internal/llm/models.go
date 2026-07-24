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

// OpenRouterModel is a curated OpenRouter chat model the operator may select
// in dashboard Settings. Same rationale as CerebrasModels: code-defined so
// Settings never depends on reaching OpenRouter just to render its options.
type OpenRouterModel struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	IsDefault bool   `json:"isDefault"`
}

// DefaultOpenRouterModel is used whenever a task's persisted model is empty.
const DefaultOpenRouterModel = "deepseek/deepseek-chat-v3-0324:free"

// OpenRouterModels is the curated model list surfaced to the dashboard. The
// ":free" suffix pins the zero-cost variant, matching the local-first,
// no-spend posture of the Cerebras free tier. Exactly one entry MUST have
// IsDefault set.
var OpenRouterModels = []OpenRouterModel{
	{ID: "deepseek/deepseek-chat-v3-0324:free", Label: "DeepSeek V3 (free)", IsDefault: true},
	{ID: "deepseek/deepseek-r1:free", Label: "DeepSeek R1 (free)", IsDefault: false},
	{ID: "meta-llama/llama-3.3-70b-instruct:free", Label: "Llama 3.3 70B (free)", IsDefault: false},
	{ID: "qwen/qwen3-235b-a22b:free", Label: "Qwen 3 235B (free)", IsDefault: false},
	{ID: "mistralai/mistral-small-3.2-24b-instruct:free", Label: "Mistral Small 3.2 24B (free)", IsDefault: false},
	{ID: "google/gemini-2.0-flash-exp:free", Label: "Gemini 2.0 Flash (free)", IsDefault: false},
}

// IsSupportedOpenRouterModel reports whether id is a known curated model, or
// empty (meaning "use the default").
func IsSupportedOpenRouterModel(id string) bool {
	if id == "" {
		return true
	}
	for _, m := range OpenRouterModels {
		if m.ID == id {
			return true
		}
	}
	return false
}
