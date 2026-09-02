package models

import (
	"fmt"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AbandonableSessionStatuses are the statuses a run can be abandoned from: it
// has started and has not finished. Completed and abandoned runs are final and
// stay refused.
//
// One list, because four sites used to carry their own and drifted: the
// learner's abandon accepted setup_failed but not the historical in_progress,
// the class reset and the unassign hook accepted in_progress but not
// setup_failed, and the terminal delete had a third variant — so a failed run
// could be abandoned by its learner but not reset by its trainer.
//
//   - in_progress is the historical name for a running session and still
//     appears on rows created before the rename.
//   - provisioning belongs here: leaving one behind hands the learner a
//     session they cannot use, and the unique partial index (which covers
//     'provisioning') then blocks a fresh start.
//   - setup_failed is terminal for the setup goroutine, which gates every
//     write on WHERE status='provisioning', so nothing can resurrect the row
//     after it flips.
var AbandonableSessionStatuses = []string{"active", "in_progress", "provisioning", "setup_failed"}

// ScenarioSession represents a user's active session working through a scenario
type ScenarioSession struct {
	entityManagementModels.BaseModel
	ScenarioID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"scenario_id"`
	UserID            string     `gorm:"type:varchar(255);not null;index" json:"user_id"`
	TerminalSessionID *string    `gorm:"type:varchar(255)" json:"terminal_session_id,omitempty"`
	CurrentStep       int        `gorm:"default:0" json:"current_step"`
	Status            string     `gorm:"type:varchar(50);default:'active'" json:"status"` // provisioning, active, completed, abandoned, setup_failed
	ProvisioningPhase string     `gorm:"type:varchar(50);default:''" json:"provisioning_phase,omitempty"`
	StartedAt         time.Time  `gorm:"not null" json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	Grade             *float64   `gorm:"type:decimal(5,2)" json:"grade,omitempty"`
	TrainerID         *string    `gorm:"type:varchar(255)" json:"trainer_id,omitempty" mapstructure:"trainer_id"`
	IsPreview         bool       `gorm:"default:false" json:"is_preview,omitempty" mapstructure:"is_preview"`

	// Locale is fixed when the session starts and never changes. The container
	// was built in one language — its directories carry that language's names —
	// so a learner who switches the interface mid-run must keep reading the
	// language their world is actually in, or the text will send them to rooms
	// that do not exist. Empty means the scenario's default.
	Locale string `gorm:"type:varchar(10)" json:"locale,omitempty" mapstructure:"locale"`

	// Relations
	StepProgress []ScenarioStepProgress `gorm:"foreignKey:SessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"step_progress,omitempty"`
	Flags        []ScenarioFlag         `gorm:"foreignKey:SessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"flags,omitempty"`
	Scenario     Scenario               `gorm:"foreignKey:ScenarioID" json:"-"`
}

// Implement interfaces for entity management system
func (s ScenarioSession) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioSession) GetReferenceObject() string {
	return "ScenarioSession"
}

// TableName specifies the table name
func (ScenarioSession) TableName() string {
	return "scenario_sessions"
}

// MigrateUniqueActiveSessionIndex creates a partial unique index to prevent
// duplicate active/provisioning sessions for the same user+scenario.
func MigrateUniqueActiveSessionIndex(db *gorm.DB) {
	indexName := "idx_unique_active_session"

	// Check if index already exists (idempotent)
	if db.Migrator().HasIndex(&ScenarioSession{}, indexName) {
		return
	}

	// Detect dialect for correct SQL syntax
	dialect := db.Dialector.Name()
	var sql string
	switch dialect {
	case "postgres":
		sql = fmt.Sprintf(
			`CREATE UNIQUE INDEX %s ON scenario_sessions (user_id, scenario_id) WHERE status IN ('active', 'provisioning')`,
			indexName,
		)
	case "sqlite":
		sql = fmt.Sprintf(
			`CREATE UNIQUE INDEX IF NOT EXISTS %s ON scenario_sessions (user_id, scenario_id) WHERE status IN ('active', 'provisioning')`,
			indexName,
		)
	default:
		fmt.Printf("MigrateUniqueActiveSessionIndex: unsupported dialect %s, skipping\n", dialect)
		return
	}

	if err := db.Exec(sql).Error; err != nil {
		fmt.Printf("MigrateUniqueActiveSessionIndex: failed to create index: %v\n", err)
	}
}
