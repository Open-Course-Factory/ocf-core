package services

import (
	"log/slog"
	"time"

	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"gorm.io/gorm"
)

// CleanupZombieScenarioSessions finds active scenario sessions whose terminal
// has expired, stopped, or been deleted, and marks them as abandoned.
// Returns the number of sessions abandoned.
func CleanupZombieScenarioSessions(db *gorm.DB) (int64, error) {
	now := time.Now()

	// Subquery: terminal session IDs that are expired or stopped
	deadTerminals := db.Model(&terminalModels.Terminal{}).
		Select("session_id").
		Where("status IN ?", []string{"expired", "stopped"})

	// Subquery: all known terminal session IDs (GORM auto-filters soft-deleted)
	knownTerminals := db.Model(&terminalModels.Terminal{}).
		Select("session_id")

	result := db.Model(&models.ScenarioSession{}).
		Where("status IN ?", []string{"active", "in_progress"}).
		Where("terminal_session_id IS NOT NULL").
		Where(
			db.Where("terminal_session_id IN (?)", deadTerminals).
				Or("terminal_session_id NOT IN (?)", knownTerminals),
		).
		Updates(map[string]any{
			"status":     "abandoned",
			"updated_at": now,
		})

	if result.Error != nil {
		slog.Error("failed to cleanup zombie scenario sessions", "err", result.Error)
		return 0, result.Error
	}

	return result.RowsAffected, nil
}
