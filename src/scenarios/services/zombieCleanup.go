package services

import (
	"log/slog"
	"time"

	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"gorm.io/gorm"
)

// CleanupZombieScenarioSessions abandons runs whose environment is gone, and
// returns how many it abandoned.
//
// It asks the complement of the question sessionIsResumable asks — "is this
// session's terminal still alive?" — so the two must agree, and both defer to
// the same rule: models.RunningDisplayScope, the SQL form of Terminal.IsLive.
// Selecting the live terminals and abandoning every session outside that set
// also covers the session whose terminal row has vanished entirely, without a
// second subquery.
//
// The previous version enumerated dead states instead (deleted / stopped /
// revoked). That missed the most common corpse of all: a terminal past its TTL
// whose state column still reads "running", because nothing moves that column
// when a session simply reaches its deadline. Such sessions stayed "active"
// indefinitely and blocked every relaunch of their scenario.
func CleanupZombieScenarioSessions(db *gorm.DB) (int64, error) {
	now := time.Now()

	// Subquery: the terminals a learner could still be attached to.
	liveTerminals := db.Model(&terminalModels.Terminal{}).
		Select("session_id").
		Scopes(terminalModels.RunningDisplayScope)

	result := db.Model(&models.ScenarioSession{}).
		Where("status IN ?", []string{"active", "in_progress"}).
		Where("terminal_session_id IS NOT NULL").
		Where("terminal_session_id NOT IN (?)", liveTerminals).
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
// before it is written off.
//
// Derived, not chosen. It has to sit above the longest setup that can
// legitimately still be running, or the reaper writes off a session whose
// script then succeeds — and the goroutine's own status write is guarded on
// "provisioning", which the reaper has already cleared, so the success is
// discarded silently and the session stays setup_failed forever.
//
// The longest legitimate run is the larger of the step-0 budget and the
// per-step ceiling; the margin covers the gap between a script's own timeout
// firing and the goroutine finishing its cleanup.
const stuckProvisioningReapMargin = 2 * time.Minute

var stuckProvisioningTimeout = time.Duration(
	max(bgScriptTimeoutStep0, MaxBackgroundTimeoutSeconds),
)*time.Second + stuckProvisioningReapMargin

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
