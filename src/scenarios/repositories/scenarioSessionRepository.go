package repositories

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
)

// SessionAggregate is the projection scenario analytics aggregates over: one
// row per non-preview session belonging to an active member of the group.
// Only these four columns are loaded — the whole point of the projection is
// that analytics needs nothing heavier (no step counts, no identities).
type SessionAggregate struct {
	Status      string
	Grade       *float64
	StartedAt   time.Time
	CompletedAt *time.Time
}

// ScenarioSessionRepository is the (deliberately narrow) query surface the
// teacher dashboard needs from scenario sessions. Extend it method by method
// as callers migrate off inline SQL; don't widen it speculatively.
type ScenarioSessionRepository interface {
	// GetSessionAggregatesForGroupScenario returns the analytics projection
	// for every non-preview session of the scenario run by an active member
	// of the group. Soft-deleted sessions are excluded by GORM's default
	// scope on the model.
	GetSessionAggregatesForGroupScenario(groupID, scenarioID uuid.UUID) ([]SessionAggregate, error)
}

type scenarioSessionRepository struct {
	db *gorm.DB
}

func NewScenarioSessionRepository(db *gorm.DB) ScenarioSessionRepository {
	return &scenarioSessionRepository{db: db}
}

func (r *scenarioSessionRepository) GetSessionAggregatesForGroupScenario(groupID, scenarioID uuid.UUID) ([]SessionAggregate, error) {
	var rows []SessionAggregate
	err := r.db.Model(&models.ScenarioSession{}).
		Select("scenario_sessions.status", "scenario_sessions.grade",
			"scenario_sessions.started_at", "scenario_sessions.completed_at").
		Joins("JOIN group_members ON group_members.user_id = scenario_sessions.user_id"+
			" AND group_members.group_id = ? AND group_members.is_active = ?", groupID, true).
		Where("scenario_sessions.scenario_id = ?", scenarioID).
		Where("scenario_sessions.is_preview = ?", false).
		Scan(&rows).Error
	return rows, err
}
