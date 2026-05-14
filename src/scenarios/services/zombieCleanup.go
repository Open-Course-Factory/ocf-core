package services

import (
	"log/slog"
	"time"

	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"gorm.io/gorm"
)

// CleanupZombieScenarioSessions finds active scenario sessions whose terminal
// has been stopped or deleted (or is no longer in the table), and marks them
// as abandoned. Returns the number of sessions abandoned.
//
// SSOT: Terminal.State is the canonical lifecycle field. A scenario session
// is "zombie" if its linked terminal is no longer running — i.e. State is
// 'stopped' or 'deleted' — or the terminal row has been removed entirely.
func CleanupZombieScenarioSessions(db *gorm.DB) (int64, error) {
	now := time.Now()

	// Subquery: terminal session IDs that are no longer running
	deadTerminals := db.Model(&terminalModels.Terminal{}).
		Select("session_id").
		Where("state IN ?", []string{"deleted", "stopped"})

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
