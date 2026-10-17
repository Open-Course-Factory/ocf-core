package models

import (
	"strings"

	"gorm.io/gorm"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/utils/crypto"

	"github.com/google/uuid"
)

// OrganizationAiConfig is one organization's own bring-your-own-key setup
// for AI-assisted scenario authoring (src/organizations/services/organizationAiService.go).
//
// It exists because the scenario editor's "Create/Improve with AI" flow
// (ocf-front's ScenarioAiModal) otherwise only ever writes a prompt the
// teacher must copy into their own chat assistant and paste the answer back
// — nothing on this side ever sees the exchange. An organization that wants
// to skip that round trip configures its own OpenAI-compatible endpoint
// here; the key never leaves ocf-core once set.
type OrganizationAiConfig struct {
	entityManagementModels.BaseModel

	// One config per organization — looked up by this, never listed.
	OrganizationID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"organization_id"`

	// Enabled is the organization's own on/off switch, separate from
	// whether the config is actually usable (Endpoint/Model/APIKey set) —
	// an org can configure everything and still leave it off.
	Enabled bool `gorm:"default:false" json:"enabled"`

	// Endpoint is a full OpenAI-compatible chat-completions URL (e.g.
	// "https://api.openai.com/v1/chat/completions", or a self-hosted
	// Ollama/vLLM instance exposing the same shape) — deliberately not
	// split into host/path, since the standard itself has no fixed path
	// across providers.
	Endpoint string `gorm:"type:varchar(500)" json:"endpoint"`

	// Model is sent as-is in the request body's "model" field.
	Model string `gorm:"type:varchar(200)" json:"model"`

	// APIKey is encrypted at rest the same way SshKey.PrivateKey already is
	// (src/auth/models/sshKeys.go) and never serialized — json:"-" plus the
	// generic entity framework's default JSON encoding means a GET can
	// never echo it back under any field name. HasAPIKey is the only signal
	// the frontend gets that a key is set.
	APIKey string `gorm:"type:text" json:"-"`

	// HasAPIKey is computed on read (AfterFind), never a column — the only
	// way the frontend learns "a key is configured" without the value.
	HasAPIKey bool `gorm:"-" json:"has_api_key"`
}

// BeforeSave encrypts APIKey before writing to the database. Idempotent:
// an already-encrypted value (prefixed "enc::v1:") is left unchanged so an
// update that doesn't touch the key never double-encrypts it.
func (c *OrganizationAiConfig) BeforeSave(tx *gorm.DB) error {
	if c.APIKey != "" && !strings.HasPrefix(c.APIKey, "enc::v1:") {
		encrypted, err := crypto.Encrypt(c.APIKey)
		if err != nil {
			return err
		}
		c.APIKey = encrypted
	}
	return nil
}

// AfterFind decrypts APIKey after loading and stamps HasAPIKey, so every
// caller — including the generate call that needs the plaintext key — works
// against plaintext without re-deriving the encryption state itself.
func (c *OrganizationAiConfig) AfterFind(tx *gorm.DB) error {
	c.HasAPIKey = c.APIKey != ""
	if strings.HasPrefix(c.APIKey, "enc::v1:") {
		decrypted, err := crypto.Decrypt(c.APIKey)
		if err != nil {
			return err
		}
		c.APIKey = decrypted
	}
	return nil
}

func (c OrganizationAiConfig) GetBaseModel() entityManagementModels.BaseModel { return c.BaseModel }

func (c OrganizationAiConfig) GetReferenceObject() string { return "OrganizationAiConfig" }
