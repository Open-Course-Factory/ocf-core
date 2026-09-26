package services

import (
	"log/slog"
	"time"

	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// provisioningPhaseReplay is the phase of a run whose environment is being
// rebuilt at its current step (ResumeModeRebuild).
const provisioningPhaseReplay = "replay"

// sweptStatuses are the run statuses the zombie sweep judges.
var sweptStatuses = []string{"active", "in_progress"}

// unrebuildableRun is the SQL form of the runs RunResumeMode never rebuilds:
// previews, and runs of a crash-trap scenario. Only those die with their
// container; any other run is rebuilt on resume. table qualifies the
// scenario_sessions columns when the query joins other tables.
func unrebuildableRun(db *gorm.DB, table string) *gorm.DB {
	crashTrapScenarios := db.Model(&models.Scenario{}).Select("id").Where("crash_traps = ?", true)
	return db.Where(table+".is_preview = ?", true).
		Or(table+".scenario_id IN (?)", crashTrapScenarios)
}

// OwnersToSyncBeforeSweep returns the users whose terminals the sweep cannot
// judge from the database alone: a persistent terminal tt-backend auto-stopped
// at its TTL still reads "running", and its container is kept for a while and
// then reaped without ocf-core being told. Syncing these owners first lets the
// sweep see which it is. Only owners of a non-deleted active/in_progress run
// the sweep could abandon (a crash-trap or preview run) and with an active
// terminal key are returned — any other run is rebuildable whichever way the
// sync goes, and SyncUserSessions refuses owners without a key, so asking
// would only cost a round trip or log the same error every pass.
func OwnersToSyncBeforeSweep(db *gorm.DB) ([]string, error) {
	var owners []string
	err := db.Model(&terminalModels.Terminal{}).
		Distinct("terminals.user_id").
		Joins("JOIN scenario_sessions ON scenario_sessions.terminal_session_id = terminals.session_id"+
			" AND scenario_sessions.deleted_at IS NULL").
		Joins("JOIN user_terminal_keys ON user_terminal_keys.user_id = terminals.user_id"+
			" AND user_terminal_keys.is_active = ? AND user_terminal_keys.deleted_at IS NULL", true).
		Where("scenario_sessions.status IN ?", sweptStatuses).
		Where(unrebuildableRun(db, "scenario_sessions")).
		Where("terminals.state = ? AND terminals.persistence_mode = ? AND terminals.expires_at < ?",
			terminalModels.StateRunning, terminalModels.PersistenceModePersistent, time.Now()).
		Pluck("terminals.user_id", &owners).Error
	return owners, err
}

// CleanupZombieScenarioSessions abandons runs whose environment is gone and
// cannot be rebuilt, and returns how many it abandoned.
//
// For active/in_progress runs bound to a terminal — the only rows it sweeps —
// it is the complement of RunResumeMode: it abandons exactly the crash-trap
// and preview runs whose terminal is outside models.ContainerHeldScope, the
// SQL form of Terminal.HoldsContainer. TestZombieCleanupAgreesWithRunResumeMode
// pins the two together. A run whose terminal row has vanished is outside the
// set too. Callers sync the owners OwnersToSyncBeforeSweep names first.
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
		Where(unrebuildableRun(db, "scenario_sessions")).
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
// The longest legitimate run is the larger of the initial-setup budget and the
// per-step ceiling; the margin covers the gap between a script's own timeout
// firing and the goroutine finishing its cleanup.
const stuckProvisioningReapMargin = 2 * time.Minute

var stuckProvisioningTimeout = time.Duration(
	max(bgScriptTimeoutInitial, MaxBackgroundTimeoutSeconds),
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

// ReleaseStalledReplays hands back to the resume rule the rebuilds that
// stalled — a process restart mid-replay leaves the run in provisioning/replay
// with no goroutine on it — and returns the half-built terminals they were on,
// for the caller to delete.
//
// Each run goes back to the terminal it was rebuilt from, whose container is
// gone, so it reads as rebuildable again. Left to
// CleanupStuckProvisioningSessions, which callers run after this, it would
// become setup_failed and the next launch would abandon it — losing the
// progress the rebuild existed to keep.
func ReleaseStalledReplays(db *gorm.DB) ([]string, error) {
	cutoff := time.Now().Add(-stuckProvisioningTimeout)

	var stalled []models.ScenarioSession
	if err := db.Where("status = ? AND provisioning_phase = ? AND rebuild_from_terminal_id IS NOT NULL AND updated_at < ?",
		statusProvisioning, provisioningPhaseReplay, cutoff).
		Find(&stalled).Error; err != nil {
		slog.Error("failed to list stalled scenario replays", "err", err)
		return nil, err
	}

	var released []string
	for i := range stalled {
		// Guarded per row: a replay that finishes between the read and this
		// write keeps its terminal, which must then not be handed out for
		// deletion.
		restored, err := restoreRunBeforeRebuild(db, stalled[i].ID)
		if err != nil {
			return released, err
		}
		if restored && stalled[i].TerminalSessionID != nil {
			released = append(released, *stalled[i].TerminalSessionID)
		}
	}
	return released, nil
}

// restoreRunBeforeRebuild puts a run whose rebuild is still in progress back
// as it was before it: open, on the terminal it was rebuilt from. It reports
// whether it did; a run that left provisioning meanwhile — finished, or
// abandoned — is left alone.
func restoreRunBeforeRebuild(db *gorm.DB, sessionID uuid.UUID) (bool, error) {
	result := db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status = ? AND rebuild_from_terminal_id IS NOT NULL", sessionID, statusProvisioning).
		Updates(map[string]any{
			"terminal_session_id":      gorm.Expr("rebuild_from_terminal_id"),
			"status":                   statusActive,
			"provisioning_phase":       "",
			"rebuild_from_terminal_id": nil,
		})
	return result.RowsAffected > 0, result.Error
}
