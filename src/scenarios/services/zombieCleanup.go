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
// For active/in_progress runs bound to a terminal — the only rows it sweeps —
// it is the complement of RunResumeMode: it abandons exactly the runs that rule
// reports as not resumable. Both defer to the same terminal rule —
// models.ContainerHeldScope, the SQL form of Terminal.HoldsContainer — so a
// paused run (terminal stopped with its container kept until the reap
// deadline, or a persistent terminal tt-backend auto-stopped before any sync
// moved its row off "running") is spared; TestZombieCleanupAgreesWithRunResumeMode
// pins the two together. Selecting the terminals that still hold a container
// and abandoning every session outside that set also covers the session whose
// terminal row has vanished entirely, without a second subquery.
//
// The auto-stopped persistent terminal is the one row that rule cannot judge
// from the database alone: its container is kept for a while, then reaped by
// tt-backend, and nothing tells ocf-core unless its owner comes back and
// triggers a sync. Left alone, its run stayed open forever and held the
// one-run slot. So before sweeping, syncOwners (when non-nil) is called once
// with the owners of those terminals; the sync records the pause or the
// deletion, and the sweep below reads the result.
//
// Using the live-only rule (RunningDisplayScope) here made Pause end the
// scenario: the next sweep abandoned every paused run. Enumerating dead states
// instead (deleted / stopped / revoked) missed the most common corpse of all:
// an ephemeral terminal past its TTL whose state column still reads "running",
// because nothing moves that column when a session simply reaches its deadline.
func CleanupZombieScenarioSessions(db *gorm.DB, syncOwners func(userIDs []string)) (int64, error) {
	now := time.Now()
	sweptStatuses := []string{"active", "in_progress"}

	if syncOwners != nil {
		var owners []string
		if err := db.Model(&terminalModels.Terminal{}).
			Distinct("terminals.user_id").
			Joins("JOIN scenario_sessions ON scenario_sessions.terminal_session_id = terminals.session_id").
			Where("scenario_sessions.status IN ?", sweptStatuses).
			Where("terminals.state = ? AND terminals.persistence_mode = ? AND terminals.expires_at < ?",
				terminalModels.StateRunning, terminalModels.PersistenceModePersistent, now).
			Pluck("terminals.user_id", &owners).Error; err != nil {
			// Not fatal: the sweep still spares these runs, it just cannot
			// learn that their container was reaped until the next pass.
			slog.Warn("failed to list owners of auto-stopped persistent terminals", "err", err)
		} else if len(owners) > 0 {
			syncOwners(owners)
		}
	}

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
