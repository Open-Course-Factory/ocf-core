package services

import (
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// CleanupZombieScenarioSessions finds active scenario sessions whose terminal
// has expired, stopped, or been deleted, and marks them as abandoned.
// Returns the number of sessions abandoned.
func CleanupZombieScenarioSessions(db *gorm.DB) (int64, error) {
	now := time.Now()

	result := db.Exec(`
		UPDATE scenario_sessions
		SET status = 'abandoned', updated_at = ?
		WHERE status IN ('active', 'in_progress')
		  AND terminal_session_id IS NOT NULL
		  AND (
		    terminal_session_id IN (
		      SELECT session_id FROM terminals WHERE status IN ('expired', 'stopped')
		    )
		    OR terminal_session_id NOT IN (
		      SELECT session_id FROM terminals WHERE deleted_at IS NULL
		    )
		  )
	`, now)

	if result.Error != nil {
		slog.Error("failed to cleanup zombie scenario sessions", "err", result.Error)
		return 0, result.Error
	}

	return result.RowsAffected, nil
}
