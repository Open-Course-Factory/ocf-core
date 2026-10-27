package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrAiNotConfigured is returned by Generate when the organization has no
// usable AI config — either it never set one, or it set one but left it
// switched off. Handlers map it to a 4xx with a message naming the gap
// rather than a generic failure, since the fix is "configure it", not
// "retry".
var ErrAiNotConfigured = errors.New("AI assistant is not configured for this organization")

// ErrAiKeyRequired is returned when an update moves the endpoint while
// keeping the stored key: the key would otherwise be sent to a host its owner
// never approved.
var ErrAiKeyRequired = errors.New("changing the AI endpoint requires a new API key")

// ErrAiProvider wraps every failure on the provider side: unreachable,
// refused address, timeout, redirect, error status, oversized or unusable
// answer. Its message never carries the key, the request or the provider's
// body, so it is safe to log.
var ErrAiProvider = errors.New("AI provider request failed")

// maxAiResponseBytes caps what is read from the provider; a scenario answer
// is a few dozen kilobytes.
const maxAiResponseBytes = 2 << 20

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
	db     *gorm.DB
	client *http.Client
}

// NewOrganizationAiService only ever reaches public https hosts (see
// NewAiProviderClient).
func NewOrganizationAiService(db *gorm.DB) OrganizationAiService {
	return NewOrganizationAiServiceWithClient(db, NewAiProviderClient(isPublicAddr, nil))
}

func NewOrganizationAiServiceWithClient(db *gorm.DB, client *http.Client) OrganizationAiService {
	return &organizationAiService{db: db, client: client}
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
// key is already stored untouched; APIKey pointing at "" clears it. An empty
// endpoint is allowed (not configured yet); any other must pass
// validateAiEndpoint, and moving it away from where a stored key was meant to
// go needs a new key in the same request.
func (s *organizationAiService) UpdateConfig(orgID uuid.UUID, input dto.UpdateAiConfigInput) (*dto.AiConfigOutput, error) {
	if input.Endpoint != "" {
		if err := validateAiEndpoint(input.Endpoint); err != nil {
			return nil, err
		}
	}
	cfg, err := s.loadConfig(orgID)
	if err != nil {
		return nil, err
	}
	if input.Endpoint != cfg.Endpoint && cfg.APIKey != "" && input.APIKey == nil {
		return nil, ErrAiKeyRequired
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

	answer, err := s.complete(cfg, prompt)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAiProvider, err)
	}
	return answer, nil
}

// complete POSTs one chat-completions request. Its errors name what went
// wrong, never the key, the request or the provider's body.
func (s *organizationAiService) complete(cfg *models.OrganizationAiConfig, prompt string) (string, error) {
	body, err := json.Marshal(aiChatCompletionsRequest{
		Model:    cfg.Model,
		Messages: []aiChatMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", errors.New("could not encode the request")
	}
	req, err := http.NewRequest(http.MethodPost, cfg.Endpoint, bytes.NewReader(body))
	// Rows saved before endpoint validation existed may still be plain http;
	// the key must never travel unencrypted.
	if err != nil || req.URL.Scheme != "https" {
		return "", errors.New("endpoint is not a valid https URL")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("provider answered HTTP %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAiResponseBytes+1))
	if err != nil {
		return "", errors.New("could not read the answer")
	}
	if len(raw) > maxAiResponseBytes {
		return "", fmt.Errorf("answer larger than %d bytes", maxAiResponseBytes)
	}
	var result aiChatCompletionsResponse
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Choices) == 0 {
		return "", errors.New("answer is not a chat completion")
	}
	return result.Choices[0].Message.Content, nil
}
