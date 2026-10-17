package dto

// AiConfigOutput is the body of GET /organizations/:id/ai-config. The API
// key itself is never included — HasAPIKey is the only signal the frontend
// gets that one is set.
type AiConfigOutput struct {
	Enabled   bool   `json:"enabled"`
	Endpoint  string `json:"endpoint"`
	Model     string `json:"model"`
	HasAPIKey bool   `json:"has_api_key"`
}

// UpdateAiConfigInput is the body of PUT /organizations/:id/ai-config.
//
// APIKey is a pointer and deliberately optional: the frontend never holds
// the real key to send back, so omitting it (nil) must mean "leave the
// stored key untouched" rather than "clear it". An explicit empty string
// IS a clear — the admin unchecked a "keep existing key" affordance, or
// typed and then emptied the field on purpose.
type UpdateAiConfigInput struct {
	Enabled  bool    `json:"enabled"`
	Endpoint string  `json:"endpoint"`
	Model    string  `json:"model"`
	APIKey   *string `json:"api_key"`
}

// GenerateInput is the body of POST /organizations/:id/ai-config/generate.
type GenerateInput struct {
	Prompt string `json:"prompt" binding:"required"`
}

// GenerateOutput is the body returned on success: the assistant's raw
// completion text, parsed and validated by the frontend exactly the way a
// manually pasted answer already is (extractJsonObject in
// src/utils/scenarioAiPrompt.ts) — this endpoint does not interpret it.
type GenerateOutput struct {
	Answer string `json:"answer"`
}
