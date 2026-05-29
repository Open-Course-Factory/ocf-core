package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TerminalState is the typed lifecycle state of a terminal session. The
// underlying string values are the wire format (GORM column + JSON field) and
// must NOT change — they are pinned by TestTerminalStateConstants and consumed
// by the ocf-front TerminalSession union type (src/types/terminal.ts).
type TerminalState string

const (
	StateRunning     TerminalState = "running"
	StateStopped     TerminalState = "stopped"
	StateDeleted     TerminalState = "deleted"
	StateStarting    TerminalState = "starting"
	StateResuming    TerminalState = "resuming"
	StateHibernating TerminalState = "hibernating"
)

// TerminalStatesOccupyingSlot are the state values that count toward
// the "occupies a slot" predicate used by the org dashboard and other
// listing surfaces. Stopped sessions still occupy a slot because they
// preserve disk and can be resumed — only deleted (or other
// terminal-state) rows free a slot.
var TerminalStatesOccupyingSlot = []string{"running", "stopped"}

// OccupiesSlotScope is a GORM scope that filters terminals which still
// "occupy capacity" — both in the slot sense (disk/identity reserved)
// AND in the budget sense (CPU/RAM reserved). Single source of truth
// for every counting/listing surface that needs to know "is this row
// still consuming the user's plan?".
//
// Used by:
//   - QuotaService.sumActiveResources* — the CPU/RAM budget gate.
//   - QuotaService.GetBudgetUsage — the dashboard "Utilisation Actuelle"
//     bars.
//   - GetUserTerminalUsage — the dashboard session list.
//   - CountUserOccupiedSlots / CountOrgOccupiedSlots — org dashboard.
//
// The canonical rule is:
//
//	state IN ('running', 'stopped')   -- lifecycle still owes capacity
//	AND deleted_at IS NULL            -- gorm-soft-delete excluded
//	AND expires_at > NOW()            -- past-expiry rows are "zombie" slots
//
// Note (decision D6' — locked 2026-05-28, supersedes D6 from
// project_resource_quota_refactor.md): the persistence_mode distinction
// is NOT part of this predicate. "A stop is a stop" — every stopped
// session reserves its slot until tt-backend confirms the container is
// gone, at which point SyncUserSessions step 5b marks the row deleted
// and capacity is freed. The previous design carried a separate
// BudgetOccupyingScope that excluded stopped ephemeral; that scope was
// removed because it caused user-visible drift between the dashboard
// session list and the budget bars (commit 1dcc19a introduced it; the
// supersession deletes it).
//
// The expires_at clause aligns the backend with the UI's
// `getEffectiveSessionState` invariant, which treats any row whose
// `expires_at` is in the past as effectively terminated. Without that
// clause, stale rows whose proxy session is long gone but whose state
// column was never updated would keep blocking new sessions ("30 active
// sessions" with zero visible) — a quota mismatch between the
// per-second-aware UI and the dumb state-only backend count.
//
// `expires_at` is compared against `time.Now()` bound as a SQL
// parameter (not `NOW()` / `CURRENT_TIMESTAMP`) for two reasons:
//   - SQLite (used in unit tests) does not have `NOW()`; production
//     PostgreSQL does. A bound parameter is dialect-portable.
//   - The Go side owns the clock, so there is no drift between the app
//     server and the database when the two are on different machines.
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
	return tx.Where(
		"terminals.state IN ? AND terminals.deleted_at IS NULL AND terminals.expires_at > ?",
		TerminalStatesOccupyingSlot,
		time.Now(),
	)
}

// RunningDisplayScope is a GORM scope that filters terminals currently
// running and usable — the "is this terminal alive RIGHT NOW?" predicate.
// Distinct from OccupiesSlotScope, which also counts 'stopped' sessions
// for quota purposes. Use this for display/listing surfaces that want to
// show only sessions the user can interact with this instant.
//
// The canonical rule is:
//
//	state = 'running'                 -- not stopped / deleted / errored
//	AND deleted_at IS NULL            -- gorm-soft-delete excluded
//	AND expires_at > NOW()            -- past-expiry rows are "zombie" running
//
// Mirrors the SSOT pattern set by OccupiesSlotScope: every display/listing
// query that wants "currently running" terminals must route through this
// scope so the rule stays expressed in exactly one place. Past-expiry
// rows whose state column was never updated (proxy session is gone, the
// stale row remains) are excluded — without this clause, the dumb
// state-only count drifts from the per-second-aware UI which already
// treats past-expiry as terminated.
//
// `expires_at` is compared against `time.Now()` bound as a SQL parameter
// (not `NOW()` / `CURRENT_TIMESTAMP`) for dialect portability (SQLite,
// used in unit tests, has no `NOW()`) and clock locality. Same rationale
// as OccupiesSlotScope.
//
// Columns are qualified with the `terminals.` prefix so the scope is
// safe to combine with JOINs against other tables that share these
// column names (e.g. organization_members via gorm.Model).
//
// Usage:
//
//	db.Table("terminals").Scopes(models.RunningDisplayScope).
//	    Where("terminals.user_id = ?", userID).Count(&count)
func RunningDisplayScope(tx *gorm.DB) *gorm.DB {
	return tx.Where(
		"terminals.state = ? AND terminals.deleted_at IS NULL AND terminals.expires_at > ?",
		"running",
		time.Now(),
	)
}

// CountUserOccupiedSlots returns the number of terminals owned by the
// user that still "occupy a slot" (running or stopped, not expired,
// not deleted). If orgID is nil, counts all terminals across all orgs
// (and personal). If orgID is non-nil, counts only terminals tied to
// that org context via the direct terminals.organization_id column.
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
// member of the org (joined via organization_members) that still
// "occupy a slot" — used by org dashboards and listing surfaces.
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
	// State is the session lifecycle field driven by the proxy + persistence
	// rules: running, stopped, hibernating, resuming, deleted, etc. It is the
	// SSOT — every reader and writer must use this field.
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
	// Denormalized resource footprint (MR-CORE-3). Populated by the
	// BeforeCreate hook in a later MR from the size catalog so the budget
	// quota counter can sum CPU/RAM without re-joining against tt-backend.
	// Existing rows remain at 0 until the backfill runs.
	//
	// SizeCPU is in millicores (mCPU): 1000 = 1 vCPU. Matches the unit
	// on catalog.MachineSize.CPU and SubscriptionPlan.MaxCPU so the budget
	// summing query stays a pure SQL aggregate without any conversion.
	SizeCPU      int `gorm:"default:0" json:"size_cpu,omitempty"`
	SizeMemoryMB int `gorm:"default:0" json:"size_memory_mb,omitempty"`
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
