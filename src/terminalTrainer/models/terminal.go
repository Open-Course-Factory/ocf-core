package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// Terminal représente une session de terminal actif
type Terminal struct {
	entityManagementModels.BaseModel
	SessionID         string    `gorm:"type:varchar(255);uniqueIndex" json:"session_id"`
	UserID            string    `gorm:"type:varchar(255);not null;index" json:"user_id"`
	Status            string    `gorm:"type:varchar(50);default:'active'" json:"status"` // active, stopped, expired
	ExpiresAt         time.Time `gorm:"not null" json:"expires_at"`
	UserTerminalKeyID uuid.UUID `gorm:"not null;index" json:"user_terminal_key_id"`
}

// UserTerminalKey stocke la clé API Terminal Trainer pour chaque utilisateur
type UserTerminalKey struct {
	entityManagementModels.BaseModel
	UserID      string `gorm:"type:varchar(255);uniqueIndex;not null" json:"user_id"`
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
