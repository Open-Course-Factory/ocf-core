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
// StateStopped, StateDeleted or StateRevoked (billing revocation, #388) — or
// the terminal row has been removed entirely.
func CleanupZombieScenarioSessions(db *gorm.DB) (int64, error) {
	now := time.Now()

	// Subquery: terminal session IDs that are no longer running
	deadTerminals := db.Model(&terminalModels.Terminal{}).
		Select("session_id").
		Where("state IN ?", []terminalModels.TerminalState{terminalModels.StateDeleted, terminalModels.StateStopped, terminalModels.StateRevoked})

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

// stuckProvisioningTimeout is how long a session may stay in "provisioning"
// before it is written off. It is well above the longest step timeout
// (bgScriptTimeoutStep0, 5 minutes) so a legitimately slow setup is never
// reaped mid-run.
const stuckProvisioningTimeout = 10 * time.Minute

// CleanupStuckProvisioningSessions marks long-stalled provisioning sessions as
// setup_failed. Returns the number of sessions released.
//
// Setup runs in a goroutine, so a process restart (or a panic outside the
// recover) leaves the row in "provisioning" forever. That row is not merely
// cosmetic: the unique partial index on (user_id, scenario_id) covers
// 'provisioning', so it also blocks the learner from ever restarting the
// scenario. Moving it to setup_failed is what StartScenario needs to auto-
// abandon it on the next attempt.
func CleanupStuckProvisioningSessions(db *gorm.DB) (int64, error) {
	cutoff := time.Now().Add(-stuckProvisioningTimeout)

	result := db.Model(&models.ScenarioSession{}).
		Where("status = ?", "provisioning").
		Where("updated_at < ?", cutoff).
		Updates(map[string]any{
			"status":             "setup_failed",
			"provisioning_phase": "",
		})

	if result.Error != nil {
		slog.Error("failed to cleanup stuck provisioning scenario sessions", "err", result.Error)
		return 0, result.Error
	}

	return result.RowsAffected, nil
}
