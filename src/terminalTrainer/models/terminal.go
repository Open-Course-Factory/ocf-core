package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// Terminal représente une session de terminal actif
type Terminal struct {
	entityManagementModels.BaseModel
	SessionID            string     `gorm:"type:varchar(255);uniqueIndex" json:"session_id"`
	UserID               string     `gorm:"type:varchar(255);not null;index" json:"user_id"`
	Name                 string     `gorm:"type:varchar(255)" json:"name"` // User-friendly name for the terminal session
	// Status is the legacy lifecycle field (active, stopped, expired) kept for
	// backward compatibility with existing code paths. It is being phased out
	// in favor of State below — new code should consume State.
	Status string `gorm:"type:varchar(50);default:'active'" json:"status"`
	// State is the new session lifecycle field driven by the proxy + persistence
	// rules: running, paused, hibernating, resuming, terminated, etc.
	State                string     `gorm:"type:varchar(50);default:'running'" json:"state"`
	// PersistenceMode controls whether the session is ephemeral (default) or
	// persistent across stop/start cycles. Values: "ephemeral", "persistent".
	PersistenceMode      string     `gorm:"type:varchar(20);default:'ephemeral'" json:"persistence_mode"`
	// LastStartedAt records the most recent transition into a running state.
	// Server-managed; not user-editable.
	LastStartedAt        time.Time  `json:"last_started_at"`
	// IdleUntil is the absolute deadline after which an idle session may be
	// reaped or hibernated. Nil means no idle policy currently applies.
	// Server-managed; not user-editable.
	IdleUntil            *time.Time `json:"idle_until,omitempty"`
	ExpiresAt            time.Time  `gorm:"not null" json:"expires_at"`
	InstanceType         string     `gorm:"type:varchar(100)" json:"instance_type"` // préfixe du type d'instance utilisé
	MachineSize          string     `gorm:"type:varchar(10)" json:"machine_size"`   // XS, S, M, L, XL (taille réelle utilisée)
	Backend              string     `gorm:"type:varchar(255);default:''" json:"backend"`
	OrganizationID       *uuid.UUID `gorm:"index" json:"organization_id,omitempty"`
	SubscriptionPlanID   *uuid.UUID `gorm:"type:uuid;index" json:"subscription_plan_id,omitempty"`
	UserTerminalKeyID    uuid.UUID  `gorm:"not null;index" json:"user_terminal_key_id"`
	ComposedDistribution string     `gorm:"type:varchar(100)" json:"composed_distribution,omitempty"`
	ComposedSize         string     `gorm:"type:varchar(10)" json:"composed_size,omitempty"`
	ComposedFeatures     string     `gorm:"type:text" json:"composed_features,omitempty"`
	UserTerminalKey      UserTerminalKey
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
