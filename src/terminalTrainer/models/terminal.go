package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TerminalStatesOccupyingSlot are the state values that count toward
// the concurrent_terminals limit. Stopped sessions still occupy a slot
// because they preserve disk and can be resumed — only deleted (or
// other terminal-state) rows free a slot.
var TerminalStatesOccupyingSlot = []string{"running", "stopped"}

// OccupiesSlotScope is a GORM scope that filters terminals counted
// against the concurrent_terminals quota. Single source of truth for
// the "occupies a slot" rule — every counter / quota query must use
// this scope so the rule stays expressed in exactly one place.
//
// The canonical rule is:
//
//	state IN ('running', 'stopped')   -- lifecycle still owes capacity
//	AND deleted_at IS NULL            -- gorm-soft-delete excluded
//	AND expires_at > NOW()            -- past-expiry rows are "zombie" slots
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

// BudgetOccupyingScope is a GORM scope that filters terminals whose
// CPU/RAM is currently reserved against the user's (or org's) budget.
// Single source of truth for the "occupies budget RIGHT NOW" rule —
// both the budget gate (CheckBudget via sumActiveResources*) and the
// dashboard live-recalc for the concurrent_terminals metric must route
// through this scope so the rule stays expressed in exactly one place.
//
// The canonical rule is:
//
//	deleted_at IS NULL                                 -- gorm-soft-delete excluded
//	AND expires_at > NOW()                             -- past-expiry rows are "zombie"
//	AND (state = 'running' OR persistence_mode = 'persistent')
//
// This matches design decision D6: a terminal reserves capacity iff it
// is currently running OR it is persistent (because a persistent stopped
// session can be resumed without going through a fresh budget check —
// its capacity remains reserved on the gate side, so it must appear in
// the dashboard count too).
//
// Set membership vs. the two sibling scopes:
//
//   - OccupiesSlotScope (state IN ('running','stopped')) counts EVERY
//     stopped session — including ephemeral stopped — against the
//     legacy concurrent_terminals slot rule. That rule predates the
//     CPU/RAM budget and is broader than what budget enforcement needs:
//     a stopped ephemeral has no resume path, so its CPU/RAM is freed,
//     but it still "occupies a slot" in the slot-count sense.
//   - RunningDisplayScope (state = 'running') is the "alive RIGHT NOW"
//     predicate — useful for display surfaces that want to show only
//     sessions the user can interact with this instant. It excludes
//     stopped persistent sessions, so it must NOT be used for budget
//     accounting or for the dashboard "Actifs" counter.
//   - BudgetOccupyingScope (THIS scope) is the budget-accounting
//     predicate. It includes stopped persistent (capacity reserved)
//     and excludes stopped ephemeral (capacity released).
//
// `expires_at` is compared against `time.Now()` bound as a SQL parameter
// (not `NOW()` / `CURRENT_TIMESTAMP`) for dialect portability (SQLite,
// used in unit tests, has no `NOW()`) and clock locality. Same rationale
// as OccupiesSlotScope and RunningDisplayScope.
//
// Columns are qualified with the `terminals.` prefix so the scope is
// safe to combine with JOINs against other tables that share these
// column names (e.g. organization_members via gorm.Model).
//
// Usage:
//
//	db.Table("terminals").Scopes(models.BudgetOccupyingScope).
//	    Where("terminals.user_id = ?", userID).Count(&count)
//
// See: src/payment/services/quotaService.go::sumActiveResourcesForUser,
// sumActiveResourcesForOrg, and liveBudgetOccupyingTerminals.
func BudgetOccupyingScope(tx *gorm.DB) *gorm.DB {
	return tx.Where(
		"terminals.deleted_at IS NULL AND terminals.expires_at > ? AND (terminals.state = ? OR terminals.persistence_mode = ?)",
		time.Now(), "running", "persistent",
	)
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
