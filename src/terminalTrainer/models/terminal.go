package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// Terminal représente une session de terminal actif
type Terminal struct {
	entityManagementModels.BaseModel
	SessionID         string     `gorm:"type:varchar(255);uniqueIndex" json:"session_id"`
	UserID            string     `gorm:"type:varchar(255);not null;index" json:"user_id"`
	Name              string     `gorm:"type:varchar(255)" json:"name"` // User-friendly name for the terminal session
	Status            string     `gorm:"type:varchar(50);default:'active'" json:"status"` // active, stopped, expired
	ExpiresAt         time.Time  `gorm:"not null" json:"expires_at"`
	InstanceType      string     `gorm:"type:varchar(100)" json:"instance_type"` // préfixe du type d'instance utilisé
	MachineSize       string     `gorm:"type:varchar(10)" json:"machine_size"`   // XS, S, M, L, XL (taille réelle utilisée)
	UserTerminalKeyID uuid.UUID  `gorm:"not null;index" json:"user_terminal_key_id"`
	IsHiddenByOwner   bool       `gorm:"default:false" json:"is_hidden_by_owner"`
	HiddenByOwnerAt   *time.Time `json:"hidden_by_owner_at,omitempty"`
	UserTerminalKey   UserTerminalKey
}

// UserTerminalKey stocke la clé API Terminal Trainer pour chaque utilisateur
type UserTerminalKey struct {
	entityManagementModels.BaseModel
	UserID      string `gorm:"type:varchar(255);not null" json:"user_id"`
	APIKey      string `gorm:"type:varchar(255);not null" json:"api_key"`
	KeyName     string `gorm:"type:varchar(255);not null" json:"key_name"`
	IsActive    bool   `gorm:"default:true" json:"is_active"`
	MaxSessions int    `gorm:"default:5" json:"max_sessions"`

	// Relations
	Terminals []Terminal
}

// Implémentation des interfaces pour le système générique
func (t Terminal) GetBaseModel() entityManagementModels.BaseModel {
	return t.BaseModel
}

func (t Terminal) GetReferenceObject() string {
	return "Terminal"
}

func (u UserTerminalKey) GetBaseModel() entityManagementModels.BaseModel {
	return u.BaseModel
}

func (u UserTerminalKey) GetReferenceObject() string {
	return "UserTerminalKey"
}

// IsHiddenByOwner vérifie si le terminal est masqué par le propriétaire
func (t *Terminal) IsHidden() bool {
	return t.IsHiddenByOwner
}

// Hide masque le terminal pour le propriétaire
func (t *Terminal) Hide() {
	t.IsHiddenByOwner = true
	now := time.Now()
	t.HiddenByOwnerAt = &now
}

// Unhide affiche à nouveau le terminal pour le propriétaire
func (t *Terminal) Unhide() {
	t.IsHiddenByOwner = false
	t.HiddenByOwnerAt = nil
}

// CanBeHidden vérifie si le terminal peut être masqué (seulement les terminaux inactifs)
func (t *Terminal) CanBeHidden() bool {
	return t.Status != "active"
}
