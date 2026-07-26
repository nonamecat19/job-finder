package dto

type LlmTaskSettingDto struct {
	TaskKey  string `json:"taskKey"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type LlmSettingsResponseDto struct {
	CredentialConfigured           bool                `json:"credentialConfigured"`
	OpenRouterCredentialConfigured bool                `json:"openRouterCredentialConfigured"`
	Tasks                          []LlmTaskSettingDto `json:"tasks"`
}

type UpdateLlmSettingsRequestDto struct {
	Tasks []LlmTaskSettingDto `json:"tasks"`
}

type AiFeatureSettingDto struct {
	Feature   string `json:"feature"`
	Enabled   bool   `json:"enabled"`
	Threshold int    `json:"threshold"`
}

type CerebrasModelDto struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	IsDefault bool   `json:"isDefault"`
}

type OpenRouterModelDto struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	IsDefault bool   `json:"isDefault"`
}

type LlmModelsResponseDto struct {
	Cerebras   []CerebrasModelDto   `json:"cerebras"`
	OpenRouter []OpenRouterModelDto `json:"openrouter"`
}
