package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TerminalStatusesOccupyingSlot are the status values that count toward
// the concurrent_terminals limit. Stopped sessions still occupy a slot
// because they preserve disk and can be resumed — only deleted/expired
// rows free a slot.
var TerminalStatusesOccupyingSlot = []string{"active", "stopped"}

// OccupiesSlotScope is a GORM scope that filters terminals counted
// against the concurrent_terminals quota. Single source of truth for
// the "occupies a slot" rule — every counter / quota query must use
// this scope so the rule stays expressed in exactly one place.
//
// Columns are qualified with the `terminals.` prefix so the scope is
// safe to combine with JOINs against other tables that share a
// `deleted_at` column (e.g. organization_members via gorm.Model).
//
// Usage:
//
//	db.Table("terminals").Scopes(models.OccupiesSlotScope).
//	    Where("terminals.user_id = ?", userID).Count(&count)
func OccupiesSlotScope(tx *gorm.DB) *gorm.DB {
	return tx.Where("terminals.status IN ? AND terminals.deleted_at IS NULL", TerminalStatusesOccupyingSlot)
}

// CountUserOccupiedSlots returns the number of terminals owned by the
// user that count against their concurrent_terminals quota. If orgID
// is nil, counts all terminals across all orgs (and personal). If
// orgID is non-nil, counts only terminals tied to that org context
// via the direct terminals.organization_id column.
//
// Single source of truth for "how many slots is this user using?".
// Always uses OccupiesSlotScope — callers must never write their own
// count query.
func CountUserOccupiedSlots(db *gorm.DB, userID string, orgID *uuid.UUID) (int64, error) {
	q := db.Model(&Terminal{}).Scopes(OccupiesSlotScope).Where("terminals.user_id = ?", userID)
	if orgID != nil {
		q = q.Where("terminals.organization_id = ?", orgID)
	}
	var count int64
	return count, q.Count(&count).Error
}

// CountOrgOccupiedSlots returns the number of terminals owned by any
// member of the org (joined via organization_members) that count
// against the org's concurrent_terminals quota.
//
// Single source of truth for "how many slots is this org using?".
// Always uses OccupiesSlotScope. Mirrors the join shape used by
// payment/services/organizationSubscriptionService.go (the production
// quota gate), so an org's quota counter and the helper agree.
func CountOrgOccupiedSlots(db *gorm.DB, orgID uuid.UUID) (int64, error) {
	var count int64
	err := db.Model(&Terminal{}).Scopes(OccupiesSlotScope).
		Joins("JOIN organization_members ON organization_members.user_id = terminals.user_id").
		Where("organization_members.organization_id = ? AND organization_members.deleted_at IS NULL", orgID).
		Count(&count).Error
	return count, err
}

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
