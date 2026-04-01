package models

import (
	"fmt"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

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
