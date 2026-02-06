package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// Terminal access level constants
const (
	AccessLevelRead  = "read"
	AccessLevelWrite = "write"
	AccessLevelOwner = "owner"
)

// AccessLevelHierarchy maps access levels to their numeric rank for comparison
var AccessLevelHierarchy = map[string]int{
	AccessLevelRead:  1,
	AccessLevelWrite: 2,
	AccessLevelOwner: 3,
}

// IsValidAccessLevel checks if the given level is a known access level
func IsValidAccessLevel(level string) bool {
	_, ok := AccessLevelHierarchy[level]
	return ok
}

// TerminalShare représente un partage de terminal entre utilisateurs ou groupes
type TerminalShare struct {
	entityManagementModels.BaseModel
	TerminalID uuid.UUID `gorm:"not null;index" json:"terminal_id"`

	// Share can be to a user OR a group (one must be set, the other must be NULL)
	SharedWithUserID  *string    `gorm:"type:varchar(255);index" json:"shared_with_user_id,omitempty"`
	SharedWithGroupID *uuid.UUID `gorm:"type:uuid;index" json:"shared_with_group_id,omitempty"`

	SharedByUserID      string     `gorm:"type:varchar(255);not null;index" json:"shared_by_user_id"`
	AccessLevel         string     `gorm:"type:varchar(50);default:'read'" json:"access_level"` // read, write, owner
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	IsActive            bool       `gorm:"default:true" json:"is_active"`
	IsHiddenByRecipient bool       `gorm:"default:false" json:"is_hidden_by_recipient"`
	HiddenAt            *time.Time `json:"hidden_at,omitempty"`

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

	currentLevel, exists := AccessLevelHierarchy[ts.AccessLevel]
	if !exists {
		return false
	}

	requiredLevelInt, exists := AccessLevelHierarchy[requiredLevel]
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

// IsUserShare checks if this share is for a specific user
func (ts *TerminalShare) IsUserShare() bool {
	return ts.SharedWithUserID != nil && *ts.SharedWithUserID != ""
}

// IsGroupShare checks if this share is for a group
func (ts *TerminalShare) IsGroupShare() bool {
	return ts.SharedWithGroupID != nil
}

// GetRecipientIdentifier returns the recipient ID (user ID or group ID as string)
func (ts *TerminalShare) GetRecipientIdentifier() string {
	if ts.IsUserShare() {
		return *ts.SharedWithUserID
	}
	if ts.IsGroupShare() {
		return ts.SharedWithGroupID.String()
	}
	return ""
}

// GetShareType returns "user" or "group"
func (ts *TerminalShare) GetShareType() string {
	if ts.IsUserShare() {
		return "user"
	}
	if ts.IsGroupShare() {
		return "group"
	}
	return "unknown"
}
