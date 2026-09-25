package services

import (
	"log/slog"
	"time"

	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"gorm.io/gorm"
)

// sweptStatuses are the run statuses the zombie sweep judges.
var sweptStatuses = []string{"active", "in_progress"}

// OwnersToSyncBeforeSweep returns the users whose terminals the sweep cannot
// judge from the database alone: a persistent terminal tt-backend auto-stopped
// at its TTL still reads "running", and its container is kept for a while and
// then reaped without ocf-core being told. Syncing these owners first lets the
// sweep see which it is. Only owners of a non-deleted active/in_progress run
// and with an active terminal key are returned — SyncUserSessions refuses the
// others, so asking would only log the same error every pass.
func OwnersToSyncBeforeSweep(db *gorm.DB) ([]string, error) {
	var owners []string
	err := db.Model(&terminalModels.Terminal{}).
		Distinct("terminals.user_id").
		Joins("JOIN scenario_sessions ON scenario_sessions.terminal_session_id = terminals.session_id"+
			" AND scenario_sessions.deleted_at IS NULL").
		Joins("JOIN user_terminal_keys ON user_terminal_keys.user_id = terminals.user_id"+
			" AND user_terminal_keys.is_active = ? AND user_terminal_keys.deleted_at IS NULL", true).
		Where("scenario_sessions.status IN ?", sweptStatuses).
		Where("terminals.state = ? AND terminals.persistence_mode = ? AND terminals.expires_at < ?",
			terminalModels.StateRunning, terminalModels.PersistenceModePersistent, time.Now()).
		Pluck("terminals.user_id", &owners).Error
	return owners, err
}

// CleanupZombieScenarioSessions abandons runs whose environment is gone, and
// returns how many it abandoned.
//
// For active/in_progress runs bound to a terminal — the only rows it sweeps —
// it is the complement of RunResumeMode: it abandons exactly the runs whose
// terminal is outside models.ContainerHeldScope, the SQL form of
// Terminal.HoldsContainer. TestZombieCleanupAgreesWithRunResumeMode pins the
// two together. A run whose terminal row has vanished is outside the set too.
// Callers sync the owners OwnersToSyncBeforeSweep names first.
func CleanupZombieScenarioSessions(db *gorm.DB) (int64, error) {
	now := time.Now()

	// Subquery: the terminals a learner could still come back to.
	heldTerminals := db.Model(&terminalModels.Terminal{}).
		Select("session_id").
		Scopes(terminalModels.ContainerHeldScope)

	result := db.Model(&models.ScenarioSession{}).
		Where("status IN ?", sweptStatuses).
		Where("terminal_session_id IS NOT NULL").
		Where("terminal_session_id NOT IN (?)", heldTerminals).
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
