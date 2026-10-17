package services

import (
	"errors"
	"fmt"
	"time"

	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrAiNotConfigured is returned by Generate when the organization has no
// usable AI config — either it never set one, or it set one but left it
// switched off. Handlers map it to a 4xx with a message naming the gap
// rather than a generic failure, since the fix is "configure it", not
// "retry".
var ErrAiNotConfigured = errors.New("AI assistant is not configured for this organization")

// aiChatCompletionsRequest / aiChatCompletionsResponse are the OpenAI
// "chat completions" shapes — the de-facto standard every provider this
// feature targets (OpenAI itself, Mistral, self-hosted Ollama/vLLM, and
// Anthropic/others behind an OpenAI-compatible proxy) already speaks, so one
// request shape covers all of them without per-provider branching.
type aiChatCompletionsRequest struct {
	Model    string          `json:"model"`
	Messages []aiChatMessage `json:"messages"`
}

type aiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiChatCompletionsResponse struct {
	Choices []struct {
		Message aiChatMessage `json:"message"`
	} `json:"choices"`
}

// OrganizationAiService is the per-organization "bring your own AI key"
// config that lets the scenario editor's AI-assisted authoring flow call an
// OpenAI-compatible endpoint directly instead of the teacher copying a
// prompt into their own chat assistant and pasting the answer back.
type OrganizationAiService interface {
	GetConfig(orgID uuid.UUID) (*dto.AiConfigOutput, error)
	UpdateConfig(orgID uuid.UUID, input dto.UpdateAiConfigInput) (*dto.AiConfigOutput, error)
	Generate(orgID uuid.UUID, prompt string) (string, error)
}

type organizationAiService struct {
	db *gorm.DB
}

func NewOrganizationAiService(db *gorm.DB) OrganizationAiService {
	return &organizationAiService{db: db}
}

// loadConfig returns the organization's config row, or a zero-value
// (unsaved) one if it never created one — every org starts unconfigured and
// disabled, not missing.
func (s *organizationAiService) loadConfig(orgID uuid.UUID) (*models.OrganizationAiConfig, error) {
	var cfg models.OrganizationAiConfig
	err := s.db.Where("organization_id = ?", orgID).First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &models.OrganizationAiConfig{OrganizationID: orgID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *organizationAiService) GetConfig(orgID uuid.UUID) (*dto.AiConfigOutput, error) {
	cfg, err := s.loadConfig(orgID)
	if err != nil {
		return nil, err
	}
	return &dto.AiConfigOutput{
		Enabled:   cfg.Enabled,
		Endpoint:  cfg.Endpoint,
		Model:     cfg.Model,
		HasAPIKey: cfg.HasAPIKey,
	}, nil
}

// UpdateConfig upserts the organization's row. APIKey nil leaves whatever
// key is already stored untouched; APIKey pointing at "" clears it.
func (s *organizationAiService) UpdateConfig(orgID uuid.UUID, input dto.UpdateAiConfigInput) (*dto.AiConfigOutput, error) {
	cfg, err := s.loadConfig(orgID)
	if err != nil {
		return nil, err
	}

	cfg.Enabled = input.Enabled
	cfg.Endpoint = input.Endpoint
	cfg.Model = input.Model
	if input.APIKey != nil {
		cfg.APIKey = *input.APIKey
	}

	if cfg.ID == uuid.Nil {
		if err := s.db.Create(cfg).Error; err != nil {
			return nil, err
		}
	} else {
		// Save, not Update: BeforeSave must run to (re-)encrypt APIKey, and
		// Save writes every field — including an APIKey left untouched
		// above, which AfterFind already decrypted back to plaintext, so
		// BeforeSave's "already encrypted?" prefix check does not
		// re-encrypt an already-encrypted value by mistake.
		if err := s.db.Save(cfg).Error; err != nil {
			return nil, err
		}
	}

	return &dto.AiConfigOutput{
		Enabled:   cfg.Enabled,
		Endpoint:  cfg.Endpoint,
		Model:     cfg.Model,
		HasAPIKey: cfg.APIKey != "",
	}, nil
}

// Generate calls the organization's configured endpoint with the given
// prompt and returns the assistant's raw completion text. The caller
// (ScenarioAiModal, via the generate route) parses and validates it exactly
// the way a manually pasted answer already is — this function does not
// interpret the response as a scenario.
func (s *organizationAiService) Generate(orgID uuid.UUID, prompt string) (string, error) {
	cfg, err := s.loadConfig(orgID)
	if err != nil {
		return "", err
	}
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.APIKey == "" {
		return "", ErrAiNotConfigured
	}

	payload := aiChatCompletionsRequest{
		Model:    cfg.Model,
		Messages: []aiChatMessage{{Role: "user", Content: prompt}},
	}

	opts := utils.DefaultHTTPClientOptions()
	opts.Timeout = 90 * time.Second
	opts.Headers["Authorization"] = "Bearer " + cfg.APIKey

	var result aiChatCompletionsResponse
	if err := utils.MakeExternalAPIJSONRequest("AI assistant", "POST", cfg.Endpoint, payload, &result, opts); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("AI assistant returned no choices")
	}
	return result.Choices[0].Message.Content, nil
}
