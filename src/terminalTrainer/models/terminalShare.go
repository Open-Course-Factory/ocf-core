package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// TerminalShare représente un partage de terminal entre utilisateurs
type TerminalShare struct {
	entityManagementModels.BaseModel
	TerminalID           uuid.UUID  `gorm:"not null;index" json:"terminal_id"`
	SharedWithUserID     string     `gorm:"type:varchar(255);not null;index" json:"shared_with_user_id"`
	SharedByUserID       string     `gorm:"type:varchar(255);not null;index" json:"shared_by_user_id"`
	AccessLevel          string     `gorm:"type:varchar(50);default:'read'" json:"access_level"` // read, write, admin
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	IsActive             bool       `gorm:"default:true" json:"is_active"`
	IsHiddenByRecipient  bool       `gorm:"default:false" json:"is_hidden_by_recipient"`
	HiddenAt             *time.Time `json:"hidden_at,omitempty"`

	// Relations
	Terminal Terminal `gorm:"foreignKey:TerminalID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

// Implémentation des interfaces pour le système générique
func (ts TerminalShare) GetBaseModel() entityManagementModels.BaseModel {
	return ts.BaseModel
}

func (ts TerminalShare) GetReferenceObject() string {
	return "TerminalShare"
}

// IsExpired vérifie si le partage a expiré
func (ts *TerminalShare) IsExpired() bool {
	if ts.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*ts.ExpiresAt)
}

// HasAccess vérifie si le partage donne accès au niveau requis
func (ts *TerminalShare) HasAccess(requiredLevel string) bool {
	if !ts.IsActive || ts.IsExpired() {
		return false
	}

	// Hiérarchie des niveaux d'accès
	accessLevels := map[string]int{
		"read":  1,
		"write": 2,
		"admin": 3,
	}

	currentLevel, exists := accessLevels[ts.AccessLevel]
	if !exists {
		return false
	}

	requiredLevelInt, exists := accessLevels[requiredLevel]
	if !exists {
		return false
	}

	return currentLevel >= requiredLevelInt
}

// IsHidden vérifie si le terminal est masqué par le destinataire
func (ts *TerminalShare) IsHidden() bool {
	return ts.IsHiddenByRecipient
}

// Hide masque le terminal pour le destinataire
func (ts *TerminalShare) Hide() {
	ts.IsHiddenByRecipient = true
	now := time.Now()
	ts.HiddenAt = &now
}

// Unhide affiche à nouveau le terminal pour le destinataire
func (ts *TerminalShare) Unhide() {
	ts.IsHiddenByRecipient = false
	ts.HiddenAt = nil
}